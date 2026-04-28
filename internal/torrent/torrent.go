package torrent

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"pxe/internal/observability"
)

func Bencode(v any) ([]byte, error) {
	switch x := v.(type) {
	case string:
		return []byte(strconv.Itoa(len(x)) + ":" + x), nil
	case []byte:
		return append([]byte(strconv.Itoa(len(x))+":"), x...), nil
	case int:
		return []byte("i" + strconv.Itoa(x) + "e"), nil
	case int64:
		return []byte("i" + strconv.FormatInt(x, 10) + "e"), nil
	case []any:
		out := []byte("l")
		for _, item := range x {
			b, err := Bencode(item)
			if err != nil {
				return nil, err
			}
			out = append(out, b...)
		}
		return append(out, 'e'), nil
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := []byte("d")
		for _, k := range keys {
			kb, _ := Bencode(k)
			vb, err := Bencode(x[k])
			if err != nil {
				return nil, err
			}
			out = append(out, kb...)
			out = append(out, vb...)
		}
		return append(out, 'e'), nil
	default:
		return nil, fmt.Errorf("不支持的 bencode 类型 %T", v)
	}
}

type Result struct {
	TorrentPath string `json:"torrent_path"`
	InfoHash    string `json:"info_hash"`
	WebSeed     string `json:"web_seed"`
}

func Create(filePath, announceURL, webSeedURL string, pieceSize int) (Result, error) {
	if pieceSize <= 0 {
		pieceSize = 262144
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return Result{}, err
	}
	if info.IsDir() {
		return Result{}, fmt.Errorf("暂不支持目录种子")
	}
	f, err := os.Open(filePath)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	var pieces []byte
	buf := make([]byte, pieceSize)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			sum := sha1.Sum(buf[:n])
			pieces = append(pieces, sum[:]...)
		}
		if err != nil {
			break
		}
	}
	infoDict := map[string]any{
		"name":         filepath.Base(filePath),
		"piece length": pieceSize,
		"pieces":       pieces,
		"length":       info.Size(),
	}
	infoEncoded, err := Bencode(infoDict)
	if err != nil {
		return Result{}, err
	}
	infoHash := sha1.Sum(infoEncoded)
	torrent := map[string]any{
		"announce": announceURL,
		"info":     infoDict,
		"url-list": []any{webSeedURL},
	}
	encoded, err := Bencode(torrent)
	if err != nil {
		return Result{}, err
	}
	target := filePath[:len(filePath)-len(filepath.Ext(filePath))] + ".torrent"
	if err := os.WriteFile(target, encoded, 0644); err != nil {
		return Result{}, err
	}
	return Result{TorrentPath: target, InfoHash: hex.EncodeToString(infoHash[:]), WebSeed: webSeedURL}, nil
}

type peer struct {
	IP        string
	Port      string
	UpdatedAt time.Time
}

func RunTracker(ctx context.Context, addr string, events *observability.Hub) {
	peers := map[string]map[string]peer{}
	mux := http.NewServeMux()
	mux.HandleFunc("/announce", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		infoHash := q.Get("info_hash")
		peerID := q.Get("peer_id")
		port := q.Get("port")
		if infoHash == "" || peerID == "" || port == "" {
			writeBencoded(w, http.StatusBadRequest, map[string]any{"failure reason": "missing required parameter"})
			return
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if peers[infoHash] == nil {
			peers[infoHash] = map[string]peer{}
		}
		now := time.Now()
		for id, item := range peers[infoHash] {
			if now.Sub(item.UpdatedAt) > 30*time.Minute {
				delete(peers[infoHash], id)
			}
		}
		peers[infoHash][peerID] = peer{IP: host, Port: port, UpdatedAt: now}
		var list []any
		for id, item := range peers[infoHash] {
			if id == peerID {
				continue
			}
			p, _ := strconv.Atoi(item.Port)
			list = append(list, map[string]any{"peer id": id, "ip": item.IP, "port": p})
		}
		writeBencoded(w, http.StatusOK, map[string]any{"interval": 900, "peers": list})
	})
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	events.Publish("info", "torrent", "内置 Tracker 已启动: "+addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		events.Publish("error", "torrent", "内置 Tracker 启动失败: "+err.Error())
	}
}

func writeBencoded(w http.ResponseWriter, status int, data map[string]any) {
	b, err := Bencode(data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}
