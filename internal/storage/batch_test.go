package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBatchCreationRollsBackOnConflict(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, filepath.Join(dir, "db"), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.UpsertClient(ctx, Client{Name: "existing", IP: "192.168.1.102"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.BatchCreateClients(ctx, "PC", "192.168.1.101", 3); err == nil {
		t.Fatal("conflict accepted")
	}
	rows, err := s.ListClients(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("partial records: %v %v", rows, err)
	}
	if _, err = s.BatchCreateClients(ctx, "PC", "192.168.1.110", 3); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListClients(ctx)
	if len(rows) != 4 {
		t.Fatal("batch not committed")
	}
}
