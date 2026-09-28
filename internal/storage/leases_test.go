package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func leaseStore(t *testing.T) (*Store, ServiceSettings) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "pxe.db"), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	cfg := s.DefaultSettings()
	cfg.Server.AdvertiseIP = "192.168.1.10"
	cfg.DHCP.Mode = "dhcp"
	cfg.DHCP.Router = "192.168.1.1"
	cfg.DHCP.SubnetMask = "255.255.255.0"
	cfg.DHCP.PoolStart = "192.168.1.200"
	cfg.DHCP.PoolEnd = "192.168.1.201"
	return s, cfg
}
func TestObservationsNeverOverwriteReservations(t *testing.T) {
	s, _ := leaseStore(t)
	ctx := context.Background()
	mac := "02:00:00:00:00:01"
	c, err := s.UpsertClient(ctx, Client{Name: "static", IP: "192.168.1.210", MAC: mac})
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"192.168.1.210", "0.0.0.0"} {
		if err = s.UpsertClientSeen(ctx, mac, ip, "uefi_x64", "pxe"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetClient(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.IP != c.IP || got.ObservedIP != c.IP {
		t.Fatalf("reservation or observation changed: %+v", got)
	}
	for _, m := range []string{"02:00:00:00:00:02", "02:00:00:00:00:03"} {
		if err = s.UpsertClientSeen(ctx, m, "0.0.0.0", "bios", "pxe"); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.ListClients(ctx)
	if err != nil || len(rows) != 3 {
		t.Fatalf("unknown-IP clients lost: %v %v", rows, err)
	}
}
func TestExpiredObservationCannotReuseAnotherLease(t *testing.T) {
	s, cfg := leaseStore(t)
	ctx := context.Background()
	a := "02:00:00:00:00:01"
	b := "02:00:00:00:00:02"
	ip, err := s.LeaseAddress(ctx, cfg, a, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.UpsertClientSeen(ctx, a, ip, "bios", "pxe"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE leases SET expires=0`); err != nil {
		t.Fatal(err)
	}
	ipB, err := s.LeaseAddress(ctx, cfg, b, ip, true)
	if err != nil || ipB != ip {
		t.Fatalf("B: %s %v", ipB, err)
	}
	if got, err := s.LeaseAddress(ctx, cfg, a, ip, true); err != nil || got != "" {
		t.Fatalf("stale ACK: %s %v", got, err)
	}
	got, err := s.LeaseAddress(ctx, cfg, a, "", false)
	if err != nil || got == ip || got == "" {
		t.Fatalf("new offer: %s %v", got, err)
	}
}
func TestLeasesPersistAndStaticEditsRespectOccupancy(t *testing.T) {
	s, cfg := leaseStore(t)
	ctx := context.Background()
	mac := "02:00:00:00:00:01"
	ip, err := s.LeaseAddress(ctx, cfg, mac, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.LeaseAddress(ctx, cfg, mac, ip, true); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpsertClient(ctx, Client{Name: "conflict", IP: ip, MAC: "02:00:00:00:00:02"}); err == nil {
		t.Fatal("reservation stole live lease")
	}
	s2, err := Open(ctx, filepath.Join(s.dataDir, "pxe.db"), s.dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if got, err := s2.LeaseAddress(ctx, cfg, mac, ip, true); err != nil || got != ip {
		t.Fatalf("renew after reopen: %s %v", got, err)
	}
	if err = s2.ReleaseLease(ctx, mac, ip, false); err != nil {
		t.Fatal(err)
	}
	if got, err := s2.LeaseAddress(ctx, cfg, "02:00:00:00:00:02", ip, true); err != nil || got != ip {
		t.Fatalf("reuse after release: %s %v", got, err)
	}
}
func TestConcurrentOffersUnique(t *testing.T) {
	s, cfg := leaseStore(t)
	cfg.DHCP.PoolEnd = "192.168.1.220"
	ctx := context.Background()
	var wg sync.WaitGroup
	ips := make(chan string, 20)
	for i := 1; i <= 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ip, err := s.LeaseAddress(ctx, cfg, fmt.Sprintf("02:00:00:00:00:%02x", i), "", false)
			if err != nil {
				t.Error(err)
			}
			ips <- ip
		}(i)
	}
	wg.Wait()
	close(ips)
	seen := map[string]bool{}
	for ip := range ips {
		if ip == "" || seen[ip] {
			t.Fatalf("duplicate or empty: %s", ip)
		}
		seen[ip] = true
	}
}
func TestStaticAndInfrastructureAddressesExcluded(t *testing.T) {
	s, cfg := leaseStore(t)
	ctx := context.Background()
	cfg.DHCP.PoolStart = "192.168.1.0"
	cfg.DHCP.PoolEnd = "192.168.1.10"
	if _, err := s.UpsertClient(ctx, Client{Name: "reserved", IP: "192.168.1.2"}); err != nil {
		t.Fatal(err)
	}
	ip, err := s.LeaseAddress(ctx, cfg, "02:00:00:00:00:01", "", false)
	if err != nil || ip != "192.168.1.3" {
		t.Fatalf("unexpected allocation %s %v", ip, err)
	}
	if err = s.ReleaseLease(ctx, "02:00:00:00:00:01", ip, true); err != nil {
		t.Fatal(err)
	}
	ip2, err := s.LeaseAddress(ctx, cfg, "02:00:00:00:00:02", ip, true)
	if err != nil || ip2 != "" {
		t.Fatalf("declined address reused %s %v", ip2, err)
	}
}
