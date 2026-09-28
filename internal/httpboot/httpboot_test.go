package httpboot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pxe/internal/observability"
	"pxe/internal/storage"
)

func TestFileHandlerDirectoryListingDisabled(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.HTTPBoot.DirectoryListing = false

	req := httptest.NewRequest(http.MethodGet, "http://pxe.local/", nil)
	req.RemoteAddr = "192.168.1.50:12345"
	rec := httptest.NewRecorder()
	fileHandler(settings, observability.NewHub(store)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disabled directory listing, got %d", rec.Code)
	}
}

func TestFileHandlerDisablesRangeRequests(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.HTTPBoot.RangeRequests = false
	if err := os.WriteFile(filepath.Join(settings.HTTPBoot.Root, "kernel"), []byte("abcdef"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://pxe.local/kernel", nil)
	req.Header.Set("Range", "bytes=0-2")
	req.RemoteAddr = "192.168.1.50:12345"
	rec := httptest.NewRecorder()
	fileHandler(settings, observability.NewHub(store)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected full response when range disabled, got %d", rec.Code)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "none" {
		t.Fatalf("expected Accept-Ranges none, got %q", got)
	}
	if body := rec.Body.String(); body != "abcdef" {
		t.Fatalf("expected full body, got %q", body)
	}
}

func TestHTTPFileSentMessageIncludesTransferDetails(t *testing.T) {
	msg := httpFileSentMessage("win10.iso", http.MethodGet, http.StatusPartialContent, "bytes=0-1023", 1024, 5044211712, 150*time.Millisecond, "10.43.180.161")
	for _, want := range []string{
		"HTTP 文件已响应: win10.iso",
		"method=GET",
		"status=206",
		"range=bytes=0-1023",
		"sent=1024",
		"total=5044211712",
		"duration=150ms",
		"client=10.43.180.161",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected message to contain %q, got %q", want, msg)
		}
	}
}

func testStoreAndSettings(t *testing.T, ctx context.Context) (*storage.Store, storage.ServiceSettings) {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.Open(ctx, filepath.Join(dir, "pxe.db"), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	settings := store.DefaultSettings()
	settings.HTTPBoot.Root = filepath.Join(dir, "http")
	if err := os.MkdirAll(settings.HTTPBoot.Root, 0755); err != nil {
		t.Fatal(err)
	}
	return store, settings
}

func TestStaticScriptIsServed(t *testing.T) {
	store, cfg := testStoreAndSettings(t, context.Background())
	handler := Handler(cfg, observability.NewHub(store))
	script := "#!ipxe\nexit\n"
	if err := os.WriteFile(filepath.Join(cfg.HTTPBoot.Root, "boot.ipxe"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/boot.ipxe", nil))
	if rec.Code != 200 || rec.Body.String() != script {
		t.Fatal("boot script not served verbatim")
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/dynamic.ipxe", nil))
	if rec.Code != 404 {
		t.Fatal("retired generator still active")
	}
}
func TestDirectoryNamesEscaped(t *testing.T) {
	ctx := context.Background()
	store, cfg := testStoreAndSettings(t, ctx)
	name := "a&b.txt"
	if err := os.WriteFile(filepath.Join(cfg.HTTPBoot.Root, name), nil, 0644); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	fileHandler(cfg, observability.NewHub(store)).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Body.String(), "a&amp;b.txt") {
		t.Fatalf("unescaped listing: %s", rec.Body)
	}
}

func TestEqualSizeSameSecondEditChangesETag(t *testing.T) {
	store, cfg := testStoreAndSettings(t, context.Background())
	file := filepath.Join(cfg.HTTPBoot.Root, "script.ipxe")
	stamp := time.Unix(1700000000, 100000000)
	if err := os.WriteFile(file, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	handler := Handler(cfg, observability.NewHub(store))
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest("GET", "/script.ipxe", nil))
	if err := os.WriteFile(file, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	stamp = stamp.Add(100 * time.Millisecond)
	if err := os.Chtimes(file, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/script.ipxe", nil)
	req.Header.Set("If-None-Match", first.Header().Get("ETag"))
	next := httptest.NewRecorder()
	handler.ServeHTTP(next, req)
	if next.Code != 200 || next.Body.String() != "new" {
		t.Fatal("stale cached content", next.Code, next.Body.String())
	}
}
