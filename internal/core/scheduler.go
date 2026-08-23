package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// SchedulerConfig drives the no-LLM tick loop.
type SchedulerConfig struct {
	Enabled    bool
	Tick       time.Duration
	QuietStart string
	QuietEnd   string
	Location   *time.Location
}

type Scheduler struct {
	Schedules ports.ScheduleStore
	Tasks     ports.TaskStore
	Inbox     ports.InboxStore
	Clock     ports.Clock
	Config    SchedulerConfig
}

// Start begins the ticker. Never calls the model.
func (s *Scheduler) Start(ctx context.Context) error {
	if !s.Config.Enabled {
		return nil
	}
	tick := s.Config.Tick
	if tick <= 0 {
		tick = 30 * time.Second
	}
	if s.Clock == nil {
		s.Clock = ports.RealClock{}
	}
	if s.Config.Location == nil {
		s.Config.Location = time.Local
	}
	go func() {
		t := time.NewTicker(tick)
		defer t.Stop()
		s.TickOnce(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.TickOnce(ctx)
			}
		}
	}()
	return nil
}

func (s *Scheduler) TickOnce(ctx context.Context) {
	now := s.Clock.Now().In(s.Config.Location)
	if InQuietHours(now, s.Config.QuietStart, s.Config.QuietEnd) {
		return
	}
	_ = s.fireSchedules(ctx, now)
	_ = s.wakeDueTasks(ctx, now)
}

func (s *Scheduler) fireSchedules(ctx context.Context, now time.Time) error {
	list, err := s.Schedules.ListEnabled(ctx)
	if err != nil {
		return err
	}
	for _, sch := range list {
		if !scheduleDue(sch, now) {
			continue
		}
		tmpl := map[string]any{}
		_ = json.Unmarshal(sch.TaskTemplate, &tmpl)
		title, _ := tmpl["title"].(string)
		if title == "" {
			title = sch.Name
		}
		owner := ports.TaskOwnerAgent
		if o, ok := tmpl["owner"].(string); ok && o != "" {
			owner = ports.TaskOwner(o)
		}
		doneWhen, _ := tmpl["done_when"].(string)
		reqAppr, _ := tmpl["requires_approval"].(bool)
		dedupe := expandDedupe(sch.DedupeTemplate, now)
		if dedupe != "" {
			if _, err := s.Tasks.GetByDedupeKey(ctx, dedupe); err == nil {
				_ = s.Schedules.MarkFired(ctx, sch.ID, now.UTC())
				continue
			}
		}
		wake := sch.Wake
		if wake == "" {
			wake = "on_mint"
		}
		task := ports.Task{
			Title:            title,
			Owner:            owner,
			Kind:             "procedure",
			Status:           ports.TaskOpen,
			Source:           "schedule:" + sch.ID,
			DoneWhen:         doneWhen,
			RequiresApproval: reqAppr,
			DedupeKey:        dedupe,
			Wake:             wake,
			Metadata:         sch.TaskTemplate,
		}
		created, err := s.Tasks.Create(ctx, task)
		if err != nil {
			continue
		}
		_ = s.Schedules.MarkFired(ctx, sch.ID, now.UTC())
		if wake == "on_mint" || wake == "when_due" {
			_ = s.enqueueTaskWake(ctx, created)
		}
	}
	return nil
}

func (s *Scheduler) wakeDueTasks(ctx context.Context, now time.Time) error {
	due, err := s.Tasks.ListDueOpen(ctx, now.UTC(), 50)
	if err != nil {
		return err
	}
	for _, t := range due {
		// none = never wake; empty defaults to when_due for timed tasks
		if t.Wake == "none" {
			continue
		}
		_ = s.enqueueTaskWake(ctx, t) // external_id dedupe prevents double wake
	}
	return nil
}

func (s *Scheduler) enqueueTaskWake(ctx context.Context, t ports.Task) error {
	session := t.SessionID
	if session == "" {
		session = "system:scheduler"
	}
	ext := "task-wake:" + t.ID
	payload, _ := json.Marshal(map[string]string{
		"task_id": t.ID,
		"title":   t.Title,
		"text":    "Task due: " + t.Title,
	})
	_, err := s.Inbox.Enqueue(ctx, ports.InboxItem{
		SessionID:  session,
		Channel:    "scheduler",
		ExternalID: ext,
		TaskID:     t.ID,
		Kind:       "task_wake",
		Payload:    payload,
		State:      ports.InboxPending,
	})
	return err
}

func scheduleDue(sch ports.Schedule, now time.Time) bool {
	spec := strings.TrimSpace(sch.Spec)
	if spec == "" {
		return false
	}
	// Interval duration: fire if never fired or last+interval <= now
	if d, err := time.ParseDuration(spec); err == nil {
		if sch.LastFiredAt == nil {
			return true
		}
		return !sch.LastFiredAt.Add(d).After(now.UTC())
	}
	// Minimal cron: "m h * * *" five-field; only minute+hour matched for v1
	parts := strings.Fields(spec)
	if len(parts) >= 2 {
		minOK := cronFieldMatch(parts[0], now.Minute())
		hourOK := cronFieldMatch(parts[1], now.Hour())
		if !minOK || !hourOK {
			return false
		}
		if sch.LastFiredAt != nil {
			// once per minute bucket
			lf := sch.LastFiredAt.In(now.Location())
			if lf.Year() == now.Year() && lf.YearDay() == now.YearDay() && lf.Hour() == now.Hour() && lf.Minute() == now.Minute() {
				return false
			}
		}
		return true
	}
	return false
}

func cronFieldMatch(field string, value int) bool {
	field = strings.TrimSpace(field)
	if field == "*" {
		return true
	}
	var n int
	if _, err := fmt.Sscanf(field, "%d", &n); err == nil {
		return n == value
	}
	return false
}

func expandDedupe(tmpl string, now time.Time) string {
	if tmpl == "" {
		return ""
	}
	out := tmpl
	out = strings.ReplaceAll(out, "{{date}}", now.Format("2006-01-02"))
	out = strings.ReplaceAll(out, "{{hour}}", now.Format("15"))
	return out
}
