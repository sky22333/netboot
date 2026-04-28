//go:build !windows

package dhcp

import (
	"context"
	"net"
	"syscall"
)

func listenPacket(ctx context.Context, network, address string) (net.PacketConn, error) {
	lc := net.ListenConfig{Control: func(network, address string, c syscall.RawConn) error {
		var sockErr error
		if err := c.Control(func(fd uintptr) {
			if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
				sockErr = err
				return
			}
			sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		}); err != nil {
			return err
		}
		return sockErr
	}}
	return lc.ListenPacket(ctx, network, address)
}
