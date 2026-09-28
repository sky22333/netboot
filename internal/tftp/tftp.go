package tftp

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"pxe/internal/filetree"
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

func Serve(ctx context.Context, settings storage.ServiceSettings, store *storage.Store, events *observability.Hub, conn net.PacketConn) error {
	ctx, cancel := context.WithCancel(ctx)
	addr := conn.LocalAddr().String()
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	events.Publish("info", "tftp", "TFTP 已启动: "+addr)
	sem := make(chan struct{}, settings.TFTP.MaxTransfers)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	buf := make([]byte, 1500)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				events.Publish("info", "tftp", "TFTP 已停止")
				return nil
			default:
				return err
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
	path, err := filetree.Path(name)
	if err != nil {
		events.Publish("error", "tftp", "请求路径非法: "+name+" -> "+client.String()+" error="+err.Error())
		sendErrorCode(client, errAccessViolation, "非法路径")
		return
	}
	tree, err := os.OpenRoot(settings.TFTP.Root)
	if err != nil {
		sendErrorCode(client, errAccessViolation, "根目录不可读")
		return
	}
	defer tree.Close()
	f, err := tree.Open(path)
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

func sendContent(ctx context.Context, settings storage.ServiceSettings, events *observability.Hub, name string, client net.Addr, options map[string]string, reader io.Reader, size int64) {
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	events.Publish("info", "tftp", "开始传输: "+name+" -> "+client.String())
	blockSize := 512
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
		n, err := io.ReadFull(reader, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
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

func receiveFile(ctx context.Context, settings storage.ServiceSettings, store *storage.Store, events *observability.Hub, name string, client net.Addr, options map[string]string) {
	path, err := filetree.Path(name)
	if err != nil {
		sendErrorCode(client, errAccessViolation, "非法路径")
		return
	}
	tree, err := os.OpenRoot(settings.TFTP.Root)
	if err != nil {
		sendErrorCode(client, errAccessViolation, "根目录不可写")
		return
	}
	defer tree.Close()
	if _, err := tree.Stat(path); err == nil {
		sendErrorCode(client, errFileExists, "文件已存在")
		return
	}
	if err := tree.MkdirAll(filepath.Dir(path), 0755); err != nil {
		sendErrorCode(client, errAccessViolation, "目录不可写")
		return
	}
	f, err := tree.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		sendErrorCode(client, errAccessViolation, "文件不可写")
		return
	}
	complete := false
	defer func() {
		_ = f.Close()
		if !complete {
			_ = tree.Remove(path)
		}
	}()
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	blockSize := 512
	if requested, ok := options["blksize"]; ok {
		if v, err := strconv.Atoi(requested); err == nil {
			blockSize = max(512, min(v, min(settings.TFTP.BlockSizeMax, 1428)))
		}
	}
	if len(buildOACKPayload(options, blockSize, 0)) > 0 {
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
			return
		}
		if _, err := f.Write(chunk); err != nil {
			sendErrorCode(client, errDiskFull, "写入失败")
			return
		}
		written += int64(len(chunk))
		writeAck(conn, client, block)
		if len(chunk) < blockSize {
			events.Publish("info", "tftp", "上传完成: "+name+" <- "+client.String())
			complete = true
			_ = f.Close()
			tryParseHealthReport(ctx, store, events, tree, path, client)
			return
		}
		expected++
	}
}

func tryParseHealthReport(ctx context.Context, store *storage.Store, events *observability.Hub, tree *os.Root, path string, client net.Addr) {
	f, err := tree.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
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
