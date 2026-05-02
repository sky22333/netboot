package httpboot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pxe/internal/observability"
	"pxe/internal/storage"
)

func TestResolveReadPathRejectsTraversalAndMapsNetboot(t *testing.T) {
	settings := testSettings(t)
	if _, _, err := resolveReadPath(settings, "../secret.iso"); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}

	root, target, err := resolveReadPath(settings, "netboot/netboot.xyz.efi")
	if err != nil {
		t.Fatal(err)
	}
	if root != settings.NetbootXYZ.DownloadDir {
		t.Fatalf("expected netboot root %q, got %q", settings.NetbootXYZ.DownloadDir, root)
	}
	if target != filepath.Join(settings.NetbootXYZ.DownloadDir, "netboot.xyz.efi") {
		t.Fatalf("unexpected netboot target %q", target)
	}
}

func TestFileHandlerDirectoryListingDisabled(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.HTTPBoot.DirectoryListing = false

	req := httptest.NewRequest(http.MethodGet, "http://pxe.local/", nil)
	req.RemoteAddr = "192.168.1.50:12345"
	rec := httptest.NewRecorder()
	fileHandler(settings, store, observability.NewHub()).ServeHTTP(rec, req)
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
	fileHandler(settings, store, observability.NewHub()).ServeHTTP(rec, req)
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

func TestFileHandlerServesNetbootFile(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	if err := os.WriteFile(filepath.Join(settings.NetbootXYZ.DownloadDir, "netboot.xyz.efi"), []byte("efi"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://pxe.local/netboot/netboot.xyz.efi", nil)
	req.RemoteAddr = "192.168.1.50:12345"
	rec := httptest.NewRecorder()
	fileHandler(settings, store, observability.NewHub()).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "efi") {
		t.Fatalf("expected netboot file response, status=%d body=%q", rec.Code, rec.Body.String())
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
	settings.NetbootXYZ.DownloadDir = filepath.Join(dir, "netboot")
	if err := os.MkdirAll(settings.HTTPBoot.Root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(settings.NetbootXYZ.DownloadDir, 0755); err != nil {
		t.Fatal(err)
	}
	return store, settings
}

func testSettings(t *testing.T) storage.ServiceSettings {
	t.Helper()
	dir := t.TempDir()
	return storage.ServiceSettings{
		HTTPBoot:   storage.HTTPBootSettings{Root: filepath.Join(dir, "http")},
		NetbootXYZ: storage.NetbootXYZSettings{DownloadDir: filepath.Join(dir, "netboot")},
	}
}
