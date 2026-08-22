package tool

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"

	"github.com/Syluxso/byzclaw/internal/core"
	"github.com/Syluxso/byzclaw/internal/ports"
)

func TestShellDisabledByPolicy(t *testing.T) {
	p, err := core.NewPathPolicy(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p.ShellEnabled = false
	s := &Shell{Policy: p}
	args, _ := json.Marshal(map[string]string{"command": "echo hi"})
	tr, err := s.Exec(context.Background(), ports.ToolCall{ID: "1", Name: "shell", Args: args})
	if err != nil {
		t.Fatal(err)
	}
	if !tr.IsError {
		t.Fatal("expected policy deny")
	}
}

func TestShellEchoWhenEnabled(t *testing.T) {
	p, err := core.NewPathPolicy(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p.ShellEnabled = true
	s := &Shell{Policy: p}
	cmd := "echo hello"
	if runtime.GOOS == "windows" {
		cmd = "echo hello"
	}
	args, _ := json.Marshal(map[string]string{"command": cmd})
	tr, err := s.Exec(context.Background(), ports.ToolCall{ID: "1", Name: "shell", Args: args})
	if err != nil {
		t.Fatal(err)
	}
	if tr.IsError {
		t.Fatalf("unexpected error: %s", tr.Content)
	}
	if tr.Content == "" {
		t.Fatal("empty output")
	}
}
