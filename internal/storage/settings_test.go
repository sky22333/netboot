package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGetSettingsRestoresMissingSections(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := Open(ctx, filepath.Join(dir, "pxe.db"), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	truncated := `{
  "server": {"listen_ip": "0.0.0.0", "advertise_ip": "192.168.1.10"},
  "dhcp": {"enabled": true, "mode": "proxy", "non_pxe_action": "network_only"},
  "tftp": {"enabled": true, "root": "` + filepath.ToSlash(filepath.Join(dir, "boot", "tftp")) + `", "max_transfers": 64, "block_size_max": 1428, "retry_count": 5, "timeout_seconds": 3},
  "httpboot": {"enabled": true, "addr": ":80", "root": "` + filepath.ToSlash(filepath.Join(dir, "boot", "http")) + `"},
  "smb": {"enabled": false},
  "torrent": {"enabled": false, "addr": ":6969"}
}`
	if _, err := store.RawDB().ExecContext(ctx, `UPDATE settings SET value=? WHERE key='service'`, truncated); err != nil {
		t.Fatal(err)
	}

	settings, err := store.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Security.AdminAuthEnabled {
		t.Fatal("expected missing security section to restore admin auth default")
	}
	if settings.BootFiles.BIOS == "" || settings.BootFiles.UEFIX64 == "" || settings.BootFiles.UEFIARM64 == "" {
		t.Fatalf("expected missing boot files to be restored, got %+v", settings.BootFiles)
	}
	if settings.NetbootXYZ.BaseURL == "" || len(settings.NetbootXYZ.Files) == 0 {
		t.Fatalf("expected missing netboot.xyz settings to be restored, got %+v", settings.NetbootXYZ)
	}
}
