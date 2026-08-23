package ports

import (
	"context"
	"encoding/json"
	"time"
)

type InboxState string

const (
	InboxPending   InboxState = "pending"
	InboxDelivered InboxState = "delivered"
	InboxDropped   InboxState = "dropped"
)

type InboxItem struct {
	ID             string
	SessionID      string
	Channel        string
	ExternalID     string
	TaskID         string // optional link — task is NOT moved into inbox
	Kind           string // user_message | task_wake | system
	Payload        json.RawMessage
	State          InboxState
	ArrivedAt      time.Time
	DeliveredRunID string
	Attempts       int
	LastError      string
}

type InboxStore interface {
	Enqueue(ctx context.Context, item InboxItem) (InboxItem, error)
	ListPending(ctx context.Context, sessionID string, limit int) ([]InboxItem, error)
	ListPendingAny(ctx context.Context, limit int) ([]InboxItem, error)
	MarkDelivered(ctx context.Context, id string, runID string) error
	Drop(ctx context.Context, id string, reason string) error
	CountPending(ctx context.Context, sessionID string) (int, error)
	HasPendingExternal(ctx context.Context, sessionID, externalID string) (bool, error)
}
