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

func TestHealthReportUsesSenderAndStaticScriptIsServed(t *testing.T) {
	ctx := context.Background()
	store, cfg := testStoreAndSettings(t, ctx)
	if err := store.UpsertClientSeen(ctx, "02:00:00:00:00:01", "192.168.1.50", "bios", "pxe"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertClientSeen(ctx, "02:00:00:00:00:02", "192.168.1.51", "bios", "pxe"); err != nil {
		t.Fatal(err)
	}
	handler := Handler(cfg, store, observability.NewHub())
	req := httptest.NewRequest("POST", "/client/report", strings.NewReader(`{"ip":"192.168.1.51","disk_health":"OK"}`))
	req.RemoteAddr = "192.168.1.50:23456"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("report: %d", rec.Code)
	}
	clients, err := store.ListClients(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range clients {
		if c.ObservedIP == "192.168.1.50" && c.DiskHealth != "OK" {
			t.Fatal("sender not updated")
		}
		if c.ObservedIP == "192.168.1.51" && c.DiskHealth == "OK" {
			t.Fatal("spoofed IP updated")
		}
	}
	script := "#!ipxe\nexit\n"
	if err = os.WriteFile(filepath.Join(cfg.HTTPBoot.Root, "boot.ipxe"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
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
	fileHandler(cfg, store, observability.NewHub()).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Body.String(), "a&amp;b.txt") {
		t.Fatalf("unescaped listing: %s", rec.Body)
	}
}
