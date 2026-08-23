package core

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

type memClock struct{ t time.Time }

func (c memClock) Now() time.Time { return c.t }

type memTasks struct {
	byID     map[string]ports.Task
	byDedupe map[string]ports.Task
	seq      int
}

func newMemTasks() *memTasks {
	return &memTasks{byID: map[string]ports.Task{}, byDedupe: map[string]ports.Task{}}
}

func (m *memTasks) Create(_ context.Context, t ports.Task) (ports.Task, error) {
	if t.DedupeKey != "" {
		if _, ok := m.byDedupe[t.DedupeKey]; ok {
			return ports.Task{}, fmt.Errorf("dedupe")
		}
	}
	m.seq++
	t.ID = fmt.Sprintf("task-%d", m.seq)
	m.byID[t.ID] = t
	if t.DedupeKey != "" {
		m.byDedupe[t.DedupeKey] = t
	}
	return t, nil
}

func (m *memTasks) Get(_ context.Context, id string) (ports.Task, error) {
	t, ok := m.byID[id]
	if !ok {
		return ports.Task{}, fmt.Errorf("missing")
	}
	return t, nil
}

func (m *memTasks) List(_ context.Context, _ ports.TaskFilter) ([]ports.Task, error) {
	return nil, nil
}

func (m *memTasks) ListOpen(_ context.Context, _ ports.TaskOwner, _ int) ([]ports.Task, error) {
	return nil, nil
}

func (m *memTasks) UpdateStatus(_ context.Context, id string, status ports.TaskStatus, reason string) error {
	t := m.byID[id]
	t.Status = status
	t.BlockedReason = reason
	m.byID[id] = t
	return nil
}

func (m *memTasks) Complete(_ context.Context, id string) error {
	return m.UpdateStatus(context.Background(), id, ports.TaskDone, "")
}

func (m *memTasks) Block(_ context.Context, id string, reason string) error {
	return m.UpdateStatus(context.Background(), id, ports.TaskBlocked, reason)
}

func (m *memTasks) Cancel(_ context.Context, id string, reason string) error {
	return m.UpdateStatus(context.Background(), id, ports.TaskCancelled, reason)
}

func (m *memTasks) GetByDedupeKey(_ context.Context, key string) (ports.Task, error) {
	t, ok := m.byDedupe[key]
	if !ok {
		return ports.Task{}, fmt.Errorf("missing")
	}
	return t, nil
}

func (m *memTasks) ListDueOpen(_ context.Context, now time.Time, limit int) ([]ports.Task, error) {
	var out []ports.Task
	for _, t := range m.byID {
		if t.Status != ports.TaskOpen || t.DueAt == nil || t.DueAt.After(now) {
			continue
		}
		out = append(out, t)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

type memSchedules struct {
	items []ports.Schedule
}

func (m *memSchedules) ListEnabled(_ context.Context) ([]ports.Schedule, error) {
	var out []ports.Schedule
	for _, s := range m.items {
		if s.Enabled {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *memSchedules) Get(_ context.Context, id string) (ports.Schedule, error) {
	for _, s := range m.items {
		if s.ID == id {
			return s, nil
		}
	}
	return ports.Schedule{}, fmt.Errorf("missing")
}

func (m *memSchedules) Upsert(_ context.Context, s ports.Schedule) (ports.Schedule, error) {
	for i, x := range m.items {
		if x.ID == s.ID {
			m.items[i] = s
			return s, nil
		}
	}
	m.items = append(m.items, s)
	return s, nil
}

func (m *memSchedules) MarkFired(_ context.Context, id string, at time.Time) error {
	for i, s := range m.items {
		if s.ID == id {
			s.LastFiredAt = &at
			m.items[i] = s
			return nil
		}
	}
	return fmt.Errorf("missing")
}

type memInbox struct {
	items []ports.InboxItem
}

func (m *memInbox) Enqueue(_ context.Context, item ports.InboxItem) (ports.InboxItem, error) {
	for _, x := range m.items {
		if item.ExternalID != "" && x.ExternalID == item.ExternalID && x.State == ports.InboxPending {
			return x, nil
		}
	}
	item.ID = fmt.Sprintf("in-%d", len(m.items)+1)
	if item.State == "" {
		item.State = ports.InboxPending
	}
	m.items = append(m.items, item)
	return item, nil
}

func (m *memInbox) ListPending(_ context.Context, _ string, _ int) ([]ports.InboxItem, error) {
	return nil, nil
}

func (m *memInbox) ListPendingAny(_ context.Context, _ int) ([]ports.InboxItem, error) {
	return nil, nil
}

func (m *memInbox) MarkDelivered(_ context.Context, _ string, _ string) error { return nil }

func (m *memInbox) Drop(_ context.Context, _ string, _ string) error { return nil }

func (m *memInbox) CountPending(_ context.Context, _ string) (int, error) { return 0, nil }

func (m *memInbox) HasPendingExternal(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

func TestSchedulerMintsOncePerDedupe(t *testing.T) {
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	tmpl, _ := json.Marshal(map[string]any{"title": "daily check"})
	sched := &memSchedules{items: []ports.Schedule{{
		ID: "s1", Name: "daily", Enabled: true, Spec: "1h",
		DedupeTemplate: "daily-{{date}}", Wake: "on_mint", TaskTemplate: tmpl,
	}}}
	tasks := newMemTasks()
	inbox := &memInbox{}
	s := &Scheduler{
		Schedules: sched, Tasks: tasks, Inbox: inbox,
		Clock:  memClock{t: now},
		Config: SchedulerConfig{Enabled: true, Tick: time.Minute, Location: time.UTC},
	}
	s.TickOnce(context.Background())
	s.TickOnce(context.Background())
	if len(tasks.byID) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks.byID))
	}
	if len(inbox.items) != 1 {
		t.Fatalf("expected 1 inbox wake, got %d", len(inbox.items))
	}
}

func TestSchedulerWakeNoneNeverInboxes(t *testing.T) {
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	tmpl, _ := json.Marshal(map[string]any{"title": "silent"})
	sched := &memSchedules{items: []ports.Schedule{{
		ID: "s1", Name: "silent", Enabled: true, Spec: "1h",
		Wake: "none", TaskTemplate: tmpl,
	}}}
	tasks := newMemTasks()
	inbox := &memInbox{}
	s := &Scheduler{
		Schedules: sched, Tasks: tasks, Inbox: inbox,
		Clock:  memClock{t: now},
		Config: SchedulerConfig{Enabled: true, Location: time.UTC},
	}
	s.TickOnce(context.Background())
	if len(tasks.byID) != 1 {
		t.Fatalf("expected task minted")
	}
	if len(inbox.items) != 0 {
		t.Fatalf("wake=none must not enqueue inbox, got %d", len(inbox.items))
	}
}

func TestSchedulerDueTaskEnqueuesOnce(t *testing.T) {
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	tasks := newMemTasks()
	t1 := ports.Task{
		ID: "task-1", Title: "due me", Owner: ports.TaskOwnerAgent,
		Status: ports.TaskOpen, DueAt: &past, Wake: "when_due", SessionID: "s",
	}
	tasks.byID[t1.ID] = t1
	inbox := &memInbox{}
	s := &Scheduler{
		Schedules: &memSchedules{}, Tasks: tasks, Inbox: inbox,
		Clock:  memClock{t: now},
		Config: SchedulerConfig{Enabled: true, Location: time.UTC},
	}
	s.TickOnce(context.Background())
	s.TickOnce(context.Background())
	if len(inbox.items) != 1 {
		t.Fatalf("expected single wake via external_id dedupe, got %d", len(inbox.items))
	}
}
