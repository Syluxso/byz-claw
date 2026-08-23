package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *SQLite) Create(ctx context.Context, t ports.Task) (ports.Task, error) {
	if t.ID == "" {
		t.ID = newID()
	}
	if t.Status == "" {
		t.Status = ports.TaskOpen
	}
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if len(t.Metadata) == 0 {
		t.Metadata = json.RawMessage(`{}`)
	}
	var due any
	if t.DueAt != nil {
		due = t.DueAt.UTC().Format(time.RFC3339Nano)
	}
	req := 0
	if t.RequiresApproval {
		req = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO tasks (
  id, title, body_ref, owner, kind, status, blocked_reason, source,
  session_id, run_id, parent_id, blocks_task_id, done_when, requires_approval,
  dedupe_key, due_at, wake, metadata, created_at, updated_at, completed_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULL)
`, t.ID, t.Title, t.BodyRef, string(t.Owner), t.Kind, string(t.Status), t.BlockedReason, t.Source,
		t.SessionID, t.RunID, t.ParentID, t.BlocksTaskID, t.DoneWhen, req,
		t.DedupeKey, due, t.Wake, string(t.Metadata),
		t.CreatedAt.Format(time.RFC3339Nano), t.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return ports.Task{}, err
	}
	return t, nil
}

func (s *SQLite) Get(ctx context.Context, id string) (ports.Task, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+taskCols+` FROM tasks WHERE id = ?`, id)
	return scanTask(row)
}

func (s *SQLite) GetByDedupeKey(ctx context.Context, key string) (ports.Task, error) {
	if key == "" {
		return ports.Task{}, fmt.Errorf("empty dedupe key")
	}
	row := s.db.QueryRowContext(ctx, `SELECT `+taskCols+` FROM tasks WHERE dedupe_key = ?`, key)
	return scanTask(row)
}

func (s *SQLite) List(ctx context.Context, f ports.TaskFilter) ([]ports.Task, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT ` + taskCols + ` FROM tasks WHERE 1=1`
	args := []any{}
	if f.Owner != "" {
		q += ` AND owner = ?`
		args = append(args, string(f.Owner))
	}
	if f.Status != "" {
		q += ` AND status = ?`
		args = append(args, string(f.Status))
	}
	if f.Session != "" {
		q += ` AND session_id = ?`
		args = append(args, f.Session)
	}
	if f.DueOnly {
		q += ` AND status = 'open' AND due_at IS NOT NULL AND due_at <= ?`
		args = append(args, time.Now().UTC().Format(time.RFC3339Nano))
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *SQLite) ListOpen(ctx context.Context, owner ports.TaskOwner, limit int) ([]ports.Task, error) {
	return s.List(ctx, ports.TaskFilter{Owner: owner, Status: ports.TaskOpen, Limit: limit})
}

func (s *SQLite) ListDueOpen(ctx context.Context, now time.Time, limit int) ([]ports.Task, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT `+taskCols+` FROM tasks
WHERE status = 'open' AND due_at IS NOT NULL AND due_at <= ?
ORDER BY due_at ASC LIMIT ?
`, now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *SQLite) UpdateStatus(ctx context.Context, id string, status ports.TaskStatus, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var completed any
	if status == ports.TaskDone || status == ports.TaskCancelled {
		completed = now
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE tasks SET status = ?, blocked_reason = ?, updated_at = ?, completed_at = COALESCE(?, completed_at)
WHERE id = ?
`, string(status), reason, now, completed, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task not found")
	}
	return nil
}

func (s *SQLite) Complete(ctx context.Context, id string) error {
	return s.UpdateStatus(ctx, id, ports.TaskDone, "")
}

func (s *SQLite) Block(ctx context.Context, id string, reason string) error {
	return s.UpdateStatus(ctx, id, ports.TaskBlocked, reason)
}

func (s *SQLite) Cancel(ctx context.Context, id string, reason string) error {
	return s.UpdateStatus(ctx, id, ports.TaskCancelled, reason)
}

const taskCols = `id, title, body_ref, owner, kind, status, blocked_reason, source,
  session_id, run_id, parent_id, blocks_task_id, done_when, requires_approval,
  dedupe_key, due_at, wake, metadata, created_at, updated_at, completed_at`

type scannable interface {
	Scan(dest ...any) error
}

func scanTask(row scannable) (ports.Task, error) {
	var t ports.Task
	var owner, status, due, created, updated, completed, meta string
	var req int
	var dueNS, completedNS sql.NullString
	err := row.Scan(
		&t.ID, &t.Title, &t.BodyRef, &owner, &t.Kind, &status, &t.BlockedReason, &t.Source,
		&t.SessionID, &t.RunID, &t.ParentID, &t.BlocksTaskID, &t.DoneWhen, &req,
		&t.DedupeKey, &dueNS, &t.Wake, &meta, &created, &updated, &completedNS,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.Task{}, fmt.Errorf("task not found")
	}
	if err != nil {
		return ports.Task{}, err
	}
	t.Owner = ports.TaskOwner(owner)
	t.Status = ports.TaskStatus(status)
	t.RequiresApproval = req != 0
	t.Metadata = json.RawMessage(meta)
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	if dueNS.Valid {
		if tm, err := time.Parse(time.RFC3339Nano, dueNS.String); err == nil {
			t.DueAt = &tm
		}
		_ = due
	}
	if completedNS.Valid {
		if tm, err := time.Parse(time.RFC3339Nano, completedNS.String); err == nil {
			t.CompletedAt = &tm
		}
		_ = completed
	}
	return t, nil
}

func scanTasks(rows *sql.Rows) ([]ports.Task, error) {
	out := make([]ports.Task, 0)
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
