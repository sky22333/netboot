package web

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"pxe/internal/filetree"
)

func (h *Handler) uploadFile(c *gin.Context) {
	limit := h.app.BootConfig().Admin.MaxUploadBytes
	if c.ContentType() != "application/octet-stream" {
		Fail(c, 415, "UPLOAD_TYPE", "上传需要 application/octet-stream 文件流")
		return
	}
	if c.Request.ContentLength < 0 {
		Fail(c, 411, "UPLOAD_LENGTH", "上传必须提供文件长度")
		return
	}
	if c.Request.ContentLength > limit {
		Fail(c, 413, "UPLOAD_TOO_LARGE", fmt.Sprintf("单文件上限为 %d 字节", limit))
		return
	}
	select {
	case h.uploadSlots <- struct{}{}:
		defer func() { <-h.uploadSlots }()
	default:
		Fail(c, 429, "UPLOAD_BUSY", "最多同时上传两个文件，请稍后重试")
		return
	}
	settings, err := h.app.Storage().GetSettings(c.Request.Context())
	if err != nil {
		Fail(c, 500, "CONFIG_READ_FAILED", "读取配置失败")
		return
	}
	root, err := fileRoot(settings, c.DefaultQuery("root", "http"))
	if err != nil {
		Fail(c, 400, "ROOT_INVALID", "文件目录不可用")
		return
	}
	defer root.Close()
	path, err := filetree.Path(c.Query("path"))
	if err != nil || path == "." {
		Fail(c, 400, "PATH_INVALID", "目标文件路径无效")
		return
	}
	dir, err := root.OpenRoot(filepath.Dir(path))
	if err != nil {
		Fail(c, 400, "PATH_INVALID", "目标目录不可用")
		return
	}
	defer dir.Close()
	name := filepath.Base(path)
	if _, err := dir.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		Fail(c, 409, "FILE_EXISTS", "目标已存在或不可访问")
		return
	}
	// Bound slow uploads without buffering the request. The browser sends the File directly.
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(time.Now().Add(6 * time.Hour))
	defer controller.SetReadDeadline(time.Time{})
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	if err := saveUpload(c.Request.Context(), dir, name, c.Request.Body, c.Request.ContentLength); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, os.ErrExist) {
			status = http.StatusConflict
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			status = http.StatusBadRequest
		}
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		Fail(c, status, "UPLOAD_FAILED", "上传未完成："+err.Error())
		return
	}
	_ = h.app.Storage().AddEvent(c.Request.Context(), "info", "files", "上传文件", gin.H{"path": path})
	OK(c, gin.H{"path": path})
}

func saveUpload(ctx context.Context, root *os.Root, name string, src io.Reader, size int64) error {
	cleanupUploads(root)
	temp := filetree.UploadTempPrefix + rand.Text()
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	defer f.Close()
	n, err := io.Copy(f, src)
	if err != nil {
		return err
	}
	if n != size {
		return io.ErrUnexpectedEOF
	}
	if err := f.Chmod(0644); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Link publishes a complete file atomically and never replaces an existing target.
	// Both names are in the same directory/filesystem. Unsupported filesystems fail safely.
	if err := ctx.Err(); err != nil {
		return err
	}
	return root.Link(temp, name)
}

// Aborted requests are removed immediately. Crash leftovers are reclaimed on the
// next upload to that directory; 24h exceeds the maximum active upload lifetime.
func cleanupUploads(root *os.Root) {
	dir, err := root.Open(".")
	if err != nil {
		return
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !filetree.IsUploadTemp(entry.Name()) || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			_ = root.Remove(entry.Name())
		}
	}
}
