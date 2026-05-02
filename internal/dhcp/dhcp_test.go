package dhcp

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"pxe/internal/observability"
	"pxe/internal/storage"
)

func TestProxyDiscoverReturnsOffer(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	req := testPXEPacket(1,
		testOpt(60, []byte("PXEClient")),
		testOpt(93, []byte{0, 7}),
	)

	resp := buildResponse(ctx, settings, store, observability.NewHub(), req, true, nil)
	if got := responseMessageType(resp); got != 2 {
		t.Fatalf("expected proxy discover to return offer, got message type %d", got)
	}
}

func TestSelectedMenuItemUsesConfiguredServerIP(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	if err := store.SaveMenus(ctx, []storage.Menu{{
		MenuType:       "uefi",
		Enabled:        true,
		Prompt:         "UEFI",
		TimeoutSeconds: 6,
		Items: []storage.MenuItem{
			{SortOrder: 1, Title: "Custom TFTP", BootFile: "custom.efi", PXEType: "8002", ServerIP: "192.168.1.20", Enabled: true},
		},
	}}); err != nil {
		t.Fatal(err)
	}
	req := testPXEPacket(3,
		testOpt(60, []byte("PXEClient")),
		testOpt(93, []byte{0, 7}),
		testOpt(43, []byte{71, 4, 0x80, 0x02, 0, 0}),
	)

	resp := buildResponse(ctx, settings, store, observability.NewHub(), req, true, nil)
	if got := net.IP(resp[20:24]).String(); got != "192.168.1.20" {
		t.Fatalf("expected siaddr to use selected menu server, got %s", got)
	}
	if got := string(parseOptions(resp[240:])[66]); got != "192.168.1.20" {
		t.Fatalf("expected option 66 to use selected menu server, got %q", got)
	}
}

func TestIPXEClientSeenStatus(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	req := testPXEPacket(1,
		testOpt(60, []byte("PXEClient")),
		testOpt(77, []byte("iPXE")),
		testOpt(93, []byte{0, 7}),
	)

	resp := buildResponse(ctx, settings, store, observability.NewHub(), req, true, nil)
	if len(resp) == 0 {
		t.Fatal("expected response")
	}
	clients, err := store.ListClients(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 1 || clients[0].Status != "ipxe" {
		t.Fatalf("expected one ipxe client, got %+v", clients)
	}
}

func TestCompleteDHCPUEFIDirectBootWhenNativeMenuDisabled(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.DHCP.Mode = "dhcp"
	settings.DHCP.PoolStart = "192.168.1.200"
	settings.DHCP.PoolEnd = "192.168.1.200"
	settings.DHCP.Router = "192.168.1.1"
	settings.DHCP.DNS = []string{"192.168.1.1"}
	settings.BootFiles.UEFIX64 = "ipxe-x86_64.efi"
	if err := store.SaveMenus(ctx, []storage.Menu{{
		MenuType:       "uefi",
		Enabled:        false,
		Prompt:         "UEFI",
		TimeoutSeconds: 6,
		Items: []storage.MenuItem{
			{SortOrder: 1, Title: "iPXE UEFI x64", BootFile: "ipxe-x86_64.efi", PXEType: "8002", ServerIP: "%tftpserver%", Enabled: true},
		},
	}}); err != nil {
		t.Fatal(err)
	}
	req := testPXEPacket(1,
		testOpt(60, []byte("PXEClient")),
		testOpt(93, []byte{0, 7}),
	)

	resp := buildResponse(ctx, settings, store, observability.NewHub(), req, false, newLeasePool(settings))
	opts := parseOptions(resp[240:])
	if got := string(opts[67]); got != "ipxe-x86_64.efi\x00" {
		t.Fatalf("expected direct UEFI boot file, got %q", got)
	}
	if option43HasSuboption(opts[43], 9) || option43HasSuboption(opts[43], 10) {
		t.Fatalf("expected no native PXE menu option, got %x", opts[43])
	}
}

func TestCompleteDHCPLeaseOfferThenAckKeepsSameIP(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.DHCP.Mode = "dhcp"
	settings.DHCP.PoolStart = "192.168.1.200"
	settings.DHCP.PoolEnd = "192.168.1.201"
	settings.DHCP.Router = "192.168.1.1"
	settings.DHCP.DNS = []string{"192.168.1.1"}
	pool := newLeasePool(settings)

	discover := testPXEPacket(1, testOpt(60, []byte("PXEClient")), testOpt(93, []byte{0, 0}))
	offer := buildResponse(ctx, settings, store, observability.NewHub(), discover, false, pool)
	if got := responseMessageType(offer); got != 2 {
		t.Fatalf("expected offer, got message type %d", got)
	}
	offeredIP := net.IP(offer[16:20]).String()
	if offeredIP != "192.168.1.200" {
		t.Fatalf("unexpected offered IP %s", offeredIP)
	}

	request := testPXEPacket(3, testOpt(60, []byte("PXEClient")), testOpt(93, []byte{0, 0}), testOpt(50, net.ParseIP(offeredIP).To4()))
	ack := buildResponse(ctx, settings, store, observability.NewHub(), request, false, pool)
	if got := responseMessageType(ack); got != 5 {
		t.Fatalf("expected ack, got message type %d", got)
	}
	if got := net.IP(ack[16:20]).String(); got != offeredIP {
		t.Fatalf("expected ack to keep IP %s, got %s", offeredIP, got)
	}
}

func TestCompleteDHCPNonPXEClientGetsNetworkOnlyConfig(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.DHCP.Mode = "dhcp"
	settings.DHCP.PoolStart = "192.168.1.210"
	settings.DHCP.PoolEnd = "192.168.1.210"
	settings.DHCP.Router = "192.168.1.1"
	settings.DHCP.DNS = []string{"192.168.1.1"}

	req := testPXEPacket(1)
	resp := buildResponse(ctx, settings, store, observability.NewHub(), req, false, newLeasePool(settings))
	opts := parseOptions(resp[240:])
	if len(opts[67]) != 0 || len(opts[60]) != 0 {
		t.Fatalf("expected network-only response without PXE options, got option60=%q option67=%q", opts[60], opts[67])
	}
	if got := net.IP(resp[16:20]).String(); got != "192.168.1.210" {
		t.Fatalf("expected assigned IP, got %s", got)
	}
}

func TestCompleteDHCPNonPXEIgnoreReturnsNoResponse(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.DHCP.Mode = "dhcp"
	settings.DHCP.NonPXEAction = "ignore"
	settings.DHCP.PoolStart = "192.168.1.210"
	settings.DHCP.PoolEnd = "192.168.1.210"

	resp := buildResponse(ctx, settings, store, observability.NewHub(), testPXEPacket(1), false, newLeasePool(settings))
	if len(resp) != 0 {
		t.Fatalf("expected no response for ignored non-PXE client, got %d bytes", len(resp))
	}
}

func TestCompleteDHCPRequestForOtherServerIsIgnored(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.DHCP.Mode = "dhcp"

	req := testPXEPacket(3, testOpt(54, net.ParseIP("192.168.1.99").To4()), testOpt(50, net.ParseIP("192.168.1.20").To4()))
	resp := buildResponse(ctx, settings, store, observability.NewHub(), req, false, newLeasePool(settings))
	if len(resp) != 0 {
		t.Fatalf("expected request for another DHCP server to be ignored, got %d bytes", len(resp))
	}
}

func TestIPXEHTTPFeatureUsesDynamicMenuURL(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.HTTPBoot.Addr = ":8080"
	req := testPXEPacket(1,
		testOpt(60, []byte("PXEClient")),
		testOpt(77, []byte("iPXE")),
		testOpt(175, []byte{0x13, 0x01, 0x01, 0xff}),
	)

	resp := buildResponse(ctx, settings, store, observability.NewHub(), req, true, nil)
	if got := string(parseOptions(resp[240:])[67]); got != "http://192.168.1.10:8080/dynamic.ipxe?bootfile=ipxemenu\x00" {
		t.Fatalf("unexpected iPXE boot target %q", got)
	}
}

func TestExecutableBootFileUsesArchitectureSpecificNetbootFiles(t *testing.T) {
	ctx := context.Background()
	_, settings := testStoreAndSettings(t, ctx)
	settings.NetbootXYZ.DownloadDir = t.TempDir()
	mustWriteFile(t, filepath.Join(settings.NetbootXYZ.DownloadDir, "netboot.xyz.efi"))
	mustWriteFile(t, filepath.Join(settings.NetbootXYZ.DownloadDir, "netboot.xyz-arm64.efi"))
	mustWriteFile(t, filepath.Join(settings.NetbootXYZ.DownloadDir, "netboot.xyz.kpxe"))

	cases := map[string]string{
		"bios":       "netboot/netboot.xyz.kpxe",
		"uefi_x64":   "netboot/netboot.xyz.efi",
		"uefi_arm64": "netboot/netboot.xyz-arm64.efi",
		"uefi_ia32":  settings.BootFiles.UEFIIA32,
		"uefi_arm32": settings.BootFiles.UEFIARM32,
	}
	for arch, want := range cases {
		if got := executableBootFile(settings, arch); got != want {
			t.Fatalf("executableBootFile(%s) = %q, want %q", arch, got, want)
		}
	}
}

func TestARM64DoesNotReuseX64NetbootEFI(t *testing.T) {
	ctx := context.Background()
	_, settings := testStoreAndSettings(t, ctx)
	settings.NetbootXYZ.DownloadDir = t.TempDir()
	mustWriteFile(t, filepath.Join(settings.NetbootXYZ.DownloadDir, "netboot.xyz.efi"))

	if got := executableBootFile(settings, "uefi_arm64"); got != settings.BootFiles.UEFIARM64 {
		t.Fatalf("expected ARM64 to fall back to ARM64 boot file, got %q", got)
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
	settings.Server.AdvertiseIP = "192.168.1.10"
	settings.DHCP.Mode = "proxy"
	return store, settings
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("boot"), 0644); err != nil {
		t.Fatal(err)
	}
}

func option43HasSuboption(raw []byte, code byte) bool {
	for i := 0; i+1 < len(raw); {
		if raw[i] == 255 {
			return false
		}
		ln := int(raw[i+1])
		if i+2+ln > len(raw) {
			return false
		}
		if raw[i] == code {
			return true
		}
		i += 2 + ln
	}
	return false
}

type testOption struct {
	code byte
	val  []byte
}

func testOpt(code byte, val []byte) testOption {
	return testOption{code: code, val: val}
}

func testPXEPacket(msgType byte, extra ...testOption) []byte {
	pkt := make([]byte, 240)
	pkt[0], pkt[1], pkt[2] = 1, 1, 6
	copy(pkt[4:8], []byte{1, 2, 3, 4})
	copy(pkt[28:34], []byte{0, 17, 34, 51, 68, 85})
	copy(pkt[236:240], []byte(magicCookie))
	pkt = opt(pkt, 53, []byte{msgType})
	for _, item := range extra {
		pkt = opt(pkt, item.code, item.val)
	}
	return append(pkt, 255)
}
