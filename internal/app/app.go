package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"pxe/internal/config"
	"pxe/internal/dhcp"
	"pxe/internal/httpboot"
	"pxe/internal/observability"
	"pxe/internal/smb"
	"pxe/internal/storage"
	"pxe/internal/tftp"
	"pxe/internal/web"
)

type Status struct {
	AdminHTTP string            `json:"admin_http"`
	Services  map[string]string `json:"services"`
	StartedAt string            `json:"started_at"`
}
type serviceHandle struct {
	finished atomic.Bool
	cancel   context.CancelFunc
	done     chan struct{}
	err      error
}
type App struct {
	Boot      config.BootConfig
	Store     *storage.Store
	Events    *observability.Hub
	startedAt string
	mu        sync.Mutex // serializes the entire lifecycle, including SMB and rollback
	services  map[string]*serviceHandle
	activeSMB *storage.SMBSettings
	applySMB  func(context.Context, storage.SMBSettings, bool) error
	logger    *observability.RotatingLog
}

func New(ctx context.Context, boot config.BootConfig) (*App, error) {
	store, err := storage.Open(ctx, boot.Database.Path, boot.Data.Dir)
	if err != nil {
		return nil, err
	}
	logger, err := observability.OpenLog(filepath.Join(boot.Data.Dir, "logs", "pxe.log"))
	if err != nil {
		store.Close()
		return nil, err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(logger, &slog.HandlerOptions{Level: slog.LevelInfo})))
	return &App{Boot: boot, Store: store, Events: observability.NewHub(store), startedAt: time.Now().Format(time.RFC3339), services: map[string]*serviceHandle{}, applySMB: func(ctx context.Context, settings storage.SMBSettings, start bool) error {
		return smb.Apply(ctx, settings, start, boot.Data.Dir)
	}, logger: logger}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer a.logger.Close()
	defer a.Store.Close()
	listener, err := net.Listen("tcp", a.Boot.Admin.AdminAddr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: web.NewRouter(a), ReadHeaderTimeout: 10 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	fmt.Println("Web 面板: http://" + listener.Addr().String())
	a.Events.Publish("info", "web", "管理 Web 已启动: "+listener.Addr().String())
	select {
	case <-ctx.Done():
	case err = <-done:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	stopErr := a.StopServices(shutdown)
	httpErr := server.Shutdown(shutdown)
	if httpErr != nil {
		_ = server.Close()
	}
	return errors.Join(err, stopErr, httpErr)
}

func (a *App) Status() any {
	a.mu.Lock()
	defer a.mu.Unlock()
	states := map[string]string{"dhcp": "stopped", "proxy_dhcp_67": "stopped", "proxy_dhcp": "stopped", "tftp": "stopped", "httpboot": "stopped", "smb": "stopped"}
	for name, h := range a.services {
		if h.finished.Load() {
			if h.err != nil {
				states[name] = "failed"
			}
		} else {
			states[name] = "running"
		}
	}
	if a.activeSMB != nil {
		states["smb"] = "running"
	}
	return Status{AdminHTTP: a.Boot.Admin.AdminAddr, Services: states, StartedAt: a.startedAt}
}
func (a *App) Storage() *storage.Store       { return a.Store }
func (a *App) EventHub() *observability.Hub  { return a.Events }
func (a *App) BootConfig() config.BootConfig { return a.Boot }

func (a *App) StartServices(ctx context.Context) (err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	settings, err := a.Store.GetSettings(ctx)
	if err != nil {
		return err
	}
	if err = a.stopLocked(ctx); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = errors.Join(err, a.stopLocked(cleanup))
			a.Events.Publish("error", "services", err.Error())
		}
	}()
	for _, dir := range []string{settings.HTTPBoot.Root, settings.TFTP.Root} {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	if settings.HTTPBoot.Enabled {
		ln, e := new(net.ListenConfig).Listen(ctx, "tcp", settings.HTTPBoot.Addr)
		if e != nil {
			return fmt.Errorf("HTTP Boot 监听失败: %w", e)
		}
		server := &http.Server{Handler: httpboot.Handler(settings, a.Events), ReadHeaderTimeout: 10 * time.Second}
		a.start("httpboot", func(ctx context.Context) error {
			stop := context.AfterFunc(ctx, func() { _ = server.Close() })
			defer stop()
			e := server.Serve(ln)
			if errors.Is(e, http.ErrServerClosed) {
				return nil
			}
			return e
		})
	}
	if settings.TFTP.Enabled {
		conn, e := new(net.ListenConfig).ListenPacket(ctx, "udp4", net.JoinHostPort(settings.Server.ListenIP, "69"))
		if e != nil {
			return fmt.Errorf("TFTP 监听失败: %w", e)
		}
		a.start("tftp", func(ctx context.Context) error { return tftp.Serve(ctx, settings, a.Events, conn) })
	}
	if settings.DHCP.Enabled {
		if settings.DHCP.Mode == "dhcp" && settings.DHCP.DetectConflicts {
			servers, e := dhcp.DetectServers(ctx, settings.Server.ListenIP, 2*time.Second, settings.Server.AdvertiseIP)
			if e != nil {
				return fmt.Errorf("DHCP 冲突探测失败: %w", e)
			}
			if len(servers) > 0 {
				return fmt.Errorf("检测到已有 DHCP 服务: %v", servers)
			}
		}
		for _, port := range []string{"67", "4011"} {
			conn, e := dhcp.ListenPacket(ctx, "udp4", net.JoinHostPort(settings.Server.ListenIP, port))
			if e != nil {
				return fmt.Errorf("DHCP %s 监听失败: %w", port, e)
			}
			proxy := port == "4011" || settings.DHCP.Mode == "proxy"
			name := "dhcp"
			if port == "4011" {
				name = "proxy_dhcp"
			} else if proxy {
				name = "proxy_dhcp_67"
			}
			a.start(name, func(ctx context.Context) error { return dhcp.Serve(ctx, settings, a.Store, a.Events, conn, proxy) })
		}
	}
	if settings.SMB.Enabled {
		if err = a.applySMB(ctx, settings.SMB, true); err != nil {
			return err
		}
		copy := settings.SMB
		a.activeSMB = &copy
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	a.Events.Publish("info", "services", "启用的服务已完成监听")
	return nil
}

// Called only while holding mu; a service owns its listener and cancellation.
func (a *App) start(name string, run func(context.Context) error) {
	ctx, cancel := context.WithCancel(context.Background())
	h := &serviceHandle{cancel: cancel, done: make(chan struct{})}
	a.services[name] = h
	go func() {
		defer close(h.done)
		defer cancel()
		h.err = run(ctx)
		h.finished.Store(true)
		if h.err != nil {
			a.Events.Publish("error", name, h.err.Error())
		}
	}()
}
func (a *App) StopServices(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	err := a.stopLocked(ctx)
	if err == nil {
		a.Events.Publish("info", "services", "服务已停止")
	}
	return err
}
func (a *App) stopLocked(ctx context.Context) error {
	for _, h := range a.services {
		h.cancel()
	}
	var errs []error
	for name, h := range a.services {
		select {
		case <-h.done:
			delete(a.services, name)
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("停止 %s: %w", name, ctx.Err()))
		}
	}
	if a.activeSMB != nil {
		if err := a.applySMB(ctx, *a.activeSMB, false); err != nil {
			errs = append(errs, err)
		} else {
			a.activeSMB = nil
		}
	}
	return errors.Join(errs...)
}
