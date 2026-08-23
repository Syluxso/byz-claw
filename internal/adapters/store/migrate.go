package store

import "context"

func (s *SQLite) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  tool_calls_json TEXT NOT NULL DEFAULT '[]',
  tool_call_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, created_at);

CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  status TEXT NOT NULL,
  iteration INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_runs_incomplete ON runs(status);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  body_ref TEXT NOT NULL DEFAULT '',
  owner TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'goal',
  status TEXT NOT NULL,
  blocked_reason TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  session_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  parent_id TEXT NOT NULL DEFAULT '',
  blocks_task_id TEXT NOT NULL DEFAULT '',
  done_when TEXT NOT NULL DEFAULT '',
  requires_approval INTEGER NOT NULL DEFAULT 0,
  dedupe_key TEXT NOT NULL DEFAULT '',
  due_at TEXT,
  wake TEXT NOT NULL DEFAULT 'none',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  completed_at TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_dedupe ON tasks(dedupe_key) WHERE dedupe_key != '';
CREATE INDEX IF NOT EXISTS idx_tasks_status_owner ON tasks(status, owner);
CREATE INDEX IF NOT EXISTS idx_tasks_due ON tasks(due_at) WHERE status = 'open' AND due_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS inbox (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  channel TEXT NOT NULL DEFAULT '',
  external_id TEXT NOT NULL DEFAULT '',
  task_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'user_message',
  payload TEXT NOT NULL DEFAULT '{}',
  state TEXT NOT NULL,
  arrived_at TEXT NOT NULL,
  delivered_run_id TEXT NOT NULL DEFAULT '',
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_inbox_pending ON inbox(session_id, state, arrived_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_inbox_ext ON inbox(session_id, external_id) WHERE external_id != '' AND state = 'pending';

CREATE TABLE IF NOT EXISTS schedules (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  spec TEXT NOT NULL,
  task_template TEXT NOT NULL DEFAULT '{}',
  wake TEXT NOT NULL DEFAULT 'on_mint',
  dedupe_template TEXT NOT NULL DEFAULT '',
  quiet_hours_json TEXT NOT NULL DEFAULT '',
  last_fired_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS skills (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  source TEXT NOT NULL DEFAULT 'local',
  tools_json TEXT NOT NULL DEFAULT '[]',
  content TEXT NOT NULL DEFAULT '',
  md_ref TEXT NOT NULL DEFAULT '',
  playbook_json TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
`)
	return err
}
