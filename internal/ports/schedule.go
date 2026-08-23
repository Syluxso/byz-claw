package ports

import (
	"context"
	"encoding/json"
	"time"
)

type Schedule struct {
	ID             string
	Name           string
	Enabled        bool
	Spec           string // cron expression OR interval duration (e.g. "1h", "30m")
	TaskTemplate   json.RawMessage
	Wake           string // none | on_mint | when_due
	DedupeTemplate string
	QuietHoursJSON string
	LastFiredAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ScheduleStore interface {
	Upsert(ctx context.Context, s Schedule) (Schedule, error)
	Get(ctx context.Context, id string) (Schedule, error)
	ListEnabled(ctx context.Context) ([]Schedule, error)
	MarkFired(ctx context.Context, id string, at time.Time) error
}
