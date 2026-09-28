package filetree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTraversal(t *testing.T) {
	for _, p := range []string{"../outside", "a/../../outside", "/absolute"} {
		if _, err := Path(p); err == nil {
			t.Errorf("accepted %q", p)
		}
	}
}
func TestRootRejectsExternalSymlink(t *testing.T) {
	base := t.TempDir()
	rootDir := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	os.Mkdir(rootDir, 0755)
	os.Mkdir(outside, 0755)
	os.WriteFile(filepath.Join(outside, "secret"), []byte("private"), 0644)
	if err := os.Symlink(outside, filepath.Join(rootDir, "escape")); err != nil {
		t.Skipf("symlink permission unavailable: %v", err)
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err = root.ReadFile("escape/secret"); err == nil {
		t.Fatal("read escaped")
	}
	if err = root.WriteFile("escape/new", []byte("bad"), 0644); err == nil {
		t.Fatal("write escaped")
	}
}
