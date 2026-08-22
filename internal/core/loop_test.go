package core_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Syluxso/byzclaw/internal/adapters/model"
	"github.com/Syluxso/byzclaw/internal/adapters/store"
	tooladapter "github.com/Syluxso/byzclaw/internal/adapters/tool"
	"github.com/Syluxso/byzclaw/internal/core"
	"github.com/Syluxso/byzclaw/internal/ports"
)

func TestLoopTextOnly(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	fake := &model.Fake{Queue: []ports.CompletionResponse{{Content: "hello"}}}
	loop := core.NewLoop(s, fake, nil, nil, core.DefaultLoopConfig())
	out, err := loop.Handle(context.Background(), ports.Inbound{SessionID: "cli:local", Text: "hi", Channel: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "hello" {
		t.Fatalf("got %q", out.Text)
	}
}

func TestLoopPersistsToolCallsBeforeExec(t *testing.T) {
	root := t.TempDir()
	policy, err := core.NewPathPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ws := &tooladapter.Workspace{Policy: policy}
	fake := &model.Fake{Queue: []ports.CompletionResponse{
		{
			Content: "",
			ToolCalls: []ports.ToolCall{{
				ID:   "call1",
				Name: "workspace_write",
				Args: json.RawMessage(`{"path":"hi.md","content":"hello"}`),
			}},
		},
		{Content: "wrote it"},
	}}
	loop := core.NewLoop(s, fake, ws.Tools(), policy, core.DefaultLoopConfig())
	out, err := loop.Handle(context.Background(), ports.Inbound{SessionID: "cli:local", Text: "write hi", Channel: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "wrote it" {
		t.Fatalf("got %q", out.Text)
	}
	abs, err := policy.Resolve("hi.md")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(abs)
	if err != nil || string(raw) != "hello" {
		t.Fatalf("workspace file missing/wrong: %v %q", err, raw)
	}
	msgs, _ := s.ListMessages(context.Background(), "cli:local", 50)
	var sawAssistantWithTools, sawToolResult bool
	for _, m := range msgs {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			sawAssistantWithTools = true
		}
		if m.Role == "tool" && m.ToolCallID == "call1" {
			sawToolResult = true
		}
	}
	if !sawAssistantWithTools || !sawToolResult {
		t.Fatalf("missing durable tool transcript: %+v", msgs)
	}
}

func TestMaxIterations(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Always request the same missing tool → iterations burn.
	q := make([]ports.CompletionResponse, 0, 20)
	for i := 0; i < 20; i++ {
		q = append(q, ports.CompletionResponse{
			ToolCalls: []ports.ToolCall{{ID: "c", Name: "nope", Args: json.RawMessage(`{}`)}},
		})
	}
	fake := &model.Fake{Queue: q}
	cfg := core.DefaultLoopConfig()
	cfg.MaxToolIterations = 3
	loop := core.NewLoop(s, fake, nil, nil, cfg)
	out, err := loop.Handle(context.Background(), ports.Inbound{SessionID: "s", Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text == "" {
		t.Fatal("expected user-visible stop message")
	}
}
