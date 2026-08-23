package ports

import (
	"context"
	"encoding/json"
	"time"
)

// SkillRecord is the DB registry row (runtime truth after doctor sync).
type SkillRecord struct {
	ID           string
	Name         string
	Enabled      bool
	Source       string // local | remote
	ToolsJSON    string
	Content      string // optional short prose
	MdRef        string
	PlaybookJSON string
	Version      string
	UpdatedAt    time.Time
}

func (s SkillRecord) ToolAllowlist() []string {
	var tools []string
	_ = json.Unmarshal([]byte(s.ToolsJSON), &tools)
	return tools
}

type SkillRegistry interface {
	Upsert(ctx context.Context, s SkillRecord) error
	ListEnabled(ctx context.Context) ([]SkillRecord, error)
	Get(ctx context.Context, id string) (SkillRecord, error)
	ListAll(ctx context.Context) ([]SkillRecord, error)
}
