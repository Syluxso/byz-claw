package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathPolicy jails filesystem ops under Root (absolute). Uses EvalSymlinks.
type PathPolicy struct {
	Root           string
	EnabledTools   map[string]bool // empty = all allowed except denied
	DisabledTools  map[string]bool
	ShellEnabled   bool
}

func NewPathPolicy(root string) (*PathPolicy, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	// Ensure root exists for EvalSymlinks of children later.
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// brand-new empty dir is fine
		resolved = abs
	}
	return &PathPolicy{Root: resolved}, nil
}

func (p *PathPolicy) AllowTool(_ context.Context, name string, _ json.RawMessage) error {
	if name == "shell" && !p.ShellEnabled {
		return fmt.Errorf("shell tool is disabled")
	}
	if p.DisabledTools[name] {
		return fmt.Errorf("tool %q is disabled by policy", name)
	}
	if len(p.EnabledTools) > 0 && !p.EnabledTools[name] {
		return fmt.Errorf("tool %q is not enabled", name)
	}
	return nil
}

// AllowPath resolves relOrAbs against Root and ensures the result stays inside Root.
func (p *PathPolicy) AllowPath(path string, op string) error {
	_ = op
	clean, err := p.Resolve(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(p.Root, clean)
	if err != nil {
		return fmt.Errorf("path outside workspace: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes workspace root")
	}
	return nil
}

// Resolve returns the absolute, symlink-evaluated path under Root.
func (p *PathPolicy) Resolve(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	joined := path
	if !filepath.IsAbs(path) {
		joined = filepath.Join(p.Root, path)
	}
	cleaned := filepath.Clean(joined)

	// If the target does not exist yet (write), evaluate parent + append base.
	if _, err := os.Lstat(cleaned); err != nil {
		parent := filepath.Dir(cleaned)
		base := filepath.Base(cleaned)
		parentResolved, err := filepath.EvalSymlinks(parent)
		if err != nil {
			// walk up until we find an existing ancestor under root
			parentResolved, err = p.resolveExistingAncestor(parent)
			if err != nil {
				return "", err
			}
		}
		cleaned = filepath.Join(parentResolved, base)
	} else {
		resolved, err := filepath.EvalSymlinks(cleaned)
		if err != nil {
			return "", err
		}
		cleaned = resolved
	}

	root := p.Root
	if !strings.HasPrefix(cleaned, root+string(filepath.Separator)) && cleaned != root {
		return "", fmt.Errorf("path escapes workspace root")
	}
	return cleaned, nil
}

func (p *PathPolicy) resolveExistingAncestor(dir string) (string, error) {
	cur := filepath.Clean(dir)
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			if strings.HasPrefix(resolved, p.Root+string(filepath.Separator)) || resolved == p.Root {
				return resolved, nil
			}
			return "", fmt.Errorf("path escapes workspace root")
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("cannot resolve path under workspace")
		}
		cur = parent
	}
}
