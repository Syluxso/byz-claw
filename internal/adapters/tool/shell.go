package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Syluxso/byzclaw/internal/core"
	"github.com/Syluxso/byzclaw/internal/ports"
)

// Shell runs a command with the workspace root as cwd. Disabled by default (policy).
type Shell struct {
	Policy  *core.PathPolicy
	Timeout time.Duration
	MaxOut  int
}

func (s *Shell) Name() string { return "shell" }
func (s *Shell) Description() string {
	return "Run a shell command with cwd=workspace (disabled unless tools.shell.enabled)."
}
func (s *Shell) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"Shell command to run"}},"required":["command"]}`)
}

func (s *Shell) Exec(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil || strings.TrimSpace(args.Command) == "" {
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: "command required", IsError: true}, nil
	}
	if s.Policy != nil {
		if err := s.Policy.AllowTool(ctx, "shell", call.Args); err != nil {
			return ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}, nil
		}
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	maxOut := s.MaxOut
	if maxOut <= 0 {
		maxOut = 100_000
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cctx, "cmd", "/C", args.Command)
	} else {
		cmd = exec.CommandContext(cctx, "sh", "-c", args.Command)
	}
	if s.Policy != nil {
		cmd.Dir = s.Policy.Root
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String()
	errOut := stderr.String()
	combined := strings.TrimSpace(out)
	if errOut != "" {
		if combined != "" {
			combined += "\n"
		}
		combined += "stderr:\n" + strings.TrimSpace(errOut)
	}
	if len(combined) > maxOut {
		combined = combined[:maxOut] + "\n…(truncated)"
	}
	if err != nil {
		if combined == "" {
			combined = err.Error()
		} else {
			combined = fmt.Sprintf("%s\nexit: %v", combined, err)
		}
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: combined, IsError: true}, nil
	}
	if combined == "" {
		combined = "(no output)"
	}
	return ports.ToolResult{ID: call.ID, Name: call.Name, Content: combined}, nil
}
