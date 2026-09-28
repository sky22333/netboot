package httpboot

import (
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"pxe/internal/filetree"
	"strings"
	"time"

	"pxe/internal/observability"
	"pxe/internal/storage"
)

func Handler(settings storage.ServiceSettings, events *observability.Hub) http.Handler {
	return fileHandler(settings, events)
}

func fileHandler(settings storage.ServiceSettings, events *observability.Hub) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		target, err := filetree.Path(strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			events.Publish("error", "httpboot", fmt.Sprintf("HTTP 文件路径非法: %s client=%s error=%s", r.URL.Path, clientIP(r), err.Error()))
			http.Error(w, "非法路径", http.StatusForbidden)
			return
		}
		rel := filepath.ToSlash(target)
		tree, err := os.OpenRoot(settings.HTTPBoot.Root)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer tree.Close()
		f, err := tree.Open(target)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		info, err := f.Stat()
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
			if !strings.HasSuffix(r.URL.Path, "/") {
				http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)
				return
			}
			serveDirectory(w, r, f, r.URL.Path)
			return
		}
		etag := fmt.Sprintf(`W/"%x-%x"`, info.ModTime().UnixNano(), info.Size())
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !settings.HTTPBoot.RangeRequests {
			w.Header().Set("Accept-Ranges", "none")
			r.Header.Del("Range")
			w = noRangeResponseWriter{ResponseWriter: w}
		}
		started := time.Now()
		rangeHeader := r.Header.Get("Range")
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		http.ServeContent(rec, r, info.Name(), info.ModTime(), f)
		if rec.status < 400 {

			events.Publish("info", "httpboot", httpFileSentMessage(rel, r.Method, rec.status, rangeHeader, rec.written, info.Size(), time.Since(started), clientIP(r)))
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status  int
	written int64
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.written += int64(n)
	return n, err
}

func httpFileSentMessage(path, method string, status int, rangeHeader string, sent, total int64, duration time.Duration, client string) string {
	parts := []string{
		fmt.Sprintf("HTTP 文件已响应: %s", path),
		"method=" + method,
		fmt.Sprintf("status=%d", status),
		fmt.Sprintf("sent=%d", sent),
		fmt.Sprintf("total=%d", total),
		"duration=" + duration.Round(time.Millisecond).String(),
		"client=" + client,
	}
	if rangeHeader != "" {
		parts = append(parts[:3], append([]string{"range=" + rangeHeader}, parts[3:]...)...)
	}
	return strings.Join(parts, " ")
}

type noRangeResponseWriter struct {
	http.ResponseWriter
}

func (w noRangeResponseWriter) WriteHeader(code int) {
	w.Header().Set("Accept-Ranges", "none")
	w.ResponseWriter.WriteHeader(code)
}

func serveDirectory(w http.ResponseWriter, r *http.Request, dir *os.File, requestPath string) {
	entries, err := dir.ReadDir(-1)
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
		if filetree.IsUploadTemp(entry.Name()) {
			continue
		}
		name := entry.Name()
		href := url.PathEscape(name)
		if entry.IsDir() {
			href += "/"
			name += "/"
		}
		_, _ = fmt.Fprintf(w, `<li><a href="%s">%s</a></li>`, href, html.EscapeString(name))
	}
	_, _ = io.WriteString(w, "</ul></body>")
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
