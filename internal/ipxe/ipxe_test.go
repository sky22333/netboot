package ipxe

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"pxe/internal/storage"
)

func TestConfigMenuChainsDynamicBootItem(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.HTTPBoot.Addr = ":8080"

	script := Generator{Settings: settings, Store: store}.Generate(ctx, Request{Params: url.Values{"bootfile": {"ipxemenu"}}})
	for _, want := range []string{
		"isset ${net0/ip} || dhcp || goto failed",
		"set bootserver http://192.168.1.10:8080",
		"chain http://192.168.1.10:8080/dynamic.ipxe?bootfile=boot.ipxe",
		"chain https://boot.netboot.xyz",
		"item show_info Show Boot Information",
		"chain http://192.168.1.10:8080/dynamic.ipxe?bootfile=show_info",
		"iseq ${platform} efi && exit || sanboot --no-describe --drive 0x80",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected generated menu to contain %q, got:\n%s", want, script)
		}
	}
}

func TestGenerateRejectsInvalidBootPath(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)

	script := Generator{Settings: settings, Store: store}.Generate(ctx, Request{Params: url.Values{"bootfile": {"../secret.ipxe"}}})
	if !strings.Contains(script, "Invalid boot path") || !strings.Contains(script, "dynamic.ipxe?bootfile=ipxemenu") {
		t.Fatalf("expected invalid path fallback, got:\n%s", script)
	}
}

func TestGenerateDirectRelativeIPXEScript(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.HTTPBoot.Addr = ":8081"

	script := Generator{Settings: settings, Store: store}.Generate(ctx, Request{Params: url.Values{"bootfile": {"scripts/install ipxe.ipxe"}}})
	if !strings.Contains(script, "chain http://192.168.1.10:8081/scripts/install%20ipxe.ipxe") {
		t.Fatalf("expected escaped relative chain target, got:\n%s", script)
	}
}

func TestDisabledIPXEMenuFallsBackToLocalDisk(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	if err := store.SaveMenus(ctx, []storage.Menu{{MenuType: "ipxe", Enabled: false, Prompt: "iPXE", TimeoutSeconds: 6}}); err != nil {
		t.Fatal(err)
	}

	script := Generator{Settings: settings, Store: store}.Generate(ctx, Request{Params: url.Values{"bootfile": {"ipxemenu"}}})
	if !strings.Contains(script, "iPXE menu disabled") || !strings.Contains(script, "iseq ${platform} efi && exit || sanboot --no-describe --drive 0x80") {
		t.Fatalf("expected disabled menu local boot fallback, got:\n%s", script)
	}
}

func TestGenerateShowInfoScriptReturnsToMenu(t *testing.T) {
	ctx := context.Background()
	store, settings := testStoreAndSettings(t, ctx)
	settings.HTTPBoot.Addr = ":8080"

	script := Generator{Settings: settings, Store: store}.Generate(ctx, Request{Params: url.Values{"bootfile": {"show_info"}}})
	for _, want := range []string{
		"echo iPXE boot information",
		"echo proxydhcp next-server: ${proxydhcp/next-server}",
		"echo bootserver: http://192.168.1.10:8080",
		"chain http://192.168.1.10:8080/dynamic.ipxe?bootfile=ipxemenu",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("expected show_info script to contain %q, got:\n%s", want, script)
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
	settings.Server.AdvertiseIP = "192.168.1.10"
	return store, settings
}
