package storage

import (
	"strings"
	"testing"
)

func TestValidateSettingsLimitsDHCPPoolSize(t *testing.T) {
	settings := ServiceSettings{
		NetbootXYZ: NetbootXYZSettings{BaseURL: "https://boot.netboot.xyz/ipxe"},
		Server:     ServerSettings{ListenIP: "0.0.0.0", AdvertiseIP: "192.168.1.10"},
		DHCP:       DHCPSettings{Enabled: true, Mode: "dhcp", NonPXEAction: "network_only", PoolStart: "10.0.0.0", PoolEnd: "10.0.255.255", SubnetMask: "255.255.0.0", Router: "10.0.0.1", DNS: []string{"10.0.0.1"}, LeaseTimeSeconds: 86400},
		TFTP:       TFTPSettings{Enabled: true, Root: "tftp", MaxTransfers: 64, BlockSizeMax: 1428, RetryCount: 5, TimeoutSeconds: 3},
		HTTPBoot:   HTTPBootSettings{Enabled: true, Addr: ":80", Root: "http"},
		SMB:        SMBSettings{Enabled: false, Root: "smb", ShareName: "pxe", Permissions: "read"},
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
