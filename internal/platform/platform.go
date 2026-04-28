package platform

import (
	"net"
	"os"
	"runtime"
)

type NetworkInterface struct {
	Name  string   `json:"name"`
	Flags string   `json:"flags"`
	IPs   []string `json:"ips"`
}

func Interfaces() []NetworkInterface {
	ifaces, _ := net.Interfaces()
	out := make([]NetworkInterface, 0, len(ifaces))
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		item := NetworkInterface{Name: iface.Name, Flags: iface.Flags.String()}
		for _, addr := range addrs {
			item.IPs = append(item.IPs, addr.String())
		}
		out = append(out, item)
	}
	return out
}

func IsAdminLike() bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return os.Geteuid() == 0
}
