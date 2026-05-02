package tftp

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"pxe/internal/storage"
)

func TestResolveReadPathKeepsRequestsInsideRoot(t *testing.T) {
	settings := testSettings(t)
	if _, err := resolveReadPath(settings, "../secret.efi"); err == nil {
		t.Fatal("expected path traversal to be rejected")
	}

	got, err := resolveReadPath(settings, "boot/loader.efi")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(settings.TFTP.Root, "boot", "loader.efi")
	if got != want {
		t.Fatalf("resolveReadPath() = %q, want %q", got, want)
	}
}

func TestResolveReadPathMapsNetbootPrefix(t *testing.T) {
	settings := testSettings(t)
	got, err := resolveReadPath(settings, "netboot/netboot.xyz.efi")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(settings.NetbootXYZ.DownloadDir, "netboot.xyz.efi")
	if got != want {
		t.Fatalf("resolveReadPath() = %q, want %q", got, want)
	}
}

func TestVirtualIPXEScriptOnlyForKnownNames(t *testing.T) {
	settings := testSettings(t)
	script, ok := virtualIPXEScript(settings, "boot.ipxe")
	if !ok || !strings.Contains(script, "chain http://192.168.1.10:8080/dynamic.ipxe?bootfile=ipxemenu") {
		t.Fatalf("expected virtual iPXE script, ok=%v script=%q", ok, script)
	}
	if _, ok := virtualIPXEScript(settings, "other.ipxe"); ok {
		t.Fatal("expected arbitrary iPXE filename to use filesystem, not virtual script")
	}
}

func TestBuildOACKPayload(t *testing.T) {
	got := buildOACKPayload(map[string]string{"blksize": "900", "tsize": "0"}, 900, 12345)
	if !bytes.Equal(got, []byte("blksize\x00900\x00tsize\x0012345\x00")) {
		t.Fatalf("unexpected OACK payload %q", got)
	}
}

func testSettings(t *testing.T) storage.ServiceSettings {
	t.Helper()
	dir := t.TempDir()
	return storage.ServiceSettings{
		Server:     storage.ServerSettings{AdvertiseIP: "192.168.1.10"},
		TFTP:       storage.TFTPSettings{Root: filepath.Join(dir, "tftp"), BlockSizeMax: 1428},
		HTTPBoot:   storage.HTTPBootSettings{Addr: ":8080"},
		NetbootXYZ: storage.NetbootXYZSettings{DownloadDir: filepath.Join(dir, "netboot")},
	}
}
