package booturl

import (
	"fmt"
	"net"
	"strings"

	"pxe/internal/storage"
)

func HTTPBootBase(settings storage.ServiceSettings) string {
	return HTTPBase(settings.Server.AdvertiseIP, settings.HTTPBoot.Addr)
}

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

func HTTPBaseWithListenHost(advertiseIP, listenAddr string) string {
	listenAddr = strings.TrimSpace(listenAddr)
	if host, port, err := net.SplitHostPort(listenAddr); err == nil {
		if host != "" && host != "0.0.0.0" && host != "::" {
			return "http://" + net.JoinHostPort(host, port)
		}
	}
	return HTTPBase(advertiseIP, listenAddr)
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
