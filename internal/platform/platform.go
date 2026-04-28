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

type PermissionInfo struct {
	AdminLike bool   `json:"admin_like"`
	Status    string `json:"status"`
	Label     string `json:"label"`
	Detail    string `json:"detail"`
}

func Interfaces() []NetworkInterface {
	ifaces, _ := net.Interfaces()
	out := make([]NetworkInterface, 0, len(ifaces))
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		item := NetworkInterface{Name: iface.Name, Flags: iface.Flags.String(), IPs: []string{}}
		for _, addr := range addrs {
			item.IPs = append(item.IPs, addr.String())
		}
		out = append(out, item)
	}
	return out
}

func IsAdminLike() bool {
	return Permission().AdminLike
}

func Permission() PermissionInfo {
	if runtime.GOOS == "windows" {
		return PermissionInfo{
			AdminLike: true,
			Status:    "ok",
			Label:     "Windows 权限正常",
			Detail:    "Windows 通常不限制低端口绑定；若无法响应客户端，请检查防火墙、网卡绑定和同网段配置。",
		}
	}
	if os.Geteuid() == 0 {
		return PermissionInfo{
			AdminLike: true,
			Status:    "ok",
			Label:     "具备低端口权限",
			Detail:    "当前进程具备绑定 67/69/80 等低端口所需权限。",
		}
	}
	return PermissionInfo{
		AdminLike: false,
		Status:    "warning",
		Label:     "可能缺少低端口权限",
		Detail:    "Linux/macOS 绑定 1024 以下端口通常需要 root、CAP_NET_BIND_SERVICE 或端口转发规则。",
	}
}
