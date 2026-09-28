package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

const maxDHCPPoolSize = 65536

type Store struct {
	db      *sql.DB
	dataDir string
}

func Open(ctx context.Context, dbPath, dataDir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(abs)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	dsn := url.URL{Scheme: "file", Path: uriPath, RawQuery: "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, dataDir: dataDir}
	if err := s.initializeSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := s.EnsureDefaults(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) initializeSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'admin', enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS sessions (token_hash TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,expires INTEGER NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS clients (id INTEGER PRIMARY KEY, seq INTEGER NOT NULL, name TEXT NOT NULL, ip TEXT, observed_ip TEXT, mac TEXT, firmware TEXT NOT NULL DEFAULT 'unknown', status TEXT NOT NULL DEFAULT 'unknown', created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_ip ON clients(ip) WHERE ip IS NOT NULL AND ip != '';`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_mac ON clients(mac) WHERE mac IS NOT NULL AND mac != '';`,
		`CREATE TABLE IF NOT EXISTS leases (mac TEXT PRIMARY KEY, ip TEXT UNIQUE NOT NULL, expires INTEGER NOT NULL, confirmed INTEGER NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY, time TEXT NOT NULL, level TEXT NOT NULL, source TEXT NOT NULL, message TEXT NOT NULL);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) EnsureDefaults(ctx context.Context) error {
	if _, err := s.GetSettings(ctx); errors.Is(err, sql.ErrNoRows) {
		if err := s.SaveSettings(ctx, s.DefaultSettings()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

func (s *Store) DefaultSettings() ServiceSettings {
	advertiseIP := preferredAdvertiseIP()
	prefix := ipv4Prefix(advertiseIP)
	return ServiceSettings{
		Server:     ServerSettings{ListenIP: "0.0.0.0", AdvertiseIP: advertiseIP},
		DHCP:       DHCPSettings{Enabled: true, Mode: "proxy", NonPXEAction: "network_only", PoolStart: prefix + ".200", PoolEnd: prefix + ".250", SubnetMask: "255.255.255.0", Router: prefix + ".1", DNS: []string{prefix + ".1"}, LeaseTimeSeconds: 86400, DetectConflicts: true},
		TFTP:       TFTPSettings{Enabled: true, Root: filepath.Join(s.dataDir, "boot", "tftp"), MaxTransfers: 64, BlockSizeMax: 1428, RetryCount: 5, TimeoutSeconds: 3},
		HTTPBoot:   HTTPBootSettings{Enabled: true, Addr: ":80", Root: filepath.Join(s.dataDir, "boot", "http"), DirectoryListing: true, RangeRequests: true},
		SMB:        SMBSettings{Enabled: false, Root: filepath.Join(s.dataDir, "smb"), ShareName: "pxe", Permissions: "read"},
		BootFiles:  BootFilesSettings{BIOS: "undionly.kpxe", UEFIX64: "ipxe-x86_64.efi", UEFIARM64: "ipxe-arm64.efi"},
		NetbootXYZ: NetbootXYZSettings{BaseURL: "https://boot.netboot.xyz/ipxe", Files: []string{"netboot.xyz.kpxe", "netboot.xyz-undionly.kpxe", "netboot.xyz.efi", "netboot.xyz-arm64.efi"}},
	}
}

func preferredAdvertiseIP() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok && addr.IP.To4() != nil && !addr.IP.IsLoopback() {
			return addr.IP.String()
		}
	}
	return firstPrivateIP()
}

func ipv4Prefix(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return "192.168.1"
	}
	return strings.Join(parts[:3], ".")
}

func firstPrivateIP() string {
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && ip.To4() != nil && (strings.HasPrefix(ip.String(), "192.168.") || strings.HasPrefix(ip.String(), "10.") || strings.HasPrefix(ip.String(), "172.")) {
				return ip.String()
			}
		}
	}
	return "192.168.1.100"
}

func (s *Store) GetSettings(ctx context.Context) (ServiceSettings, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='service'`).Scan(&raw); err != nil {
		return ServiceSettings{}, err
	}
	var settings ServiceSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return ServiceSettings{}, err
	}
	if err := ValidateSettings(settings); err != nil {
		return ServiceSettings{}, err
	}
	return settings, nil
}

func (s *Store) SaveSettings(ctx context.Context, settings ServiceSettings) error {
	if err := ValidateSettings(settings); err != nil {
		return err
	}
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES('service',?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, string(b), Now())
	return err
}

func ValidateSettings(settings ServiceSettings) error {
	if settings.SMB.Enabled && runtime.GOOS != "windows" {
		return fmt.Errorf("SMB 自动管理仅支持 Windows，请使用系统 Samba")
	}
	if net.ParseIP(settings.Server.ListenIP).To4() == nil {
		return fmt.Errorf("server.listen_ip 无效")
	}
	if !usableIPv4(settings.Server.AdvertiseIP) {
		return fmt.Errorf("server.advertise_ip 无效")
	}
	if settings.DHCP.Mode != "proxy" && settings.DHCP.Mode != "dhcp" {
		return fmt.Errorf("dhcp.mode 必须是 proxy 或 dhcp")
	}
	if settings.DHCP.NonPXEAction != "" && settings.DHCP.NonPXEAction != "ignore" && settings.DHCP.NonPXEAction != "network_only" {
		return fmt.Errorf("dhcp.non_pxe_action 必须是 ignore 或 network_only")
	}
	if settings.TFTP.MaxTransfers <= 0 || settings.TFTP.MaxTransfers > 256 {
		return fmt.Errorf("tftp.max_transfers 必须为 1-256")
	}
	if settings.TFTP.BlockSizeMax < 512 || settings.TFTP.BlockSizeMax > 1428 {
		return fmt.Errorf("tftp.block_size_max 必须在 512 到 1428 之间")
	}
	if settings.TFTP.RetryCount < 1 || settings.TFTP.RetryCount > 20 {
		return fmt.Errorf("tftp.retry_count 必须在 1 到 20 之间")
	}
	if settings.TFTP.TimeoutSeconds < 1 || settings.TFTP.TimeoutSeconds > 60 {
		return fmt.Errorf("tftp.timeout_seconds 必须在 1 到 60 之间")
	}
	host, port, err := net.SplitHostPort(settings.HTTPBoot.Addr)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 1 || n > 65535 || (host != "" && net.ParseIP(host) == nil) {
		return fmt.Errorf("httpboot.addr 必须为 IP:端口或 :端口")
	}
	for _, file := range []string{settings.BootFiles.BIOS, settings.BootFiles.UEFIIA32, settings.BootFiles.UEFIX64, settings.BootFiles.UEFIARM32, settings.BootFiles.UEFIARM64} {
		if file != "" && (!filepath.IsLocal(file) || len(file) > 127 || strings.ContainsAny(file, "\r\n\x00:")) {
			return fmt.Errorf("启动文件必须是长度不超过 127 字节的相对路径")
		}
	}
	source, e := url.Parse(settings.NetbootXYZ.BaseURL)
	if e != nil || source.Host == "" || (source.Scheme != "http" && source.Scheme != "https") {
		return fmt.Errorf("固件下载来源必须是 HTTP(S) URL")
	}
	for _, file := range settings.NetbootXYZ.Files {
		if file == "" || !filepath.IsLocal(file) || filepath.Base(file) != file || strings.ContainsAny(file, "/\\:\r\n\x00") {
			return fmt.Errorf("下载文件名无效")
		}
	}
	if settings.HTTPBoot.Root == "" {
		return fmt.Errorf("httpboot.root 不能为空")
	}
	if settings.TFTP.Root == "" {
		return fmt.Errorf("tftp.root 不能为空")
	}
	if settings.SMB.Enabled {
		if strings.TrimSpace(settings.SMB.Root) == "" {
			return fmt.Errorf("smb.root 不能为空")
		}
		if strings.TrimSpace(settings.SMB.ShareName) == "" {
			return fmt.Errorf("smb.share_name 不能为空")
		}
		if settings.SMB.Permissions != "read" && settings.SMB.Permissions != "full" {
			return fmt.Errorf("smb.permissions 必须是 read 或 full")
		}
	}
	if settings.DHCP.Enabled && settings.DHCP.Mode == "dhcp" {
		start := net.ParseIP(settings.DHCP.PoolStart).To4()
		end := net.ParseIP(settings.DHCP.PoolEnd).To4()
		if start == nil || end == nil {
			return fmt.Errorf("dhcp 地址池必须是有效 IPv4")
		}
		if binaryBig(start) > binaryBig(end) {
			return fmt.Errorf("dhcp.pool_end 必须大于或等于 pool_start")
		}
		if binaryBig(end)-binaryBig(start)+1 > maxDHCPPoolSize {
			return fmt.Errorf("dhcp 地址池不能超过 %d 个 IP", maxDHCPPoolSize)
		}
		if net.ParseIP(settings.DHCP.SubnetMask).To4() == nil {
			return fmt.Errorf("dhcp.subnet_mask 无效")
		}
		if net.ParseIP(settings.DHCP.Router).To4() == nil {
			return fmt.Errorf("dhcp.router 无效")
		}
		mask := net.IPMask(net.ParseIP(settings.DHCP.SubnetMask).To4())
		ones, bits := mask.Size()
		if bits != 32 || ones < 1 || ones > 30 {
			return fmt.Errorf("DHCP 子网掩码必须连续且至少有两个可用地址")
		}
		network := net.ParseIP(settings.DHCP.Router).To4().Mask(mask)
		if !start.Mask(mask).Equal(network) || !end.Mask(mask).Equal(network) {
			return fmt.Errorf("地址池和网关必须位于同一子网")
		}
		if settings.DHCP.LeaseTimeSeconds < 300 || settings.DHCP.LeaseTimeSeconds > 31536000 {
			return fmt.Errorf("dhcp.lease_time_seconds 不能小于 300")
		}
		for _, dns := range settings.DHCP.DNS {
			if net.ParseIP(dns).To4() == nil {
				return fmt.Errorf("dhcp.dns 包含无效 IPv4: %s", dns)
			}
		}
	}
	return nil
}

func binaryBig(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func (s *Store) ListClients(ctx context.Context) ([]Client, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,seq,name,COALESCE(ip,''),COALESCE(observed_ip,''),COALESCE(mac,''),firmware,status,created_at,updated_at FROM clients ORDER BY seq DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Client{}
	for rows.Next() {
		var c Client
		if err := rows.Scan(&c.ID, &c.Seq, &c.Name, &c.IP, &c.ObservedIP, &c.MAC, &c.Firmware, &c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetClient(ctx context.Context, id int64) (Client, error) {
	var c Client
	err := s.db.QueryRowContext(ctx, `SELECT id,seq,name,COALESCE(ip,''),COALESCE(observed_ip,''),COALESCE(mac,''),firmware,status,created_at,updated_at FROM clients WHERE id=?`, id).
		Scan(&c.ID, &c.Seq, &c.Name, &c.IP, &c.ObservedIP, &c.MAC, &c.Firmware, &c.Status, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

// Observations never modify the administrator's static reservation.
func (s *Store) UpsertClientSeen(ctx context.Context, mac, ip, firmware, status string) error {
	mac = NormalizeMAC(mac)
	if !validMAC(mac) {
		return fmt.Errorf("MAC 地址无效")
	}
	if !usableIPv4(ip) {
		ip = ""
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO clients(seq,name,observed_ip,mac,firmware,status,created_at,updated_at)
 VALUES((SELECT COALESCE(MAX(seq),0)+1 FROM clients),?,?,?,?,?,?,?)
 ON CONFLICT(mac) WHERE mac IS NOT NULL AND mac != '' DO UPDATE SET
 observed_ip=COALESCE(NULLIF(excluded.observed_ip,''),clients.observed_ip),firmware=excluded.firmware,status=excluded.status,updated_at=excluded.updated_at`,
		"客户端-"+mac[len(mac)-5:], nullEmpty(ip), mac, firmware, status, Now(), Now())
	return err
}

func validMAC(mac string) bool {
	hw, err := net.ParseMAC(strings.ReplaceAll(mac, "-", ":"))
	return err == nil && len(hw) == 6 && hw[0]&1 == 0 && hw.String() != "00:00:00:00:00:00"
}

func usableIPv4(ip string) bool {
	v := net.ParseIP(ip).To4()
	return v != nil && !v.IsUnspecified() && !v.IsMulticast() && !v.Equal(net.IPv4bcast)
}

func (s *Store) UpsertClient(ctx context.Context, c Client) (Client, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Client{}, err
	}
	// A committed transaction needs no rollback; preserve the original error otherwise.
	defer func() { _ = tx.Rollback() }()
	c, err = upsertClient(ctx, tx, c)
	if err != nil {
		return Client{}, err
	}
	if err = tx.Commit(); err != nil {
		return Client{}, err
	}
	return s.GetClient(ctx, c.ID)
}

func upsertClient(ctx context.Context, tx *sql.Tx, c Client) (Client, error) {
	c.MAC = NormalizeMAC(strings.TrimSpace(c.MAC))
	c.IP = strings.TrimSpace(c.IP)
	if c.MAC != "" && !validMAC(c.MAC) {
		return Client{}, fmt.Errorf("MAC 地址无效")
	}
	if c.IP != "" && !usableIPv4(c.IP) {
		return Client{}, fmt.Errorf("静态 IPv4 地址无效")
	}
	var err error
	var conflict int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM leases WHERE ip=? AND mac!=? AND expires>unixepoch()`, c.IP, c.MAC).Scan(&conflict); err != nil {
		return Client{}, err
	}
	if conflict > 0 {
		return Client{}, fmt.Errorf("地址仍被其他客户端租用，请等待租约到期")
	}
	if c.Seq == 0 {
		if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM clients`).Scan(&c.Seq); err != nil {
			return Client{}, err
		}
	}
	if c.ID == 0 {
		res, e := tx.ExecContext(ctx, `INSERT INTO clients(seq,name,ip,mac,created_at,updated_at) VALUES(?,?,?,?,?,?)`, c.Seq, c.Name, nullEmpty(c.IP), nullEmpty(c.MAC), Now(), Now())
		if e != nil {
			return Client{}, e
		}
		c.ID, err = res.LastInsertId()
	} else {
		res, e := tx.ExecContext(ctx, `UPDATE clients SET seq=?,name=?,ip=?,mac=?,updated_at=? WHERE id=?`, c.Seq, c.Name, nullEmpty(c.IP), nullEmpty(c.MAC), Now(), c.ID)
		if e != nil {
			return Client{}, e
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return Client{}, fmt.Errorf("客户端不存在")
		}
	}
	if err != nil {
		return Client{}, err
	}
	return c, nil
}

func (s *Store) BatchCreateClients(ctx context.Context, prefix, ipStart string, count int) ([]Client, error) {
	if count <= 0 || count > 1000 {
		return nil, fmt.Errorf("批量数量必须在 1 到 1000 之间")
	}
	start := net.ParseIP(ipStart).To4()
	if start == nil {
		return nil, fmt.Errorf("起始 IP 无效")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	// A committed transaction needs no rollback; preserve the original error otherwise.
	defer func() { _ = tx.Rollback() }()
	base := binaryBig(start)
	if uint64(base)+uint64(count)-1 > 0xffffffff {
		return nil, fmt.Errorf("地址范围溢出")
	}
	out := make([]Client, 0, count)
	for i := 0; i < count; i++ {
		ip := make(net.IP, 4)
		putBinary(ip, base+uint32(i))
		c, err := upsertClient(ctx, tx, Client{Name: fmt.Sprintf("%s%03d", prefix, i+1), IP: ip.String()})
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func putBinary(ip net.IP, v uint32) {
	ip[0] = byte(v >> 24)
	ip[1] = byte(v >> 16)
	ip[2] = byte(v >> 8)
	ip[3] = byte(v)
}

func (s *Store) ClearClientMAC(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE clients SET mac=NULL,status='unassigned',updated_at=? WHERE id=?`, Now(), id)
	return err
}

func (s *Store) DeleteClient(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM clients WHERE id=?`, id)
	return err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,username,role,enabled,created_at,updated_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		var enabled int
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &enabled, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.Enabled = enabled == 1
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	var firstID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&firstID); err != nil {
		return err
	}
	if id == firstID {
		return fmt.Errorf("默认管理员不能删除")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("用户不存在")
	}
	return nil
}

func NormalizeMAC(mac string) string {
	clean := strings.ToUpper(strings.NewReplacer(":", "", "-", "", ".", "").Replace(mac))
	if len(clean) != 12 {
		return mac
	}
	parts := make([]string, 0, 6)
	for i := 0; i < 12; i += 2 {
		parts = append(parts, clean[i:i+2])
	}
	return strings.Join(parts, "-")
}

func nullEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) RecordEvent(e Event) (Event, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return e, err
	}
	// A committed transaction needs no rollback; preserve the original error otherwise.
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(`INSERT INTO events(time,level,source,message) VALUES(?,?,?,?)`, e.Time, e.Level, e.Source, e.Message)
	if err != nil {
		return e, err
	}
	e.ID, err = res.LastInsertId()
	if err != nil {
		return e, err
	}
	if _, err = tx.Exec(`DELETE FROM events WHERE id<=?`, e.ID-5000); err != nil {
		return e, err
	}
	return e, tx.Commit()
}

func (s *Store) RecentEvents(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,time,level,source,message FROM (SELECT id,time,level,source,message FROM events ORDER BY id DESC LIMIT ?) ORDER BY id ASC`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Time, &e.Level, &e.Source, &e.Message); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
