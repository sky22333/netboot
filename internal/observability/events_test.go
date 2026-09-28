package observability

import (
	"context"
	"path/filepath"
	"pxe/internal/storage"
	"testing"
)

func TestEventIdentitySurvivesHubRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.Open(context.Background(), filepath.Join(dir, "db"), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	hub := NewHub(s)
	ch, unsub := hub.Subscribe()
	defer unsub()
	hub.Publish("info", "test", "first")
	live := <-ch
	rows, err := s.RecentEvents(context.Background(), 10)
	if err != nil || len(rows) != 1 || rows[0] != live {
		t.Fatal("different live/history identity", rows, live, err)
	}
	next := NewHub(s)
	next.Publish("info", "test", "second")
	if next.Recent()[0].ID <= live.ID {
		t.Fatal("event ID reused")
	}
}
