package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type BootConfig struct {
	Path     string   `toml:"-"`
	Data     Data     `toml:"data"`
	Admin    Admin    `toml:"admin"`
	Database Database `toml:"database"`
}

type Data struct {
	Dir string `toml:"dir"`
}

type Admin struct {
	AdminAddr string `toml:"admin_addr"`
}

type Database struct {
	Path string `toml:"path"`
}

func Default() BootConfig {
	return BootConfig{
		Data:     Data{Dir: "./data"},
		Admin:    Admin{AdminAddr: "127.0.0.1:8088"},
		Database: Database{Path: "pxe.db"},
	}
}

func LoadOrCreate(configPath, dataDir, host, port string) (BootConfig, error) {
	cfg := Default()
	if dataDir != "" {
		cfg.Data.Dir = dataDir
	}
	if configPath == "" {
		configPath = filepath.Join(cfg.Data.Dir, "pxe.toml")
	}
	cfg.Path = configPath

	if b, err := os.ReadFile(configPath); err == nil {
		if err := toml.Unmarshal(b, &cfg); err != nil {
			return BootConfig{}, fmt.Errorf("解析 %s: %w", configPath, err)
		}
		cfg.Path = configPath
	} else if errors.Is(err, os.ErrNotExist) {
		if err := SaveAtomic(configPath, cfg); err != nil {
			return BootConfig{}, err
		}
	} else {
		return BootConfig{}, err
	}

	if dataDir != "" {
		cfg.Data.Dir = dataDir
	}
	if host != "" || port != "" {
		h, p, _ := net.SplitHostPort(cfg.Admin.AdminAddr)
		if host != "" {
			h = host
		}
		if port != "" {
			p = port
		}
		cfg.Admin.AdminAddr = net.JoinHostPort(h, p)
	}
	if err := cfg.Normalize(); err != nil {
		return BootConfig{}, err
	}
	if err := cfg.Validate(); err != nil {
		return BootConfig{}, err
	}
	if err := cfg.EnsureRuntime(); err != nil {
		return BootConfig{}, err
	}
	return cfg, nil
}

func (c *BootConfig) Normalize() error {
	var err error
	c.Data.Dir, err = filepath.Abs(c.Data.Dir)
	if err != nil {
		return err
	}
	c.Database.Path = expandPath(c.Database.Path, c.Data.Dir)
	return nil
}

func expandPath(pathValue, dataDir string) string {
	if filepath.IsAbs(pathValue) {
		return filepath.Clean(pathValue)
	}
	clean := filepath.Clean(pathValue)
	return filepath.Join(dataDir, clean)
}

func (c BootConfig) Validate() error {
	if c.Data.Dir == "" {
		return errors.New("data.dir 不能为空")
	}
	if c.Database.Path == "" {
		return errors.New("database.path 不能为空")
	}
	if _, _, err := net.SplitHostPort(c.Admin.AdminAddr); err != nil {
		return fmt.Errorf("admin.admin_addr 无效: %w", err)
	}
	return nil
}

func (c BootConfig) EnsureRuntime() error {
	dirs := []string{
		c.Data.Dir,
		filepath.Dir(c.Database.Path),
		filepath.Join(c.Data.Dir, "logs"),
		filepath.Join(c.Data.Dir, "boot", "tftp"),
		filepath.Join(c.Data.Dir, "boot", "http"),
		filepath.Join(c.Data.Dir, "boot", "netboot"),
		filepath.Join(c.Data.Dir, "smb"),
		filepath.Join(c.Data.Dir, "exports"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func SaveAtomic(path string, cfg BootConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
