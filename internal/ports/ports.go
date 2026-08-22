package ports

import (
	"context"
	"encoding/json"
	"time"
)

type Inbound struct {
	Channel   string
	SessionID string
	UserID    string
	Text      string
	Kind      string // "user" | "heartbeat" | "system"
	Meta      map[string]string
}

type Outbound struct {
	Channel   string
	SessionID string
	Text      string
	Meta      map[string]string
}

type Channel interface {
	Name() string
	Start(ctx context.Context, inbox chan<- Inbound) error
	Send(ctx context.Context, msg Outbound) error
}

type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
}

type ToolResult struct {
	ID      string
	Name    string
	Content string
	IsError bool
}

type ToolSpec struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Exec(ctx context.Context, call ToolCall) (ToolResult, error)
}

type Message struct {
	ID         string
	SessionID  string
	Role       string // user | assistant | tool | system
	Content    string
	ToolCalls  []ToolCall // assistant
	ToolCallID string     // tool
	CreatedAt  time.Time
}

type RunStatus string

const (
	RunAccepted    RunStatus = "accepted"
	RunModel       RunStatus = "model"
	RunToolPending RunStatus = "tool_pending"
	RunToolDone    RunStatus = "tool_done"
	RunCompleted   RunStatus = "completed"
	RunFailed      RunStatus = "failed"
	RunCancelled   RunStatus = "cancelled"
)

type Run struct {
	ID        string
	SessionID string
	Status    RunStatus
	Iteration int
	Error     string
	UpdatedAt time.Time
}

type Store interface {
	SaveMessage(ctx context.Context, m Message) error
	ListMessages(ctx context.Context, sessionID string, limit int) ([]Message, error)
	SaveRun(ctx context.Context, r Run) error
	LoadRun(ctx context.Context, id string) (Run, error)
	ListIncompleteRuns(ctx context.Context) ([]Run, error)
}

type CompletionRequest struct {
	Messages []Message
	Tools    []ToolSpec
	Model    string
}

type CompletionResponse struct {
	Content   string
	ToolCalls []ToolCall
	Usage     TokenUsage
}

type TokenUsage struct {
	Input  int
	Output int
}

type Model interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}

type Policy interface {
	AllowTool(ctx context.Context, name string, args json.RawMessage) error
	AllowPath(path string, op string) error // read | write | list
}

type Skill struct {
	ID    string
	Name  string
	Body  string
	Tools []string
}

type SkillSource interface {
	List(ctx context.Context) ([]Skill, error)
	Get(ctx context.Context, id string) (Skill, error)
}

type Secrets interface {
	Get(ctx context.Context, name string) (string, error)
}

type Memory interface {
	Read(ctx context.Context, relPath string) (string, error)
	Write(ctx context.Context, relPath string, content string) error
}

// Sandbox is a placeholder port; v1 uses "none".
type Sandbox interface {
	Name() string
}

type Clock interface {
	Now() time.Time
}

type HookPoint string

const (
	HookOnInbound   HookPoint = "on_inbound"
	HookBeforeModel HookPoint = "before_model"
	HookAfterModel  HookPoint = "after_model"
	HookBeforeTool  HookPoint = "before_tool"
	HookAfterTool   HookPoint = "after_tool"
	HookOnLimit     HookPoint = "on_limit"
	HookOnError     HookPoint = "on_error"
	HookOnComplete  HookPoint = "on_complete"
)

type HookContext struct {
	Run      *Run
	Session  string
	Inbound  *Inbound
	Notes    []string
	ToolName string
	ToolArgs json.RawMessage
}

type VetoError struct{ Reason string }

func (e VetoError) Error() string { return e.Reason }

type Hook interface {
	Point() HookPoint
	Name() string
	Run(ctx context.Context, hc *HookContext) error
}
