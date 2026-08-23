package store

import (
	"context"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// Thin wrappers so one *SQLite can satisfy multiple ports without method-name clashes.

type Tasks struct{ DB *SQLite }

func (t Tasks) Create(ctx context.Context, task ports.Task) (ports.Task, error) {
	return t.DB.Create(ctx, task)
}
func (t Tasks) Get(ctx context.Context, id string) (ports.Task, error) { return t.DB.Get(ctx, id) }
func (t Tasks) List(ctx context.Context, f ports.TaskFilter) ([]ports.Task, error) {
	return t.DB.List(ctx, f)
}
func (t Tasks) ListOpen(ctx context.Context, owner ports.TaskOwner, limit int) ([]ports.Task, error) {
	return t.DB.ListOpen(ctx, owner, limit)
}
func (t Tasks) UpdateStatus(ctx context.Context, id string, status ports.TaskStatus, reason string) error {
	return t.DB.UpdateStatus(ctx, id, status, reason)
}
func (t Tasks) Complete(ctx context.Context, id string) error { return t.DB.Complete(ctx, id) }
func (t Tasks) Block(ctx context.Context, id string, reason string) error {
	return t.DB.Block(ctx, id, reason)
}
func (t Tasks) Cancel(ctx context.Context, id string, reason string) error {
	return t.DB.Cancel(ctx, id, reason)
}
func (t Tasks) GetByDedupeKey(ctx context.Context, key string) (ports.Task, error) {
	return t.DB.GetByDedupeKey(ctx, key)
}
func (t Tasks) ListDueOpen(ctx context.Context, now time.Time, limit int) ([]ports.Task, error) {
	return t.DB.ListDueOpen(ctx, now, limit)
}

type Inbox struct{ DB *SQLite }

func (i Inbox) Enqueue(ctx context.Context, item ports.InboxItem) (ports.InboxItem, error) {
	return i.DB.Enqueue(ctx, item)
}
func (i Inbox) ListPending(ctx context.Context, sessionID string, limit int) ([]ports.InboxItem, error) {
	return i.DB.ListPending(ctx, sessionID, limit)
}
func (i Inbox) ListPendingAny(ctx context.Context, limit int) ([]ports.InboxItem, error) {
	return i.DB.ListPendingAny(ctx, limit)
}
func (i Inbox) MarkDelivered(ctx context.Context, id string, runID string) error {
	return i.DB.MarkDelivered(ctx, id, runID)
}
func (i Inbox) Drop(ctx context.Context, id string, reason string) error {
	return i.DB.Drop(ctx, id, reason)
}
func (i Inbox) CountPending(ctx context.Context, sessionID string) (int, error) {
	return i.DB.CountPending(ctx, sessionID)
}
func (i Inbox) HasPendingExternal(ctx context.Context, sessionID, externalID string) (bool, error) {
	return i.DB.HasPendingExternal(ctx, sessionID, externalID)
}

type Schedules struct{ DB *SQLite }

func (s Schedules) Upsert(ctx context.Context, sch ports.Schedule) (ports.Schedule, error) {
	return s.DB.UpsertSchedule(ctx, sch)
}
func (s Schedules) Get(ctx context.Context, id string) (ports.Schedule, error) {
	return s.DB.GetSchedule(ctx, id)
}
func (s Schedules) ListEnabled(ctx context.Context) ([]ports.Schedule, error) {
	return s.DB.ListEnabledSchedules(ctx)
}
func (s Schedules) MarkFired(ctx context.Context, id string, at time.Time) error {
	return s.DB.MarkFired(ctx, id, at)
}

type Skills struct{ DB *SQLite }

func (s Skills) Upsert(ctx context.Context, sk ports.SkillRecord) error {
	return s.DB.UpsertSkill(ctx, sk)
}
func (s Skills) ListEnabled(ctx context.Context) ([]ports.SkillRecord, error) {
	return s.DB.ListEnabledSkills(ctx)
}
func (s Skills) Get(ctx context.Context, id string) (ports.SkillRecord, error) {
	return s.DB.GetSkill(ctx, id)
}
func (s Skills) ListAll(ctx context.Context) ([]ports.SkillRecord, error) {
	return s.DB.ListAllSkills(ctx)
}
