package netboot

import (
	"fmt"
	"os"
	"path/filepath"

	"pxe/internal/booturl"
	"pxe/internal/observability"
)

const LocalVarsFile = "local-vars.ipxe"

const (
	debianKernelPath = "/debian/dists/bookworm/main/installer-amd64/current/images/netboot/debian-installer/amd64/linux"
	debianInitrdPath = "/debian/dists/bookworm/main/installer-amd64/current/images/netboot/debian-installer/amd64/initrd.gz"
	alpineKernelPath = "/alpine/v3.23/releases/x86_64/netboot/vmlinuz-lts"
	alpineInitrdPath = "/alpine/v3.23/releases/x86_64/netboot/initramfs-lts"
)

func EnsureLocalVars(tftpRoot, advertiseIP, httpAddr string, events *observability.Hub) (string, bool, error) {
	if err := os.MkdirAll(tftpRoot, 0755); err != nil {
		return "", false, err
	}
	target := filepath.Join(tftpRoot, LocalVarsFile)
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		if events != nil {
			events.Publish("info", "netboot.xyz", "local-vars.ipxe 已存在，跳过生成")
		}
		return target, false, nil
	}
	script := LocalVarsScript(advertiseIP, httpAddr)
	if err := os.WriteFile(target, []byte(script), 0644); err != nil {
		return "", false, err
	}
	if events != nil {
		events.Publish("info", "netboot.xyz", "已生成 local-vars.ipxe: "+target)
	}
	return target, true, nil
}

func LocalVarsScript(advertiseIP, httpAddr string) string {
	base := booturl.HTTPBase(advertiseIP, httpAddr)
	return fmt.Sprintf(`#!ipxe
set menu-timeout 60000
set public-mirror https://mirrors.tuna.tsinghua.edu.cn
set local-mirror %s

menu PXE Install Menu
item public_debian Public Install Debian 12
item public_alpine Public Install Alpine Linux
item local_debian Local Install Debian 12
item local_alpine Local Install Alpine Linux
item shell iPXE Shell
item exit Continue netboot.xyz
choose --timeout ${menu-timeout} selected || goto exit
goto ${selected}

:public_debian
imgfree
kernel ${public-mirror}%s initrd=initrd.gz ip=dhcp
initrd ${public-mirror}%s
boot || goto failed

:public_alpine
imgfree
kernel ${public-mirror}%s initrd=initramfs-lts ip=dhcp alpine_repo=${public-mirror}/alpine/v3.23/main
initrd ${public-mirror}%s
boot || goto failed

:local_debian
imgfree
kernel ${local-mirror}%s initrd=initrd.gz ip=dhcp
initrd ${local-mirror}%s
boot || goto failed

:local_alpine
imgfree
kernel ${local-mirror}%s initrd=initramfs-lts ip=dhcp alpine_repo=${local-mirror}/alpine/v3.23/main
initrd ${local-mirror}%s
boot || goto failed

:shell
shell
goto exit

:failed
echo Boot failed. Check network, files and boot parameters.
sleep 5
shell

:exit
exit
`, base, debianKernelPath, debianInitrdPath, alpineKernelPath, alpineInitrdPath, debianKernelPath, debianInitrdPath, alpineKernelPath, alpineInitrdPath)
}
