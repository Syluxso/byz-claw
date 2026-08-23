package store

import (
	"context"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

func (s *SQLite) UpsertSkill(ctx context.Context, sk ports.SkillRecord) error {
	if sk.UpdatedAt.IsZero() {
		sk.UpdatedAt = time.Now().UTC()
	}
	en := 0
	if sk.Enabled {
		en = 1
	}
	if sk.ToolsJSON == "" {
		sk.ToolsJSON = "[]"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO skills (id, name, enabled, source, tools_json, content, md_ref, playbook_json, version, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  name=excluded.name, enabled=excluded.enabled, source=excluded.source,
  tools_json=excluded.tools_json, content=excluded.content, md_ref=excluded.md_ref,
  playbook_json=excluded.playbook_json, version=excluded.version, updated_at=excluded.updated_at
`, sk.ID, sk.Name, en, sk.Source, sk.ToolsJSON, sk.Content, sk.MdRef, sk.PlaybookJSON, sk.Version,
		sk.UpdatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *SQLite) GetSkill(ctx context.Context, id string) (ports.SkillRecord, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, enabled, source, tools_json, content, md_ref, playbook_json, version, updated_at
FROM skills WHERE id = ?
`, id)
	return scanSkill(row)
}

func (s *SQLite) ListEnabledSkills(ctx context.Context) ([]ports.SkillRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, enabled, source, tools_json, content, md_ref, playbook_json, version, updated_at
FROM skills WHERE enabled = 1 ORDER BY name ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSkills(rows)
}

func (s *SQLite) ListAllSkills(ctx context.Context) ([]ports.SkillRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, enabled, source, tools_json, content, md_ref, playbook_json, version, updated_at
FROM skills ORDER BY name ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSkills(rows)
}

func scanSkill(row scannable) (ports.SkillRecord, error) {
	var sk ports.SkillRecord
	var en int
	var updated string
	err := row.Scan(&sk.ID, &sk.Name, &en, &sk.Source, &sk.ToolsJSON, &sk.Content, &sk.MdRef, &sk.PlaybookJSON, &sk.Version, &updated)
	if err != nil {
		return ports.SkillRecord{}, err
	}
	sk.Enabled = en != 0
	sk.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return sk, nil
}

func scanSkills(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]ports.SkillRecord, error) {
	out := make([]ports.SkillRecord, 0)
	for rows.Next() {
		sk, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}
