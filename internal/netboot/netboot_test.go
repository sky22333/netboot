package netboot

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func TestDownloadDoesNotPublishPartialFile(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err = saveDownload(root, "firmware.efi", failingReader{}); err == nil {
		t.Fatal("accepted truncated transfer")
	}
	if _, err = root.Stat("firmware.efi"); !os.IsNotExist(err) {
		t.Fatal("partial target exists")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("temporary file leaked")
	}
	if _, err = saveDownload(root, "firmware.efi", strings.NewReader("firmware")); err != nil {
		t.Fatal(err)
	}
}
func TestMalformedDownloadURLReturnsError(t *testing.T) {
	if _, err := tryDownload(context.Background(), http.DefaultClient, "http://[invalid"); err == nil {
		t.Fatal("malformed URL accepted")
	}
}
