package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"pxe/internal/config"
	"pxe/internal/dhcp"
	"pxe/internal/httpboot"
	"pxe/internal/observability"
	"pxe/internal/smb"
	"pxe/internal/storage"
	"pxe/internal/tftp"
	"pxe/internal/torrent"
	"pxe/internal/web"
)

type Status struct {
	AdminHTTP string            `json:"admin_http"`
	Services  map[string]string `json:"services"`
	StartedAt string            `json:"started_at"`
}

type App struct {
	Boot      config.BootConfig
	Store     *storage.Store
	Events    *observability.Hub
	startedAt string

	mu       sync.Mutex
	services map[string]serviceHandle
	admin    *http.Server
}

type serviceHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func New(ctx context.Context, boot config.BootConfig) (*App, error) {
	setupLogger(boot)
	store, err := storage.Open(ctx, boot.Database.Path, boot.Data.Dir)
	if err != nil {
		return nil, err
	}
	app := &App{
		Boot:      boot,
		Store:     store,
		Events:    observability.NewHub(),
		startedAt: time.Now().Format(time.RFC3339),
		services:  map[string]serviceHandle{},
	}
	return app, nil
}

func setupLogger(boot config.BootConfig) {
	logPath := filepath.Join(boot.Data.Dir, "logs", "pxe.log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0755)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))
}

func (a *App) Run(ctx context.Context) error {
	router := web.NewRouter(a)
	a.admin = &http.Server{Addr: a.Boot.Admin.AdminAddr, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		a.Events.Publish("info", "web", "管理 Web 服务已启动: http://"+a.Boot.Admin.AdminAddr)
		if err := a.admin.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		a.StopServices(shutdownCtx)
		_ = a.admin.Shutdown(shutdownCtx)
		return a.Store.Close()
	case err := <-errCh:
		return err
	}
}

func (a *App) Status() any {
	a.mu.Lock()
	defer a.mu.Unlock()
	services := map[string]string{
		"dhcp": "stopped", "proxy_dhcp_67": "stopped", "proxy_dhcp": "stopped", "tftp": "stopped", "httpboot": "stopped", "torrent": "stopped",
	}
	for name, handle := range a.services {
		select {
		case <-handle.done:
			services[name] = "stopped"
		default:
			services[name] = "running"
		}
	}
	return Status{AdminHTTP: a.Boot.Admin.AdminAddr, Services: services, StartedAt: a.startedAt}
}

func (a *App) Storage() *storage.Store {
	return a.Store
}

func (a *App) EventHub() *observability.Hub {
	return a.Events
}

func (a *App) BootConfig() config.BootConfig {
	return a.Boot
}

func (a *App) StartServices(ctx context.Context) error {
	a.StopServices(ctx)
	settings, err := a.Store.GetSettings(ctx)
	if err != nil {
		return err
	}
	if settings.HTTPBoot.Enabled {
		a.start("httpboot", func(ctx context.Context) { httpboot.Run(ctx, settings, a.Store, a.Events) })
	}
	if settings.SMB.Enabled {
		if err := smb.Apply(settings.SMB, true); err != nil {
			a.Events.Publish("error", "smb", "SMB 共享启动失败: "+err.Error())
		} else {
			a.Events.Publish("info", "smb", "SMB 共享已启用")
		}
	}
	if settings.TFTP.Enabled {
		a.start("tftp", func(ctx context.Context) { tftp.Run(ctx, settings, a.Store, a.Events) })
	}
	if settings.Torrent.Enabled {
		a.start("torrent", func(ctx context.Context) { torrent.RunTracker(ctx, settings.Torrent.Addr, a.Events) })
	}
	if settings.DHCP.Enabled {
		if settings.DHCP.Mode == "dhcp" && settings.DHCP.DetectConflicts {
			if servers, err := dhcp.DetectServers(ctx, settings.Server.ListenIP, 2*time.Second, settings.Server.AdvertiseIP); err == nil && len(servers) > 0 {
				a.Events.Publish("warning", "dhcp", "检测到局域网内已有 DHCP 服务，完整 DHCP 模式可能发生冲突")
			}
		}
		if settings.DHCP.Mode == "dhcp" {
			a.start("dhcp", func(ctx context.Context) { dhcp.RunDHCP(ctx, settings, a.Store, a.Events) })
		} else {
			a.start("proxy_dhcp_67", func(ctx context.Context) { dhcp.RunProxyDiscover(ctx, settings, a.Store, a.Events) })
		}
		a.start("proxy_dhcp", func(ctx context.Context) { dhcp.RunProxy(ctx, settings, a.Store, a.Events) })
	}
	a.Events.Publish("info", "services", "已启动启用的 PXE 服务")
	return nil
}

func (a *App) start(name string, run func(context.Context)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	a.services[name] = serviceHandle{cancel: cancel, done: done}
	go func() {
		defer close(done)
		run(ctx)
	}()
}

func (a *App) StopServices(ctx context.Context) {
	a.mu.Lock()
	handles := a.services
	a.services = map[string]serviceHandle{}
	a.mu.Unlock()
	for _, handle := range handles {
		handle.cancel()
	}
	settings, err := a.Store.GetSettings(ctx)
	if err == nil && settings.SMB.Enabled {
		if err := smb.Apply(settings.SMB, false); err != nil {
			a.Events.Publish("warning", "smb", "SMB 共享停止失败: "+err.Error())
		}
	}
	for name, handle := range handles {
		select {
		case <-handle.done:
		case <-ctx.Done():
			a.Events.Publish("warning", "services", "服务停止超时: "+name)
		case <-time.After(2 * time.Second):
			a.Events.Publish("warning", "services", "服务停止等待超时: "+name)
		}
	}
	if len(handles) > 0 {
		a.Events.Publish("info", "services", "所有服务已停止")
	}
}
