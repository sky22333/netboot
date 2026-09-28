package netboot

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pxe/internal/observability"
	"pxe/internal/storage"
)

type Result struct {
	File       string `json:"file"`
	URL        string `json:"url"`
	TargetPath string `json:"target_path"`
	SHA256     string `json:"sha256"`
	OK         bool   `json:"ok"`
	Existing   bool   `json:"existing"`
	Error      string `json:"error,omitempty"`
}

func Download(ctx context.Context, settings storage.NetbootXYZSettings, events *observability.Hub) []Result {
	if err := os.MkdirAll(settings.DownloadDir, 0755); err != nil {
		return []Result{{Error: err.Error()}}
	}
	root, err := os.OpenRoot(settings.DownloadDir)
	if err != nil {
		return []Result{{Error: err.Error()}}
	}
	defer root.Close()
	client := &http.Client{Timeout: 90 * time.Second}
	results := []Result{}
	for _, name := range settings.Files {
		name = filepath.Base(name)
		target := filepath.Join(settings.DownloadDir, name)
		source := strings.TrimRight(settings.BaseURL, "/") + "/" + name
		res := Result{File: name, URL: source, TargetPath: target}
		if info, err := root.Stat(name); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			if sum, err := sha256File(root, name); err == nil {
				res.SHA256 = sum
			}
			res.OK = true
			res.Existing = true
			events.Publish("info", "netboot.xyz", "文件已存在，跳过下载 "+name)
			results = append(results, res)
			continue
		}
		events.Publish("info", "netboot.xyz", "开始下载 "+res.URL+" -> "+target)
		resp, err := tryDownload(ctx, client, source)
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		res.URL = resp.Request.URL.String()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			res.Error = resp.Status
			_ = resp.Body.Close()
			results = append(results, res)
			continue
		}
		sum, err := saveDownload(root, name, resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		res.SHA256 = sum
		res.OK = true
		events.Publish("info", "netboot.xyz", "下载完成 "+name)
		results = append(results, res)
	}
	return results
}

func sha256File(root *os.Root, path string) (string, error) {
	f, err := root.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func tryDownload(ctx context.Context, client *http.Client, source string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

// Unique temporary names avoid races between concurrent downloads. The final
// file only becomes visible after the complete response has been written.
func saveDownload(root *os.Root, name string, body io.Reader) (string, error) {
	tmp := ".download-" + rand.Text()
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	defer func() { f.Close(); _ = root.Remove(tmp) }()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(body, (64<<20)+1))
	if err != nil {
		return "", err
	}
	if n == 0 || n > 64<<20 {
		return "", fmt.Errorf("固件必须为 1 字节至 64 MiB")
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = root.Rename(tmp, name); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
