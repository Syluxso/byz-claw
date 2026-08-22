package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// Loop is the fixed agent state machine. Not a plugin.
type Loop struct {
	Store        ports.Store
	Model        ports.Model
	Tools        map[string]ports.Tool
	Policy       ports.Policy
	Hooks        []ports.Hook
	Clock        ports.Clock
	Config       LoopConfig
	Channel      ports.Channel // optional outbound
	SystemPrompt string

	mu     sync.Mutex
	active map[string]bool // sessionID → running
}

func NewLoop(store ports.Store, model ports.Model, tools []ports.Tool, policy ports.Policy, cfg LoopConfig) *Loop {
	m := make(map[string]ports.Tool, len(tools))
	for _, t := range tools {
		m[t.Name()] = t
	}
	if cfg.MaxToolIterations == 0 {
		cfg = DefaultLoopConfig()
	}
	return &Loop{
		Store:  store,
		Model:  model,
		Tools:  m,
		Policy: policy,
		Clock:  ports.RealClock{},
		Config: cfg,
		active: map[string]bool{},
	}
}

func (l *Loop) toolSpecs() []ports.ToolSpec {
	out := make([]ports.ToolSpec, 0, len(l.Tools))
	for _, t := range l.Tools {
		out = append(out, ports.ToolSpec{Name: t.Name(), Description: t.Description(), Schema: t.Schema()})
	}
	return out
}

func (l *Loop) runHooks(ctx context.Context, point ports.HookPoint, hc *ports.HookContext) error {
	for _, h := range l.Hooks {
		if h.Point() != point {
			continue
		}
		if err := h.Run(ctx, hc); err != nil {
			return err
		}
	}
	return nil
}

// Handle processes one inbound message. One active run per session.
func (l *Loop) Handle(ctx context.Context, in ports.Inbound) (ports.Outbound, error) {
	if in.SessionID == "" {
		in.SessionID = "cli:local"
	}
	l.mu.Lock()
	if l.active[in.SessionID] {
		l.mu.Unlock()
		return ports.Outbound{}, fmt.Errorf("session %s already has an active run", in.SessionID)
	}
	l.active[in.SessionID] = true
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		delete(l.active, in.SessionID)
		l.mu.Unlock()
	}()

	run := ports.Run{
		ID:        newID(),
		SessionID: in.SessionID,
		Status:    ports.RunAccepted,
		UpdatedAt: l.Clock.Now(),
	}
	_ = l.Store.SaveRun(ctx, run)

	hc := &ports.HookContext{Run: &run, Session: in.SessionID, Inbound: &in}
	if err := l.runHooks(ctx, ports.HookOnInbound, hc); err != nil {
		return l.fail(ctx, run, err)
	}

	userMsg := ports.Message{
		ID: newID(), SessionID: in.SessionID, Role: "user", Content: in.Text, CreatedAt: l.Clock.Now(),
	}
	if err := l.Store.SaveMessage(ctx, userMsg); err != nil {
		return l.fail(ctx, run, err)
	}

	errCounts := map[string]int{}

	for {
		if run.Iteration >= l.Config.MaxToolIterations {
			_ = l.runHooks(ctx, ports.HookOnLimit, hc)
			out := ports.Outbound{Channel: in.Channel, SessionID: in.SessionID, Text: "Stopped: max tool iterations reached."}
			run.Status = ports.RunFailed
			run.Error = "max_tool_iterations"
			run.UpdatedAt = l.Clock.Now()
			_ = l.Store.SaveRun(ctx, run)
			return out, nil
		}

		msgs, err := l.Store.ListMessages(ctx, in.SessionID, 500)
		if err != nil {
			return l.fail(ctx, run, err)
		}
		if l.SystemPrompt != "" {
			msgs = append([]ports.Message{{
				Role: "system", Content: l.SystemPrompt, CreatedAt: l.Clock.Now(),
			}}, msgs...)
		}
		if err := l.runHooks(ctx, ports.HookBeforeModel, hc); err != nil {
			return l.fail(ctx, run, err)
		}

		run.Status = ports.RunModel
		run.UpdatedAt = l.Clock.Now()
		_ = l.Store.SaveRun(ctx, run)

		resp, err := l.Model.Complete(ctx, ports.CompletionRequest{
			Messages: msgs,
			Tools:    l.toolSpecs(),
		})
		if err != nil {
			_ = l.runHooks(ctx, ports.HookOnError, hc)
			return l.fail(ctx, run, err)
		}
		if err := l.runHooks(ctx, ports.HookAfterModel, hc); err != nil {
			return l.fail(ctx, run, err)
		}

		asst := ports.Message{
			ID: newID(), SessionID: in.SessionID, Role: "assistant",
			Content: resp.Content, ToolCalls: resp.ToolCalls, CreatedAt: l.Clock.Now(),
		}
		// Persist assistant (including tool_calls) BEFORE any tool execution.
		if err := l.Store.SaveMessage(ctx, asst); err != nil {
			return l.fail(ctx, run, err)
		}

		if len(resp.ToolCalls) == 0 {
			run.Status = ports.RunCompleted
			run.UpdatedAt = l.Clock.Now()
			_ = l.Store.SaveRun(ctx, run)
			_ = l.runHooks(ctx, ports.HookOnComplete, hc)
			out := ports.Outbound{Channel: in.Channel, SessionID: in.SessionID, Text: resp.Content}
			if l.Channel != nil {
				_ = l.Channel.Send(ctx, out)
			}
			return out, nil
		}

		// Serial tool_calls in v1.
		run.Status = ports.RunToolPending
		run.UpdatedAt = l.Clock.Now()
		_ = l.Store.SaveRun(ctx, run)

		for _, call := range resp.ToolCalls {
			hc.ToolName = call.Name
			hc.ToolArgs = call.Args
			if err := l.runHooks(ctx, ports.HookBeforeTool, hc); err != nil {
				return l.fail(ctx, run, err)
			}
			if l.Policy != nil {
				if err := l.Policy.AllowTool(ctx, call.Name, call.Args); err != nil {
					tr := ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}
					if err := l.persistTool(ctx, in.SessionID, tr); err != nil {
						return l.fail(ctx, run, err)
					}
					continue
				}
			}
			tool, ok := l.Tools[call.Name]
			var tr ports.ToolResult
			if !ok {
				tr = ports.ToolResult{ID: call.ID, Name: call.Name, Content: "unknown tool: " + call.Name, IsError: true}
			} else {
				tr, err = tool.Exec(ctx, call)
				if err != nil {
					tr = ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}
				}
			}
			if err := l.runHooks(ctx, ports.HookAfterTool, hc); err != nil {
				return l.fail(ctx, run, err)
			}
			// Persist tool result BEFORE next model call.
			if err := l.persistTool(ctx, in.SessionID, tr); err != nil {
				return l.fail(ctx, run, err)
			}
			if tr.IsError {
				key := call.Name + "|" + tr.Content
				errCounts[key]++
				if errCounts[key] >= 3 {
					run.Status = ports.RunFailed
					run.Error = "repeated tool error: " + call.Name
					run.UpdatedAt = l.Clock.Now()
					_ = l.Store.SaveRun(ctx, run)
					return ports.Outbound{
						Channel: in.Channel, SessionID: in.SessionID,
						Text: "Stopped: tool " + call.Name + " failed repeatedly.",
					}, nil
				}
			}
		}

		run.Status = ports.RunToolDone
		run.Iteration++
		run.UpdatedAt = l.Clock.Now()
		_ = l.Store.SaveRun(ctx, run)
	}
}

func (l *Loop) persistTool(ctx context.Context, sessionID string, tr ports.ToolResult) error {
	return l.Store.SaveMessage(ctx, ports.Message{
		ID: newID(), SessionID: sessionID, Role: "tool",
		Content: tr.Content, ToolCallID: tr.ID, CreatedAt: l.Clock.Now(),
	})
}

func (l *Loop) fail(ctx context.Context, run ports.Run, err error) (ports.Outbound, error) {
	run.Status = ports.RunFailed
	run.Error = err.Error()
	run.UpdatedAt = l.Clock.Now()
	_ = l.Store.SaveRun(ctx, run)
	return ports.Outbound{}, err
}

// RecoverIncomplete marks unsafe incomplete runs failed. Resume of partial tool results is later.
func (l *Loop) RecoverIncomplete(ctx context.Context) error {
	runs, err := l.Store.ListIncompleteRuns(ctx)
	if err != nil {
		return err
	}
	for _, r := range runs {
		// v1: mark failed rather than invent tool results.
		r.Status = ports.RunFailed
		r.Error = "interrupted; marked failed on recovery"
		r.UpdatedAt = l.Clock.Now()
		_ = l.Store.SaveRun(ctx, r)
	}
	return nil
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
