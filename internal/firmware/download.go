package firmware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"pxe/internal/filetree"
	"pxe/internal/observability"
)

const maxFirmwareBytes = 64 << 20

type Result struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256,omitempty"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

func request(ctx context.Context, client *http.Client, source string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "pxe-firmware")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("下载源返回 HTTP %d，请稍后重试", resp.StatusCode)
	}
	return resp, nil
}

// Each project file uses the release's latest/download redirect directly.
// Existing files are replaced only after the full new response is saved.
func Download(ctx context.Context, client *http.Client, source Source, netbootURL string, events *observability.Hub) ([]Result, error) {
	if err := os.MkdirAll(source.Directory, 0755); err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(source.Directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	results := make([]Result, 0, len(source.Files))
	for _, file := range source.Files {
		res := Result{File: file.Name}
		downloadURL := strings.TrimRight(netbootURL, "/") + "/" + url.PathEscape(file.Name)
		if source.ID == "project" {
			downloadURL = projectReleases + "/latest/download/" + url.PathEscape(file.Name)
		}

		if err := ctx.Err(); err != nil {
			return results, err
		}
		events.Publish("info", "firmware", "开始下载 "+file.Name)
		resp, err := request(ctx, client, downloadURL)
		if err == nil {
			if resp.ContentLength > maxFirmwareBytes {
				err = fmt.Errorf("固件不能超过 64 MiB")
			} else {
				res.SHA256, err = saveDownload(ctx, root, file.Name, resp.Body, resp.ContentLength)
			}
			resp.Body.Close()
		}
		if err != nil {
			res.Error = err.Error()
			events.Publish("error", "firmware", file.Name+"："+res.Error)
		} else {
			res.OK = true
			events.Publish("info", "firmware", "下载完成 "+file.Name)
		}
		results = append(results, res)
	}
	return results, nil
}

func saveDownload(ctx context.Context, root *os.Root, name string, body io.Reader, expectedLength int64) (string, error) {
	temp := filetree.UploadTempPrefix + rand.Text()
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	defer func() { f.Close(); root.Remove(temp) }()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(body, maxFirmwareBytes+1))
	if err != nil {
		return "", err
	}
	if n == 0 || n > maxFirmwareBytes {
		return "", fmt.Errorf("固件必须为 1 字节至 64 MiB")
	}
	if expectedLength >= 0 && n != expectedLength {
		return "", fmt.Errorf("下载不完整，请重试")
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = root.Rename(temp, name); err != nil {
		return "", err
	}
	return sum, nil
}
