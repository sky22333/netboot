package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"pxe/internal/app"
	"pxe/internal/config"
)

func main() {
	configPath := flag.String("config", "", "pxe.toml path")
	dataDir := flag.String("data-dir", "", "data directory override")
	host := flag.String("host", "", "admin host override")
	port := flag.String("port", "", "admin port override")
	noBrowser := flag.Bool("no-browser", false, "do not open browser automatically")
	flag.Parse()

	if *configPath == "" && *dataDir == "" {
		if exe, err := os.Executable(); err == nil {
			_ = os.Chdir(filepathDir(exe))
		}
	}

	boot, err := config.LoadOrCreate(*configPath, *dataDir, *host, *port)
	if err != nil {
		slog.Error("启动配置加载失败", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server, err := app.New(ctx, boot)
	if err != nil {
		slog.Error("应用初始化失败", "error", err)
		os.Exit(1)
	}
	if !*noBrowser {
		go func() {
			time.Sleep(700 * time.Millisecond)
			openBrowser("http://" + browserAddr(boot.Admin.AdminAddr))
		}()
	}
	if err := server.Run(ctx); err != nil {
		slog.Error("应用运行失败", "error", err)
		os.Exit(1)
	}
}

func filepathDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '\\' || path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

func browserAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func openBrowser(rawURL string) {
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	_ = cmd.Start()
}
