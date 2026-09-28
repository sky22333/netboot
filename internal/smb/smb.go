package smb

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pxe/internal/command"
	"pxe/internal/storage"
	"runtime"
	"strings"
)

func Apply(ctx context.Context, settings storage.SMBSettings, start bool, dataDir string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("SMB 自动管理仅支持 Windows，请关闭此选项并在系统中配置 Samba")
	}
	root, err := filepath.Abs(settings.Root)
	if err != nil {
		return err
	}
	ownerPath, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	if start {
		if err = os.MkdirAll(root, 0755); err != nil {
			return err
		}
	}
	action := "stop"
	if start {
		action = "start"
	}
	// Values are passed as environment variables, never interpolated into PowerShell code.
	script := `$ErrorActionPreference = 'Stop'
$shares = @(Get-SmbShare)
$share = $shares | Where-Object Name -EQ $env:PXE_SHARE_NAME
if ($share) {
 if ($share.Description -ne $env:PXE_SHARE_OWNER) { throw '共享名称已被其他程序使用' }
}
$shares | Where-Object Description -EQ $env:PXE_SHARE_OWNER | Remove-SmbShare -Force -Confirm:$false
if ($env:PXE_SHARE_ACTION -eq 'start') {
 $everyone = ([System.Security.Principal.SecurityIdentifier]'S-1-1-0').Translate([System.Security.Principal.NTAccount]).Value
 $params = @{ Name=$env:PXE_SHARE_NAME; Path=$env:PXE_SHARE_ROOT; Description=$env:PXE_SHARE_OWNER }
 if ($env:PXE_SHARE_PERMISSION -eq 'full') { $params.FullAccess = $everyone } else { $params.ReadAccess = $everyone }
 New-SmbShare @params | Out-Null
}`
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "PXE_SHARE_NAME="+settings.ShareName, "PXE_SHARE_ROOT="+root,
		fmt.Sprintf("PXE_SHARE_OWNER=netboot:%x", sha256.Sum256([]byte(strings.ToLower(ownerPath)))), "PXE_SHARE_ACTION="+action, "PXE_SHARE_PERMISSION="+settings.Permissions)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SMB 操作失败: %w: %s", err, command.DecodeOutput(out))
	}
	return nil
}
