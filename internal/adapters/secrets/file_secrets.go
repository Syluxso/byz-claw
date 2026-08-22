package secrets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// FileSecrets reads $HOME/secrets/<name> (mode ideally 0600).
type FileSecrets struct {
	Dir string
}

func (f *FileSecrets) Get(_ context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid secret name")
	}
	path := filepath.Join(f.Dir, name)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("secret %q: %w", name, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func (f *FileSecrets) Put(name, value string) error {
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(f.Dir, name)
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(path, 0o600)
	}
	return nil
}

func ModeOK(path string) (bool, os.FileMode, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, 0, err
	}
	mode := fi.Mode().Perm()
	if runtime.GOOS == "windows" {
		return true, mode, nil // ACLs differ; doctor warns lightly
	}
	return mode == 0o600, mode, nil
}
