package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPathJailBlocksDotDot(t *testing.T) {
	root := t.TempDir()
	p, err := NewPathPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AllowPath("../outside.txt", "read"); err == nil {
		t.Fatal("expected .. escape to fail")
	}
	if err := p.AllowPath("ok.txt", "write"); err != nil {
		t.Fatalf("relative write should be allowed: %v", err)
	}
}

func TestPathJailBlocksSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Symlink creation often requires admin on Windows CI; skip if we cannot create.
	}
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	p, err := NewPathPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.AllowPath("escape/secret.txt", "read"); err == nil {
		t.Fatal("expected symlink escape to fail")
	}
}

func TestPathJailAllowsNestedWrite(t *testing.T) {
	root := t.TempDir()
	p, err := NewPathPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := p.AllowPath("notes/hi.md", "write"); err != nil {
		t.Fatal(err)
	}
	abs, err := p.Resolve("notes/hi.md")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(abs) != filepath.Join(p.Root, "notes") && filepath.Clean(filepath.Dir(abs)) != filepath.Clean(filepath.Join(p.Root, "notes")) {
		// On Windows EvalSymlinks may change casing; ensure still under root.
		rel, relErr := filepath.Rel(p.Root, abs)
		if relErr != nil || rel == ".." || len(rel) >= 2 && rel[:2] == ".." {
			t.Fatalf("resolved path not under root: %s", abs)
		}
	}
}

func TestShellDisabled(t *testing.T) {
	p, err := NewPathPolicy(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p.ShellEnabled = false
	if err := p.AllowTool(nil, "shell", nil); err == nil {
		t.Fatal("shell should be denied")
	}
}
