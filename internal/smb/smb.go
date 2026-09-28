package smb

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"pxe/internal/command"
	"pxe/internal/storage"
	"runtime"
)

func Apply(ctx context.Context, settings storage.SMBSettings, start bool) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("当前平台请手动配置 Samba 或系统共享")
	}
	if settings.ShareName == "" {
		return fmt.Errorf("SMB 共享名称不能为空")
	}
	if start {
		if err := exec.CommandContext(ctx, "net", "share", settings.ShareName).Run(); err == nil {
			return fmt.Errorf("共享 %s 已存在，请选择其他名称", settings.ShareName)
		}
		if err := os.MkdirAll(settings.Root, 0755); err != nil {
			return err
		}
		perm := "/grant:Everyone,READ"
		if settings.Permissions == "full" {
			perm = "/grant:Everyone,FULL"
		}
		out, err := exec.CommandContext(ctx, "net", "share", settings.ShareName+"="+settings.Root, perm).CombinedOutput()
		if err != nil {
			return fmt.Errorf("创建共享失败: %w: %s", err, command.DecodeOutput(out))
		}
		return nil
	}
	out, err := exec.CommandContext(ctx, "net", "share", settings.ShareName, "/delete").CombinedOutput()
	if err != nil {
		return fmt.Errorf("停止共享失败: %w: %s", err, command.DecodeOutput(out))
	}
	return nil
}
