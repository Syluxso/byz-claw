package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

func (s *SQLite) UpsertSchedule(ctx context.Context, sch ports.Schedule) (ports.Schedule, error) {
	if sch.ID == "" {
		sch.ID = newID()
	}
	now := time.Now().UTC()
	if sch.CreatedAt.IsZero() {
		sch.CreatedAt = now
	}
	sch.UpdatedAt = now
	en := 0
	if sch.Enabled {
		en = 1
	}
	var last any
	if sch.LastFiredAt != nil {
		last = sch.LastFiredAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO schedules (id, name, enabled, spec, task_template, wake, dedupe_template, quiet_hours_json, last_fired_at, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  name=excluded.name, enabled=excluded.enabled, spec=excluded.spec,
  task_template=excluded.task_template, wake=excluded.wake,
  dedupe_template=excluded.dedupe_template, quiet_hours_json=excluded.quiet_hours_json,
  updated_at=excluded.updated_at
`, sch.ID, sch.Name, en, sch.Spec, string(sch.TaskTemplate), sch.Wake, sch.DedupeTemplate, sch.QuietHoursJSON,
		last, sch.CreatedAt.Format(time.RFC3339Nano), sch.UpdatedAt.Format(time.RFC3339Nano))
	return sch, err
}

func (s *SQLite) GetSchedule(ctx context.Context, id string) (ports.Schedule, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, enabled, spec, task_template, wake, dedupe_template, quiet_hours_json, last_fired_at, created_at, updated_at
FROM schedules WHERE id = ?
`, id)
	return scanSchedule(row)
}

func (s *SQLite) ListEnabledSchedules(ctx context.Context) ([]ports.Schedule, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, enabled, spec, task_template, wake, dedupe_template, quiet_hours_json, last_fired_at, created_at, updated_at
FROM schedules WHERE enabled = 1
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ports.Schedule, 0)
	for rows.Next() {
		sch, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sch)
	}
	return out, rows.Err()
}

func (s *SQLite) MarkFired(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE schedules SET last_fired_at = ?, updated_at = ? WHERE id = ?
`, at.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

func scanSchedule(row scannable) (ports.Schedule, error) {
	var sch ports.Schedule
	var en int
	var last, created, updated sql.NullString
	var tmpl string
	err := row.Scan(&sch.ID, &sch.Name, &en, &sch.Spec, &tmpl, &sch.Wake, &sch.DedupeTemplate, &sch.QuietHoursJSON, &last, &created, &updated)
	if err == sql.ErrNoRows {
		return ports.Schedule{}, fmt.Errorf("schedule not found")
	}
	if err != nil {
		return ports.Schedule{}, err
	}
	sch.Enabled = en != 0
	sch.TaskTemplate = []byte(tmpl)
	if last.Valid {
		if tm, e := time.Parse(time.RFC3339Nano, last.String); e == nil {
			sch.LastFiredAt = &tm
		}
	}
	if created.Valid {
		sch.CreatedAt, _ = time.Parse(time.RFC3339Nano, created.String)
	}
	if updated.Valid {
		sch.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated.String)
	}
	return sch, nil
}
