package firmware

import (
	"fmt"
	"os"
	"time"

	"pxe/internal/storage"
)

const projectReleases = "https://github.com/sky22333/netboot/releases"

type File struct {
	Name         string    `json:"name"`
	Architecture string    `json:"architecture"`
	BootPath     string    `json:"boot_path"`
	Exists       bool      `json:"exists"`
	Size         int64     `json:"size"`
	Modified     time.Time `json:"modified,omitempty"`
}

type Source struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Website     string `json:"website"`
	Directory   string `json:"directory"`
	Files       []File `json:"files"`
}

func sources(settings storage.ServiceSettings) []Source {
	project := Source{ID: "project", Name: "项目固件", Description: "包含本项目的网络安装菜单，来自 GitHub 最新稳定版本。", Website: projectReleases, Directory: settings.TFTP.Root, Files: []File{
		{Name: "ipxe-x86_64.efi", Architecture: "UEFI x64", BootPath: "ipxe-x86_64.efi"},
		{Name: "ipxe-arm64.efi", Architecture: "UEFI ARM64", BootPath: "ipxe-arm64.efi"},
		{Name: "undionly.kpxe", Architecture: "BIOS", BootPath: "undionly.kpxe"},
	}}
	netboot := Source{ID: "netboot", Name: "netboot.xyz", Description: "使用 netboot.xyz 的在线安装菜单。", Website: "https://netboot.xyz", Directory: settings.NetbootXYZ.DownloadDir, Files: []File{}}
	architectures := map[string]string{"netboot.xyz.kpxe": "BIOS", "netboot.xyz-undionly.kpxe": "BIOS · UNDI", "netboot.xyz.efi": "UEFI x64", "netboot.xyz-arm64.efi": "UEFI ARM64"}
	for _, name := range settings.NetbootXYZ.Files {
		architecture := architectures[name]
		if architecture == "" {
			architecture = "自定义"
		}
		netboot.Files = append(netboot.Files, File{Name: name, Architecture: architecture, BootPath: "netboot/" + name})
	}
	return []Source{project, netboot}
}

// Catalog is local-only, so opening the page works without reaching GitHub.
func Catalog(settings storage.ServiceSettings) ([]Source, error) {
	catalog := sources(settings)
	for i := range catalog {
		root, err := os.OpenRoot(catalog[i].Directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for j := range catalog[i].Files {
			file := &catalog[i].Files[j]
			if info, err := root.Stat(file.Name); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
				file.Exists, file.Size, file.Modified = true, info.Size(), info.ModTime()
			}
		}
		root.Close()
	}
	return catalog, nil
}

// Select accepts only catalog entries, never client-supplied paths or URLs.
func Select(settings storage.ServiceSettings, id string, names []string) (Source, error) {
	for _, source := range sources(settings) {
		if source.ID != id {
			continue
		}
		if len(names) == 0 || len(names) > len(source.Files) {
			return Source{}, fmt.Errorf("请选择固件")
		}
		selected := []File{}
		seen := map[string]bool{}
		for _, name := range names {
			found := false
			for _, file := range source.Files {
				if file.Name == name && !seen[name] {
					selected = append(selected, file)
					seen[name] = true
					found = true
					break
				}
			}
			if !found {
				return Source{}, fmt.Errorf("固件名称无效或重复：%s", name)
			}
		}
		source.Files = selected
		return source, nil
	}
	return Source{}, fmt.Errorf("固件来源无效")
}
