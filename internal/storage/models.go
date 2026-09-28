package storage

import "time"

type ServiceSettings struct {
	Server     ServerSettings     `json:"server"`
	DHCP       DHCPSettings       `json:"dhcp"`
	TFTP       TFTPSettings       `json:"tftp"`
	HTTPBoot   HTTPBootSettings   `json:"httpboot"`
	SMB        SMBSettings        `json:"smb"`
	BootFiles  BootFilesSettings  `json:"boot_files"`
	NetbootXYZ NetbootXYZSettings `json:"netboot_xyz"`
}

type ServerSettings struct {
	ListenIP    string `json:"listen_ip"`
	AdvertiseIP string `json:"advertise_ip"`
}

type DHCPSettings struct {
	Enabled          bool     `json:"enabled"`
	Mode             string   `json:"mode"`
	NonPXEAction     string   `json:"non_pxe_action"`
	PoolStart        string   `json:"pool_start"`
	PoolEnd          string   `json:"pool_end"`
	SubnetMask       string   `json:"subnet_mask"`
	Router           string   `json:"router"`
	DNS              []string `json:"dns"`
	LeaseTimeSeconds int      `json:"lease_time_seconds"`
	DetectConflicts  bool     `json:"detect_conflicts"`
}

type TFTPSettings struct {
	Enabled        bool   `json:"enabled"`
	Root           string `json:"root"`
	AllowUpload    bool   `json:"allow_upload"`
	MaxTransfers   int    `json:"max_transfers"`
	BlockSizeMax   int    `json:"block_size_max"`
	RetryCount     int    `json:"retry_count"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxUploadBytes int64  `json:"max_upload_bytes"`
}

type HTTPBootSettings struct {
	Enabled          bool   `json:"enabled"`
	Addr             string `json:"addr"`
	Root             string `json:"root"`
	DirectoryListing bool   `json:"directory_listing"`
	RangeRequests    bool   `json:"range_requests"`
}

type SMBSettings struct {
	Enabled     bool   `json:"enabled"`
	Root        string `json:"root"`
	ShareName   string `json:"share_name"`
	Permissions string `json:"permissions"`
}

type BootFilesSettings struct {
	BIOS      string `json:"bios"`
	UEFIIA32  string `json:"uefi_ia32"`
	UEFIX64   string `json:"uefi_x64"`
	UEFIARM32 string `json:"uefi_arm32"`
	UEFIARM64 string `json:"uefi_arm64"`
}

type NetbootXYZSettings struct {
	DownloadDir string   `json:"download_dir"`
	BaseURL     string   `json:"base_url"`
	Files       []string `json:"files"`
}

type Client struct {
	ID         int64  `json:"id"`
	Seq        int64  `json:"seq"`
	Name       string `json:"name"`
	IP         string `json:"ip"`
	MAC        string `json:"mac"`
	Firmware   string `json:"firmware"`
	Status     string `json:"status"`
	DiskHealth string `json:"disk_health"`
	NetSpeed   string `json:"net_speed"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Event struct {
	ID         int64  `json:"id"`
	TS         string `json:"ts"`
	Level      string `json:"level"`
	Source     string `json:"source"`
	Message    string `json:"message"`
	FieldsJSON string `json:"fields_json"`
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
