package torrent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBencode(t *testing.T) {
	got, err := Bencode(map[string]any{"b": "two", "a": int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "d1:ai1e1:b3:twoe" {
		t.Fatalf("unexpected bencode %s", got)
	}
}

func TestCreateTorrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "boot.iso")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := Create(path, "http://127.0.0.1:6969/announce", "http://127.0.0.1/boot.iso", 4)
	if err != nil {
		t.Fatal(err)
	}
	if result.InfoHash == "" {
		t.Fatal("missing info hash")
	}
	if _, err := os.Stat(result.TorrentPath); err != nil {
		t.Fatal(err)
	}
}
