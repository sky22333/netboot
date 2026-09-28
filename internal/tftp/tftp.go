package tftp

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
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
	errIllegalOperation = 4
)

func Serve(ctx context.Context, settings storage.ServiceSettings, events *observability.Hub, conn net.PacketConn) error {
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
			handle(ctx, settings, events, packet, addr)
		}()
	}
}

func handle(ctx context.Context, settings storage.ServiceSettings, events *observability.Hub, packet []byte, client net.Addr) {
	if len(packet) < 4 || len(packet) > 512 {
		return
	}
	op := int(binary.BigEndian.Uint16(packet[:2]))
	parts := strings.Split(string(packet[2:]), "\x00")
	if len(parts) < 3 || parts[len(parts)-1] != "" || (len(parts)-3)%2 != 0 {
		return
	}
	if !strings.EqualFold(parts[1], "octet") {
		sendErrorCode(client, errIllegalOperation, "Only octet mode is supported")
		return
	}
	name := strings.TrimLeft(strings.ReplaceAll(parts[0], "\\", "/"), "/")
	options, err := parseRequestOptions(parts)
	if err != nil {
		sendErrorCode(client, 8, err.Error())
		return
	}
	switch op {
	case opRRQ:
		sendFile(ctx, settings, events, name, client, options)
	case opWRQ:
		sendErrorCode(client, errAccessViolation, "TFTP is read-only")
	default:
		sendErrorCode(client, errIllegalOperation, "不支持的 TFTP 操作")
	}
}

func parseRequestOptions(parts []string) (map[string]string, error) {
	opts := map[string]string{}
	for i := 2; i+1 < len(parts); i += 2 {
		key := strings.ToLower(strings.TrimSpace(parts[i]))
		if _, exists := opts[key]; key == "" || exists {
			return nil, fmt.Errorf("Invalid or duplicate option")
		}
		opts[key] = strings.TrimSpace(parts[i+1])
	}
	return opts, nil
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
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		sendErrorCode(client, errAccessViolation, "Not a regular file")
		return
	}
	size := info.Size()
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
	blockSize, err := negotiatedBlockSize(options, settings.TFTP.BlockSizeMax)
	if err != nil {
		sendErrorCode(client, 8, err.Error())
		return
	}
	if payload := buildOACKPayload(options, blockSize, size); len(payload) > 0 {
		if !sendWithAck(conn, client, append([]byte{0, opOACK}, payload...), 0, settings.TFTP.RetryCount, settings.TFTP.TimeoutSeconds) {
			events.Publish("error", "tftp", "选项协商失败: "+name)
			return
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

func negotiatedBlockSize(options map[string]string, maximum int) (int, error) {
	size := 512
	if raw, ok := options["blksize"]; ok {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 8 || n > 65464 {
			return 0, fmt.Errorf("Invalid blksize")
		}
		if maximum < 8 {
			maximum = 1428
		}
		size = min(n, maximum, 1428)
	}
	if raw, ok := options["tsize"]; ok && raw != "0" {
		return 0, fmt.Errorf("Invalid tsize")
	}
	return size, nil
}

func sendWithAck(conn net.PacketConn, client net.Addr, data []byte, block uint16, retryCount, timeoutSeconds int) bool {
	buf := make([]byte, 516)
	for i := 0; i < normalizedRetry(retryCount); i++ {
		if _, err := conn.WriteTo(data, client); err != nil {
			return false
		}
		_ = conn.SetReadDeadline(time.Now().Add(timeoutDuration(timeoutSeconds)))
		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				if e, ok := err.(net.Error); ok && e.Timeout() {
					break
				}
				return false
			}
			if addr.String() != client.String() || n < 4 {
				continue
			}
			op := binary.BigEndian.Uint16(buf[:2])
			if op == opERROR {
				return false
			}
			if n == 4 && op == opACK && binary.BigEndian.Uint16(buf[2:4]) == block {
				return true
			}
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
