package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"

	_ "modernc.org/sqlite"
)

type SQLite struct {
	db *sql.DB
}

func Open(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &SQLite{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLite) SaveMessage(ctx context.Context, m ports.Message) error {
	if m.ID == "" {
		return fmt.Errorf("message id required")
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	tc, err := json.Marshal(m.ToolCalls)
	if err != nil {
		return err
	}
	if tc == nil {
		tc = []byte("[]")
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO messages (id, session_id, role, content, tool_calls_json, tool_call_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  content=excluded.content,
  tool_calls_json=excluded.tool_calls_json,
  tool_call_id=excluded.tool_call_id
`, m.ID, m.SessionID, m.Role, m.Content, string(tc), m.ToolCallID, m.CreatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLite) DeleteMessages(ctx context.Context, sessionID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `DELETE FROM messages WHERE session_id = ? AND id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range ids {
		if _, err := stmt.ExecContext(ctx, sessionID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) ListMessages(ctx context.Context, sessionID string, limit int) ([]ports.Message, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, role, content, tool_calls_json, tool_call_id, created_at
FROM messages WHERE session_id = ? ORDER BY created_at ASC LIMIT ?
`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ports.Message, 0)
	for rows.Next() {
		var m ports.Message
		var tcJSON string
		var created string
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &tcJSON, &m.ToolCallID, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(tcJSON), &m.ToolCalls)
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *SQLite) SaveRun(ctx context.Context, r ports.Run) error {
	if r.ID == "" {
		return fmt.Errorf("run id required")
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO runs (id, session_id, status, iteration, error, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  status=excluded.status,
  iteration=excluded.iteration,
  error=excluded.error,
  updated_at=excluded.updated_at
`, r.ID, r.SessionID, string(r.Status), r.Iteration, r.Error, r.UpdatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLite) LoadRun(ctx context.Context, id string) (ports.Run, error) {
	var r ports.Run
	var status, updated string
	err := s.db.QueryRowContext(ctx, `
SELECT id, session_id, status, iteration, error, updated_at FROM runs WHERE id = ?
`, id).Scan(&r.ID, &r.SessionID, &status, &r.Iteration, &r.Error, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ports.Run{}, fmt.Errorf("run not found")
	}
	if err != nil {
		return ports.Run{}, err
	}
	r.Status = ports.RunStatus(status)
	r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return r, nil
}

func (s *SQLite) ListIncompleteRuns(ctx context.Context) ([]ports.Run, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, session_id, status, iteration, error, updated_at
FROM runs
WHERE status IN ('accepted','model','tool_pending','tool_done')
ORDER BY updated_at ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ports.Run, 0)
	for rows.Next() {
		var r ports.Run
		var status, updated string
		if err := rows.Scan(&r.ID, &r.SessionID, &status, &r.Iteration, &r.Error, &updated); err != nil {
			return nil, err
		}
		r.Status = ports.RunStatus(status)
		r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, r)
	}
	return out, rows.Err()
}
