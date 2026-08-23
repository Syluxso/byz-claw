package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

// SyncSkillsDir upserts skills/*/SKILL.md into the registry (doctor / onboard).
func SyncSkillsDir(ctx context.Context, reg ports.SkillRegistry, dir string) (upserted int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "SKILL.md")
		sk, err := ParseSkillFile(path)
		if err != nil {
			continue
		}
		if sk.ID == "" {
			sk.ID = e.Name()
		}
		tools, _ := json.Marshal(sk.Tools)
		rec := ports.SkillRecord{
			ID:        sk.ID,
			Name:      sk.Name,
			Enabled:   true,
			Source:    "local",
			ToolsJSON: string(tools),
			Content:   sk.Body,
			MdRef:     path,
			UpdatedAt: time.Now().UTC(),
		}
		if err := reg.Upsert(ctx, rec); err != nil {
			return upserted, err
		}
		upserted++
	}
	return upserted, nil
}

// SkillsFromRegistry converts enabled DB skills to prompt Skill structs.
func SkillsFromRegistry(recs []ports.SkillRecord) []ports.Skill {
	out := make([]ports.Skill, 0, len(recs))
	for _, r := range recs {
		body := r.Content
		if body == "" && r.MdRef != "" {
			if b, err := os.ReadFile(r.MdRef); err == nil {
				if parsed, err := ParseSkillMarkdown(string(b)); err == nil {
					body = parsed.Body
				}
			}
		}
		// Keep short — Code First: no skill novels
		if len(body) > 4000 {
			body = body[:4000] + "…"
		}
		out = append(out, ports.Skill{
			ID:    r.ID,
			Name:  r.Name,
			Body:  body,
			Tools: r.ToolAllowlist(),
		})
	}
	return out
}
