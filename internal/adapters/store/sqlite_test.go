package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

func TestSaveAndListMessages(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	m := ports.Message{
		ID:        "m1",
		SessionID: "cli:local",
		Role:      "user",
		Content:   "hi",
		CreatedAt: time.Now().UTC(),
	}
	if err := s.SaveMessage(ctx, m); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListMessages(ctx, "cli:local", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Content != "hi" {
		t.Fatalf("unexpected list: %+v", list)
	}
}

func TestDeleteMessages(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_ = s.SaveMessage(ctx, ports.Message{ID: "a", SessionID: "s", Role: "user", Content: "1", CreatedAt: time.Now().UTC()})
	_ = s.SaveMessage(ctx, ports.Message{ID: "b", SessionID: "s", Role: "user", Content: "2", CreatedAt: time.Now().UTC()})
	if err := s.DeleteMessages(ctx, "s", []string{"a"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListMessages(ctx, "s", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "b" {
		t.Fatalf("%+v", list)
	}
}

func TestIncompleteRuns(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_ = s.SaveRun(ctx, ports.Run{ID: "r1", SessionID: "s", Status: ports.RunToolPending, UpdatedAt: time.Now().UTC()})
	_ = s.SaveRun(ctx, ports.Run{ID: "r2", SessionID: "s", Status: ports.RunCompleted, UpdatedAt: time.Now().UTC()})
	list, err := s.ListIncompleteRuns(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "r1" {
		t.Fatalf("expected one incomplete run, got %+v", list)
	}
}
