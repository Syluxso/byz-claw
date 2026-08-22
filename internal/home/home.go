package home

import (
	"os"
	"path/filepath"
)

// Resolve returns absolute BYZCLAW_HOME from flag, env, or ~/.byzclaw.
func Resolve(flagHome string) (string, error) {
	h := flagHome
	if h == "" {
		h = os.Getenv("BYZCLAW_HOME")
	}
	if h == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		h = filepath.Join(userHome, ".byzclaw")
	}
	return filepath.Abs(h)
}

type Paths struct {
	Root      string
	Config    string
	Secrets   string
	Workspace string
	Skills    string
	Memory    string
	Data      string
	DB        string
	Soul      string
	MemoryMD  string
	Heartbeat string
}

func PathsFor(root string) Paths {
	return Paths{
		Root:      root,
		Config:    filepath.Join(root, "config.yaml"),
		Secrets:   filepath.Join(root, "secrets"),
		Workspace: filepath.Join(root, "workspace"),
		Skills:    filepath.Join(root, "skills"),
		Memory:    filepath.Join(root, "memory"),
		Data:      filepath.Join(root, "data"),
		DB:        filepath.Join(root, "data", "byzclaw.db"),
		Soul:      filepath.Join(root, "SOUL.md"),
		MemoryMD:  filepath.Join(root, "MEMORY.md"),
		Heartbeat: filepath.Join(root, "HEARTBEAT.md"),
	}
}

func EnsureLayout(p Paths) error {
	for _, d := range []string{p.Root, p.Secrets, p.Workspace, p.Skills, p.Memory, p.Data} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
