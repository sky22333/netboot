package tftp

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"pxe/internal/observability"
	"testing"
	"time"

	"pxe/internal/storage"
)

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
		Server:   storage.ServerSettings{AdvertiseIP: "192.168.1.10"},
		TFTP:     storage.TFTPSettings{Root: filepath.Join(dir, "tftp"), BlockSizeMax: 1428},
		HTTPBoot: storage.HTTPBootSettings{Addr: ":8080"},
	}
}

func TestStandardTransferEndsWithEmptyBlock(t *testing.T) {
	client, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	cfg := testSettings(t)
	cfg.TFTP.TimeoutSeconds = 1
	cfg.TFTP.RetryCount = 2
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		sendContent(ctx, cfg, observability.NewHub(), "kernel", client.LocalAddr(), nil, bytes.NewReader(make([]byte, 512)), 512)
	}()
	for i, want := range []int{516, 4} {
		buf := make([]byte, 2048)
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, remote, err := client.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}
		if n != want || binary.BigEndian.Uint16(buf[:2]) != opDATA || binary.BigEndian.Uint16(buf[2:4]) != uint16(i+1) {
			t.Fatalf("packet %d: %x (%d)", i, buf[:n], n)
		}
		ack := []byte{0, opACK, 0, byte(i + 1)}
		client.WriteTo(ack, remote)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("transfer did not finish")
	}
}
func TestTransferCancellationInterruptsAckWait(t *testing.T) {
	client, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	cfg := testSettings(t)
	cfg.TFTP.TimeoutSeconds = 60
	cfg.TFTP.RetryCount = 20
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		sendContent(ctx, cfg, observability.NewHub(), "kernel", client.LocalAddr(), nil, bytes.NewReader([]byte("x")), 1)
	}()
	buf := make([]byte, 1024)
	client.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err = client.ReadFrom(buf); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation waited for transfer timeout")
	}
}

func TestFirmwareServedFromTFTPRoot(t *testing.T) {
	for _, name := range []string{"ipxe-x86_64.efi", "ipxe-arm64.efi", "undionly.kpxe", "netboot.xyz.efi"} {
		t.Run(name, func(t *testing.T) {
			cfg := testSettings(t)
			cfg.TFTP.TimeoutSeconds = 1
			if err := os.MkdirAll(cfg.TFTP.Root, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cfg.TFTP.Root, name), []byte("firmware"), 0644); err != nil {
				t.Fatal(err)
			}
			client, err := net.ListenPacket("udp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			done := make(chan struct{})
			go func() { defer close(done); sendFile(ctx, cfg, observability.NewHub(), name, client.LocalAddr(), nil) }()
			client.SetReadDeadline(time.Now().Add(2 * time.Second))
			buf := make([]byte, 1024)
			n, remote, err := client.ReadFrom(buf)
			if err != nil {
				t.Fatal(err)
			}
			if n != 12 || binary.BigEndian.Uint16(buf[:2]) != opDATA || string(buf[4:n]) != "firmware" {
				t.Fatalf("unexpected packet %x", buf[:n])
			}
			if _, err := client.WriteTo([]byte{0, opACK, 0, 1}, remote); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("transfer did not finish")
			}
		})
	}
}
