package smb

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"pxe/internal/storage"
)

func Apply(settings storage.SMBSettings, start bool) error {
	if !settings.Enabled && start {
		return nil
	}
	if runtime.GOOS != "windows" {
		if start {
			return fmt.Errorf("当前平台不支持自动创建 SMB 共享，请手动配置 Samba 或系统共享")
		}
		return nil
	}
	if settings.ShareName == "" {
		return fmt.Errorf("SMB 共享名称不能为空")
	}
	_ = exec.Command("net", "share", settings.ShareName, "/delete").Run()
	if !start {
		return nil
	}
	if err := os.MkdirAll(settings.Root, 0755); err != nil {
		return err
	}
	perm := "/grant:Everyone,READ"
	if settings.Permissions == "full" {
		perm = "/grant:Everyone,FULL"
	}
	cmd := exec.Command("net", "share", settings.ShareName+"="+settings.Root, perm)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, string(out))
	}
	return nil
}
