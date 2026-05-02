package ipxe

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"pxe/internal/bootmenu"
	"pxe/internal/booturl"
	"pxe/internal/storage"
)

type Request struct {
	Params   url.Values
	ClientIP string
}

type Generator struct {
	Settings storage.ServiceSettings
	Store    *storage.Store
}

func (g Generator) Generate(ctx context.Context, req Request) string {
	httpURI := g.httpURI()
	bootfile := strings.Trim(req.Params.Get("bootfile"), "\" ")
	if req.Params.Get("myip") != "" && req.Params.Get("mymac") != "" {
		ip := req.Params.Get("myip")
		mac := req.Params.Get("mymac")
		if err := g.Store.AssignMACToIP(ctx, ip, mac); err != nil {
			return fmt.Sprintf("#!ipxe\necho Bind failed: %s\nsleep 8\nshell\n", sanitizeIPXE(err.Error()))
		}
		return fmt.Sprintf("#!ipxe\necho Bound %s to %s\necho Rebooting in 5 seconds\nsleep 5\nreboot\n", sanitizeIPXE(mac), sanitizeIPXE(ip))
	}
	switch strings.ToLower(bootfile) {
	case "":
		return g.configMenu(ctx, httpURI)
	case "ipxemenu":
		return g.configMenu(ctx, httpURI)
	case "getmyip":
		return fmt.Sprintf("#!ipxe\nset ip %s\nset gateway %s\nset dns1 %s\n", req.ClientIP, g.Settings.DHCP.Router, firstDNS(g.Settings.DHCP.DNS))
	case "getmyxml":
		return `<?xml version="1.0" encoding="utf-8"?><unattend xmlns="urn:schemas-microsoft-com:unattend"></unattend>`
	case "whoami":
		return g.whoamiMenu(ctx, httpURI)
	default:
		if !validBootPath(bootfile) {
			return fmt.Sprintf("#!ipxe\necho Invalid boot path: %s\nsleep 5\nchain %s/dynamic.ipxe?bootfile=ipxemenu\n", sanitizeIPXE(bootfile), httpURI)
		}
		return g.chainScript(bootfile, httpURI)
	}
}

func (g Generator) whoamiMenu(ctx context.Context, httpURI string) string {
	clients, err := g.Store.UnassignedClients(ctx)
	if err != nil {
		return "#!ipxe\necho Failed to read unassigned clients\nsleep 5\nshell\n"
	}
	if len(clients) == 0 {
		return "#!ipxe\necho No unassigned clients. Add clients in Web UI first.\nsleep 5\nexit\n"
	}
	var b strings.Builder
	b.WriteString("#!ipxe\nmenu Select this machine\n")
	for _, c := range clients {
		if c.IP == "" {
			continue
		}
		fmt.Fprintf(&b, "item %s %s - %s\n", c.IP, sanitizeIPXE(c.Name), c.IP)
	}
	fmt.Fprintf(&b, "choose --timeout 30000 selected || exit\nchain %s/dynamic.ipxe?myip=${selected:uristring}&mymac=${net0/mac:uristring}\n", httpURI)
	return b.String()
}

func sanitizeIPXE(v string) string {
	replacer := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ", "\"", "'", "`", "'")
	return replacer.Replace(v)
}

func validBootPath(v string) bool {
	if strings.TrimSpace(v) == "" {
		return false
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		u, err := url.Parse(v)
		return err == nil && u.Host != ""
	}
	clean := path.Clean(strings.ReplaceAll(v, "\\", "/"))
	return clean != "." && !strings.HasPrefix(clean, "../") && clean != ".." && !strings.Contains(clean, "\x00")
}

func (g Generator) httpURI() string {
	return booturl.HTTPBaseWithListenHost(g.Settings.Server.AdvertiseIP, g.Settings.HTTPBoot.Addr)
}

func (g Generator) configMenu(ctx context.Context, httpURI string) string {
	menus, err := g.Store.ListMenus(ctx)
	if err != nil {
		return "#!ipxe\necho Failed to read boot menu\nsleep 5\nexit\n"
	}
	var menu storage.Menu
	for _, m := range menus {
		if m.MenuType == "ipxe" {
			menu = m
			break
		}
	}
	if !menu.Enabled {
		return "#!ipxe\necho iPXE menu disabled\nsanboot --no-describe --drive 0x80\n"
	}
	var b strings.Builder
	b.WriteString("#!ipxe\nisset ${net0/ip} || dhcp || goto failed\n")
	fmt.Fprintf(&b, "set bootserver %s\nset menu-timeout %d\nmenu %s\n", httpURI, bootmenu.TimeoutMillis(menu), sanitizeIPXE(menu.Prompt))
	type menuAction struct {
		name   string
		script string
	}
	var actions []menuAction
	idx := 0
	for _, item := range menu.Items {
		if !item.Enabled {
			continue
		}
		name := fmt.Sprintf("item_%d", idx)
		idx++
		fmt.Fprintf(&b, "item %s %s\n", name, sanitizeIPXE(item.Title))
		actions = append(actions, menuAction{name: name, script: actionFor(item.BootFile, httpURI)})
	}
	if len(actions) == 0 {
		b.WriteString("item local Boot Local Disk\n")
		actions = append(actions, menuAction{name: "local", script: "sanboot --no-describe --drive 0x80"})
	}
	b.WriteString("choose --timeout ${menu-timeout} selected || goto local\ngoto ${selected}\n\n")
	for _, action := range actions {
		fmt.Fprintf(&b, ":%s\n%s || goto failed\ngoto end\n\n", action.name, action.script)
	}
	b.WriteString(":local\nsanboot --no-describe --drive 0x80 || goto failed\n\n:failed\necho Boot failed. Check boot file, HTTP Boot address and network.\nsleep 5\nshell\n:end\nexit\n")
	return b.String()
}

func actionFor(bootFile, httpURI string) string {
	if strings.TrimSpace(bootFile) == "" {
		return "sanboot --no-describe --drive 0x80"
	}
	if strings.Contains(bootFile, "%dynamicboot%") {
		value := bootFile
		if parts := strings.SplitN(bootFile, "=", 2); len(parts) == 2 {
			value = parts[1]
		}
		return fmt.Sprintf("chain %s/dynamic.ipxe?bootfile=%s", httpURI, url.QueryEscape(value))
	}
	if strings.HasPrefix(bootFile, "http://") || strings.HasPrefix(bootFile, "https://") {
		return "chain " + bootFile
	}
	return fmt.Sprintf("chain %s/%s", httpURI, escapePath(strings.TrimLeft(bootFile, "/")))
}

func (g Generator) chainScript(bootfile, httpURI string) string {
	if strings.HasPrefix(bootfile, "http://") || strings.HasPrefix(bootfile, "https://") {
		return directChainScript(bootfile, httpURI)
	}
	ext := strings.ToLower(filepath.Ext(bootfile))
	if ext != ".ipxe" && ext != ".efi" {
		return fmt.Sprintf("#!ipxe\necho Unsupported direct boot file: %s\necho Use boot.ipxe for explicit boot steps.\nsleep 5\nchain %s/dynamic.ipxe?bootfile=ipxemenu\n", sanitizeIPXE(bootfile), httpURI)
	}
	target := fmt.Sprintf("%s/%s", httpURI, escapePath(strings.TrimLeft(bootfile, "/")))
	return directChainScript(target, httpURI)
}

func directChainScript(target, httpURI string) string {
	return fmt.Sprintf("#!ipxe\nisset ${net0/ip} || dhcp || goto failed\nimgfree\nchain %s || goto failed\ngoto end\n:failed\necho iPXE script failed. Check URL and script content.\nsleep 5\nchain %s/dynamic.ipxe?bootfile=ipxemenu\n:end\nexit\n", target, httpURI)
}

func escapePath(path string) string {
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func firstDNS(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}
