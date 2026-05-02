package netboot

import (
	"fmt"
	"os"
	"path/filepath"

	"pxe/internal/booturl"
	"pxe/internal/observability"
)

const LocalVarsFile = "local-vars.ipxe"

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
isset ${net0/ip} || dhcp || goto failed
set menu-timeout 60000
set public-mirror https://mirrors.cernet.edu.cn
set local-mirror %s
isset ${proxydhcp/next-server} && set use_proxydhcp_settings true ||
isset ${buildarch} && set arch ${buildarch} || set arch unknown
iseq ${buildarch} x86_64 && set debian_arch amd64 ||
iseq ${buildarch} i386 && set debian_arch i386 ||
iseq ${buildarch} arm64 && set debian_arch arm64 ||
isset ${debian_arch} || cpuid --ext 29 && set debian_arch amd64 || set debian_arch i386
iseq ${debian_arch} amd64 && set alpine_arch x86_64 ||
iseq ${debian_arch} i386 && set alpine_arch x86 ||
iseq ${debian_arch} arm64 && set alpine_arch aarch64 ||
isset ${alpine_arch} || set alpine_arch x86_64

:main_menu
menu PXE Install Menu
item --gap -- OS Installation
item public_debian Public Install Debian 12
item public_alpine Public Install Alpine Linux
item local_debian Local Install Debian 12
item local_alpine Local Install Alpine Linux
item --gap -- Tools
item show_info Show Boot Information
item shell iPXE Shell
item exit Continue netboot.xyz
choose --timeout ${menu-timeout} --default public_debian selected || goto exit
goto ${selected}

:public_debian
imgfree
set debian-base ${public-mirror}/debian/dists/bookworm/main/installer-${debian_arch}/current/images/netboot/debian-installer/${debian_arch}
kernel ${debian-base}/linux initrd=initrd.gz ip=dhcp
initrd ${debian-base}/initrd.gz
boot || goto failed

:public_alpine
imgfree
set alpine-base ${public-mirror}/alpine/v3.23/releases/${alpine_arch}/netboot
kernel ${alpine-base}/vmlinuz-lts initrd=initramfs-lts ip=dhcp alpine_repo=${public-mirror}/alpine/v3.23/main modloop=${alpine-base}/modloop-lts
initrd ${alpine-base}/initramfs-lts
boot || goto failed

:local_debian
imgfree
set local-debian-base ${local-mirror}/debian/dists/bookworm/main/installer-${debian_arch}/current/images/netboot/debian-installer/${debian_arch}
kernel ${local-debian-base}/linux initrd=initrd.gz ip=dhcp
initrd ${local-debian-base}/initrd.gz
boot || goto failed

:local_alpine
imgfree
set local-alpine-base ${local-mirror}/alpine/v3.23/releases/${alpine_arch}/netboot
kernel ${local-alpine-base}/vmlinuz-lts initrd=initramfs-lts ip=dhcp alpine_repo=${local-mirror}/alpine/v3.23/main modloop=${local-alpine-base}/modloop-lts
initrd ${local-alpine-base}/initramfs-lts
boot || goto failed

:show_info
echo
echo PXE boot information
echo buildarch: ${buildarch}
echo debian_arch: ${debian_arch}
echo alpine_arch: ${alpine_arch}
echo platform: ${platform}
echo mac: ${net0/mac}
echo ip: ${net0/ip}
echo next-server: ${next-server}
echo proxydhcp next-server: ${proxydhcp/next-server}
echo filename: ${filename}
echo proxydhcp filename: ${proxydhcp/filename}
echo public mirror: ${public-mirror}
echo local mirror: ${local-mirror}
sleep 8
goto main_menu

:shell
shell
goto main_menu

:failed
echo Boot failed. Check network, files and boot parameters.
sleep 5
shell

:exit
iseq ${platform} efi && exit ||
sanboot --no-describe --drive 0x80 || exit
`, base)
}
