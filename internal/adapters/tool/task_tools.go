package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// TaskTools exposes task_* verbs. Completion truth is TaskStore only.
type TaskTools struct {
	Store                  ports.TaskStore
	DefaultRequiresApproval bool
}

func (t *TaskTools) Tools() []ports.Tool {
	return []ports.Tool{
		&taskTool{t: t, name: "task_create", desc: "Create a durable task", schema: json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"},"owner":{"type":"string"},"done_when":{"type":"string"},"requires_approval":{"type":"boolean"},"due_at":{"type":"string"},"wake":{"type":"string"}},"required":["title"]}`), fn: t.create},
		&taskTool{t: t, name: "task_list", desc: "List open tasks", schema: json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string"},"limit":{"type":"integer"}}}`), fn: t.list},
		&taskTool{t: t, name: "task_get", desc: "Get a task by id", schema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`), fn: t.get},
		&taskTool{t: t, name: "task_complete", desc: "Mark task done (respects approval / owner=user rules)", schema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`), fn: t.complete},
		&taskTool{t: t, name: "task_block", desc: "Block a task with reason", schema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"reason":{"type":"string"}},"required":["id"]}`), fn: t.block},
	}
}

type taskTool struct {
	t      *TaskTools
	name   string
	desc   string
	schema json.RawMessage
	fn     func(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error)
}

func (x *taskTool) Name() string            { return x.name }
func (x *taskTool) Description() string     { return x.desc }
func (x *taskTool) Schema() json.RawMessage { return x.schema }
func (x *taskTool) Exec(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	return x.fn(ctx, call)
}

func (t *TaskTools) create(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	var args struct {
		Title            string `json:"title"`
		Owner            string `json:"owner"`
		DoneWhen         string `json:"done_when"`
		RequiresApproval *bool  `json:"requires_approval"`
		DueAt            string `json:"due_at"`
		Wake             string `json:"wake"`
	}
	_ = json.Unmarshal(call.Args, &args)
	if strings.TrimSpace(args.Title) == "" {
		return errResult(call, "title required"), nil
	}
	owner := ports.TaskOwnerAgent
	if args.Owner != "" {
		owner = ports.TaskOwner(args.Owner)
	}
	req := t.DefaultRequiresApproval
	if args.RequiresApproval != nil {
		req = *args.RequiresApproval
	}
	task := ports.Task{
		Title:            args.Title,
		Owner:            owner,
		Kind:             "goal",
		Status:           ports.TaskOpen,
		Source:           "tool",
		DoneWhen:         args.DoneWhen,
		RequiresApproval: req,
		Wake:             args.Wake,
	}
	if args.DueAt != "" {
		if tm, err := time.Parse(time.RFC3339, args.DueAt); err == nil {
			task.DueAt = &tm
		}
	}
	created, err := t.Store.Create(ctx, task)
	if err != nil {
		return errResult(call, err.Error()), nil
	}
	b, _ := json.Marshal(map[string]any{"id": created.ID, "status": created.Status, "owner": created.Owner})
	return ports.ToolResult{ID: call.ID, Name: call.Name, Content: string(b)}, nil
}

func (t *TaskTools) list(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	var args struct {
		Owner string `json:"owner"`
		Limit int    `json:"limit"`
	}
	_ = json.Unmarshal(call.Args, &args)
	f := ports.TaskFilter{Status: ports.TaskOpen, Limit: args.Limit}
	if args.Owner != "" {
		f.Owner = ports.TaskOwner(args.Owner)
	}
	list, err := t.Store.List(ctx, f)
	if err != nil {
		return errResult(call, err.Error()), nil
	}
	type row struct {
		ID, Owner, Title, Status, DoneWhen string `json:"id"`
	}
	out := make([]map[string]string, 0, len(list))
	for _, x := range list {
		out = append(out, map[string]string{
			"id": string(x.ID), "owner": string(x.Owner), "title": x.Title,
			"status": string(x.Status), "done_when": x.DoneWhen,
		})
	}
	b, _ := json.Marshal(out)
	return ports.ToolResult{ID: call.ID, Name: call.Name, Content: string(b)}, nil
}

func (t *TaskTools) get(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	var args struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(call.Args, &args)
	task, err := t.Store.Get(ctx, args.ID)
	if err != nil {
		return errResult(call, err.Error()), nil
	}
	b, _ := json.Marshal(task)
	return ports.ToolResult{ID: call.ID, Name: call.Name, Content: string(b)}, nil
}

func (t *TaskTools) complete(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	var args struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(call.Args, &args)
	task, err := t.Store.Get(ctx, args.ID)
	if err != nil {
		return errResult(call, err.Error()), nil
	}
	// Agent tools may not complete owner=user tasks.
	if task.Owner == ports.TaskOwnerUser {
		return errResult(call, "cannot complete owner=user task via agent tool"), nil
	}
	if task.RequiresApproval && task.Status != ports.TaskBlocked {
		// Block parent + create user approval task
		_ = t.Store.Block(ctx, task.ID, "awaiting_approval")
		approval, err := t.Store.Create(ctx, ports.Task{
			Title:        "Approve: " + task.Title,
			Owner:        ports.TaskOwnerUser,
			Kind:         "approval",
			Status:       ports.TaskOpen,
			ParentID:     task.ID,
			BlocksTaskID: task.ID,
			Source:       "approval",
			DoneWhen:     "user approves",
		})
		if err != nil {
			return errResult(call, err.Error()), nil
		}
		return ports.ToolResult{ID: call.ID, Name: call.Name, Content: fmt.Sprintf("blocked pending approval task %s", approval.ID)}, nil
	}
	if err := t.Store.Complete(ctx, task.ID); err != nil {
		return errResult(call, err.Error()), nil
	}
	// If this was an approval task completing, unblock/complete parent
	if task.Kind == "approval" && task.ParentID != "" {
		_ = t.Store.Complete(ctx, task.ParentID)
	}
	return ports.ToolResult{ID: call.ID, Name: call.Name, Content: "completed " + task.ID}, nil
}

func (t *TaskTools) block(ctx context.Context, call ports.ToolCall) (ports.ToolResult, error) {
	var args struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(call.Args, &args)
	if err := t.Store.Block(ctx, args.ID, args.Reason); err != nil {
		return errResult(call, err.Error()), nil
	}
	return ports.ToolResult{ID: call.ID, Name: call.Name, Content: "blocked " + args.ID}, nil
}

func errResult(call ports.ToolCall, msg string) ports.ToolResult {
	return ports.ToolResult{ID: call.ID, Name: call.Name, Content: msg, IsError: true}
}
