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

func TestRecoverIncompleteRunsMissingTools(t *testing.T) {
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
	ctx := context.Background()

	// Simulate crash: assistant tool_calls persisted, no tool result yet.
	_ = s.SaveMessage(ctx, ports.Message{
		ID: "u1", SessionID: "s1", Role: "user", Content: "write hi", CreatedAt: ports.RealClock{}.Now(),
	})
	args, _ := json.Marshal(map[string]string{"path": "hi.md", "content": "hello"})
	_ = s.SaveMessage(ctx, ports.Message{
		ID: "a1", SessionID: "s1", Role: "assistant",
		ToolCalls: []ports.ToolCall{{ID: "call1", Name: "workspace_write", Args: args}},
		CreatedAt: ports.RealClock{}.Now(),
	})
	_ = s.SaveRun(ctx, ports.Run{ID: "r1", SessionID: "s1", Status: ports.RunToolPending})

	fake := &model.Fake{Queue: []ports.CompletionResponse{{Content: "recovered"}}}
	ws := &tooladapter.Workspace{Policy: policy}
	loop := core.NewLoop(s, fake, ws.Tools(), policy, core.DefaultLoopConfig())
	if err := loop.RecoverIncomplete(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "hi.md"))
	if err != nil || string(raw) != "hello" {
		t.Fatalf("file after resume: %v %q", err, raw)
	}
	run, err := s.LoadRun(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != ports.RunCompleted {
		t.Fatalf("status=%s err=%s", run.Status, run.Error)
	}
}
