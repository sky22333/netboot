package filetree

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
)

func TestRenameNeverReplacesFilesOrDirectories(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, directory := range []bool{false, true} {
		from, to := "a", "b"
		if directory {
			from, to = "da", "db"
			root.Mkdir(from, 0755)
			root.Mkdir(to, 0755)
		} else {
			root.WriteFile(from, []byte("A"), 0644)
			root.WriteFile(to, []byte("B"), 0644)
		}
		if err := Rename(root, from, to); !errors.Is(err, os.ErrExist) {
			t.Fatalf("overwrite directory=%v: %v", directory, err)
		}
		if _, err := root.Stat(from); err != nil {
			t.Fatal("source lost", err)
		}
		if !directory {
			data, _ := root.ReadFile(to)
			if string(data) != "B" {
				t.Fatal("target changed")
			}
		}
		if err := Rename(root, from, from+"-renamed"); err != nil {
			t.Fatal("rename", err)
		}
	}
}

func TestTextCreationConflictAndConcurrentRevision(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ctx := context.Background()
	if err := WriteText(ctx, root, "script.ipxe", "original", ""); err != nil {
		t.Fatal(err)
	}
	if err := WriteText(ctx, root, "script.ipxe", "", ""); !errors.Is(err, ErrConflict) {
		t.Fatal("creation overwrote", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, content := range []string{"one", "two"} {
		wg.Add(1)
		go func(content string) {
			defer wg.Done()
			results <- WriteText(ctx, root, "script.ipxe", content, Revision([]byte("original")))
		}(content)
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflict=%d", successes, conflicts)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	data, _ := root.ReadFile("script.ipxe")
	if err := WriteText(canceled, root, "script.ipxe", "lost", Revision(data)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	after, _ := root.ReadFile("script.ipxe")
	if string(after) != string(data) {
		t.Fatal("failed save changed file")
	}
}
