package booturl

import (
	"fmt"
	"net"
	"strings"
)

func HTTPBase(advertiseIP, listenAddr string) string {
	host := strings.TrimSpace(advertiseIP)
	if host == "" {
		host = "${next-server}"
	}
	port := Port(listenAddr, "80")
	if port == "" || port == "80" {
		return "http://" + host
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

func Port(addr, fallback string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return fallback
	}
	if strings.HasPrefix(addr, ":") {
		port := strings.TrimPrefix(addr, ":")
		if port != "" {
			return port
		}
		return fallback
	}
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return port
	}
	return fallback
}
