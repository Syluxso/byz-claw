package ports

import (
	"context"
	"encoding/json"
	"time"
)

type TaskOwner string

const (
	TaskOwnerAgent   TaskOwner = "agent"
	TaskOwnerUser    TaskOwner = "user"
	TaskOwnerRun     TaskOwner = "run"
	TaskOwnerProcess TaskOwner = "process"
	TaskOwnerSystem  TaskOwner = "system"
)

type TaskStatus string

const (
	TaskOpen      TaskStatus = "open"
	TaskBlocked   TaskStatus = "blocked"
	TaskDone      TaskStatus = "done"
	TaskCancelled TaskStatus = "cancelled"
)

type Task struct {
	ID               string
	Title            string
	BodyRef          string
	Owner            TaskOwner
	Kind             string // goal | step | procedure | approval
	Status           TaskStatus
	BlockedReason    string
	Source           string
	SessionID        string
	RunID            string
	ParentID         string
	BlocksTaskID     string
	DoneWhen         string
	RequiresApproval bool
	DedupeKey        string
	DueAt            *time.Time
	Wake             string // none | on_mint | when_due — used when minted from schedule; on task = wake eligibility
	Metadata         json.RawMessage
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CompletedAt      *time.Time
}

type TaskFilter struct {
	Owner    TaskOwner
	Status   TaskStatus
	Session  string
	Limit    int
	DueOnly  bool // due_at <= now and open
}

type TaskStore interface {
	Create(ctx context.Context, t Task) (Task, error)
	Get(ctx context.Context, id string) (Task, error)
	List(ctx context.Context, f TaskFilter) ([]Task, error)
	ListOpen(ctx context.Context, owner TaskOwner, limit int) ([]Task, error)
	UpdateStatus(ctx context.Context, id string, status TaskStatus, reason string) error
	Complete(ctx context.Context, id string) error
	Block(ctx context.Context, id string, reason string) error
	Cancel(ctx context.Context, id string, reason string) error
	GetByDedupeKey(ctx context.Context, key string) (Task, error)
	ListDueOpen(ctx context.Context, now time.Time, limit int) ([]Task, error)
}
