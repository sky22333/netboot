package netboot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	_ = os.MkdirAll(settings.DownloadDir, 0755)
	client := &http.Client{Timeout: 90 * time.Second}
	results := []Result{}
	for _, name := range settings.Files {
		name = filepath.Base(name)
		target := filepath.Join(settings.DownloadDir, name)
		urls := netbootURLs(settings.BaseURL, name)
		res := Result{File: name, URL: urls[0], TargetPath: target}
		if info, err := os.Stat(target); err == nil && info.Size() > 0 {
			if sum, err := sha256File(target); err == nil {
				res.SHA256 = sum
			}
			res.OK = true
			res.Existing = true
			events.Publish("info", "netboot.xyz", "文件已存在，跳过下载 "+name)
			results = append(results, res)
			continue
		}
		events.Publish("info", "netboot.xyz", "开始下载 "+name)
		resp, err := tryDownload(ctx, client, urls)
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
		tmp := target + ".tmp"
		f, err := os.Create(tmp)
		if err != nil {
			res.Error = err.Error()
			_ = resp.Body.Close()
			results = append(results, res)
			continue
		}
		hash := sha256.New()
		_, err = io.Copy(io.MultiWriter(f, hash), resp.Body)
		_ = resp.Body.Close()
		_ = f.Close()
		if err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		if err := os.Rename(tmp, target); err != nil {
			res.Error = err.Error()
			results = append(results, res)
			continue
		}
		res.SHA256 = hex.EncodeToString(hash.Sum(nil))
		res.OK = true
		events.Publish("info", "netboot.xyz", "下载完成 "+name)
		results = append(results, res)
	}
	return results
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
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

func tryDownload(ctx context.Context, client *http.Client, urls []string) (*http.Response, error) {
	var last *http.Response
	for _, rawURL := range urls {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusNotFound {
			return resp, nil
		}
		_ = resp.Body.Close()
		last = resp
	}
	return last, nil
}

func netbootURLs(baseURL, name string) []string {
	base := strings.TrimRight(baseURL, "/")
	candidates := []string{base + "/" + name}
	if base == "https://boot.netboot.xyz" {
		candidates = append([]string{"https://boot.netboot.xyz/ipxe/" + name}, candidates...)
	}
	found := false
	for _, candidate := range candidates {
		if candidate == "https://boot.netboot.xyz/ipxe/"+name {
			found = true
			break
		}
	}
	if !found {
		candidates = append(candidates, "https://boot.netboot.xyz/ipxe/"+name)
	}
	return candidates
}
