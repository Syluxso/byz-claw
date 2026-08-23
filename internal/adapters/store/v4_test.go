package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Syluxso/byzclaw/internal/ports"
)

func TestTaskDedupeAndComplete(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tasks := Tasks{DB: s}
	ctx := context.Background()
	_, err = tasks.Create(ctx, ports.Task{
		Title: "a", Owner: ports.TaskOwnerAgent, Status: ports.TaskOpen, DedupeKey: "k1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tasks.Create(ctx, ports.Task{
		Title: "b", Owner: ports.TaskOwnerAgent, Status: ports.TaskOpen, DedupeKey: "k1",
	})
	if err == nil {
		t.Fatal("expected dedupe unique fail")
	}
	got, err := tasks.GetByDedupeKey(ctx, "k1")
	if err != nil || got.Title != "a" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := tasks.Complete(ctx, got.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = tasks.Get(ctx, got.ID)
	if got.Status != ports.TaskDone {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestInboxDedupeExternal(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	inbox := Inbox{DB: s}
	ctx := context.Background()
	a, err := inbox.Enqueue(ctx, ports.InboxItem{
		SessionID: "s", ExternalID: "e1", Kind: "user_message", State: ports.InboxPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := inbox.Enqueue(ctx, ports.InboxItem{
		SessionID: "s", ExternalID: "e1", Kind: "user_message", State: ports.InboxPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Fatalf("expected same pending item %s vs %s", a.ID, b.ID)
	}
}

func TestDueTaskList(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tasks := Tasks{DB: s}
	ctx := context.Background()
	past := time.Now().UTC().Add(-time.Hour)
	_, _ = tasks.Create(ctx, ports.Task{
		Title: "due", Owner: ports.TaskOwnerAgent, Status: ports.TaskOpen, DueAt: &past, Wake: "when_due",
	})
	future := time.Now().UTC().Add(time.Hour)
	_, _ = tasks.Create(ctx, ports.Task{
		Title: "later", Owner: ports.TaskOwnerAgent, Status: ports.TaskOpen, DueAt: &future, Wake: "when_due",
	})
	due, err := tasks.ListDueOpen(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].Title != "due" {
		t.Fatalf("%+v", due)
	}
}
