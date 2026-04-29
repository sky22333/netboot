package httpboot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pxe/internal/ipxe"
	"pxe/internal/observability"
	"pxe/internal/storage"
)

func Run(ctx context.Context, settings storage.ServiceSettings, store *storage.Store, events *observability.Hub) {
	mux := http.NewServeMux()
	mux.HandleFunc("/client/report", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var report struct {
			IP         string `json:"ip"`
			DiskHealth string `json:"disk_health"`
			NetSpeed   string `json:"net_speed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if report.IP == "" {
			report.IP = clientIP(r)
		}
		_ = store.UpdateClientHealth(r.Context(), report.IP, report.DiskHealth, report.NetSpeed)
		events.Publish("info", "clients", "收到客户端健康报告: "+report.IP)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/dynamic.ipxe", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			return
		}
		params, _ := url.ParseQuery(r.URL.RawQuery)
		gen := ipxe.Generator{Settings: settings, Store: store}
		script := gen.Generate(r.Context(), ipxe.Request{Params: params, ClientIP: clientIP(r)})
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(script))
	})
	mux.Handle("/", fileHandler(settings, store, events))
	server := &http.Server{Addr: settings.HTTPBoot.Addr, Handler: loggingHandler(mux, events), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	events.Publish("info", "httpboot", "HTTP Boot 已启动: "+settings.HTTPBoot.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		events.Publish("error", "httpboot", "HTTP Boot 启动失败: "+err.Error())
		slog.Error("httpboot failed", "error", err)
	}
	events.Publish("info", "httpboot", "HTTP Boot 已停止")
}

func fileHandler(settings storage.ServiceSettings, store *storage.Store, events *observability.Hub) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		root, target, err := resolveReadPath(settings, strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			events.Publish("error", "httpboot", fmt.Sprintf("HTTP 文件路径非法: %s client=%s error=%s", r.URL.Path, clientIP(r), err.Error()))
			http.Error(w, "非法路径", http.StatusForbidden)
			return
		}
		rel, _ := filepath.Rel(root, target)
		rel = filepath.ToSlash(rel)
		info, err := os.Stat(target)
		if err != nil {
			events.Publish("error", "httpboot", fmt.Sprintf("HTTP 文件不存在: %s -> %s client=%s", r.URL.Path, target, clientIP(r)))
			http.NotFound(w, r)
			return
		}
		if info.IsDir() {
			events.Publish("info", "httpboot", fmt.Sprintf("HTTP 目录请求: %s -> %s client=%s", r.URL.Path, target, clientIP(r)))
			if !settings.HTTPBoot.DirectoryListing {
				http.Error(w, "目录浏览已关闭", http.StatusForbidden)
				return
			}
			serveDirectory(w, r, target, r.URL.Path)
			return
		}
		f, err := os.Open(target)
		if err != nil {
			events.Publish("error", "httpboot", fmt.Sprintf("HTTP 文件不可读: %s -> %s client=%s error=%s", r.URL.Path, target, clientIP(r), err.Error()))
			http.Error(w, "文件不可读", http.StatusForbidden)
			return
		}
		defer f.Close()
		etag := fmt.Sprintf(`W/"%x-%x"`, info.ModTime().Unix(), info.Size())
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !settings.HTTPBoot.RangeRequests {
			w.Header().Set("Accept-Ranges", "none")
			r.Header.Del("Range")
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		http.ServeContent(rec, r, info.Name(), info.ModTime(), f)
		if r.Method == http.MethodGet && rec.status < 400 {
			_ = store.AddEvent(r.Context(), "info", "httpboot", "客户端请求 HTTP 文件", map[string]any{"path": rel, "size": info.Size(), "client": clientIP(r), "status": rec.status})
			events.Publish("info", "httpboot", fmt.Sprintf("HTTP 文件已发送: %s size=%d client=%s", rel, info.Size(), clientIP(r)))
		}
	})
}

func resolveReadPath(settings storage.ServiceSettings, requestPath string) (string, string, error) {
	clean := strings.TrimLeft(strings.ReplaceAll(requestPath, "\\", "/"), "/")
	if rel, ok := strings.CutPrefix(clean, "netboot/"); ok {
		root, _ := filepath.Abs(settings.NetbootXYZ.DownloadDir)
		target, err := safeJoin(root, rel)
		return root, target, err
	}
	root, _ := filepath.Abs(settings.HTTPBoot.Root)
	target, err := safeJoin(root, clean)
	return root, target, err
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func serveDirectory(w http.ResponseWriter, r *http.Request, dir, requestPath string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "目录不可读", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><meta charset=\"utf-8\"><title>PXE 文件目录</title><body><h1>PXE 文件目录</h1><ul>")
	if requestPath != "/" {
		_, _ = io.WriteString(w, `<li><a href="../">../</a></li>`)
	}
	for _, entry := range entries {
		name := entry.Name()
		href := url.PathEscape(name)
		if entry.IsDir() {
			href += "/"
			name += "/"
		}
		_, _ = fmt.Fprintf(w, `<li><a href="%s">%s</a></li>`, href, name)
	}
	_, _ = io.WriteString(w, "</ul></body>")
}

func safeJoin(root, request string) (string, error) {
	clean := filepath.Clean(strings.ReplaceAll(request, "/", string(filepath.Separator)))
	if clean == "." {
		clean = ""
	}
	target := filepath.Join(root, clean)
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root")
	}
	return abs, nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func loggingHandler(next http.Handler, events *observability.Hub) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/dynamic.ipxe") {
			events.Publish("info", "httpboot", "生成动态 iPXE 脚本: "+r.URL.RawQuery)
		}
		next.ServeHTTP(w, r)
	})
}
