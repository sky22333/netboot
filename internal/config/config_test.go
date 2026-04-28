package config

import (
	"path/filepath"
	"testing"
)

func TestLoadOrCreate(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadOrCreate("", dir, "127.0.0.1", "19088")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Data.Dir != dir {
		t.Fatalf("unexpected data dir %s", cfg.Data.Dir)
	}
	if cfg.Admin.AdminAddr != "127.0.0.1:19088" {
		t.Fatalf("unexpected admin addr %s", cfg.Admin.AdminAddr)
	}
	if cfg.Database.Path != filepath.Join(dir, "pxe.db") {
		t.Fatalf("unexpected db path %s", cfg.Database.Path)
	}
}
