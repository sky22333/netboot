package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"pxe/internal/booturl"
	"pxe/internal/config"
	"pxe/internal/dhcp"
	"pxe/internal/ipxe"
	"pxe/internal/netboot"
	"pxe/internal/observability"
	"pxe/internal/platform"
	"pxe/internal/storage"
	"pxe/internal/torrent"
)

//go:embed dist/*
var webFS embed.FS

type Backend interface {
	Status() any
	StartServices(context.Context) error
	StopServices(context.Context)
	Storage() *storage.Store
	EventHub() *observability.Hub
	BootConfig() config.BootConfig
}

type Handler struct {
	app          Backend
	sessions     *SessionManager
	loginLimiter *LoginLimiter
}

func NewRouter(app Backend) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.MaxMultipartMemory = 2 << 30
	r.Use(gin.Recovery(), bodyLimit(128<<20))
	h := &Handler{app: app, sessions: NewSessionManager(), loginLimiter: NewLoginLimiter()}

	api := r.Group("/api/v1")
	api.GET("/setup/status", h.setupStatus)
	api.POST("/setup", h.setup)
	api.POST("/auth/login", h.login)
	api.POST("/auth/logout", h.logout)

	protected := api.Group("")
	protected.Use(h.requireAuth)
	protected.GET("/status", h.status)
	protected.GET("/diagnostics", h.diagnostics)
	protected.GET("/diagnostics/dhcp", h.dhcpDiagnostics)
	protected.GET("/config", h.getConfig)
	protected.PUT("/config", h.saveConfig)
	protected.POST("/config/validate", h.validateConfig)
	protected.POST("/services/start", h.startServices)
	protected.POST("/services/stop", h.stopServices)
	protected.POST("/services/restart", h.restartServices)
	protected.GET("/clients", h.listClients)
	protected.POST("/clients", h.saveClient)
	protected.POST("/clients/batch", h.batchClients)
	protected.POST("/clients/report", h.clientReport)
	protected.PUT("/clients/:id", h.saveClient)
	protected.DELETE("/clients/:id", h.deleteClient)
	protected.POST("/clients/:id/wol", h.wol)
	protected.POST("/clients/:id/clear-mac", h.clearClientMAC)
	protected.GET("/menus", h.listMenus)
	protected.PUT("/menus", h.saveMenus)
	protected.GET("/actions", h.listActions)
	protected.PUT("/actions", h.saveActions)
	protected.POST("/actions/:id/execute", h.executeAction)
	protected.GET("/users", h.listUsers)
	protected.POST("/users", h.createUserAPI)
	protected.POST("/users/:id/password", h.changeUserPassword)
	protected.DELETE("/users/:id", h.deleteUser)
	protected.GET("/files", h.listFiles)
	protected.GET("/files/content", h.getFileContent)
	protected.PUT("/files/content", h.saveFileContent)
	protected.POST("/files/upload", h.uploadFile)
	protected.POST("/files/mkdir", h.mkdirFile)
	protected.POST("/files/rename", h.renameFile)
	protected.DELETE("/files", h.deleteFile)
	protected.POST("/files/torrent", h.createTorrent)
	protected.GET("/logs", h.logs)
	protected.GET("/events/stream", h.eventStream)
	protected.GET("/netbootxyz/files", h.netbootFiles)
	protected.POST("/netbootxyz/download", h.netbootDownload)

	r.GET("/dynamic.ipxe", h.dynamicProxy)
	r.HEAD("/dynamic.ipxe", h.dynamicProxy)
	r.NoRoute(staticHandler())
	return r
}

func bodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil && !strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

func (h *Handler) setupStatus(c *gin.Context) {
	OK(c, gin.H{"has_user": h.hasUsers(c.Request.Context())})
}

func (h *Handler) setup(c *gin.Context) {
	if h.hasUsers(c.Request.Context()) {
		Fail(c, http.StatusConflict, "SETUP_DONE", "初始化已经完成")
		return
	}
	var req struct{ Username, Password string }
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "VALIDATION_ERROR", "请求格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 8 {
		Fail(c, 400, "VALIDATION_ERROR", "用户名不能为空，密码至少 8 位")
		return
	}
	if err := h.createUser(c.Request.Context(), req.Username, req.Password); err != nil {
		Fail(c, 500, "SETUP_FAILED", err.Error())
		return
	}
	h.app.EventHub().Publish("info", "auth", "管理员账号已初始化")
	OK(c, gin.H{"message": "初始化完成"})
}

func (h *Handler) login(c *gin.Context) {
	var req struct{ Username, Password string }
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "VALIDATION_ERROR", "请求格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	key := c.ClientIP() + "|" + strings.ToLower(req.Username)
	if !h.loginLimiter.Allow(key) {
		Fail(c, http.StatusTooManyRequests, "LOGIN_RATE_LIMITED", "登录尝试过多，请 10 分钟后再试")
		return
	}
	if !h.checkLogin(c.Request.Context(), req.Username, req.Password) {
		h.loginLimiter.Fail(key)
		Fail(c, 401, "LOGIN_FAILED", "用户名或密码错误")
		return
	}
	h.loginLimiter.Success(key)
	token := h.sessions.Create(req.Username)
	http.SetCookie(c.Writer, &http.Cookie{Name: "pxe_session", Value: token, Path: "/", MaxAge: 86400, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	OK(c, gin.H{"username": req.Username})
}

func (h *Handler) logout(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{Name: "pxe_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	OK(c, gin.H{"message": "已退出"})
}

func (h *Handler) status(c *gin.Context) {
	OK(c, h.app.Status())
}

func (h *Handler) diagnostics(c *gin.Context) {
	boot := h.app.BootConfig()
	permission := platform.Permission()
	OK(c, gin.H{
		"data_dir":    boot.Data.Dir,
		"db":          boot.Database.Path,
		"admin_addr":  boot.Admin.AdminAddr,
		"is_admin":    permission.AdminLike,
		"permission":  permission,
		"interfaces":  platform.Interfaces(),
		"suggestions": []string{"若客户端无法获取启动文件，请确认程序已被系统防火墙放行，并且监听 IP、通告 IP 与客户端处于可达网络。"},
	})
}

func (h *Handler) dhcpDiagnostics(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	exclusions := []string{}
	if settings.DHCP.Enabled && settings.DHCP.Mode == "dhcp" && settings.Server.AdvertiseIP != "" {
		exclusions = append(exclusions, settings.Server.AdvertiseIP)
	}
	probes, _ := dhcp.DetectServersByInterface(c.Request.Context(), settings.Server.ListenIP, 3*time.Second, exclusions...)
	note := "DHCP 探测会按可用 IPv4 网卡逐个发送短时探测包，仅作为辅助诊断；部分热点、桥接网卡、防火墙或交换机可能拦截探测包。"
	if len(exclusions) > 0 {
		note = "完整 DHCP 已启用，探测结果已排除本程序通告 IP；其余结果仅作为辅助诊断。"
	}
	OK(c, gin.H{
		"dhcp_servers":          uniqueProbeServers(probes),
		"dhcp_interface_probes": probes,
		"dhcp_probe_exclusions": exclusions,
		"dhcp_probe_note":       note,
	})
}

func uniqueProbeServers(probes []dhcp.InterfaceProbe) []string {
	seen := map[string]bool{}
	for _, probe := range probes {
		for _, server := range probe.Servers {
			seen[server] = true
		}
	}
	out := make([]string, 0, len(seen))
	for server := range seen {
		out = append(out, server)
	}
	return out
}

func (h *Handler) getConfig(c *gin.Context) {
	settings, err := h.app.Storage().GetSettings(c.Request.Context())
	if err != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", err.Error())
		return
	}
	OK(c, settings)
}

func (h *Handler) validateConfig(c *gin.Context) {
	var settings storage.ServiceSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		Fail(c, 400, "CONFIG_INVALID", "配置格式错误")
		return
	}
	if err := storage.ValidateSettings(settings); err != nil {
		Fail(c, 400, "CONFIG_INVALID", err.Error())
		return
	}
	OK(c, gin.H{"valid": true})
}

func (h *Handler) saveConfig(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		Fail(c, 400, "CONFIG_INVALID", "配置读取失败")
		return
	}
	var settings storage.ServiceSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		Fail(c, 400, "CONFIG_INVALID", "配置格式错误")
		return
	}
	current, err := h.app.Storage().GetSettings(c.Request.Context())
	if err != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", err.Error())
		return
	}
	settings = preserveMissingConfigSections(raw, settings, current)
	if err := h.app.Storage().SaveSettings(c.Request.Context(), settings); err != nil {
		Fail(c, 400, "CONFIG_SAVE_FAILED", err.Error())
		return
	}
	h.app.EventHub().Publish("info", "config", "服务配置已保存")
	_ = h.app.Storage().AddEvent(c.Request.Context(), "info", "config", "服务配置已保存", nil)
	OK(c, settings)
}

func preserveMissingConfigSections(raw []byte, settings, current storage.ServiceSettings) storage.ServiceSettings {
	if len(raw) == 0 {
		return settings
	}
	var sections map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sections); err != nil {
		return settings
	}
	if _, ok := sections["boot_files"]; !ok {
		settings.BootFiles = current.BootFiles
	}
	if _, ok := sections["netboot_xyz"]; !ok {
		settings.NetbootXYZ = current.NetbootXYZ
	}
	if _, ok := sections["security"]; !ok {
		settings.Security = current.Security
	}
	return settings
}

func (h *Handler) startServices(c *gin.Context) {
	if err := h.app.StartServices(c.Request.Context()); err != nil {
		Fail(c, 500, "SERVICE_START_FAILED", err.Error())
		return
	}
	OK(c, h.app.Status())
}

func (h *Handler) stopServices(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	h.app.StopServices(ctx)
	OK(c, h.app.Status())
}

func (h *Handler) restartServices(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	h.app.StopServices(ctx)
	if err := h.app.StartServices(c.Request.Context()); err != nil {
		Fail(c, 500, "SERVICE_RESTART_FAILED", err.Error())
		return
	}
	OK(c, h.app.Status())
}

func (h *Handler) listClients(c *gin.Context) {
	clients, err := h.app.Storage().ListClients(c.Request.Context())
	if err != nil {
		Fail(c, 500, "CLIENT_LIST_FAILED", err.Error())
		return
	}
	OK(c, clients)
}

func (h *Handler) saveClient(c *gin.Context) {
	var client storage.Client
	if err := c.ShouldBindJSON(&client); err != nil {
		Fail(c, 400, "CLIENT_INVALID", "客户端格式错误")
		return
	}
	if id := c.Param("id"); id != "" {
		client.ID, _ = strconv.ParseInt(id, 10, 64)
	}
	if client.Name == "" {
		Fail(c, 400, "CLIENT_INVALID", "客户端名称不能为空")
		return
	}
	out, err := h.app.Storage().UpsertClient(c.Request.Context(), client)
	if err != nil {
		Fail(c, 500, "CLIENT_SAVE_FAILED", err.Error())
		return
	}
	OK(c, out)
}

func (h *Handler) deleteClient(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.app.Storage().DeleteClient(c.Request.Context(), id); err != nil {
		Fail(c, 500, "CLIENT_DELETE_FAILED", err.Error())
		return
	}
	_ = h.app.Storage().AddEvent(c.Request.Context(), "warning", "clients", "删除客户端", gin.H{"id": id})
	OK(c, gin.H{"deleted": id})
}

func (h *Handler) batchClients(c *gin.Context) {
	var req struct {
		Prefix  string `json:"prefix"`
		IPStart string `json:"ip_start"`
		Count   int    `json:"count"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "BATCH_INVALID", "批量参数格式错误")
		return
	}
	if req.Prefix == "" {
		req.Prefix = "PC-"
	}
	out, err := h.app.Storage().BatchCreateClients(c.Request.Context(), req.Prefix, req.IPStart, req.Count)
	if err != nil {
		Fail(c, 400, "BATCH_FAILED", err.Error())
		return
	}
	_ = h.app.Storage().AddEvent(c.Request.Context(), "info", "clients", "批量添加客户端", gin.H{"count": len(out)})
	OK(c, out)
}

func (h *Handler) clearClientMAC(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.app.Storage().ClearClientMAC(c.Request.Context(), id); err != nil {
		Fail(c, 500, "CLIENT_CLEAR_MAC_FAILED", err.Error())
		return
	}
	_ = h.app.Storage().AddEvent(c.Request.Context(), "warning", "clients", "清除客户端 MAC", gin.H{"id": id})
	OK(c, gin.H{"id": id})
}

func (h *Handler) clientReport(c *gin.Context) {
	var report struct {
		IP         string `json:"ip"`
		DiskHealth string `json:"disk_health"`
		NetSpeed   string `json:"net_speed"`
	}
	if err := c.ShouldBindJSON(&report); err != nil || report.IP == "" {
		Fail(c, 400, "REPORT_INVALID", "健康报告格式错误")
		return
	}
	if err := h.app.Storage().UpdateClientHealth(c.Request.Context(), report.IP, report.DiskHealth, report.NetSpeed); err != nil {
		Fail(c, 500, "REPORT_SAVE_FAILED", err.Error())
		return
	}
	OK(c, gin.H{"message": "健康报告已记录"})
}

func (h *Handler) wol(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	client, err := h.app.Storage().GetClient(c.Request.Context(), id)
	if err != nil {
		Fail(c, 404, "CLIENT_NOT_FOUND", "客户端不存在")
		return
	}
	if err := sendWOL(client.MAC); err != nil {
		Fail(c, 400, "WOL_FAILED", err.Error())
		return
	}
	h.app.EventHub().Publish("info", "clients", "已发送 WOL 唤醒包: "+client.MAC)
	OK(c, gin.H{"message": "已发送唤醒包"})
}

func (h *Handler) listMenus(c *gin.Context) {
	menus, err := h.app.Storage().ListMenus(c.Request.Context())
	if err != nil {
		Fail(c, 500, "MENU_LIST_FAILED", err.Error())
		return
	}
	OK(c, menus)
}

func (h *Handler) saveMenus(c *gin.Context) {
	var menus []storage.Menu
	if err := c.ShouldBindJSON(&menus); err != nil {
		Fail(c, 400, "MENU_INVALID", "菜单格式错误")
		return
	}
	if err := h.app.Storage().SaveMenus(c.Request.Context(), menus); err != nil {
		Fail(c, 500, "MENU_SAVE_FAILED", err.Error())
		return
	}
	_ = h.app.Storage().AddEvent(c.Request.Context(), "info", "menus", "启动菜单已保存", nil)
	OK(c, menus)
}

func (h *Handler) listActions(c *gin.Context) {
	actions, err := h.app.Storage().ListActions(c.Request.Context())
	if err != nil {
		Fail(c, 500, "ACTION_LIST_FAILED", err.Error())
		return
	}
	OK(c, actions)
}

func (h *Handler) saveActions(c *gin.Context) {
	var actions []storage.ClientAction
	if err := c.ShouldBindJSON(&actions); err != nil {
		Fail(c, 400, "ACTION_INVALID", "操作菜单格式错误")
		return
	}
	if err := h.app.Storage().SaveActions(c.Request.Context(), actions); err != nil {
		Fail(c, 500, "ACTION_SAVE_FAILED", err.Error())
		return
	}
	_ = h.app.Storage().AddEvent(c.Request.Context(), "info", "actions", "客户端操作菜单已保存", nil)
	OK(c, actions)
}

func (h *Handler) executeAction(c *gin.Context) {
	actionID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		ClientIDs []int64 `json:"client_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.ClientIDs) == 0 {
		Fail(c, 400, "ACTION_EXEC_INVALID", "请选择客户端")
		return
	}
	action, err := h.app.Storage().GetAction(c.Request.Context(), actionID)
	if err != nil || !action.Enabled {
		Fail(c, 404, "ACTION_NOT_FOUND", "操作不存在或未启用")
		return
	}
	results := make([]gin.H, 0, len(req.ClientIDs))
	for _, id := range req.ClientIDs {
		client, err := h.app.Storage().GetClient(c.Request.Context(), id)
		if err != nil {
			results = append(results, gin.H{"client_id": id, "ok": false, "error": "客户端不存在"})
			continue
		}
		args := replaceActionVars(action.Args, client)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
		cmd := exec.CommandContext(ctx, action.Command, splitArgs(args)...)
		out, err := cmd.CombinedOutput()
		cancel()
		item := gin.H{"client_id": id, "client": client.Name, "output": string(out), "ok": err == nil}
		if err != nil {
			item["error"] = err.Error()
		}
		results = append(results, item)
	}
	_ = h.app.Storage().AddEvent(c.Request.Context(), "warning", "actions", "执行客户端操作", gin.H{"action": action.Name, "count": len(req.ClientIDs)})
	OK(c, results)
}

func replaceActionVars(args string, c storage.Client) string {
	r := strings.NewReplacer("%IP%", c.IP, "%MAC%", c.MAC, "%NAME%", c.Name, "%STATUS%", c.Status, "%FIRMWARE%", c.Firmware, "%DISKHEALTH%", c.DiskHealth, "%NETSPEED%", c.NetSpeed)
	return r.Replace(args)
}

func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch r {
		case '"':
			inQuote = !inQuote
		case ' ', '\t':
			if inQuote {
				cur.WriteRune(r)
			} else if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func (h *Handler) listUsers(c *gin.Context) {
	users, err := h.app.Storage().ListUsers(c.Request.Context())
	if err != nil {
		Fail(c, 500, "USER_LIST_FAILED", err.Error())
		return
	}
	OK(c, users)
}

func (h *Handler) createUserAPI(c *gin.Context) {
	var req struct{ Username, Password, Role string }
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "USER_INVALID", "请求格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Password) < 8 {
		Fail(c, 400, "USER_INVALID", "用户名不能为空，密码至少 8 位")
		return
	}
	if err := h.createUserWithRole(c.Request.Context(), req.Username, req.Password, req.Role); err != nil {
		Fail(c, 500, "USER_CREATE_FAILED", err.Error())
		return
	}
	OK(c, gin.H{"message": "用户已创建"})
}

func (h *Handler) changeUserPassword(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct{ Password string }
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Password) < 8 {
		Fail(c, 400, "PASSWORD_INVALID", "密码至少 8 位")
		return
	}
	if err := h.changePassword(c.Request.Context(), id, req.Password); err != nil {
		Fail(c, 500, "PASSWORD_CHANGE_FAILED", err.Error())
		return
	}
	OK(c, gin.H{"message": "密码已修改"})
}

func (h *Handler) deleteUser(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		Fail(c, 400, "USER_INVALID", "用户 ID 无效")
		return
	}
	if err := h.app.Storage().DeleteUser(c.Request.Context(), id); err != nil {
		Fail(c, 400, "USER_DELETE_FAILED", err.Error())
		return
	}
	OK(c, gin.H{"deleted": id})
}

func (h *Handler) listFiles(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	rootType := c.DefaultQuery("root", "http")
	root, err := fileRoot(settings, rootType)
	if err != nil {
		Fail(c, 400, "ROOT_INVALID", "文件目录类型无效")
		return
	}
	rel := c.DefaultQuery("path", ".")
	target, err := safeJoin(root, rel)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "路径无效")
		return
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		Fail(c, 500, "FILE_LIST_FAILED", err.Error())
		return
	}
	files := []gin.H{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, gin.H{"name": entry.Name(), "dir": entry.IsDir(), "size": info.Size(), "mod_time": info.ModTime(), "editable": !entry.IsDir() && isEditableTextPath(entry.Name()) && info.Size() <= maxEditableFileBytes})
	}
	OK(c, gin.H{"root": rootType, "path": rel, "base_path": root, "files": files})
}

func (h *Handler) uploadFile(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	root, err := fileRoot(settings, c.DefaultPostForm("root", "http"))
	if err != nil {
		Fail(c, 400, "ROOT_INVALID", "文件目录类型无效")
		return
	}
	dir, err := safeJoin(root, c.DefaultPostForm("path", "."))
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "路径无效")
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		Fail(c, 400, "UPLOAD_INVALID", "请选择文件")
		return
	}
	if file.Size > 2<<30 {
		Fail(c, 400, "UPLOAD_TOO_LARGE", "单个上传文件不能超过 2 GiB")
		return
	}
	dst := filepath.Join(dir, filepath.Base(file.Filename))
	if err := c.SaveUploadedFile(file, dst); err != nil {
		Fail(c, 500, "UPLOAD_FAILED", err.Error())
		return
	}
	_ = h.app.Storage().AddEvent(c.Request.Context(), "info", "files", "上传文件", gin.H{"path": dst})
	OK(c, gin.H{"path": dst})
}

func (h *Handler) mkdirFile(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	var req struct{ Root, Path string }
	if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
		Fail(c, 400, "MKDIR_INVALID", "目录参数错误")
		return
	}
	root, err := fileRoot(settings, req.Root)
	if err != nil {
		Fail(c, 400, "ROOT_INVALID", "文件目录类型无效")
		return
	}
	target, err := safeJoin(root, req.Path)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "路径无效")
		return
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		Fail(c, 500, "MKDIR_FAILED", err.Error())
		return
	}
	OK(c, gin.H{"path": req.Path})
}

func (h *Handler) renameFile(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	var req struct{ Root, From, To string }
	if err := c.ShouldBindJSON(&req); err != nil || req.From == "" || req.To == "" {
		Fail(c, 400, "RENAME_INVALID", "重命名参数错误")
		return
	}
	root, err := fileRoot(settings, req.Root)
	if err != nil {
		Fail(c, 400, "ROOT_INVALID", "文件目录类型无效")
		return
	}
	from, err := safeJoin(root, req.From)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "源路径无效")
		return
	}
	to, err := safeJoin(root, req.To)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "目标路径无效")
		return
	}
	if err := os.Rename(from, to); err != nil {
		Fail(c, 500, "RENAME_FAILED", err.Error())
		return
	}
	_ = h.app.Storage().AddEvent(c.Request.Context(), "warning", "files", "重命名文件", gin.H{"from": req.From, "to": req.To})
	OK(c, gin.H{"from": req.From, "to": req.To})
}

func (h *Handler) deleteFile(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	root, err := fileRoot(settings, c.DefaultQuery("root", "http"))
	if err != nil {
		Fail(c, 400, "ROOT_INVALID", "文件目录类型无效")
		return
	}
	target, err := safeJoin(root, c.Query("path"))
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "路径无效")
		return
	}
	if err := os.Remove(target); err != nil {
		Fail(c, 500, "FILE_DELETE_FAILED", err.Error())
		return
	}
	_ = h.app.Storage().AddEvent(c.Request.Context(), "warning", "files", "删除文件", gin.H{"path": c.Query("path")})
	OK(c, gin.H{"deleted": c.Query("path")})
}

func (h *Handler) getFileContent(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	rootType := c.DefaultQuery("root", "http")
	root, err := fileRoot(settings, rootType)
	if err != nil {
		Fail(c, 400, "ROOT_INVALID", "文件目录类型无效")
		return
	}
	rel := c.Query("path")
	target, err := safeJoin(root, rel)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "路径无效")
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		Fail(c, 404, "FILE_NOT_FOUND", "文件不存在")
		return
	}
	if info.IsDir() {
		Fail(c, 400, "FILE_IS_DIRECTORY", "目录不能在线编辑")
		return
	}
	if !isEditableTextPath(target) {
		Fail(c, 400, "FILE_NOT_EDITABLE", "该文件类型不支持在线编辑")
		return
	}
	if info.Size() > maxEditableFileBytes {
		Fail(c, 400, "FILE_TOO_LARGE", "文件超过在线编辑大小限制")
		return
	}
	data, err := os.ReadFile(target)
	if err != nil {
		Fail(c, 500, "FILE_READ_FAILED", err.Error())
		return
	}
	if !utf8.Valid(data) {
		Fail(c, 400, "FILE_NOT_TEXT", "文件不是 UTF-8 文本")
		return
	}
	OK(c, gin.H{"root": rootType, "path": rel, "content": string(data), "size": info.Size(), "mod_time": info.ModTime()})
}

func (h *Handler) saveFileContent(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	var req struct {
		Root    string `json:"root"`
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
		Fail(c, 400, "FILE_CONTENT_INVALID", "文件内容参数错误")
		return
	}
	root, err := fileRoot(settings, req.Root)
	if err != nil {
		Fail(c, 400, "ROOT_INVALID", "文件目录类型无效")
		return
	}
	target, err := safeJoin(root, req.Path)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "路径无效")
		return
	}
	if !isEditableTextPath(target) {
		Fail(c, 400, "FILE_NOT_EDITABLE", "该文件类型不支持在线编辑")
		return
	}
	if len(req.Content) > maxEditableFileBytes {
		Fail(c, 400, "FILE_TOO_LARGE", "文件超过在线编辑大小限制")
		return
	}
	if !utf8.ValidString(req.Content) {
		Fail(c, 400, "FILE_NOT_TEXT", "文件内容必须是 UTF-8 文本")
		return
	}
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		Fail(c, 400, "FILE_IS_DIRECTORY", "目录不能在线编辑")
		return
	}
	if err := os.WriteFile(target, []byte(req.Content), 0644); err != nil {
		Fail(c, 500, "FILE_WRITE_FAILED", err.Error())
		return
	}
	_ = h.app.Storage().AddEvent(c.Request.Context(), "warning", "files", "保存文本文件", gin.H{"path": req.Path, "root": req.Root})
	OK(c, gin.H{"path": req.Path, "size": len(req.Content)})
}

func (h *Handler) createTorrent(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
		Root string `json:"root"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
		Fail(c, 400, "TORRENT_INVALID", "请选择要制作种子的文件")
		return
	}
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	if req.Root != "" && req.Root != "http" {
		Fail(c, 400, "TORRENT_ROOT_INVALID", "只有 HTTP Boot 目录支持制作种子")
		return
	}
	root := settings.HTTPBoot.Root
	target, err := safeJoin(root, req.Path)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "路径无效")
		return
	}
	rel, _ := filepath.Rel(root, target)
	webSeed := booturl.HTTPBootBase(settings) + "/" + filepath.ToSlash(rel)
	announce := "http://" + settings.Server.AdvertiseIP + ":6969/announce"
	if settings.Torrent.Addr != "" {
		if strings.HasPrefix(settings.Torrent.Addr, ":") {
			announce = "http://" + settings.Server.AdvertiseIP + settings.Torrent.Addr + "/announce"
		} else if _, p, err := net.SplitHostPort(settings.Torrent.Addr); err == nil {
			announce = "http://" + settings.Server.AdvertiseIP + ":" + p + "/announce"
		}
	}
	result, err := torrent.Create(target, announce, webSeed, 262144)
	if err != nil {
		Fail(c, 500, "TORRENT_FAILED", err.Error())
		return
	}
	OK(c, result)
}

func (h *Handler) logs(c *gin.Context) {
	limit := 500
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}
	events, err := h.app.Storage().RecentEvents(c.Request.Context(), limit)
	if err != nil || len(events) == 0 {
		OK(c, h.app.EventHub().Recent())
		return
	}
	OK(c, events)
}

func (h *Handler) eventStream(c *gin.Context) {
	ch, unsubscribe := h.app.EventHub().Subscribe()
	defer unsubscribe()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	for _, event := range h.app.EventHub().Recent() {
		b, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(c.Writer, "id: %d\ndata: %s\n\n", event.ID, b)
	}
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	c.Stream(func(w io.Writer) bool {
		select {
		case event := <-ch:
			b, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", event.ID, b)
			return true
		case <-ticker.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

func (h *Handler) netbootFiles(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	local := []gin.H{}
	for _, name := range settings.NetbootXYZ.Files {
		name = filepath.Base(name)
		target := filepath.Join(settings.NetbootXYZ.DownloadDir, name)
		item := gin.H{"file": name, "path": target, "exists": false}
		if info, err := os.Stat(target); err == nil {
			item["exists"] = true
			item["size"] = info.Size()
			item["mod_time"] = info.ModTime()
		}
		local = append(local, item)
	}
	localVarsPath := filepath.Join(settings.TFTP.Root, netboot.LocalVarsFile)
	localVars := gin.H{"file": netboot.LocalVarsFile, "path": localVarsPath, "exists": false}
	if info, err := os.Stat(localVarsPath); err == nil && !info.IsDir() {
		localVars["exists"] = true
		localVars["size"] = info.Size()
		localVars["mod_time"] = info.ModTime()
	}
	OK(c, gin.H{"base_url": settings.NetbootXYZ.BaseURL, "files": settings.NetbootXYZ.Files, "download_dir": settings.NetbootXYZ.DownloadDir, "local": local, "local_vars": localVars})
}

func (h *Handler) netbootDownload(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	results := netboot.Download(c.Request.Context(), settings.NetbootXYZ, h.app.EventHub())
	localVarsPath, created, err := netboot.EnsureLocalVars(settings.TFTP.Root, settings.Server.AdvertiseIP, settings.HTTPBoot.Addr, h.app.EventHub())
	OK(c, gin.H{"downloads": results, "local_vars": gin.H{"file": netboot.LocalVarsFile, "path": localVarsPath, "created": created, "error": errorString(err)}})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (h *Handler) dynamicProxy(c *gin.Context) {
	settings, _ := h.app.Storage().GetSettings(c.Request.Context())
	gen := ipxe.Generator{Settings: settings, Store: h.app.Storage()}
	script := gen.Generate(c.Request.Context(), ipxe.Request{Params: c.Request.URL.Query(), ClientIP: c.ClientIP()})
	c.Header("Content-Type", "text/plain; charset=utf-8")
	if c.Request.Method == http.MethodHead {
		c.Status(http.StatusOK)
		return
	}
	c.String(http.StatusOK, script)
}

const maxEditableFileBytes = 1 << 20

var editableTextExts = map[string]bool{
	".bat": true, ".cfg": true, ".cmd": true, ".conf": true, ".ini": true,
	".ipxe": true, ".json": true, ".ks": true, ".md": true, ".ps1": true,
	".seed": true, ".sh": true, ".toml": true, ".txt": true, ".xml": true,
	".yaml": true, ".yml": true,
}

func fileRoot(settings storage.ServiceSettings, rootType string) (string, error) {
	switch rootType {
	case "", "http":
		return settings.HTTPBoot.Root, nil
	case "tftp":
		return settings.TFTP.Root, nil
	case "netboot":
		return settings.NetbootXYZ.DownloadDir, nil
	default:
		return "", os.ErrInvalid
	}
}

func isEditableTextPath(path string) bool {
	return editableTextExts[strings.ToLower(filepath.Ext(path))]
}

func safeJoin(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(rootAbs, filepath.Clean(rel))
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(filepath.Separator)) {
		return "", os.ErrPermission
	}
	return targetAbs, nil
}

func sendWOL(macText string) error {
	hw, err := net.ParseMAC(strings.ReplaceAll(macText, "-", ":"))
	if err != nil {
		return err
	}
	packet := make([]byte, 6+16*len(hw))
	for i := 0; i < 6; i++ {
		packet[i] = 0xff
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*len(hw):], hw)
	}
	conn, err := net.Dial("udp4", "255.255.255.255:9")
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write(packet)
	return err
}

func staticHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		target := "dist/index.html"
		if path == "/" {
			target = "dist/index.html"
		} else {
			candidate := "dist" + path
			if _, err := fs.Stat(webFS, candidate); err == nil {
				target = candidate
			} else {
				target = "dist/index.html"
			}
		}
		data, err := webFS.ReadFile(target)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		ct := mime.TypeByExtension(filepath.Ext(target))
		if ct == "" {
			ct = "application/octet-stream"
		}
		c.Data(http.StatusOK, ct, data)
	}
}
