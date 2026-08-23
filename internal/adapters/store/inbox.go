package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

func (s *SQLite) Enqueue(ctx context.Context, item ports.InboxItem) (ports.InboxItem, error) {
	if item.ID == "" {
		item.ID = newID()
	}
	if item.State == "" {
		item.State = ports.InboxPending
	}
	if item.ArrivedAt.IsZero() {
		item.ArrivedAt = time.Now().UTC()
	}
	if len(item.Payload) == 0 {
		item.Payload = json.RawMessage(`{}`)
	}
	if item.ExternalID != "" {
		exists, err := s.HasPendingExternal(ctx, item.SessionID, item.ExternalID)
		if err != nil {
			return ports.InboxItem{}, err
		}
		if exists {
			// Return existing pending row for idempotency.
			rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, channel, external_id, task_id, kind, payload, state, arrived_at, delivered_run_id, attempts, last_error
FROM inbox WHERE session_id = ? AND external_id = ? AND state = 'pending' LIMIT 1
`, item.SessionID, item.ExternalID)
			if err != nil {
				return ports.InboxItem{}, err
			}
			defer rows.Close()
			if rows.Next() {
				return scanInbox(rows)
			}
		}
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO inbox (id, session_id, channel, external_id, task_id, kind, payload, state, arrived_at, delivered_run_id, attempts, last_error)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
`, item.ID, item.SessionID, item.Channel, item.ExternalID, item.TaskID, item.Kind, string(item.Payload),
		string(item.State), item.ArrivedAt.Format(time.RFC3339Nano), item.DeliveredRunID, item.Attempts, item.LastError)
	if err != nil {
		return ports.InboxItem{}, err
	}
	return item, nil
}

func (s *SQLite) ListPending(ctx context.Context, sessionID string, limit int) ([]ports.InboxItem, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, channel, external_id, task_id, kind, payload, state, arrived_at, delivered_run_id, attempts, last_error
FROM inbox WHERE session_id = ? AND state = 'pending' ORDER BY arrived_at ASC LIMIT ?
`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInboxes(rows)
}

func (s *SQLite) ListPendingAny(ctx context.Context, limit int) ([]ports.InboxItem, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, channel, external_id, task_id, kind, payload, state, arrived_at, delivered_run_id, attempts, last_error
FROM inbox WHERE state = 'pending' ORDER BY arrived_at ASC LIMIT ?
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInboxes(rows)
}

func (s *SQLite) MarkDelivered(ctx context.Context, id string, runID string) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE inbox SET state = 'delivered', delivered_run_id = ? WHERE id = ? AND state = 'pending'
`, runID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("inbox item not found or not pending")
	}
	return nil
}

func (s *SQLite) Drop(ctx context.Context, id string, reason string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE inbox SET state = 'dropped', last_error = ? WHERE id = ?
`, reason, id)
	return err
}

func (s *SQLite) CountPending(ctx context.Context, sessionID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inbox WHERE session_id = ? AND state = 'pending'`, sessionID).Scan(&n)
	return n, err
}

func (s *SQLite) HasPendingExternal(ctx context.Context, sessionID, externalID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM inbox WHERE session_id = ? AND external_id = ? AND state = 'pending'
`, sessionID, externalID).Scan(&n)
	return n > 0, err
}

func scanInbox(row scannable) (ports.InboxItem, error) {
	var it ports.InboxItem
	var state, arrived, payload string
	err := row.Scan(
		&it.ID, &it.SessionID, &it.Channel, &it.ExternalID, &it.TaskID, &it.Kind,
		&payload, &state, &arrived, &it.DeliveredRunID, &it.Attempts, &it.LastError,
	)
	if err != nil {
		return ports.InboxItem{}, err
	}
	it.State = ports.InboxState(state)
	it.Payload = json.RawMessage(payload)
	it.ArrivedAt, _ = time.Parse(time.RFC3339Nano, arrived)
	return it, nil
}

func scanInboxes(rows *sql.Rows) ([]ports.InboxItem, error) {
	out := make([]ports.InboxItem, 0)
	for rows.Next() {
		it, err := scanInbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
