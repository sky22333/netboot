package ipxe

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

var supported = map[string]string{
	".wim":   "wim",
	".iso":   "iso",
	".img":   "img",
	".ima":   "img",
	".efi":   "efi",
	".vhd":   "disk",
	".vhdx":  "disk",
	".vmdk":  "disk",
	".dsk":   "disk",
	".ramos": "ramos",
	".iqn":   "iqn",
}

func (g Generator) Generate(ctx context.Context, req Request) string {
	httpURI := g.httpURI()
	bootfile := strings.Trim(req.Params.Get("bootfile"), "\" ")
	if req.Params.Get("myip") != "" && req.Params.Get("mymac") != "" {
		ip := req.Params.Get("myip")
		mac := req.Params.Get("mymac")
		if err := g.Store.AssignMACToIP(ctx, ip, mac); err != nil {
			return fmt.Sprintf("#!ipxe\necho 绑定失败: %s\nsleep 8\nshell\n", sanitizeIPXE(err.Error()))
		}
		return fmt.Sprintf("#!ipxe\necho 已绑定 %s 到 %s\necho 5 秒后重启\nsleep 5\nreboot\n", sanitizeIPXE(mac), sanitizeIPXE(ip))
	}
	switch strings.ToLower(bootfile) {
	case "", "ipxefm":
		return g.fileMenu(httpURI)
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
			return fmt.Sprintf("#!ipxe\necho 启动文件路径无效: %s\nsleep 5\nchain %s/dynamic.ipxe?bootfile=ipxefm\n", sanitizeIPXE(bootfile), httpURI)
		}
		return g.chainScript(bootfile, httpURI)
	}
}

func (g Generator) whoamiMenu(ctx context.Context, httpURI string) string {
	clients, err := g.Store.UnassignedClients(ctx)
	if err != nil {
		return "#!ipxe\necho 读取待分配客户端失败\nsleep 5\nshell\n"
	}
	if len(clients) == 0 {
		return "#!ipxe\necho 没有待分配客户端，请先在 Web UI 批量添加。\nsleep 5\nexit\n"
	}
	var b strings.Builder
	b.WriteString("#!ipxe\nmenu 请选择这台机器对应的预分配名称\n")
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
	clean := filepath.Clean(strings.ReplaceAll(v, "\\", "/"))
	return clean != "." && !strings.HasPrefix(clean, "../") && clean != ".." && !strings.Contains(clean, "\x00")
}

func (g Generator) httpURI() string {
	addr := g.Settings.HTTPBoot.Addr
	port := "80"
	if strings.HasPrefix(addr, ":") && len(addr) > 1 {
		port = addr[1:]
	} else if host, p, err := net.SplitHostPort(addr); err == nil {
		if host != "" && host != "0.0.0.0" {
			return fmt.Sprintf("http://%s:%s", host, p)
		}
		port = p
	}
	return fmt.Sprintf("http://%s:%s", g.Settings.Server.AdvertiseIP, port)
}

func (g Generator) configMenu(ctx context.Context, httpURI string) string {
	menus, err := g.Store.ListMenus(ctx)
	if err != nil {
		return "#!ipxe\necho 读取菜单失败\nsleep 5\nexit\n"
	}
	var menu storage.Menu
	for _, m := range menus {
		if m.MenuType == "ipxe" {
			menu = m
			break
		}
	}
	if !menu.Enabled {
		return "#!ipxe\necho iPXE 菜单已禁用\nsanboot --no-describe --drive 0x80\n"
	}
	var b strings.Builder
	b.WriteString("#!ipxe\nisset ${net0/ip} || dhcp || goto failed\n")
	fmt.Fprintf(&b, "set bootserver %s\nset menu-timeout %d\nmenu %s\n", httpURI, menu.TimeoutSeconds*1000, sanitizeIPXE(menu.Prompt))
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
		b.WriteString("item local 从本地硬盘启动\n")
		actions = append(actions, menuAction{name: "local", script: "sanboot --no-describe --drive 0x80"})
	}
	b.WriteString("choose --timeout ${menu-timeout} selected || goto local\ngoto ${selected}\n\n")
	for _, action := range actions {
		fmt.Fprintf(&b, ":%s\n%s || goto failed\ngoto end\n\n", action.name, action.script)
	}
	b.WriteString(":local\nsanboot --no-describe --drive 0x80 || goto failed\n\n:failed\necho 启动失败，请检查启动文件、HTTP Boot 地址和网络连通性。\nsleep 5\nshell\n:end\nexit\n")
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

func (g Generator) fileMenu(httpURI string) string {
	var files []string
	root := g.Settings.HTTPBoot.Root
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := supported[ext]; !ok {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	if len(files) == 0 {
		return "#!ipxe\necho HTTP 目录中没有可启动文件\nsleep 5\nsanboot --no-describe --drive 0x80\n"
	}
	var b strings.Builder
	b.WriteString("#!ipxe\nmenu 可启动文件\n")
	for _, f := range files {
		fmt.Fprintf(&b, "item %q %q\n", f, f)
	}
	fmt.Fprintf(&b, "choose --timeout 30000 selected || exit\nchain %s/dynamic.ipxe?bootfile=${selected:uristring}\n", httpURI)
	return b.String()
}

func (g Generator) chainScript(bootfile, httpURI string) string {
	ext := strings.ToLower(filepath.Ext(bootfile))
	typ, ok := supported[ext]
	if !ok {
		return fmt.Sprintf("#!ipxe\necho 不支持的文件类型: %s\nsleep 5\nchain %s/dynamic.ipxe?bootfile=ipxefm\n", bootfile, httpURI)
	}
	if !strings.HasPrefix(bootfile, "/") {
		bootfile = "/" + bootfile
	}
	escapedBootFile := "/" + escapePath(strings.TrimLeft(bootfile, "/"))
	if typ == "efi" {
		return fmt.Sprintf("#!ipxe\nisset ${net0/ip} || dhcp || goto failed\nimgfree\nchain %s%s || goto failed\nboot || goto failed\n:failed\necho EFI 文件启动失败\nsleep 5\nchain %s/dynamic.ipxe?bootfile=ipxefm\n", httpURI, escapedBootFile, httpURI)
	}
	return fmt.Sprintf("#!ipxe\nisset ${net0/ip} || dhcp || goto failed\nimgfree\nset booturl %s\nset bootfile %s\nchain http://${booturl}/Boot/ipxefm/types/%s || goto failed\n:failed\necho 启动处理器失败，请确认 Boot/ipxefm/types/%s 存在。\nsleep 5\nchain %s/dynamic.ipxe?bootfile=ipxefm\n", strings.TrimPrefix(httpURI, "http://"), escapedBootFile, typ, typ, httpURI)
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
