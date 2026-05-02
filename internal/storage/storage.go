package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, dataDir: dataDir}
	if err := s.Migrate(ctx); err != nil {
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

func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'admin', enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS clients (id INTEGER PRIMARY KEY, seq INTEGER NOT NULL, name TEXT NOT NULL, ip TEXT, mac TEXT, firmware TEXT NOT NULL DEFAULT 'unknown', status TEXT NOT NULL DEFAULT 'unknown', last_boot_file TEXT, disk_health TEXT, net_speed TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_ip ON clients(ip) WHERE ip IS NOT NULL AND ip != '';`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_mac ON clients(mac) WHERE mac IS NOT NULL AND mac != '';`,
		`CREATE TABLE IF NOT EXISTS boot_menus (id INTEGER PRIMARY KEY, menu_type TEXT UNIQUE NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, prompt TEXT NOT NULL, timeout_seconds INTEGER NOT NULL DEFAULT 6, randomize_timeout INTEGER NOT NULL DEFAULT 0);`,
		`CREATE TABLE IF NOT EXISTS boot_menu_items (id INTEGER PRIMARY KEY, menu_id INTEGER NOT NULL, sort_order INTEGER NOT NULL, title TEXT NOT NULL, boot_file TEXT, pxe_type TEXT, server_ip TEXT, enabled INTEGER NOT NULL DEFAULT 1, FOREIGN KEY(menu_id) REFERENCES boot_menus(id) ON DELETE CASCADE);`,
		`CREATE TABLE IF NOT EXISTS client_actions (id INTEGER PRIMARY KEY, sort_order INTEGER NOT NULL, name TEXT NOT NULL, command TEXT NOT NULL, args TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1);`,
		`CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY, ts TEXT NOT NULL, level TEXT NOT NULL, source TEXT NOT NULL, message TEXT NOT NULL, fields_json TEXT);`,
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
	for _, menu := range s.defaultMenus() {
		if err := s.ensureMenu(ctx, menu); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DefaultSettings() ServiceSettings {
	advertiseIP := preferredAdvertiseIP()
	prefix := ipv4Prefix(advertiseIP)
	return ServiceSettings{
		Server:     ServerSettings{ListenIP: "0.0.0.0", AdvertiseIP: advertiseIP},
		DHCP:       DHCPSettings{Enabled: true, Mode: "proxy", NonPXEAction: "network_only", PoolStart: prefix + ".200", PoolEnd: prefix + ".250", SubnetMask: "255.255.255.0", Router: prefix + ".1", DNS: []string{prefix + ".1"}, LeaseTimeSeconds: 86400, DetectConflicts: true},
		TFTP:       TFTPSettings{Enabled: true, Root: filepath.Join(s.dataDir, "boot", "tftp"), AllowUpload: false, MaxTransfers: 64, BlockSizeMax: 1428, RetryCount: 5, TimeoutSeconds: 3, MaxUploadBytes: 256 * 1024 * 1024},
		HTTPBoot:   HTTPBootSettings{Enabled: true, Addr: ":80", Root: filepath.Join(s.dataDir, "boot", "http"), DirectoryListing: true, RangeRequests: true},
		SMB:        SMBSettings{Enabled: false, Root: filepath.Join(s.dataDir, "smb"), ShareName: "pxe", Permissions: "read"},
		BootFiles:  BootFilesSettings{BIOS: "undionly.kpxe", UEFIX64: "ipxe-x86_64.efi", UEFIARM64: "ipxe-arm64.efi"},
		NetbootXYZ: NetbootXYZSettings{Enabled: true, DownloadDir: filepath.Join(s.dataDir, "boot", "netboot"), BaseURL: "https://boot.netboot.xyz/ipxe", Files: []string{"netboot.xyz.kpxe", "netboot.xyz-undionly.kpxe", "netboot.xyz.efi", "netboot.xyz-arm64.efi"}},
		Torrent:    TorrentSettings{Enabled: false, Addr: ":6969"},
		Security:   SecuritySettings{AdminAuthEnabled: true, AllowRemoteAdmin: false},
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
	s.restoreMissingSections(raw, &settings)
	s.normalizeSettings(&settings)
	return settings, nil
}

func (s *Store) restoreMissingSections(raw string, settings *ServiceSettings) {
	var sections map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &sections); err != nil {
		return
	}
	defaults := s.DefaultSettings()
	if _, ok := sections["boot_files"]; !ok {
		settings.BootFiles = defaults.BootFiles
	}
	if _, ok := sections["netboot_xyz"]; !ok {
		settings.NetbootXYZ = defaults.NetbootXYZ
	}
	if _, ok := sections["security"]; !ok {
		settings.Security = defaults.Security
	}
}

func (s *Store) normalizeSettings(settings *ServiceSettings) {
	defaults := s.DefaultSettings()
	if settings.Server.ListenIP == "" {
		settings.Server.ListenIP = defaults.Server.ListenIP
	}
	if settings.Server.AdvertiseIP == "" {
		settings.Server.AdvertiseIP = defaults.Server.AdvertiseIP
	}
	if settings.DHCP.Mode == "" {
		settings.DHCP.Mode = defaults.DHCP.Mode
	}
	if settings.DHCP.NonPXEAction == "" {
		settings.DHCP.NonPXEAction = defaults.DHCP.NonPXEAction
	}
	if settings.DHCP.PoolStart == "" {
		settings.DHCP.PoolStart = defaults.DHCP.PoolStart
	}
	if settings.DHCP.PoolEnd == "" {
		settings.DHCP.PoolEnd = defaults.DHCP.PoolEnd
	}
	if settings.DHCP.SubnetMask == "" {
		settings.DHCP.SubnetMask = defaults.DHCP.SubnetMask
	}
	if settings.DHCP.Router == "" {
		settings.DHCP.Router = defaults.DHCP.Router
	}
	if len(settings.DHCP.DNS) == 0 {
		settings.DHCP.DNS = defaults.DHCP.DNS
	}
	if settings.DHCP.LeaseTimeSeconds == 0 {
		settings.DHCP.LeaseTimeSeconds = defaults.DHCP.LeaseTimeSeconds
	}
	if settings.TFTP.Root == "" {
		settings.TFTP.Root = defaults.TFTP.Root
	}
	if settings.TFTP.MaxTransfers == 0 {
		settings.TFTP.MaxTransfers = defaults.TFTP.MaxTransfers
	}
	if settings.TFTP.BlockSizeMax == 0 {
		settings.TFTP.BlockSizeMax = defaults.TFTP.BlockSizeMax
	}
	if settings.TFTP.RetryCount == 0 {
		settings.TFTP.RetryCount = defaults.TFTP.RetryCount
	}
	if settings.TFTP.TimeoutSeconds == 0 {
		settings.TFTP.TimeoutSeconds = defaults.TFTP.TimeoutSeconds
	}
	if settings.TFTP.MaxUploadBytes == 0 {
		settings.TFTP.MaxUploadBytes = defaults.TFTP.MaxUploadBytes
	}
	if settings.HTTPBoot.Addr == "" {
		settings.HTTPBoot.Addr = defaults.HTTPBoot.Addr
	}
	if settings.HTTPBoot.Root == "" {
		settings.HTTPBoot.Root = defaults.HTTPBoot.Root
	}
	if settings.SMB.Root == "" {
		settings.SMB.Root = defaults.SMB.Root
	}
	if settings.SMB.ShareName == "" {
		settings.SMB.ShareName = defaults.SMB.ShareName
	}
	if settings.SMB.Permissions == "" {
		settings.SMB.Permissions = defaults.SMB.Permissions
	}
	if settings.NetbootXYZ.DownloadDir == "" {
		settings.NetbootXYZ.DownloadDir = defaults.NetbootXYZ.DownloadDir
	}
	if settings.NetbootXYZ.BaseURL == "" {
		settings.NetbootXYZ.BaseURL = defaults.NetbootXYZ.BaseURL
	}
	if strings.TrimRight(settings.NetbootXYZ.BaseURL, "/") == "https://boot.netboot.xyz" {
		settings.NetbootXYZ.BaseURL = defaults.NetbootXYZ.BaseURL
	}
	if len(settings.NetbootXYZ.Files) == 0 {
		settings.NetbootXYZ.Files = defaults.NetbootXYZ.Files
	}
	if settings.Torrent.Addr == "" {
		settings.Torrent.Addr = defaults.Torrent.Addr
	}
	if settings.BootFiles.BIOS == "" {
		settings.BootFiles.BIOS = defaults.BootFiles.BIOS
	}
	if settings.BootFiles.UEFIIA32 == "" {
		settings.BootFiles.UEFIIA32 = defaults.BootFiles.UEFIIA32
	}
	if settings.BootFiles.UEFIX64 == "" {
		settings.BootFiles.UEFIX64 = defaults.BootFiles.UEFIX64
	}
	if settings.BootFiles.UEFIARM32 == "" {
		settings.BootFiles.UEFIARM32 = defaults.BootFiles.UEFIARM32
	}
	if settings.BootFiles.UEFIARM64 == "" {
		settings.BootFiles.UEFIARM64 = defaults.BootFiles.UEFIARM64
	}
}

func (s *Store) SaveSettings(ctx context.Context, settings ServiceSettings) error {
	s.normalizeSettings(&settings)
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
	if net.ParseIP(settings.Server.ListenIP) == nil {
		return fmt.Errorf("server.listen_ip 无效")
	}
	if net.ParseIP(settings.Server.AdvertiseIP) == nil {
		return fmt.Errorf("server.advertise_ip 无效")
	}
	if settings.DHCP.Mode != "proxy" && settings.DHCP.Mode != "dhcp" {
		return fmt.Errorf("dhcp.mode 必须是 proxy 或 dhcp")
	}
	if settings.DHCP.NonPXEAction != "" && settings.DHCP.NonPXEAction != "ignore" && settings.DHCP.NonPXEAction != "network_only" {
		return fmt.Errorf("dhcp.non_pxe_action 必须是 ignore 或 network_only")
	}
	if settings.TFTP.MaxTransfers <= 0 {
		return fmt.Errorf("tftp.max_transfers 必须大于 0")
	}
	if settings.TFTP.BlockSizeMax < 512 || settings.TFTP.BlockSizeMax > 65464 {
		return fmt.Errorf("tftp.block_size_max 必须在 512 到 65464 之间")
	}
	if settings.TFTP.RetryCount < 1 || settings.TFTP.RetryCount > 20 {
		return fmt.Errorf("tftp.retry_count 必须在 1 到 20 之间")
	}
	if settings.TFTP.TimeoutSeconds < 1 || settings.TFTP.TimeoutSeconds > 60 {
		return fmt.Errorf("tftp.timeout_seconds 必须在 1 到 60 之间")
	}
	if settings.TFTP.MaxUploadBytes < 0 {
		return fmt.Errorf("tftp.max_upload_bytes 不能小于 0")
	}
	if settings.HTTPBoot.Addr == "" {
		return fmt.Errorf("httpboot.addr 不能为空")
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
		if settings.DHCP.LeaseTimeSeconds < 300 {
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
	rows, err := s.db.QueryContext(ctx, `SELECT id,seq,name,COALESCE(ip,''),COALESCE(mac,''),firmware,status,COALESCE(last_boot_file,''),COALESCE(disk_health,''),COALESCE(net_speed,''),created_at,updated_at FROM clients ORDER BY seq DESC,id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Client{}
	for rows.Next() {
		var c Client
		if err := rows.Scan(&c.ID, &c.Seq, &c.Name, &c.IP, &c.MAC, &c.Firmware, &c.Status, &c.LastBootFile, &c.DiskHealth, &c.NetSpeed, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetClient(ctx context.Context, id int64) (Client, error) {
	var c Client
	err := s.db.QueryRowContext(ctx, `SELECT id,seq,name,COALESCE(ip,''),COALESCE(mac,''),firmware,status,COALESCE(last_boot_file,''),COALESCE(disk_health,''),COALESCE(net_speed,''),created_at,updated_at FROM clients WHERE id=?`, id).
		Scan(&c.ID, &c.Seq, &c.Name, &c.IP, &c.MAC, &c.Firmware, &c.Status, &c.LastBootFile, &c.DiskHealth, &c.NetSpeed, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) GetIPForMAC(ctx context.Context, mac string) (string, bool) {
	mac = NormalizeMAC(mac)
	var ip string
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(ip,'') FROM clients WHERE mac=? AND ip IS NOT NULL AND ip != ''`, mac).Scan(&ip)
	return ip, err == nil && ip != ""
}

func (s *Store) UpsertClientSeen(ctx context.Context, mac, ip, firmware, status string) {
	mac = NormalizeMAC(mac)
	if mac == "" {
		return
	}
	now := Now()
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM clients WHERE mac=?`, mac).Scan(&id)
	if err == nil {
		_, _ = s.db.ExecContext(ctx, `UPDATE clients SET ip=COALESCE(NULLIF(?,''),ip), firmware=?, status=?, updated_at=? WHERE id=?`, ip, firmware, status, now, id)
		return
	}
	var seq int64
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM clients`).Scan(&seq)
	name := "客户端-" + mac[len(mac)-5:]
	_, _ = s.db.ExecContext(ctx, `INSERT INTO clients(seq,name,ip,mac,firmware,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, seq, name, nullEmpty(ip), mac, firmware, status, now, now)
}

func (s *Store) UpsertClient(ctx context.Context, c Client) (Client, error) {
	c.MAC = NormalizeMAC(c.MAC)
	now := Now()
	if c.Seq == 0 {
		_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0)+1 FROM clients`).Scan(&c.Seq)
	}
	if c.Firmware == "" {
		c.Firmware = "unknown"
	}
	if c.Status == "" {
		c.Status = "unknown"
	}
	if c.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO clients(seq,name,ip,mac,firmware,status,last_boot_file,disk_health,net_speed,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, c.Seq, c.Name, nullEmpty(c.IP), nullEmpty(c.MAC), c.Firmware, c.Status, c.LastBootFile, c.DiskHealth, c.NetSpeed, now, now)
		if err != nil {
			return Client{}, err
		}
		c.ID, _ = res.LastInsertId()
		c.CreatedAt, c.UpdatedAt = now, now
		return c, nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE clients SET seq=?,name=?,ip=?,mac=?,firmware=?,status=?,last_boot_file=?,disk_health=?,net_speed=?,updated_at=? WHERE id=?`, c.Seq, c.Name, nullEmpty(c.IP), nullEmpty(c.MAC), c.Firmware, c.Status, c.LastBootFile, c.DiskHealth, c.NetSpeed, now, c.ID)
	c.UpdatedAt = now
	return c, err
}

func (s *Store) BatchCreateClients(ctx context.Context, prefix, ipStart string, count int) ([]Client, error) {
	if count <= 0 || count > 1000 {
		return nil, fmt.Errorf("批量数量必须在 1 到 1000 之间")
	}
	start := net.ParseIP(ipStart).To4()
	if start == nil {
		return nil, fmt.Errorf("起始 IP 无效")
	}
	base := binaryBig(start)
	out := make([]Client, 0, count)
	for i := 0; i < count; i++ {
		ip := make(net.IP, 4)
		putBinary(ip, base+uint32(i))
		c, err := s.UpsertClient(ctx, Client{Name: fmt.Sprintf("%s%03d", prefix, i+1), IP: ip.String(), MAC: "", Firmware: "unknown", Status: "unassigned"})
		if err != nil {
			return out, err
		}
		out = append(out, c)
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

func (s *Store) AssignMACToIP(ctx context.Context, ip, mac string) error {
	mac = NormalizeMAC(mac)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE clients SET mac=NULL,status='unassigned',updated_at=? WHERE mac=?`, Now(), mac); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE clients SET mac=?,status='offline',updated_at=? WHERE ip=?`, mac, Now(), ip)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("未找到 IP 为 %s 的待分配客户端", ip)
	}
	return tx.Commit()
}

func (s *Store) UnassignedClients(ctx context.Context) ([]Client, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,seq,name,COALESCE(ip,''),COALESCE(mac,''),firmware,status,COALESCE(last_boot_file,''),COALESCE(disk_health,''),COALESCE(net_speed,''),created_at,updated_at FROM clients WHERE (mac IS NULL OR mac='') ORDER BY seq,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Client{}
	for rows.Next() {
		var c Client
		if err := rows.Scan(&c.ID, &c.Seq, &c.Name, &c.IP, &c.MAC, &c.Firmware, &c.Status, &c.LastBootFile, &c.DiskHealth, &c.NetSpeed, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) UpdateClientHealth(ctx context.Context, ip string, diskHealth, netSpeed string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE clients SET disk_health=?,net_speed=?,status='online',updated_at=? WHERE ip=?`, diskHealth, netSpeed, Now(), ip)
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

func (s *Store) ListActions(ctx context.Context) ([]ClientAction, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,sort_order,name,command,args,enabled FROM client_actions ORDER BY sort_order,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ClientAction{}
	for rows.Next() {
		var a ClientAction
		var enabled int
		if err := rows.Scan(&a.ID, &a.SortOrder, &a.Name, &a.Command, &a.Args, &enabled); err != nil {
			return nil, err
		}
		a.Enabled = enabled == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAction(ctx context.Context, id int64) (ClientAction, error) {
	var a ClientAction
	var enabled int
	err := s.db.QueryRowContext(ctx, `SELECT id,sort_order,name,command,args,enabled FROM client_actions WHERE id=?`, id).Scan(&a.ID, &a.SortOrder, &a.Name, &a.Command, &a.Args, &enabled)
	a.Enabled = enabled == 1
	return a, err
}

func (s *Store) SaveActions(ctx context.Context, actions []ClientAction) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM client_actions`); err != nil {
		return err
	}
	for _, a := range actions {
		a.Name = strings.TrimSpace(a.Name)
		a.Command = strings.TrimSpace(a.Command)
		if a.Name == "" {
			return fmt.Errorf("操作名称不能为空")
		}
		if a.Command == "" {
			return fmt.Errorf("操作命令不能为空")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO client_actions(sort_order,name,command,args,enabled) VALUES(?,?,?,?,?)`, a.SortOrder, a.Name, a.Command, a.Args, boolInt(a.Enabled)); err != nil {
			return err
		}
	}
	return tx.Commit()
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

func (s *Store) AddEvent(ctx context.Context, level, source, message string, fields any) error {
	raw := ""
	if fields != nil {
		b, _ := json.Marshal(fields)
		raw = string(b)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO events(ts,level,source,message,fields_json) VALUES(?,?,?,?,?)`, Now(), level, source, message, raw)
	if err == nil {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM events WHERE id NOT IN (SELECT id FROM events ORDER BY id DESC LIMIT 5000)`)
	}
	return err
}

func (s *Store) RecentEvents(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,ts,level,source,message,COALESCE(fields_json,'') FROM (SELECT id,ts,level,source,message,fields_json FROM events ORDER BY id DESC LIMIT ?) ORDER BY id ASC`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.TS, &e.Level, &e.Source, &e.Message, &e.FieldsJSON); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
