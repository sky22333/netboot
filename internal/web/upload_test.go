package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"pxe/internal/filetree"
)

func TestUploadPublicationAndCleanup(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ctx := context.Background()
	if err := saveUpload(ctx, root, "image.iso", strings.NewReader("image"), 5); err != nil {
		t.Fatal(err)
	}
	if err := saveUpload(ctx, root, "image.iso", strings.NewReader("other"), 5); !errors.Is(err, os.ErrExist) {
		t.Fatalf("overwrite: %v", err)
	}
	data, _ := root.ReadFile("image.iso")
	if string(data) != "image" {
		t.Fatal("existing file changed")
	}
	if err := saveUpload(ctx, root, "short.iso", strings.NewReader("x"), 10); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if err := saveUpload(ctx, root, "broken.iso", brokenReader{}, 10); err == nil {
		t.Fatal("read error accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := saveUpload(canceled, root, "canceled.iso", strings.NewReader("x"), 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	f, _ := root.Open(".")
	defer f.Close()
	entries, _ := f.ReadDir(-1)
	if len(entries) != 1 || entries[0].Name() != "image.iso" {
		t.Fatalf("partial files leaked: %v", entries)
	}
	stale := filetree.UploadTempPrefix + "stale"
	fresh := filetree.UploadTempPrefix + "fresh"
	if err := root.WriteFile(stale, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(fresh, nil, 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := root.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	cleanupUploads(root)
	if _, err := root.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale file retained")
	}
	if _, err := root.Stat(fresh); err != nil {
		t.Fatal("active file removed")
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("connection lost") }

func TestStreamingUploadHTTP(t *testing.T) {
	r, store := testRouter(t)
	token := setupAdmin(t, r)
	settings, err := store.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(settings.HTTPBoot.Root, 0755); err != nil {
		t.Fatal(err)
	}
	send := func(path, contentType, body string, size int64) *httptest.ResponseRecorder {
		q := httptest.NewRequest("POST", "/api/v1/files/upload?root=http&path="+path, strings.NewReader(body))
		q.Header.Set("Content-Type", contentType)
		q.ContentLength = size
		q.AddCookie(&http.Cookie{Name: "pxe_session", Value: token})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, q)
		return w
	}
	for _, tc := range []struct {
		path, kind, body string
		size             int64
		status           int
	}{
		{"image.iso", "application/octet-stream", "ISO", 3, 200},
		{"image.iso", "application/octet-stream", "new", 3, 409},
		{"large.iso", "application/octet-stream", "", 32<<30 + 1, 413},
		{"unknown.iso", "application/octet-stream", "", -1, 411},
		{"old.iso", "multipart/form-data", "", 0, 415},
		{"short.iso", "application/octet-stream", "x", 9, 400},
		{"../escape.iso", "application/octet-stream", "x", 1, 400},
		{".pxe-upload-private", "application/octet-stream", "x", 1, 400},
	} {
		w := send(tc.path, tc.kind, tc.body, tc.size)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
		}
	}
	root, _ := os.OpenRoot(settings.HTTPBoot.Root)
	defer root.Close()
	if err := root.WriteFile(filetree.UploadTempPrefix+"hidden", []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	w := request(r, "GET", "/api/v1/files?root=http", "", token)
	if w.Code != 200 || strings.Contains(w.Body.String(), "hidden") || !strings.Contains(w.Body.String(), "34359738368") {
		t.Fatal(w.Body)
	}
	w = request(r, "GET", "/api/v1/files/content?root=http&path=.pxe-upload-hidden", "", token)
	if w.Code != 400 {
		t.Fatal("temporary content exposed")
	}
}
