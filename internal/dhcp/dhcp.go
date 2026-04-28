package dhcp

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"pxe/internal/observability"
	"pxe/internal/pxeopt"
	"pxe/internal/storage"
)

const magicCookie = "\x63\x82\x53\x63"

func RunProxy(ctx context.Context, settings storage.ServiceSettings, store *storage.Store, events *observability.Hub) {
	run(ctx, settings, store, events, "4011", true, nil)
}

func RunProxyDiscover(ctx context.Context, settings storage.ServiceSettings, store *storage.Store, events *observability.Hub) {
	run(ctx, settings, store, events, "67", true, nil)
}

func RunDHCP(ctx context.Context, settings storage.ServiceSettings, store *storage.Store, events *observability.Hub) {
	run(ctx, settings, store, events, "67", false, newLeasePool(settings))
}

type leasePool struct {
	mu        sync.Mutex
	available []net.IP
	offered   map[string]lease
	leased    map[string]lease
	used      map[string]string
	ttl       time.Duration
}

type lease struct {
	IP      string
	Expires time.Time
}

func newLeasePool(settings storage.ServiceSettings) *leasePool {
	p := &leasePool{offered: map[string]lease{}, leased: map[string]lease{}, used: map[string]string{}, ttl: time.Duration(settings.DHCP.LeaseTimeSeconds) * time.Second}
	start := net.ParseIP(settings.DHCP.PoolStart).To4()
	end := net.ParseIP(settings.DHCP.PoolEnd).To4()
	if start == nil || end == nil {
		return p
	}
	s := binary.BigEndian.Uint32(start)
	e := binary.BigEndian.Uint32(end)
	for i := s; i <= e; i++ {
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, i)
		p.available = append(p.available, net.IP(buf))
		if i == ^uint32(0) {
			break
		}
	}
	if p.ttl <= 0 {
		p.ttl = 24 * time.Hour
	}
	return p
}

func (p *leasePool) Assign(mac, requested string) string {
	if p == nil {
		return "0.0.0.0"
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	p.cleanup(now)
	if l, ok := p.leased[mac]; ok && l.Expires.After(now) {
		return l.IP
	}
	if l, ok := p.offered[mac]; ok && l.Expires.After(now) {
		return l.IP
	}
	if requested != "" && requested != "0.0.0.0" && p.inPool(requested) && p.used[requested] == "" {
		p.offered[mac] = lease{IP: requested, Expires: now.Add(60 * time.Second)}
		p.used[requested] = mac
		return requested
	}
	for len(p.available) > 0 {
		ip := p.available[0].String()
		p.available = p.available[1:]
		if p.used[ip] == "" {
			p.offered[mac] = lease{IP: ip, Expires: now.Add(60 * time.Second)}
			p.used[ip] = mac
			return ip
		}
	}
	return ""
}

func (p *leasePool) Confirm(mac, ip string) string {
	if p == nil {
		return ip
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cleanup(time.Now())
	if ip == "" || ip == "0.0.0.0" {
		if l, ok := p.offered[mac]; ok {
			ip = l.IP
		}
	}
	if ip == "" || !p.inPool(ip) {
		return ""
	}
	if owner := p.used[ip]; owner != "" && owner != mac {
		return ""
	}
	l := lease{IP: ip, Expires: time.Now().Add(p.ttl)}
	p.leased[mac] = l
	delete(p.offered, mac)
	p.used[ip] = mac
	return ip
}

func (p *leasePool) Release(mac string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if l, ok := p.leased[mac]; ok {
		delete(p.used, l.IP)
		p.available = append(p.available, net.ParseIP(l.IP).To4())
	}
	if l, ok := p.offered[mac]; ok {
		delete(p.used, l.IP)
		p.available = append(p.available, net.ParseIP(l.IP).To4())
	}
	delete(p.leased, mac)
	delete(p.offered, mac)
}

func (p *leasePool) cleanup(now time.Time) {
	for mac, l := range p.offered {
		if !l.Expires.After(now) {
			delete(p.offered, mac)
			delete(p.used, l.IP)
			p.available = append(p.available, net.ParseIP(l.IP).To4())
		}
	}
	for mac, l := range p.leased {
		if !l.Expires.After(now) {
			delete(p.leased, mac)
			delete(p.used, l.IP)
			p.available = append(p.available, net.ParseIP(l.IP).To4())
		}
	}
}

func (p *leasePool) inPool(ip string) bool {
	if net.ParseIP(ip).To4() == nil {
		return false
	}
	for _, candidate := range p.available {
		if candidate.String() == ip {
			return true
		}
	}
	for _, l := range p.offered {
		if l.IP == ip {
			return true
		}
	}
	for _, l := range p.leased {
		if l.IP == ip {
			return true
		}
	}
	return false
}

func run(ctx context.Context, settings storage.ServiceSettings, store *storage.Store, events *observability.Hub, port string, proxy bool, pool *leasePool) {
	addr := net.JoinHostPort(settings.Server.ListenIP, port)
	conn, err := listenPacket(ctx, "udp4", addr)
	if err != nil {
		events.Publish("error", "dhcp", "监听失败 "+addr+": "+err.Error())
		return
	}
	defer conn.Close()
	name := "DHCP"
	if proxy {
		name = "ProxyDHCP"
	}
	events.Publish("info", "dhcp", name+" 已启动: "+addr)
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	buf := make([]byte, 1500)
	for {
		n, remote, err := conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				events.Publish("info", "dhcp", name+" 已停止")
				return
			default:
				slog.Warn("dhcp read error", "error", err)
				continue
			}
		}
		req := append([]byte(nil), buf[:n]...)
		resp := buildResponse(ctx, settings, store, events, req, proxy, pool)
		if len(resp) == 0 {
			continue
		}
		if proxy && requestMessageType(req) == 1 {
			offer := cloneWithMessageType(resp, 2)
			sendResponse(conn, remote, req, offer, settings, name, port, proxy, events)
		}
		sendResponse(conn, remote, req, resp, settings, name, port, proxy, events)
	}
}

func sendResponse(conn net.PacketConn, remote net.Addr, req, resp []byte, settings storage.ServiceSettings, name, port string, proxy bool, events *observability.Hub) {
	targets := make([]net.Addr, 0, 4)
	if !proxy || port == "67" {
		targets = append(targets, responseBroadcastTargets(settings)...)
	}
	if candidate := clientResponseAddr(req); candidate != nil {
		targets = append(targets, candidate)
	}
	if proxy && validResponseAddr(remote) {
		targets = append(targets, remote)
	}
	if len(targets) == 0 && validResponseAddr(remote) {
		targets = append(targets, remote)
	}
	seen := map[string]bool{}
	for _, target := range targets {
		if target == nil {
			continue
		}
		key := target.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		n, err := conn.WriteTo(resp, target)
		if err != nil {
			events.Publish("error", "dhcp", fmt.Sprintf("%s 响应发送失败: msg=%d target=%s size=%d error=%s", name, responseMessageType(resp), key, len(resp), err.Error()))
			continue
		}
		events.Publish("info", "dhcp", fmt.Sprintf("%s 响应已发送: msg=%d target=%s bytes=%d", name, responseMessageType(resp), key, n))
	}
}

func responseBroadcastTargets(settings storage.ServiceSettings) []net.Addr {
	out := make([]net.Addr, 0, 2)
	if addr, err := net.ResolveUDPAddr("udp4", "255.255.255.255:68"); err == nil {
		out = append(out, addr)
	}
	if directed := directedBroadcast(settings.Server.AdvertiseIP, settings.DHCP.SubnetMask); directed != "" && directed != "255.255.255.255" {
		if addr, err := net.ResolveUDPAddr("udp4", directed+":68"); err == nil {
			out = append(out, addr)
		}
	}
	return out
}

func directedBroadcast(ipText, maskText string) string {
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

func clientResponseAddr(req []byte) net.Addr {
	if len(req) < 240 {
		return nil
	}
	ip := net.IP(req[12:16]).To4()
	if ip == nil || ip.Equal(net.IPv4zero) {
		if requested := parseOptions(req[240:])[50]; len(requested) == 4 {
			ip = net.IP(requested).To4()
		}
	}
	if ip == nil || ip.Equal(net.IPv4zero) {
		return nil
	}
	return &net.UDPAddr{IP: ip, Port: 68}
}

func validResponseAddr(addr net.Addr) bool {
	udp, ok := addr.(*net.UDPAddr)
	if !ok {
		return addr != nil
	}
	return udp.IP != nil && !udp.IP.Equal(net.IPv4zero)
}

func requestMessageType(req []byte) byte {
	if len(req) < 240 {
		return 0
	}
	if v := parseOptions(req[240:])[53]; len(v) > 0 {
		return v[0]
	}
	return 0
}

func responseMessageType(resp []byte) byte {
	if len(resp) < 240 {
		return 0
	}
	if v := parseOptions(resp[240:]); len(v[53]) > 0 {
		return v[53][0]
	}
	return 0
}

func cloneWithMessageType(resp []byte, msgType byte) []byte {
	out := append([]byte(nil), resp...)
	if len(out) < 240 || string(out[236:240]) != magicCookie {
		return out
	}
	for i := 240; i < len(out); {
		code := out[i]
		i++
		if code == 0 {
			continue
		}
		if code == 255 || i >= len(out) {
			return out
		}
		ln := int(out[i])
		i++
		if i+ln > len(out) {
			return out
		}
		if code == 53 && ln == 1 {
			out[i] = msgType
			return out
		}
		i += ln
	}
	return out
}

func buildResponse(ctx context.Context, settings storage.ServiceSettings, store *storage.Store, events *observability.Hub, req []byte, proxy bool, pool *leasePool) []byte {
	if len(req) < 240 || string(req[236:240]) != magicCookie {
		return nil
	}
	opts := parseOptions(req[240:])
	msgType := byte(0)
	if v := opts[53]; len(v) > 0 {
		msgType = v[0]
	}
	if msgType == 4 || msgType == 7 {
		if pool != nil {
			pool.Release(macFromPacket(req))
		}
		return nil
	}
	if msgType != 1 && msgType != 3 {
		return nil
	}
	if !proxy && msgType == 3 && len(opts[54]) == 4 && net.IP(opts[54]).String() != settings.Server.AdvertiseIP {
		return nil
	}
	mac := macFromPacket(req)
	arch := archName(opts[93])
	vendorClass := string(opts[60])
	userClass := string(opts[77])
	isIPXE := contains(opts[77], "iPXE") || contains(opts[60], "iPXE") || len(opts[175]) > 0
	isPXE := isPXEClient(opts, vendorClass, isIPXE)
	clientIP := net.IP(req[12:16]).String()
	if !isPXE && settings.DHCP.NonPXEAction == "ignore" {
		events.Publish("info", "dhcp", fmt.Sprintf("忽略普通 DHCP 客户端 %s: vendor=%q", mac, vendorClass))
		return nil
	}
	if staticIP, ok := store.GetIPForMAC(ctx, mac); ok && !proxy && settings.DHCP.Mode == "dhcp" {
		clientIP = staticIP
	} else if !proxy && settings.DHCP.Mode == "dhcp" {
		requested := ""
		if v := opts[50]; len(v) == 4 {
			requested = net.IP(v).String()
		} else if ciaddr := net.IP(req[12:16]).To4(); ciaddr != nil && ciaddr.String() != "0.0.0.0" {
			requested = ciaddr.String()
		}
		if msgType == 1 {
			clientIP = pool.Assign(mac, requested)
		} else {
			clientIP = pool.Confirm(mac, requested)
		}
		if clientIP == "" {
			events.Publish("warning", "dhcp", fmt.Sprintf("地址池耗尽或请求地址不可用: %s", mac))
			if msgType == 3 {
				return nak(req, settings, "地址池耗尽或请求地址不可用")
			}
			return nil
		}
	}
	if !isPXE {
		if proxy || settings.DHCP.Mode != "dhcp" {
			return nil
		}
		store.UpsertClientSeen(ctx, mac, clientIP, "dhcp", "online")
		_ = store.AddEvent(ctx, "info", "dhcp", "普通 DHCP 客户端获取网络参数", map[string]any{"mac": mac, "ip": clientIP, "vendor": vendorClass, "msg_type": msgType})
		events.Publish("info", "dhcp", fmt.Sprintf("向普通 DHCP 客户端 %s 分配网络参数: ip=%s vendor=%q", mac, clientIP, vendorClass))
		return offerNetworkConfig(req, settings, clientIP)
	}
	store.UpsertClientSeen(ctx, mac, clientIP, arch, "pxe")
	_ = store.AddEvent(ctx, "info", "dhcp", "收到客户端请求", map[string]any{"mac": mac, "arch": arch, "ipxe": isIPXE, "vendor": vendorClass, "user_class": userClass, "msg_type": msgType, "proxy": proxy})
	events.Publish("info", "dhcp", fmt.Sprintf("客户端 %s 请求启动信息: msg=%d arch=%s vendor=%q user=%q ipxe=%v proxy=%v", mac, msgType, arch, vendorClass, userClass, isIPXE, proxy))

	menus, _ := store.ListMenus(ctx)
	menu := findMenu(menus, "bios")
	if isUEFIArch(arch) {
		menu = findMenu(menus, "uefi")
	}
	if isIPXE {
		boot := ipxeBootFile(settings, arch, opts)
		events.Publish("info", "dhcp", fmt.Sprintf("向 %s 响应 iPXE 启动目标: %s", mac, boot))
		return offerBootFile(req, settings, clientIP, boot, nil, proxy)
	}
	selected, hasSelection := pxeopt.SelectedType(opts[43])
	if hasSelection {
		for _, item := range menu.Items {
			if parseHex(item.PXEType) == selected {
				events.Publish("info", "dhcp", fmt.Sprintf("向 %s 响应菜单选择 %04x: %s", mac, selected, item.BootFile))
				return offerBootFile(req, settings, clientIP, item.BootFile, nil, proxy)
			}
		}
	}
	if menu.Enabled && !proxy && isUEFIArch(arch) {
		opt43 := pxeopt.BuildOption43(menu, settings.Server.AdvertiseIP)
		events.Publish("info", "dhcp", fmt.Sprintf("向 %s 响应原生 PXE 菜单: %s", mac, menu.MenuType))
		return offerBootFile(req, settings, clientIP, "", opt43, proxy)
	}
	boot := executableBootFile(settings, arch)
	events.Publish("info", "dhcp", fmt.Sprintf("向 %s 响应原始 PXE 可执行启动文件: %s", mac, boot))
	return offerBootFile(req, settings, clientIP, boot, []byte{6, 1, 8, 255}, proxy)
}

func executableBootFile(settings storage.ServiceSettings, arch string) string {
	if isUEFIArch(arch) {
		if netbootExists(settings, "netboot.xyz.efi") {
			return "netboot/netboot.xyz.efi"
		}
		if arch == "uefi32" && settings.BootFiles.UEFI32 != "" {
			return settings.BootFiles.UEFI32
		}
		return settings.BootFiles.UEFI64
	}
	if netbootExists(settings, "netboot.xyz.kpxe") {
		return "netboot/netboot.xyz.kpxe"
	}
	if netbootExists(settings, "netboot.xyz-undionly.kpxe") {
		return "netboot/netboot.xyz-undionly.kpxe"
	}
	return settings.BootFiles.BIOS
}

func ipxeBootFile(settings storage.ServiceSettings, arch string, opts map[byte][]byte) string {
	if ipxeHasFeature(opts, 0x13) {
		return fmt.Sprintf("%s/dynamic.ipxe?bootfile=ipxemenu", httpBootURI(settings))
	}
	return executableBootFile(settings, arch)
}

func httpBootURI(settings storage.ServiceSettings) string {
	addr := settings.HTTPBoot.Addr
	port := "80"
	if strings.HasPrefix(addr, ":") && len(addr) > 1 {
		port = addr[1:]
	} else if host, p, err := net.SplitHostPort(addr); err == nil {
		if host != "" && host != "0.0.0.0" {
			return fmt.Sprintf("http://%s:%s", host, p)
		}
		port = p
	}
	return fmt.Sprintf("http://%s:%s", settings.Server.AdvertiseIP, port)
}

func ipxeHasFeature(opts map[byte][]byte, feature byte) bool {
	encap := parseOptions(opts[175])
	v, ok := encap[feature]
	if !ok {
		return false
	}
	if len(v) == 0 {
		return true
	}
	for _, b := range v {
		if b != 0 {
			return true
		}
	}
	return false
}

func netbootExists(settings storage.ServiceSettings, name string) bool {
	if settings.NetbootXYZ.DownloadDir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(settings.NetbootXYZ.DownloadDir, name))
	return err == nil && !info.IsDir()
}

func offerBootFile(req []byte, settings storage.ServiceSettings, yiaddr, bootFile string, opt43 []byte, proxy bool) []byte {
	return offerResponse(req, settings, yiaddr, bootFile, opt43, proxy, true)
}

func offerNetworkConfig(req []byte, settings storage.ServiceSettings, yiaddr string) []byte {
	return offerResponse(req, settings, yiaddr, "", nil, false, false)
}

func offerResponse(req []byte, settings storage.ServiceSettings, yiaddr, bootFile string, opt43 []byte, proxy, includePXE bool) []byte {
	serverIP := net.ParseIP(settings.Server.AdvertiseIP).To4()
	if serverIP == nil {
		return nil
	}
	yi := net.ParseIP(yiaddr).To4()
	if yi == nil {
		yi = net.IPv4zero
	}
	msgType := byte(2)
	opts := parseOptions(req[240:])
	if proxy || (len(opts[53]) > 0 && opts[53][0] == 3) {
		msgType = 5
	}
	resp := make([]byte, 0, 548)
	resp = append(resp, 2, 1, 6, 0)
	resp = append(resp, req[4:8]...)
	resp = append(resp, 0, 0, 0x80, 0)
	resp = append(resp, req[12:16]...)
	resp = append(resp, yi...)
	resp = append(resp, serverIP...)
	resp = append(resp, req[24:28]...)
	resp = append(resp, req[28:44]...)
	resp = append(resp, make([]byte, 64)...)
	fileBytes := []byte(bootFile)
	if len(fileBytes) > 127 {
		fileBytes = fileBytes[:127]
	}
	resp = append(resp, fileBytes...)
	resp = append(resp, make([]byte, 128-len(fileBytes))...)
	resp = append(resp, []byte(magicCookie)...)
	resp = opt(resp, 53, []byte{msgType})
	resp = opt(resp, 54, serverIP)
	if includePXE {
		resp = opt(resp, 60, []byte("PXEClient"))
	}
	if v := opts[97]; len(v) > 0 {
		resp = opt(resp, 97, v)
	}
	if len(opt43) > 0 {
		resp = opt(resp, 43, opt43)
	}
	if bootFile != "" {
		resp = opt(resp, 66, []byte(settings.Server.AdvertiseIP))
		resp = opt(resp, 67, append([]byte(bootFile), 0))
	}
	if settings.DHCP.Mode == "dhcp" && !proxy {
		if mask := net.ParseIP(settings.DHCP.SubnetMask).To4(); mask != nil {
			resp = opt(resp, 1, mask)
		}
		if router := net.ParseIP(settings.DHCP.Router).To4(); router != nil {
			resp = opt(resp, 3, router)
		}
		if len(settings.DHCP.DNS) > 0 {
			var dns []byte
			for _, item := range settings.DHCP.DNS {
				if ip := net.ParseIP(item).To4(); ip != nil {
					dns = append(dns, ip...)
				}
			}
			resp = opt(resp, 6, dns)
		}
		lease := make([]byte, 4)
		binary.BigEndian.PutUint32(lease, uint32(settings.DHCP.LeaseTimeSeconds))
		resp = opt(resp, 51, lease)
		renew := make([]byte, 4)
		binary.BigEndian.PutUint32(renew, uint32(settings.DHCP.LeaseTimeSeconds/2))
		resp = opt(resp, 58, renew)
		rebinding := make([]byte, 4)
		binary.BigEndian.PutUint32(rebinding, uint32(settings.DHCP.LeaseTimeSeconds*7/8))
		resp = opt(resp, 59, rebinding)
	}
	resp = append(resp, 255)
	return resp
}

func nak(req []byte, settings storage.ServiceSettings, message string) []byte {
	serverIP := net.ParseIP(settings.Server.AdvertiseIP).To4()
	if serverIP == nil {
		return nil
	}
	resp := make([]byte, 0, 548)
	resp = append(resp, 2, 1, 6, 0)
	resp = append(resp, req[4:8]...)
	resp = append(resp, 0, 0, 0x80, 0)
	resp = append(resp, req[12:16]...)
	resp = append(resp, make([]byte, 4)...)
	resp = append(resp, serverIP...)
	resp = append(resp, req[24:28]...)
	resp = append(resp, req[28:44]...)
	resp = append(resp, make([]byte, 64+128)...)
	resp = append(resp, []byte(magicCookie)...)
	resp = opt(resp, 53, []byte{6})
	resp = opt(resp, 54, serverIP)
	resp = opt(resp, 56, []byte(message))
	resp = append(resp, 255)
	return resp
}

func parseOptions(raw []byte) map[byte][]byte {
	out := map[byte][]byte{}
	for i := 0; i < len(raw); {
		code := raw[i]
		i++
		if code == 0 {
			continue
		}
		if code == 255 || i >= len(raw) {
			break
		}
		ln := int(raw[i])
		i++
		if i+ln > len(raw) {
			break
		}
		out[code] = raw[i : i+ln]
		i += ln
	}
	return out
}

func opt(pkt []byte, code byte, val []byte) []byte {
	if len(val) == 0 {
		return pkt
	}
	if len(val) > 255 {
		val = val[:255]
	}
	pkt = append(pkt, code, byte(len(val)))
	pkt = append(pkt, val...)
	return pkt
}

func macFromPacket(pkt []byte) string {
	return storage.NormalizeMAC(net.HardwareAddr(pkt[28:34]).String())
}

func archName(v []byte) string {
	if len(v) < 2 {
		return "bios"
	}
	code := binary.BigEndian.Uint16(v[:2])
	switch code {
	case 0:
		return "bios"
	case 6:
		return "uefi32"
	case 7, 9:
		return "uefi64"
	case 10:
		return "uefi_arm32"
	case 11:
		return "uefi_arm64"
	default:
		return "bios"
	}
}

func isUEFIArch(arch string) bool {
	return arch == "uefi32" || arch == "uefi64" || arch == "uefi_arm32" || arch == "uefi_arm64"
}

func isPXEClient(opts map[byte][]byte, vendorClass string, isIPXE bool) bool {
	if isIPXE {
		return true
	}
	return contains([]byte(vendorClass), "PXEClient") || len(opts[93]) >= 2 || len(opts[97]) > 0
}

func contains(v []byte, needle string) bool {
	return len(v) > 0 && stringContains(string(v), needle)
}

func stringContains(s, needle string) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func findMenu(menus []storage.Menu, typ string) storage.Menu {
	for _, menu := range menus {
		if menu.MenuType == typ {
			return menu
		}
	}
	return storage.Menu{}
}

func parseHex(v string) uint16 {
	var out uint16
	for _, ch := range []byte(v) {
		out <<= 4
		switch {
		case ch >= '0' && ch <= '9':
			out += uint16(ch - '0')
		case ch >= 'a' && ch <= 'f':
			out += uint16(ch-'a') + 10
		case ch >= 'A' && ch <= 'F':
			out += uint16(ch-'A') + 10
		}
	}
	return out
}

func DetectServers(ctx context.Context, listenIP string, timeout time.Duration, excludeIPs ...string) ([]string, error) {
	if listenIP == "" || listenIP == "0.0.0.0" {
		listenIP = "0.0.0.0"
	}
	excluded := excludedIPs(excludeIPs)
	addr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(listenIP, "68"))
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	xid := []byte{0x50, 0x58, 0x45, 0x01}
	pkt := dhcpDiscoverPacket(xid)
	dst, _ := net.ResolveUDPAddr("udp4", "255.255.255.255:67")
	_, _ = conn.WriteToUDP(pkt, dst)
	found := readDHCPServers(ctx, conn, timeout, xid, excluded)
	return keys(found), nil
}

type InterfaceProbe struct {
	Interface string   `json:"interface"`
	IP        string   `json:"ip"`
	Broadcast string   `json:"broadcast"`
	Servers   []string `json:"servers"`
	Error     string   `json:"error,omitempty"`
}

func DetectServersByInterface(ctx context.Context, listenIP string, timeout time.Duration, excludeIPs ...string) ([]InterfaceProbe, error) {
	targets, err := dhcpProbeTargets(listenIP)
	if err != nil {
		return nil, err
	}
	excluded := excludedIPs(excludeIPs)
	results := make([]InterfaceProbe, 0, len(targets))
	for i, target := range targets {
		xid := []byte{0x50, 0x58, 0x45, byte(i + 1)}
		item := InterfaceProbe{Interface: target.name, IP: target.ip.String(), Broadcast: target.broadcast.String(), Servers: []string{}}
		servers, err := detectServersOnIP(ctx, target.ip.String(), target.broadcast.String(), timeout, xid, excluded)
		if err != nil {
			item.Error = err.Error()
		} else {
			item.Servers = keys(servers)
		}
		results = append(results, item)
	}
	return results, nil
}

type dhcpProbeTarget struct {
	name      string
	ip        net.IP
	broadcast net.IP
}

func dhcpProbeTargets(listenIP string) ([]dhcpProbeTarget, error) {
	filterIP := net.ParseIP(listenIP).To4()
	if listenIP == "" || listenIP == "0.0.0.0" {
		filterIP = nil
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var targets []dhcpProbeTarget
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
			ip = ip.To4()
			if filterIP != nil && !ip.Equal(filterIP) {
				continue
			}
			targets = append(targets, dhcpProbeTarget{name: iface.Name, ip: ip, broadcast: probeBroadcast(ip, ipNet.Mask)})
		}
	}
	return targets, nil
}

func detectServersOnIP(ctx context.Context, listenIP, broadcast string, timeout time.Duration, xid []byte, excluded map[string]bool) (map[string]bool, error) {
	addr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(listenIP, "68"))
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	pkt := dhcpDiscoverPacket(xid)
	if dst, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(broadcast, "67")); err == nil {
		_, _ = conn.WriteToUDP(pkt, dst)
	}
	if dst, err := net.ResolveUDPAddr("udp4", "255.255.255.255:67"); err == nil {
		_, _ = conn.WriteToUDP(pkt, dst)
	}
	return readDHCPServers(ctx, conn, timeout, xid, excluded), nil
}

func dhcpDiscoverPacket(xid []byte) []byte {
	pkt := make([]byte, 0, 300)
	pkt = append(pkt, 1, 1, 6, 0)
	pkt = append(pkt, xid...)
	pkt = append(pkt, 0, 0, 0x80, 0)
	pkt = append(pkt, make([]byte, 16)...)
	pkt = append(pkt, []byte{0, 17, 34, 51, 68, 85}...)
	pkt = append(pkt, make([]byte, 10+64+128)...)
	pkt = append(pkt, []byte(magicCookie)...)
	pkt = opt(pkt, 53, []byte{1})
	pkt = opt(pkt, 55, []byte{1, 3, 6, 12, 15, 54})
	pkt = append(pkt, 255)
	return pkt
}

func readDHCPServers(ctx context.Context, conn *net.UDPConn, timeout time.Duration, xid []byte, excluded map[string]bool) map[string]bool {
	deadline := time.Now().Add(timeout)
	found := map[string]bool{}
	buf := make([]byte, 1500)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return found
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil || n < 240 {
			continue
		}
		if len(xid) == 4 && string(buf[4:8]) != string(xid) {
			continue
		}
		opts := parseOptions(buf[240:n])
		if v := opts[53]; len(v) > 0 && v[0] == 2 {
			if sid := opts[54]; len(sid) == 4 {
				ip := net.IP(sid).String()
				if !excluded[ip] {
					found[ip] = true
				}
			}
		}
	}
	return found
}

func excludedIPs(excludeIPs []string) map[string]bool {
	excluded := map[string]bool{}
	for _, ip := range excludeIPs {
		if parsed := net.ParseIP(ip).To4(); parsed != nil {
			excluded[parsed.String()] = true
		}
	}
	return excluded
}

func probeBroadcast(ip net.IP, mask net.IPMask) net.IP {
	ip = ip.To4()
	out := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		out[i] = ip[i] | ^mask[i]
	}
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
