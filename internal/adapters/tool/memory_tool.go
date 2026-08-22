package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Syluxso/byzclaw/internal/core"
	"github.com/Syluxso/byzclaw/internal/ports"
)

// MemoryTools reads/writes under the memory jail (same PathPolicy pattern).
type MemoryTools struct {
	Policy *core.PathPolicy
}

func (m *MemoryTools) Tools() []ports.Tool {
	return []ports.Tool{
		&memTool{m: m, name: "memory_read", desc: "Read a file under memory/", schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`), write: false},
		&memTool{m: m, name: "memory_write", desc: "Write a file under memory/", schema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`), write: true},
	}
}

type memTool struct {
	m      *MemoryTools
	name   string
	desc   string
	schema json.RawMessage
	write  bool
}

func (t *memTool) Name() string            { return t.name }
func (t *memTool) Description() string     { return t.desc }
func (t *memTool) Schema() json.RawMessage { return t.schema }

func (t *memTool) Exec(_ context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	_ = json.Unmarshal(call.Args, &args)
	if args.Path == "" {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: "path required", IsError: true}, nil
	}
	op := "read"
	if t.write {
		op = "write"
	}
	if err := t.m.Policy.AllowPath(args.Path, op); err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	abs, err := t.m.Policy.Resolve(args.Path)
	if err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	if t.write {
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
		}
		if err := os.WriteFile(abs, []byte(args.Content), 0o644); err != nil {
			return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
		}
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: "wrote " + args.Path}, nil
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	return ports.ToolResult{ID: call.ID, Name: call.Name, Content: string(b)}, nil
}
