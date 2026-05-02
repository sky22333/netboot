package storage

import (
	"context"
	"path/filepath"
	"strings"
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

func TestValidateSettingsLimitsDHCPPoolSize(t *testing.T) {
	settings := ServiceSettings{
		Server:   ServerSettings{ListenIP: "0.0.0.0", AdvertiseIP: "192.168.1.10"},
		DHCP:     DHCPSettings{Enabled: true, Mode: "dhcp", NonPXEAction: "network_only", PoolStart: "10.0.0.0", PoolEnd: "10.0.255.255", SubnetMask: "255.255.0.0", Router: "10.0.0.1", DNS: []string{"10.0.0.1"}, LeaseTimeSeconds: 86400},
		TFTP:     TFTPSettings{Enabled: true, Root: "tftp", MaxTransfers: 64, BlockSizeMax: 1428, RetryCount: 5, TimeoutSeconds: 3},
		HTTPBoot: HTTPBootSettings{Enabled: true, Addr: ":80", Root: "http"},
		SMB:      SMBSettings{Enabled: false, Root: "smb", ShareName: "pxe", Permissions: "read"},
		Torrent:  TorrentSettings{Enabled: false, Addr: ":6969"},
	}
	if err := ValidateSettings(settings); err != nil {
		t.Fatalf("expected /16 pool to be valid, got %v", err)
	}

	settings.DHCP.PoolEnd = "10.1.0.0"
	err := ValidateSettings(settings)
	if err == nil || !strings.Contains(err.Error(), "地址池不能超过") {
		t.Fatalf("expected oversized pool error, got %v", err)
	}
}
