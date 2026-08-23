package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"context"

	"github.com/Syluxso/byzclaw/config"
	"github.com/Syluxso/byzclaw/internal/adapters/secrets"
	storeadapter "github.com/Syluxso/byzclaw/internal/adapters/store"
	"github.com/Syluxso/byzclaw/internal/core"
	"github.com/Syluxso/byzclaw/internal/home"
)

type DoctorFinding struct {
	Level   string // ok | warn | critical
	Check   string
	Message string
}

func Doctor(homeRoot string) ([]DoctorFinding, error) {
	p := home.PathsFor(homeRoot)
	var out []DoctorFinding

	cfg, err := config.Load(p.Config)
	if err != nil {
		out = append(out, DoctorFinding{"critical", "config", err.Error()})
		return out, nil
	}
	out = append(out, DoctorFinding{"ok", "config", "parsed " + p.Config})

	for _, d := range []struct{ name, path string }{
		{"workspace", p.Workspace},
		{"skills", p.Skills},
		{"memory", p.Memory},
		{"data", p.Data},
		{"secrets", p.Secrets},
	} {
		fi, err := os.Stat(d.path)
		if err != nil || !fi.IsDir() {
			out = append(out, DoctorFinding{"critical", d.name, "missing directory: " + d.path})
		} else {
			out = append(out, DoctorFinding{"ok", d.name, d.path})
		}
	}

	// sqlite writable
	if err := os.MkdirAll(p.Data, 0o755); err != nil {
		out = append(out, DoctorFinding{"critical", "sqlite", err.Error()})
	} else {
		test := filepath.Join(p.Data, ".write_test")
		if err := os.WriteFile(test, []byte("ok"), 0o600); err != nil {
			out = append(out, DoctorFinding{"critical", "sqlite", "data dir not writable"})
		} else {
			_ = os.Remove(test)
			out = append(out, DoctorFinding{"ok", "sqlite", "data dir writable"})
		}
	}

	secretPath := filepath.Join(p.Secrets, cfg.Model.APIKeySecret)
	if _, err := os.Stat(secretPath); err != nil {
		out = append(out, DoctorFinding{"warn", "model_secret", "missing secrets/" + cfg.Model.APIKeySecret + " (needed for live model)"})
	} else {
		ok, mode, _ := secrets.ModeOK(secretPath)
		if runtime.GOOS != "windows" && !ok {
			out = append(out, DoctorFinding{"warn", "model_secret", fmt.Sprintf("mode %o (prefer 0600)", mode)})
		} else {
			out = append(out, DoctorFinding{"ok", "model_secret", "present"})
		}
	}

	if cfg.Tools.Shell.Enabled {
		out = append(out, DoctorFinding{"warn", "shell", "shell tool is ENABLED"})
	} else {
		out = append(out, DoctorFinding{"ok", "shell", "disabled (safe default)"})
	}

	if cfg.Channels.Telegram.Enabled {
		if len(cfg.Channels.Telegram.AllowFrom) == 0 {
			out = append(out, DoctorFinding{"critical", "telegram", "enabled but allow_from is empty"})
		} else {
			out = append(out, DoctorFinding{"ok", "telegram", "allow_from set"})
		}
	} else {
		out = append(out, DoctorFinding{"ok", "telegram", "disabled"})
	}

	if cfg.Channels.Webhook.Enabled {
		addr := cfg.Channels.Webhook.Addr
		if (strings.HasPrefix(addr, "0.0.0.0") || strings.HasPrefix(addr, ":") || addr == "") && !cfg.Channels.Webhook.AllowPublic {
			if strings.HasPrefix(addr, "0.0.0.0") || strings.HasPrefix(addr, ":") {
				out = append(out, DoctorFinding{"critical", "webhook", "public bind without allow_public"})
			} else {
				out = append(out, DoctorFinding{"ok", "webhook", addr})
			}
		} else if cfg.Channels.Webhook.AllowPublic {
			tok := filepath.Join(p.Secrets, "webhook_token")
			if _, err := os.Stat(tok); err != nil {
				out = append(out, DoctorFinding{"critical", "webhook", "allow_public requires secrets/webhook_token"})
			} else {
				out = append(out, DoctorFinding{"ok", "webhook", addr+" (public+token)"})
			}
		} else {
			out = append(out, DoctorFinding{"ok", "webhook", addr})
		}
	} else {
		out = append(out, DoctorFinding{"ok", "webhook", "disabled"})
	}

	for _, f := range []struct{ name, path string }{
		{"SOUL.md", p.Soul},
		{"MEMORY.md", p.MemoryMD},
	} {
		fi, err := os.Stat(f.path)
		if err != nil {
			out = append(out, DoctorFinding{"warn", f.name, "missing"})
			continue
		}
		if fi.Size() > 200_000 {
			out = append(out, DoctorFinding{"warn", f.name, "large file; may bloat prompts"})
		} else {
			out = append(out, DoctorFinding{"ok", f.name, "present"})
		}
	}

	if _, err := os.Stat(p.Heartbeat); err == nil {
		out = append(out, DoctorFinding{"warn", "HEARTBEAT.md", "present but unused (v4 uses scheduler+tasks)"})
	}

	if cfg.Scheduler.Enabled {
		out = append(out, DoctorFinding{"ok", "scheduler", "enabled tick=" + cfg.Scheduler.Tick})
	} else {
		out = append(out, DoctorFinding{"warn", "scheduler", "disabled"})
	}

	if cfg.Skills.DoctorSync {
		db, err := storeadapter.Open(p.DB)
		if err != nil {
			out = append(out, DoctorFinding{"warn", "skills_sync", err.Error()})
		} else {
			n, err := core.SyncSkillsDir(context.Background(), storeadapter.Skills{DB: db}, p.Skills)
			_ = db.Close()
			if err != nil {
				out = append(out, DoctorFinding{"warn", "skills_sync", err.Error()})
			} else {
				out = append(out, DoctorFinding{"ok", "skills_sync", fmt.Sprintf("upserted %d from skills/", n)})
			}
		}
	}

	return out, nil
}

func DoctorHasCritical(findings []DoctorFinding) bool {
	for _, f := range findings {
		if f.Level == "critical" {
			return true
		}
	}
	return false
}
