//go:build windows

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
			handle := syscall.Handle(fd)
			if err := syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
				sockErr = err
				return
			}
			sockErr = syscall.SetsockoptInt(handle, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		}); err != nil {
			return err
		}
		return sockErr
	}}
	return lc.ListenPacket(ctx, network, address)
}
