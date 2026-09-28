package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"pxe/internal/config"
	"pxe/internal/dhcp"
	"pxe/internal/filetree"
	"pxe/internal/netutil"
	"pxe/internal/observability"
	"pxe/internal/platform"
	"pxe/internal/storage"
)

//go:embed dist/*
var webFS embed.FS

type Backend interface {
	Status() any
	StartServices(context.Context) error
	StopServices(context.Context) error
	Storage() *storage.Store
	EventHub() *observability.Hub
	BootConfig() config.BootConfig
}

type Handler struct {
	firmwareSlots chan struct{}
	uploadSlots   chan struct{}
	authSlots     chan struct{}
	app           Backend
	loginLimiter  *LoginLimiter
}

func NewRouter(app Backend) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(gin.Recovery(), bodyLimit(128<<20))
	h := &Handler{app: app, firmwareSlots: make(chan struct{}, 1), uploadSlots: make(chan struct{}, 2), authSlots: make(chan struct{}, 4), loginLimiter: NewLoginLimiter()}

	api := r.Group("/api/v1")
	api.GET("/setup/status", h.setupStatus)
	api.POST("/setup", h.limitAuthWork, h.setup)
	api.POST("/auth/login", h.limitAuthWork, h.login)
	api.POST("/auth/logout", h.logout)

	protected := api.Group("")
	protected.Use(h.requireAuth)
	protected.GET("/status", h.status)
	protected.GET("/diagnostics", h.diagnostics)
	protected.GET("/diagnostics/dhcp", h.dhcpDiagnostics)
	protected.GET("/config", h.getConfig)
	protected.PUT("/config", h.saveConfig)
	protected.POST("/services/start", h.startServices)
	protected.POST("/services/stop", h.stopServices)
	protected.GET("/clients", h.listClients)
	protected.POST("/clients", h.saveClient)
	protected.POST("/clients/batch", h.batchClients)
	protected.PUT("/clients/:id", h.saveClient)
	protected.DELETE("/clients/:id", h.deleteClient)
	protected.POST("/clients/:id/wol", h.wol)
	protected.POST("/clients/:id/clear-mac", h.clearClientMAC)
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
	protected.GET("/logs", h.logs)
	protected.GET("/events/stream", h.eventStream)
	protected.GET("/firmware", h.firmwareCatalog)
	protected.POST("/firmware/download", h.firmwareDownload)

	r.NoRoute(staticHandler())
	return r
}

func bodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost || c.Request.URL.Path != "/api/v1/files/upload" {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

func (h *Handler) setupStatus(c *gin.Context) {
	has, err := h.hasUsers(c.Request.Context())
	if err != nil {
		Fail(c, 500, "SETUP_STATUS_FAILED", "读取初始化状态失败")
		return
	}
	OK(c, gin.H{"has_user": has})
}

func (h *Handler) setup(c *gin.Context) {
	has, err := h.hasUsers(c.Request.Context())
	if err != nil {
		Fail(c, 500, "SETUP_STATUS_FAILED", "读取初始化状态失败")
		return
	}
	if has {
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
	if err := h.createUser(c.Request.Context(), req.Username, req.Password, true); err != nil {
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
	key := c.ClientIP()
	if !h.loginLimiter.Allow(key) {
		Fail(c, http.StatusTooManyRequests, "LOGIN_RATE_LIMITED", "登录尝试过多，请 10 分钟后再试")
		return
	}
	token, err := h.loginSession(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		h.loginLimiter.Fail(key)
		Fail(c, 401, "LOGIN_FAILED", "用户名或密码错误")
		return
	}
	h.loginLimiter.Success(key)
	http.SetCookie(c.Writer, &http.Cookie{Name: "pxe_session", Value: token, Path: "/", MaxAge: 86400, HttpOnly: true, Secure: c.Request.TLS != nil, SameSite: http.SameSiteLaxMode})
	OK(c, gin.H{"username": req.Username})
}

func (h *Handler) logout(c *gin.Context) {
	token, _ := c.Cookie("pxe_session")
	if _, err := h.app.Storage().RawDB().ExecContext(c.Request.Context(), `DELETE FROM sessions WHERE token_hash=?`, sessionHash(token)); err != nil {
		Fail(c, 500, "LOGOUT_FAILED", "退出失败")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: "pxe_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: c.Request.TLS != nil, SameSite: http.SameSiteLaxMode})
	OK(c, gin.H{"message": "已退出"})
}

func (h *Handler) status(c *gin.Context) {
	OK(c, h.app.Status())
}

func (h *Handler) diagnostics(c *gin.Context) {
	boot := h.app.BootConfig()
	permission := platform.Permission()
	OK(c, gin.H{
		"data_dir":      boot.Data.Dir,
		"db":            boot.Database.Path,
		"admin_addr":    boot.Admin.AdminAddr,
		"smb_supported": runtime.GOOS == "windows",
		"is_admin":      permission.AdminLike,
		"permission":    permission,
		"interfaces":    platform.Interfaces(),
		"suggestions":   []string{"若客户端无法获取启动文件，请确认程序已被系统防火墙放行，并且监听 IP、通告 IP 与客户端处于可达网络。"},
	})
}

func (h *Handler) dhcpDiagnostics(c *gin.Context) {
	settings, settingsErr := h.app.Storage().GetSettings(c.Request.Context())
	if settingsErr != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", "读取配置失败")
		return
	}
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
	if err := h.app.Storage().SaveSettings(c.Request.Context(), settings); err != nil {
		Fail(c, 400, "CONFIG_SAVE_FAILED", err.Error())
		return
	}
	saved, err := h.app.Storage().GetSettings(c.Request.Context())
	if err != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", err.Error())
		return
	}
	h.app.EventHub().Publish("info", "config", "服务配置已保存")

	OK(c, saved)
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
	if err := h.app.StopServices(ctx); err != nil {
		Fail(c, 500, "SERVICE_STOP_FAILED", err.Error())
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
	client.ID = 0
	if id := c.Param("id"); id != "" {
		var valid bool
		client.ID, valid = resourceID(c)
		if !valid {
			return
		}
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
	id, valid := resourceID(c)
	if !valid {
		return
	}
	if err := h.app.Storage().DeleteClient(c.Request.Context(), id); err != nil {
		Fail(c, 500, "CLIENT_DELETE_FAILED", err.Error())
		return
	}
	h.app.EventHub().Publish("warning", "clients", fmt.Sprintf("删除设备: %d", id))
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
	h.app.EventHub().Publish("info", "clients", fmt.Sprintf("批量添加设备: %d 台", len(out)))
	OK(c, out)
}

func (h *Handler) clearClientMAC(c *gin.Context) {
	id, valid := resourceID(c)
	if !valid {
		return
	}
	if err := h.app.Storage().ClearClientMAC(c.Request.Context(), id); err != nil {
		Fail(c, 500, "CLIENT_CLEAR_MAC_FAILED", err.Error())
		return
	}
	h.app.EventHub().Publish("warning", "clients", fmt.Sprintf("清除 MAC 绑定: %d", id))
	OK(c, gin.H{"id": id})
}

func (h *Handler) wol(c *gin.Context) {
	id, valid := resourceID(c)
	if !valid {
		return
	}
	client, err := h.app.Storage().GetClient(c.Request.Context(), id)
	if err != nil {
		Fail(c, 404, "CLIENT_NOT_FOUND", "客户端不存在")
		return
	}
	settings, settingsErr := h.app.Storage().GetSettings(c.Request.Context())
	if settingsErr != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", "读取配置失败")
		return
	}
	result, err := sendWOL(client.MAC, wolTargets(client, settings))
	if err != nil {
		Fail(c, 400, "WOL_FAILED", err.Error())
		return
	}
	h.app.EventHub().Publish("info", "clients", fmt.Sprintf("已发送 WOL 唤醒包: %s targets=%d", client.MAC, result.Sent))
	OK(c, gin.H{"message": "已发送唤醒包", "sent": result.Sent, "targets": result.Targets})
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
	if req.Role != "" && req.Role != "admin" {
		Fail(c, 400, "USER_INVALID", "不支持的用户角色")
		return
	}
	if err := h.createUser(c.Request.Context(), req.Username, req.Password, false); err != nil {
		Fail(c, 500, "USER_CREATE_FAILED", err.Error())
		return
	}
	OK(c, gin.H{"message": "用户已创建"})
}

func (h *Handler) changeUserPassword(c *gin.Context) {
	id, valid := resourceID(c)
	if !valid {
		return
	}
	var req struct{ Password string }
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Password) < 8 {
		Fail(c, 400, "PASSWORD_INVALID", "密码至少 8 位")
		return
	}
	if err := h.changePassword(c.Request.Context(), id, req.Password); err != nil {
		Fail(c, 500, "PASSWORD_CHANGE_FAILED", err.Error())
		return
	}
	token, _ := c.Cookie("pxe_session")
	OK(c, gin.H{"reauthenticate": !h.sessionValid(c.Request.Context(), token)})
}

func (h *Handler) deleteUser(c *gin.Context) {
	id, valid := resourceID(c)
	if !valid {
		return
	}
	if err := h.app.Storage().DeleteUser(c.Request.Context(), id); err != nil {
		Fail(c, 400, "USER_DELETE_FAILED", err.Error())
		return
	}
	OK(c, gin.H{"deleted": id})
}

func (h *Handler) listFiles(c *gin.Context) {
	settings, settingsErr := h.app.Storage().GetSettings(c.Request.Context())
	if settingsErr != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", "读取配置失败")
		return
	}
	rootType := c.DefaultQuery("root", "http")
	root, err := fileRoot(settings, rootType)
	if err != nil {
		Fail(c, 400, "ROOT_INVALID", "文件目录类型无效")
		return
	}
	defer root.Close()
	rel := c.DefaultQuery("path", ".")
	target, err := filetree.Path(rel)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "路径无效")
		return
	}
	dir, err := root.Open(target)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "目录不可读")
		return
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		Fail(c, 500, "FILE_LIST_FAILED", err.Error())
		return
	}
	files := []gin.H{}
	for _, entry := range entries {
		if filetree.IsUploadTemp(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, gin.H{"name": entry.Name(), "dir": entry.IsDir(), "size": info.Size(), "mod_time": info.ModTime(), "editable": !entry.IsDir() && isEditableTextPath(entry.Name()) && info.Size() <= maxEditableFileBytes})
	}
	OK(c, gin.H{"root": rootType, "path": rel, "base_path": root.Name(), "max_upload_bytes": h.app.BootConfig().Admin.MaxUploadBytes, "files": files})
}

func (h *Handler) mkdirFile(c *gin.Context) {
	settings, settingsErr := h.app.Storage().GetSettings(c.Request.Context())
	if settingsErr != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", "读取配置失败")
		return
	}
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
	defer root.Close()
	target, err := filetree.Path(req.Path)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "路径无效")
		return
	}
	if err := root.MkdirAll(target, 0755); err != nil {
		Fail(c, 500, "MKDIR_FAILED", err.Error())
		return
	}
	OK(c, gin.H{"path": req.Path})
}

func (h *Handler) renameFile(c *gin.Context) {
	settings, settingsErr := h.app.Storage().GetSettings(c.Request.Context())
	if settingsErr != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", "读取配置失败")
		return
	}
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
	defer root.Close()
	from, err := filetree.Path(req.From)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "源路径无效")
		return
	}
	to, err := filetree.Path(req.To)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "目标路径无效")
		return
	}
	if err := filetree.Rename(root, from, to); err != nil {
		if errors.Is(err, os.ErrExist) {
			Fail(c, 409, "FILE_EXISTS", "目标名称已存在")
			return
		}
		Fail(c, 500, "RENAME_FAILED", err.Error())
		return
	}
	h.app.EventHub().Publish("warning", "files", fmt.Sprintf("重命名文件: %s → %s", req.From, req.To))
	OK(c, gin.H{"from": req.From, "to": req.To})
}

func (h *Handler) deleteFile(c *gin.Context) {
	settings, settingsErr := h.app.Storage().GetSettings(c.Request.Context())
	if settingsErr != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", "读取配置失败")
		return
	}
	root, err := fileRoot(settings, c.DefaultQuery("root", "http"))
	if err != nil {
		Fail(c, 400, "ROOT_INVALID", "文件目录类型无效")
		return
	}
	defer root.Close()
	target, err := filetree.Path(c.Query("path"))
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "路径无效")
		return
	}
	if err := filetree.Remove(root, target); err != nil {
		Fail(c, 500, "FILE_DELETE_FAILED", err.Error())
		return
	}
	h.app.EventHub().Publish("warning", "files", "删除文件: "+c.Query("path"))
	OK(c, gin.H{"deleted": c.Query("path")})
}

func (h *Handler) getFileContent(c *gin.Context) {
	settings, settingsErr := h.app.Storage().GetSettings(c.Request.Context())
	if settingsErr != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", "读取配置失败")
		return
	}
	rootType := c.DefaultQuery("root", "http")
	root, err := fileRoot(settings, rootType)
	if err != nil {
		Fail(c, 400, "ROOT_INVALID", "文件目录类型无效")
		return
	}
	defer root.Close()
	rel := c.Query("path")
	target, err := filetree.Path(rel)
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "路径无效")
		return
	}
	info, err := root.Stat(target)
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
	f, err := root.Open(target)
	if err != nil {
		Fail(c, 400, "FILE_READ_FAILED", "文件不可读")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxEditableFileBytes+1))
	if len(data) > maxEditableFileBytes {
		Fail(c, 400, "FILE_TOO_LARGE", "文件超过在线编辑大小限制")
		return
	}
	if err != nil {
		Fail(c, 500, "FILE_READ_FAILED", err.Error())
		return
	}
	if !utf8.Valid(data) {
		Fail(c, 400, "FILE_NOT_TEXT", "文件不是 UTF-8 文本")
		return
	}
	OK(c, gin.H{"root": rootType, "path": rel, "content": string(data), "revision": filetree.Revision(data), "size": info.Size(), "mod_time": info.ModTime()})
}

func (h *Handler) saveFileContent(c *gin.Context) {
	settings, settingsErr := h.app.Storage().GetSettings(c.Request.Context())
	if settingsErr != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", "读取配置失败")
		return
	}
	var req struct {
		Root     string `json:"root"`
		Path     string `json:"path"`
		Content  string `json:"content"`
		Revision string `json:"revision"`
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
	defer root.Close()
	target, err := filetree.Path(req.Path)
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
	if info, err := root.Stat(target); err == nil && info.IsDir() {
		Fail(c, 400, "FILE_IS_DIRECTORY", "目录不能在线编辑")
		return
	}
	if err := filetree.WriteText(c.Request.Context(), root, target, req.Content, req.Revision); err != nil {
		if errors.Is(err, filetree.ErrConflict) {
			Fail(c, 409, "FILE_CONFLICT", err.Error())
			return
		}
		Fail(c, 500, "FILE_WRITE_FAILED", err.Error())
		return
	}
	h.app.EventHub().Publish("warning", "files", fmt.Sprintf("保存文本文件: %s/%s", req.Root, req.Path))
	OK(c, gin.H{"path": req.Path, "size": len(req.Content), "revision": filetree.Revision([]byte(req.Content))})
}

func (h *Handler) logs(c *gin.Context) {
	limit := 500
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}
	events, err := h.app.Storage().RecentEvents(c.Request.Context(), limit)
	if err != nil {
		Fail(c, 500, "LOG_READ_FAILED", "日志读取失败")
		return
	}
	OK(c, events)
}

func (h *Handler) eventStream(c *gin.Context) {
	token, _ := c.Cookie("pxe_session")
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
			if !h.sessionValid(c.Request.Context(), token) {
				return false
			}
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})
}

const maxEditableFileBytes = 1 << 20

var editableTextExts = map[string]bool{
	".bat": true, ".cfg": true, ".cmd": true, ".conf": true, ".ini": true,
	".ipxe": true, ".json": true, ".ks": true, ".md": true, ".ps1": true,
	".seed": true, ".sh": true, ".toml": true, ".txt": true, ".xml": true,
	".yaml": true, ".yml": true,
}

func fileRoot(settings storage.ServiceSettings, rootType string) (*os.Root, error) {
	var dir string
	switch rootType {
	case "", "http":
		dir = settings.HTTPBoot.Root
	case "tftp":
		dir = settings.TFTP.Root
	default:
		return nil, os.ErrInvalid
	}
	return os.OpenRoot(dir)
}

func isEditableTextPath(path string) bool {
	return editableTextExts[strings.ToLower(filepath.Ext(path))]
}

type wolResult struct {
	Sent    int      `json:"sent"`
	Targets []string `json:"targets"`
}

func sendWOL(macText string, targets []string) (wolResult, error) {
	hw, err := net.ParseMAC(strings.ReplaceAll(macText, "-", ":"))
	if err != nil {
		return wolResult{}, err
	}
	packet := make([]byte, 6+16*len(hw))
	for i := 0; i < 6; i++ {
		packet[i] = 0xff
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*len(hw):], hw)
	}
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return wolResult{}, err
	}
	defer conn.Close()
	result := wolResult{Targets: []string{}}
	var lastErr error
	for _, target := range targets {
		addr, err := net.ResolveUDPAddr("udp4", target)
		if err != nil {
			lastErr = err
			continue
		}
		if _, err := conn.WriteTo(packet, addr); err != nil {
			lastErr = err
			continue
		}
		result.Sent++
		result.Targets = append(result.Targets, target)
	}
	if result.Sent == 0 && lastErr != nil {
		return result, lastErr
	}
	return result, nil
}

func wolTargets(client storage.Client, settings storage.ServiceSettings) []string {
	if client.IP == "" {
		client.IP = client.ObservedIP
	}
	hosts := []string{"255.255.255.255"}
	if ip := net.ParseIP(client.IP).To4(); ip != nil {
		hosts = append(hosts, ip.String())
		if broadcast := netutil.DirectedBroadcast(ip.String(), settings.DHCP.SubnetMask); broadcast != "" {
			hosts = append(hosts, broadcast)
		}
	}
	if broadcast := netutil.DirectedBroadcast(settings.Server.AdvertiseIP, settings.DHCP.SubnetMask); broadcast != "" {
		hosts = append(hosts, broadcast)
	}
	hosts = append(hosts, netutil.InterfaceBroadcasts()...)
	seen := map[string]bool{}
	targets := []string{}
	for _, host := range hosts {
		if net.ParseIP(host).To4() == nil {
			continue
		}
		for _, port := range []string{"9", "7"} {
			target := net.JoinHostPort(host, port)
			if !seen[target] {
				seen[target] = true
				targets = append(targets, target)
			}
		}
	}
	return targets
}

func staticHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}
		pages := map[string]bool{"/": true, "/config": true, "/clients": true, "/files": true, "/netboot": true, "/users": true, "/logs": true, "/diagnostics": true}
		if !pages[path] && !strings.HasPrefix(path, "/assets/") {
			c.Status(http.StatusNotFound)
			return
		}
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

func resourceID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		Fail(c, 400, "ID_INVALID", "ID 必须是正整数")
		return 0, false
	}
	return id, true
}
