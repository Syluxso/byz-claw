package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Syluxso/byzclaw/internal/core"
	"github.com/Syluxso/byzclaw/internal/ports"
)

type Workspace struct {
	Policy *core.PathPolicy
}

func (w *Workspace) listSpec() ports.ToolSpec {
	return ports.ToolSpec{
		Name:        "workspace_list",
		Description: "List files under the workspace root (relative path).",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Relative directory under workspace"}},"required":[]}`),
	}
}

func (w *Workspace) readSpec() ports.ToolSpec {
	return ports.ToolSpec{
		Name:        "workspace_read",
		Description: "Read a text file under the workspace jail.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
	}
}

func (w *Workspace) writeSpec() ports.ToolSpec {
	return ports.ToolSpec{
		Name:        "workspace_write",
		Description: "Write a text file under the workspace jail.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
	}
}

// Tools returns the three workspace tools sharing one policy.
func (w *Workspace) Tools() []ports.Tool {
	return []ports.Tool{
		&wsTool{w: w, spec: w.listSpec(), exec: w.execList},
		&wsTool{w: w, spec: w.readSpec(), exec: w.execRead},
		&wsTool{w: w, spec: w.writeSpec(), exec: w.execWrite},
	}
}

type wsTool struct {
	w    *Workspace
	spec ports.ToolSpec
	exec func(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error)
}

func (t *wsTool) Name() string                 { return t.spec.Name }
func (t *wsTool) Description() string          { return t.spec.Description }
func (t *wsTool) Schema() json.RawMessage      { return t.spec.Schema }
func (t *wsTool) Exec(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	return t.exec(ctx, call)
}

func (w *Workspace) execList(_ context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	var args struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(call.Args, &args)
	if args.Path == "" {
		args.Path = "."
	}
	if err := w.Policy.AllowPath(args.Path, "list"); err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	abs, err := w.Policy.Resolve(args.Path)
	if err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	var lines string
	for _, e := range entries {
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		lines += e.Name() + suffix + "\n"
	}
	return ports.ToolResult{ID: call.ID, Name: call.Name, Content: lines}, nil
}

func (w *Workspace) execRead(_ context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil || args.Path == "" {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: "path required", IsError: true}, nil
	}
	if err := w.Policy.AllowPath(args.Path, "read"); err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	abs, err := w.Policy.Resolve(args.Path)
	if err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	return ports.ToolResult{ID: call.ID, Name: call.Name, Content: string(b)}, nil
}

func (w *Workspace) execWrite(_ context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil || args.Path == "" {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: "path and content required", IsError: true}, nil
	}
	if err := w.Policy.AllowPath(args.Path, "write"); err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	abs, err := w.Policy.Resolve(args.Path)
	if err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	if err := os.WriteFile(abs, []byte(args.Content), 0o644); err != nil {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
	}
	return ports.ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("wrote %s", args.Path)}, nil
}
