package firmware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pxe/internal/observability"
	"pxe/internal/storage"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestLatestDownloadsAndPartialFailure(t *testing.T) {
	dir := t.TempDir()
	settings := storage.ServiceSettings{}
	settings.TFTP.Root = dir
	names := []string{"ipxe-x86_64.efi", "ipxe-arm64.efi", "undionly.kpxe"}
	source, err := Select(settings, "project", names)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "github.com" {
			t.Fatalf("unexpected host %s", r.URL)
		}
		resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("new firmware")), ContentLength: 12, Request: r}
		if strings.Contains(r.URL.Path, "/latest/download/") {
			resp.StatusCode = 302
			resp.Header.Set("Location", strings.Replace(r.URL.String(), "/latest/download/", "/download/v-next/", 1))
		} else if !strings.Contains(r.URL.Path, "/download/v-next/") {
			t.Fatalf("unexpected URL %s", r.URL)
		}
		if strings.HasSuffix(r.URL.Path, "undionly.kpxe") {
			resp.StatusCode = 404
		}
		return resp, nil
	})}
	results, err := Download(context.Background(), client, source, "", observability.NewHub(nil))
	if err != nil || len(results) != 3 || calls != 5 {
		t.Fatalf("results=%+v calls=%d err=%v", results, calls, err)
	}
	for i, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		want := "new firmware"
		if i == 2 {
			want = "old"
		}
		if string(data) != want || results[i].OK != (i != 2) {
			t.Fatalf("%s: %q %+v", name, data, results[i])
		}
	}
	catalog, err := Catalog(settings)
	if err != nil || len(catalog) != 2 || !catalog[0].Files[0].Exists || catalog[0].Files[0].Name != names[0] {
		t.Fatalf("catalog=%+v err=%v", catalog, err)
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, fmt.Errorf("connection lost") }

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }
func TestIncompleteDownloadPreservesOriginal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reader io.Reader
		length int64
		cancel bool
	}{
		{"empty", strings.NewReader(""), 0, false},
		{"truncated", strings.NewReader("short"), 20, false},
		{"connection", failedReader{}, -1, false},
		{"oversize", io.LimitReader(zeroReader{}, maxFirmwareBytes+1), -1, false},
		{"cancel", strings.NewReader("new"), 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := os.WriteFile(filepath.Join(dir, "boot.efi"), []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			if _, err := saveDownload(ctx, root, "boot.efi", tc.reader, tc.length); err == nil {
				t.Fatal("accepted invalid download")
			}
			data, _ := os.ReadFile(filepath.Join(dir, "boot.efi"))
			entries, _ := os.ReadDir(dir)
			if string(data) != "original" || len(entries) != 1 {
				t.Fatalf("data=%q entries=%v", data, entries)
			}
		})
	}
}
func TestSelectionRejectsUnlistedFiles(t *testing.T) {
	for _, tc := range []struct {
		id    string
		names []string
	}{
		{"other", []string{"undionly.kpxe"}}, {"project", nil},
		{"project", []string{"../escape"}}, {"project", []string{"undionly.kpxe", "undionly.kpxe"}},
	} {
		if _, err := Select(storage.ServiceSettings{}, tc.id, tc.names); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
}

func TestNetbootDownloadsToTFTPRoot(t *testing.T) {
	settings := storage.ServiceSettings{}
	settings.TFTP.Root = t.TempDir()
	settings.NetbootXYZ.Files = []string{"netboot.xyz.efi"}
	source, err := Select(settings, "netboot", settings.NetbootXYZ.Files)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://boot.netboot.xyz/ipxe/netboot.xyz.efi" {
			t.Fatalf("unexpected URL %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("firmware")), ContentLength: 8, Request: r}, nil
	})}
	results, err := Download(context.Background(), client, source, "https://boot.netboot.xyz/ipxe", observability.NewHub(nil))
	if err != nil || len(results) != 1 || !results[0].OK {
		t.Fatalf("%+v %v", results, err)
	}
	catalog, err := Catalog(settings)
	if err != nil {
		t.Fatal(err)
	}
	file := catalog[1].Files[0]
	if catalog[1].Directory != settings.TFTP.Root || file.Name != "netboot.xyz.efi" || !file.Exists {
		t.Fatalf("unexpected catalog %+v", catalog)
	}
}
