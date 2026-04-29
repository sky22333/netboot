package tftp

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"pxe/internal/observability"
	"pxe/internal/storage"
)

const (
	opRRQ   = 1
	opWRQ   = 2
	opDATA  = 3
	opACK   = 4
	opERROR = 5
	opOACK  = 6
)

const (
	errNotDefined       = 0
	errFileNotFound     = 1
	errAccessViolation  = 2
	errDiskFull         = 3
	errIllegalOperation = 4
	errFileExists       = 6
)

func Run(ctx context.Context, settings storage.ServiceSettings, store *storage.Store, events *observability.Hub) {
	addr := net.JoinHostPort(settings.Server.ListenIP, "69")
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		events.Publish("error", "tftp", "TFTP 监听失败: "+err.Error())
		return
	}
	defer conn.Close()
	events.Publish("info", "tftp", "TFTP 已启动: "+addr)
	sem := make(chan struct{}, settings.TFTP.MaxTransfers)
	var wg sync.WaitGroup
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	buf := make([]byte, 1500)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				wg.Wait()
				events.Publish("info", "tftp", "TFTP 已停止")
				return
			default:
				slog.Warn("tftp read error", "error", err)
				continue
			}
		}
		packet := append([]byte(nil), buf[:n]...)
		select {
		case sem <- struct{}{}:
		default:
			sendErrorCode(addr, errNotDefined, "并发传输已达上限")
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			handle(ctx, settings, store, events, packet, addr)
		}()
	}
}

func handle(ctx context.Context, settings storage.ServiceSettings, store *storage.Store, events *observability.Hub, packet []byte, client net.Addr) {
	if len(packet) < 4 {
		return
	}
	op := int(binary.BigEndian.Uint16(packet[:2]))
	parts := strings.Split(string(packet[2:]), "\x00")
	if len(parts) < 2 {
		return
	}
	name := strings.TrimLeft(strings.ReplaceAll(parts[0], "\\", "/"), "/")
	options := parseRequestOptions(parts)
	switch op {
	case opRRQ:
		sendFile(ctx, settings, events, name, client, options)
	case opWRQ:
		if !settings.TFTP.AllowUpload {
			sendErrorCode(client, errAccessViolation, "上传已禁用")
			return
		}
		receiveFile(ctx, settings, store, events, name, client, options)
	default:
		sendErrorCode(client, errIllegalOperation, "不支持的 TFTP 操作")
	}
}

func parseRequestOptions(parts []string) map[string]string {
	opts := map[string]string{}
	for i := 2; i+1 < len(parts); i += 2 {
		key := strings.ToLower(strings.TrimSpace(parts[i]))
		if key == "" {
			continue
		}
		opts[key] = strings.TrimSpace(parts[i+1])
	}
	return opts
}

func sendFile(ctx context.Context, settings storage.ServiceSettings, events *observability.Hub, name string, client net.Addr, options map[string]string) {
	if script, ok := virtualIPXEScript(settings, name); ok {
		events.Publish("info", "tftp", "虚拟 iPXE 脚本已就绪: "+name+" size="+strconv.Itoa(len(script))+" client="+client.String())
		sendContent(ctx, settings, events, name, client, options, bytes.NewReader([]byte(script)), int64(len(script)))
		return
	}
	path, err := resolveReadPath(settings, name)
	if err != nil {
		events.Publish("error", "tftp", "请求路径非法: "+name+" -> "+client.String()+" error="+err.Error())
		sendErrorCode(client, errAccessViolation, "非法路径")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		events.Publish("error", "tftp", "文件不存在或不可读: "+name+" -> "+path+" client="+client.String()+" error="+err.Error())
		sendErrorCode(client, errFileNotFound, "文件不存在")
		return
	}
	defer f.Close()
	size := fileSize(f)
	events.Publish("info", "tftp", "文件已就绪: "+name+" -> "+path+" size="+strconv.FormatInt(size, 10)+" client="+client.String())
	sendContent(ctx, settings, events, name, client, options, f, size)
}

func resolveReadPath(settings storage.ServiceSettings, name string) (string, error) {
	clean := strings.TrimLeft(strings.ReplaceAll(name, "\\", "/"), "/")
	if rel, ok := strings.CutPrefix(clean, "netboot/"); ok {
		root, _ := filepath.Abs(settings.NetbootXYZ.DownloadDir)
		return safeJoin(root, rel)
	}
	root, _ := filepath.Abs(settings.TFTP.Root)
	return safeJoin(root, clean)
}

func sendContent(ctx context.Context, settings storage.ServiceSettings, events *observability.Hub, name string, client net.Addr, options map[string]string, reader io.Reader, size int64) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return
	}
	defer conn.Close()
	events.Publish("info", "tftp", "开始传输: "+name+" -> "+client.String())
	blockSize := settings.TFTP.BlockSizeMax
	if blockSize < 512 {
		blockSize = 512
	}
	if blockSize > 1428 {
		blockSize = 1428
	}
	if requested, ok := options["blksize"]; ok {
		if v, err := strconv.Atoi(requested); err == nil {
			blockSize = max(512, min(v, min(settings.TFTP.BlockSizeMax, 1428)))
		}
	}
	if len(options) > 0 {
		if !sendOACK(conn, client, options, blockSize, size) {
			blockSize = 512
			events.Publish("warning", "tftp", "客户端未确认 OACK，回退到标准 TFTP 模式: "+name)
		}
	}
	block := uint16(1)
	buf := make([]byte, blockSize)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := reader.Read(buf)
		if err != nil && n == 0 {
			return
		}
		data := make([]byte, 4+n)
		binary.BigEndian.PutUint16(data[0:2], opDATA)
		binary.BigEndian.PutUint16(data[2:4], block)
		copy(data[4:], buf[:n])
		if !sendWithAck(conn, client, data, block, settings.TFTP.RetryCount, settings.TFTP.TimeoutSeconds) {
			events.Publish("error", "tftp", "传输超时: "+name+" -> "+client.String())
			return
		}
		if n < blockSize {
			events.Publish("info", "tftp", "传输完成: "+name+" -> "+client.String())
			return
		}
		block++
	}
}

func virtualIPXEScript(settings storage.ServiceSettings, name string) (string, bool) {
	clean := strings.ToLower(strings.TrimLeft(strings.ReplaceAll(name, "\\", "/"), "/"))
	if clean != "boot.ipxe" && clean != "dynamic.ipxe" && clean != "ipxemenu.ipxe" {
		return "", false
	}
	httpURI := httpURI(settings)
	server := settings.Server.AdvertiseIP
	return fmt.Sprintf(`#!ipxe
isset ${net0/ip} || dhcp || goto failed
chain %s/dynamic.ipxe?bootfile=ipxemenu || goto tftp_fallback

:tftp_fallback
echo HTTP 启动不可用，尝试通过 TFTP 启动 netboot.xyz
iseq ${platform} efi && chain tftp://%s/netboot/netboot.xyz.efi || chain tftp://%s/netboot/netboot.xyz.kpxe || chain tftp://%s/netboot/netboot.xyz-undionly.kpxe || goto local

:local
sanboot --no-describe --drive 0x80 || goto failed

:failed
echo PXE 启动失败，请检查 HTTP/TFTP 服务、防火墙和 netboot.xyz 文件。
sleep 5
shell
`, httpURI, server, server, server), true
}

func httpURI(settings storage.ServiceSettings) string {
	addr := settings.HTTPBoot.Addr
	port := "80"
	if strings.HasPrefix(addr, ":") && len(addr) > 1 {
		port = addr[1:]
	} else if host, p, err := net.SplitHostPort(addr); err == nil {
		if host != "" && host != "0.0.0.0" {
			return fmt.Sprintf("http://%s:%s", host, p)
		}
		port = p
	}
	return fmt.Sprintf("http://%s:%s", settings.Server.AdvertiseIP, port)
}

func receiveFile(ctx context.Context, settings storage.ServiceSettings, store *storage.Store, events *observability.Hub, name string, client net.Addr, options map[string]string) {
	root, _ := filepath.Abs(settings.TFTP.Root)
	path, err := safeJoin(root, name)
	if err != nil {
		sendErrorCode(client, errAccessViolation, "非法路径")
		return
	}
	if _, err := os.Stat(path); err == nil {
		sendErrorCode(client, errFileExists, "文件已存在")
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		sendErrorCode(client, errAccessViolation, "目录不可写")
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		sendErrorCode(client, errAccessViolation, "文件不可写")
		return
	}
	defer f.Close()
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return
	}
	defer conn.Close()
	blockSize := 512
	if requested, ok := options["blksize"]; ok {
		if v, err := strconv.Atoi(requested); err == nil {
			blockSize = max(512, min(v, min(settings.TFTP.BlockSizeMax, 1428)))
		}
	}
	if len(options) > 0 {
		sendOACKNoWait(conn, client, options, blockSize, 0)
	} else {
		ack := make([]byte, 4)
		binary.BigEndian.PutUint16(ack[0:2], opACK)
		binary.BigEndian.PutUint16(ack[2:4], 0)
		_, _ = conn.WriteTo(ack, client)
	}
	expected := uint16(1)
	buf := make([]byte, 4+blockSize)
	var written int64
	timeout := timeoutDuration(settings.TFTP.TimeoutSeconds)
	retries := normalizedRetry(settings.TFTP.RetryCount)
	misses := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			misses++
			if misses >= retries {
				sendErrorCode(client, errNotDefined, "上传超时")
				_ = os.Remove(path)
				return
			}
			continue
		}
		if addr.String() != client.String() || n < 4 {
			continue
		}
		misses = 0
		op := binary.BigEndian.Uint16(buf[0:2])
		block := binary.BigEndian.Uint16(buf[2:4])
		if op == opERROR {
			return
		}
		if op == opDATA && block == expected-1 {
			writeAck(conn, client, block)
			continue
		}
		if op != opDATA || block != expected {
			continue
		}
		chunk := buf[4:n]
		if settings.TFTP.MaxUploadBytes > 0 && written+int64(len(chunk)) > settings.TFTP.MaxUploadBytes {
			sendErrorCode(client, errDiskFull, "上传文件超过限制")
			_ = os.Remove(path)
			return
		}
		if _, err := f.Write(chunk); err != nil {
			sendErrorCode(client, errDiskFull, "写入失败")
			_ = os.Remove(path)
			return
		}
		written += int64(len(chunk))
		writeAck(conn, client, block)
		if len(chunk) < blockSize {
			events.Publish("info", "tftp", "上传完成: "+name+" <- "+client.String())
			tryParseHealthReport(ctx, store, events, path, client)
			return
		}
		expected++
	}
}

func tryParseHealthReport(ctx context.Context, store *storage.Store, events *observability.Hub, path string, client net.Addr) {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 || len(b) > 1024*1024 {
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return
	}
	disk := "Unknown"
	if disks, ok := raw["Disks"].([]any); ok {
		disk = "OK"
		for _, item := range disks {
			if m, ok := item.(map[string]any); ok {
				status := fmt.Sprint(m["Health Status"])
				if status != "" && status != "OK" && status != "Unknown" && status != "<nil>" {
					disk = status
					break
				}
			}
		}
	}
	speed := "N/A"
	if nets, ok := raw["Network"].([]any); ok {
		for _, item := range nets {
			if m, ok := item.(map[string]any); ok {
				if v := fmt.Sprint(m["Transmit Link Speed"]); v != "" && v != "<nil>" {
					speed = v
					break
				}
			}
		}
	}
	host, _, err := net.SplitHostPort(client.String())
	if err != nil {
		host = client.String()
	}
	_ = store.UpdateClientHealth(ctx, host, disk, speed)
	events.Publish("info", "clients", "已解析客户端健康报告: "+host)
}

func sendOACKNoWait(conn net.PacketConn, client net.Addr, options map[string]string, blockSize int, size int64) {
	payload := buildOACKPayload(options, blockSize, size)
	if len(payload) == 0 {
		return
	}
	oack := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(oack[0:2], opOACK)
	copy(oack[2:], payload)
	_, _ = conn.WriteTo(oack, client)
}

func sendOACK(conn net.PacketConn, client net.Addr, options map[string]string, blockSize int, size int64) bool {
	payload := buildOACKPayload(options, blockSize, size)
	if len(payload) == 0 {
		return true
	}
	oack := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(oack[0:2], opOACK)
	copy(oack[2:], payload)
	_, _ = conn.WriteTo(oack, client)
	buf := make([]byte, 516)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, addr, err := conn.ReadFrom(buf)
	if err != nil || addr.String() != client.String() || n < 4 {
		return false
	}
	return binary.BigEndian.Uint16(buf[0:2]) == opACK && binary.BigEndian.Uint16(buf[2:4]) == 0
}

func buildOACKPayload(options map[string]string, blockSize int, size int64) []byte {
	var payload []byte
	if _, ok := options["blksize"]; ok {
		payload = append(payload, []byte("blksize\x00"+strconv.Itoa(blockSize)+"\x00")...)
	}
	if _, ok := options["tsize"]; ok {
		payload = append(payload, []byte("tsize\x00"+strconv.FormatInt(size, 10)+"\x00")...)
	}
	return payload
}

func fileSize(f *os.File) int64 {
	pos, _ := f.Seek(0, io.SeekCurrent)
	info, err := f.Stat()
	_, _ = f.Seek(pos, io.SeekStart)
	if err != nil {
		return 0
	}
	return info.Size()
}

func sendWithAck(conn net.PacketConn, client net.Addr, data []byte, block uint16, retryCount, timeoutSeconds int) bool {
	buf := make([]byte, 516)
	for i := 0; i < normalizedRetry(retryCount); i++ {
		_, _ = conn.WriteTo(data, client)
		_ = conn.SetReadDeadline(time.Now().Add(timeoutDuration(timeoutSeconds)))
		n, addr, err := conn.ReadFrom(buf)
		if err != nil || addr.String() != client.String() || n < 4 {
			continue
		}
		op := binary.BigEndian.Uint16(buf[0:2])
		if op == opERROR {
			return false
		}
		if op == opACK && binary.BigEndian.Uint16(buf[2:4]) == block {
			return true
		}
	}
	return false
}

func sendError(client net.Addr, msg string) {
	sendErrorCode(client, errNotDefined, msg)
}

func sendErrorCode(client net.Addr, code uint16, msg string) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return
	}
	defer conn.Close()
	data := make([]byte, 4+len(msg)+1)
	binary.BigEndian.PutUint16(data[0:2], opERROR)
	binary.BigEndian.PutUint16(data[2:4], code)
	copy(data[4:], []byte(msg))
	_, _ = conn.WriteTo(data, client)
}

func writeAck(conn net.PacketConn, client net.Addr, block uint16) {
	ack := make([]byte, 4)
	binary.BigEndian.PutUint16(ack[0:2], opACK)
	binary.BigEndian.PutUint16(ack[2:4], block)
	_, _ = conn.WriteTo(ack, client)
}

func normalizedRetry(v int) int {
	if v <= 0 {
		return 5
	}
	return v
}

func timeoutDuration(seconds int) time.Duration {
	if seconds <= 0 {
		seconds = 3
	}
	return time.Duration(seconds) * time.Second
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func safeJoin(root, request string) (string, error) {
	clean := filepath.Clean(strings.ReplaceAll(request, "/", string(filepath.Separator)))
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
