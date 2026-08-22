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

	return l.continueRun(ctx, &run, in, hc, map[string]int{})
}

func (l *Loop) continueRun(
	ctx context.Context,
	run *ports.Run,
	in ports.Inbound,
	hc *ports.HookContext,
	errCounts map[string]int,
) (ports.Outbound, error) {
	if errCounts == nil {
		errCounts = map[string]int{}
	}
	for {
		if run.Iteration >= l.Config.MaxToolIterations {
			_ = l.runHooks(ctx, ports.HookOnLimit, hc)
			out := ports.Outbound{Channel: in.Channel, SessionID: in.SessionID, Text: "Stopped: max tool iterations reached."}
			run.Status = ports.RunFailed
			run.Error = "max_tool_iterations"
			run.UpdatedAt = l.Clock.Now()
			_ = l.Store.SaveRun(ctx, *run)
			return out, nil
		}

		msgs, err := l.Store.ListMessages(ctx, in.SessionID, 500)
		if err != nil {
			return l.fail(ctx, *run, err)
		}
		msgs, _ = l.CompactIfNeeded(ctx, in.SessionID, msgs)

		prompt := msgs
		if l.SystemPrompt != "" {
			prompt = append([]ports.Message{{
				Role: "system", Content: l.SystemPrompt, CreatedAt: l.Clock.Now(),
			}}, msgs...)
		}
		if err := l.runHooks(ctx, ports.HookBeforeModel, hc); err != nil {
			return l.fail(ctx, *run, err)
		}

		run.Status = ports.RunModel
		run.UpdatedAt = l.Clock.Now()
		_ = l.Store.SaveRun(ctx, *run)

		resp, err := l.Model.Complete(ctx, ports.CompletionRequest{
			Messages: prompt,
			Tools:    l.toolSpecs(),
		})
		if err != nil {
			_ = l.runHooks(ctx, ports.HookOnError, hc)
			return l.fail(ctx, *run, err)
		}
		hc.Notes = append(hc.Notes, fmt.Sprintf("usage_total=%d", resp.Usage.Input+resp.Usage.Output))
		if err := l.runHooks(ctx, ports.HookAfterModel, hc); err != nil {
			return l.fail(ctx, *run, err)
		}

		asst := ports.Message{
			ID: newID(), SessionID: in.SessionID, Role: "assistant",
			Content: resp.Content, ToolCalls: resp.ToolCalls, CreatedAt: l.Clock.Now(),
		}
		if err := l.Store.SaveMessage(ctx, asst); err != nil {
			return l.fail(ctx, *run, err)
		}

		if len(resp.ToolCalls) == 0 {
			run.Status = ports.RunCompleted
			run.UpdatedAt = l.Clock.Now()
			_ = l.Store.SaveRun(ctx, *run)
			_ = l.runHooks(ctx, ports.HookOnComplete, hc)
			out := ports.Outbound{Channel: in.Channel, SessionID: in.SessionID, Text: resp.Content}
			if l.Channel != nil {
				_ = l.Channel.Send(ctx, out)
			}
			return out, nil
		}

		run.Status = ports.RunToolPending
		run.UpdatedAt = l.Clock.Now()
		_ = l.Store.SaveRun(ctx, *run)

		stop, out, err := l.execToolCalls(ctx, run, in, hc, resp.ToolCalls, errCounts)
		if err != nil {
			return l.fail(ctx, *run, err)
		}
		if stop {
			return out, nil
		}

		run.Status = ports.RunToolDone
		run.Iteration++
		run.UpdatedAt = l.Clock.Now()
		_ = l.Store.SaveRun(ctx, *run)
	}
}

func (l *Loop) execToolCalls(
	ctx context.Context,
	run *ports.Run,
	in ports.Inbound,
	hc *ports.HookContext,
	calls []ports.ToolCall,
	errCounts map[string]int,
) (stop bool, out ports.Outbound, err error) {
	for _, call := range calls {
		hc.ToolName = call.Name
		hc.ToolArgs = call.Args
		if err := l.runHooks(ctx, ports.HookBeforeTool, hc); err != nil {
			return true, ports.Outbound{}, err
		}
		var tr ports.ToolResult
		if l.Policy != nil {
			if err := l.Policy.AllowTool(ctx, call.Name, call.Args); err != nil {
				tr = ports.ToolResult{ID: call.ID, Name: call.Name, Content: err.Error(), IsError: true}
				if err := l.persistTool(ctx, in.SessionID, tr); err != nil {
					return true, ports.Outbound{}, err
				}
				_ = l.runHooks(ctx, ports.HookAfterTool, hc)
				continue
			}
		}
		tool, ok := l.Tools[call.Name]
		if !ok {
			tr = ports.ToolResult{ID: call.ID, Name: call.Name, Content: "unknown tool: " + call.Name, IsError: true}
		} else {
			var execErr error
			tr, execErr = tool.Exec(ctx, call)
			if execErr != nil {
				tr = ports.ToolResult{ID: call.ID, Name: call.Name, Content: execErr.Error(), IsError: true}
			}
		}
		if err := l.runHooks(ctx, ports.HookAfterTool, hc); err != nil {
			return true, ports.Outbound{}, err
		}
		if err := l.persistTool(ctx, in.SessionID, tr); err != nil {
			return true, ports.Outbound{}, err
		}
		if tr.IsError {
			key := call.Name + "|" + tr.Content
			errCounts[key]++
			if errCounts[key] >= 3 {
				run.Status = ports.RunFailed
				run.Error = "repeated tool error: " + call.Name
				run.UpdatedAt = l.Clock.Now()
				_ = l.Store.SaveRun(ctx, *run)
				return true, ports.Outbound{
					Channel: in.Channel, SessionID: in.SessionID,
					Text: "Stopped: tool " + call.Name + " failed repeatedly.",
				}, nil
			}
		}
	}
	return false, ports.Outbound{}, nil
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

// RecoverIncomplete resumes runs that have assistant tool_calls with missing tool results.
// Otherwise marks the run failed (never invents results).
func (l *Loop) RecoverIncomplete(ctx context.Context) error {
	runs, err := l.Store.ListIncompleteRuns(ctx)
	if err != nil {
		return err
	}
	for _, r := range runs {
		if err := l.resumeOne(ctx, r); err != nil {
			r.Status = ports.RunFailed
			r.Error = "recovery failed: " + err.Error()
			r.UpdatedAt = l.Clock.Now()
			_ = l.Store.SaveRun(ctx, r)
		}
	}
	return nil
}

func (l *Loop) resumeOne(ctx context.Context, r ports.Run) error {
	msgs, err := l.Store.ListMessages(ctx, r.SessionID, 500)
	if err != nil {
		return err
	}
	asst, missing := missingToolCalls(msgs)
	if asst == nil {
		r.Status = ports.RunFailed
		r.Error = "interrupted; no resumable tool_calls"
		r.UpdatedAt = l.Clock.Now()
		return l.Store.SaveRun(ctx, r)
	}
	if len(missing) == 0 {
		// Tools done but model not finished — continue loop without new user message.
		r.Status = ports.RunToolDone
		r.UpdatedAt = l.Clock.Now()
		_ = l.Store.SaveRun(ctx, r)
		in := ports.Inbound{Channel: "system", SessionID: r.SessionID, Kind: "system", Text: ""}
		hc := &ports.HookContext{Run: &r, Session: r.SessionID, Inbound: &in}
		_, err := l.continueRun(ctx, &r, in, hc, map[string]int{})
		return err
	}

	l.mu.Lock()
	if l.active[r.SessionID] {
		l.mu.Unlock()
		return fmt.Errorf("session busy")
	}
	l.active[r.SessionID] = true
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		delete(l.active, r.SessionID)
		l.mu.Unlock()
	}()

	in := ports.Inbound{Channel: "system", SessionID: r.SessionID, Kind: "system"}
	hc := &ports.HookContext{Run: &r, Session: r.SessionID, Inbound: &in}
	r.Status = ports.RunToolPending
	r.UpdatedAt = l.Clock.Now()
	_ = l.Store.SaveRun(ctx, r)

	stop, _, err := l.execToolCalls(ctx, &r, in, hc, missing, map[string]int{})
	if err != nil {
		return err
	}
	if stop {
		return nil
	}
	r.Status = ports.RunToolDone
	r.Iteration++
	r.UpdatedAt = l.Clock.Now()
	_ = l.Store.SaveRun(ctx, r)
	_, err = l.continueRun(ctx, &r, in, hc, map[string]int{})
	return err
}

func missingToolCalls(msgs []ports.Message) (asst *ports.Message, missing []ports.ToolCall) {
	var last *ports.Message
	for i := range msgs {
		if msgs[i].Role == "assistant" && len(msgs[i].ToolCalls) > 0 {
			last = &msgs[i]
		}
	}
	if last == nil {
		return nil, nil
	}
	have := map[string]bool{}
	seenLast := false
	for _, m := range msgs {
		if !seenLast {
			if m.ID == last.ID {
				seenLast = true
			}
			continue
		}
		if m.Role == "tool" && m.ToolCallID != "" {
			have[m.ToolCallID] = true
		}
	}
	for _, tc := range last.ToolCalls {
		if !have[tc.ID] {
			missing = append(missing, tc)
		}
	}
	return last, missing
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
