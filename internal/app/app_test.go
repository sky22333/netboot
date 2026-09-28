package app

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"pxe/internal/config"
	"pxe/internal/observability"
	"pxe/internal/storage"
	"sync"
	"testing"
	"time"
)

func testApp(t *testing.T) (*App, storage.ServiceSettings) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	store, err := storage.Open(ctx, filepath.Join(dir, "db"), dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{Store: store, Events: observability.NewHub(), Boot: config.Default(), services: map[string]*serviceHandle{}}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		a.StopServices(ctx)
		store.Close()
	})
	cfg := store.DefaultSettings()
	cfg.DHCP.Enabled = false
	cfg.TFTP.Enabled = false
	cfg.HTTPBoot.Enabled = false
	cfg.SMB.Enabled = false
	if err = store.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	return a, cfg
}
func TestStartReportsBindFailure(t *testing.T) {
	a, cfg := testApp(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cfg.HTTPBoot.Enabled = true
	cfg.HTTPBoot.Addr = ln.Addr().String()
	if err = a.Store.SaveSettings(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if err = a.StartServices(context.Background()); err == nil {
		t.Fatal("occupied port reported success")
	}
	if len(a.services) != 0 {
		t.Fatal("failed start leaked handles")
	}
}
func TestStopUsesActiveSMBConfiguration(t *testing.T) {
	a, cfg := testApp(t)
	cfg.SMB.Enabled = true
	cfg.SMB.ShareName = "old"
	ctx := context.Background()
	if err := a.Store.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	var stopped string
	a.applySMB = func(_ context.Context, s storage.SMBSettings, start bool) error {
		if !start {
			stopped = s.ShareName
		}
		return nil
	}
	if err := a.StartServices(ctx); err != nil {
		t.Fatal(err)
	}
	cfg.SMB.Enabled = false
	cfg.SMB.ShareName = "new"
	if err := a.Store.SaveSettings(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if err := a.StopServices(ctx); err != nil {
		t.Fatal(err)
	}
	if stopped != "old" {
		t.Fatalf("stopped %q", stopped)
	}
}
func TestConcurrentLifecycleAndRollback(t *testing.T) {
	a, cfg := testApp(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.HTTPBoot.Addr = ln.Addr().String()
	ln.Close()
	cfg.HTTPBoot.Enabled = true
	if err = a.Store.SaveSettings(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.StartServices(context.Background()); err != nil {
				t.Error(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := a.StopServices(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	conn, err := net.Listen("tcp", cfg.HTTPBoot.Addr)
	if err != nil {
		t.Fatalf("listener leaked: %v", err)
	}
	conn.Close()
}

func TestFailedSMBStartupRollsBackHTTPListener(t *testing.T) {
	a, cfg := testApp(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.HTTPBoot.Addr = ln.Addr().String()
	ln.Close()
	cfg.HTTPBoot.Enabled = true
	cfg.SMB.Enabled = true
	if err = a.Store.SaveSettings(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	a.applySMB = func(context.Context, storage.SMBSettings, bool) error { return fmt.Errorf("simulated SMB failure") }
	if err = a.StartServices(context.Background()); err == nil {
		t.Fatal("expected SMB error")
	}
	if len(a.services) != 0 || a.activeSMB != nil {
		t.Fatal("partial start retained state")
	}
	ln, err = net.Listen("tcp", cfg.HTTPBoot.Addr)
	if err != nil {
		t.Fatalf("HTTP listener not rolled back: %v", err)
	}
	ln.Close()
}
