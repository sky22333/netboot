package dhcp

import (
	"context"
	"net"
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
