package netutil

import "net"

func DirectedBroadcast(ipText, maskText string) string {
	ip := net.ParseIP(ipText).To4()
	mask := net.ParseIP(maskText).To4()
	if ip == nil || mask == nil {
		return ""
	}
	out := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		out[i] = ip[i] | ^mask[i]
	}
	return out.String()
}

func InterfaceBroadcasts() []string {
	seen := map[string]bool{}
	out := []string{}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, ipNet, err := net.ParseCIDR(addr.String())
			if err != nil || ip.To4() == nil || ipNet == nil {
				continue
			}
			broadcast := broadcastFromMask(ip.To4(), ipNet.Mask)
			if broadcast == "" || seen[broadcast] {
				continue
			}
			seen[broadcast] = true
			out = append(out, broadcast)
		}
	}
	return out
}

func broadcastFromMask(ip net.IP, mask net.IPMask) string {
	if len(mask) != net.IPv4len {
		return ""
	}
	out := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		out[i] = ip[i] | ^mask[i]
	}
	return out.String()
}
