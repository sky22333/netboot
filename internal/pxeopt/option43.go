package pxeopt

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"

	"pxe/internal/storage"
)

func BuildOption43(menu storage.Menu, serverIP string) []byte {
	if !menu.Enabled {
		return nil
	}
	var items []storage.MenuItem
	seen := map[uint16]bool{}
	for _, item := range menu.Items {
		if !item.Enabled || item.PXEType == "" {
			continue
		}
		v, err := strconv.ParseUint(item.PXEType, 16, 16)
		if err != nil || seen[uint16(v)] {
			continue
		}
		seen[uint16(v)] = true
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil
	}
	out := []byte{6, 1, 0x03}
	var servers []byte
	for _, item := range items {
		typ, _ := strconv.ParseUint(item.PXEType, 16, 16)
		ip := serverIP
		if item.ServerIP != "" && !strings.Contains(strings.ToLower(item.ServerIP), "%tftpserver%") && item.ServerIP != "0.0.0.0" {
			ip = item.ServerIP
		}
		ip4 := net.ParseIP(ip).To4()
		if ip4 == nil {
			continue
		}
		if len(servers)+7 > 255 {
			break
		}
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(typ))
		servers = append(servers, b...)
		servers = append(servers, 0x01)
		servers = append(servers, ip4...)
	}
	if len(servers) == 0 {
		return nil
	}
	out = append(out, 8, byte(len(servers)))
	out = append(out, servers...)
	var menuBytes []byte
	for _, item := range items {
		typ, _ := strconv.ParseUint(item.PXEType, 16, 16)
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(typ))
		title := asciiBytes(item.Title)
		if len(title) > 255 {
			title = title[:255]
		}
		if len(menuBytes)+3+len(title) > 255 {
			break
		}
		menuBytes = append(menuBytes, b...)
		menuBytes = append(menuBytes, byte(len(title)))
		menuBytes = append(menuBytes, title...)
	}
	if len(menuBytes) == 0 {
		return nil
	}
	out = append(out, 9, byte(len(menuBytes)))
	out = append(out, menuBytes...)
	prompt := asciiBytes(menu.Prompt)
	if len(prompt) > 253 {
		prompt = prompt[:253]
	}
	prompt = append(prompt, 0)
	out = append(out, 10, byte(len(prompt)+1), byte(clamp(menu.TimeoutSeconds, 0, 255)))
	out = append(out, prompt...)
	out = append(out, 0xff)
	return out
}

func asciiBytes(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r >= 32 && r <= 126 {
			out = append(out, byte(r))
		}
	}
	return out
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func SelectedType(option43 []byte) (uint16, bool) {
	for i := 0; i+1 < len(option43); {
		code := option43[i]
		if code == 255 {
			return 0, false
		}
		ln := int(option43[i+1])
		if i+2+ln > len(option43) {
			return 0, false
		}
		if code == 71 && ln >= 4 {
			return binary.BigEndian.Uint16(option43[i+2 : i+4]), true
		}
		i += 2 + ln
	}
	return 0, false
}
