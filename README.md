# PXE 网络启动服务

Go + Vue 3 构建的单二进制网络启动管理服务，支持 Windows、Linux、macOS。

- DHCP/ProxyDHCP、TFTP、HTTP Boot；Windows 系统 SMB 共享。
- 按 BIOS、UEFI IA32/x64/ARM32/ARM64 明确选择启动固件。
- 客户端绑定、网络唤醒、文件管理、项目与 netboot.xyz 固件下载、实时日志和网络诊断。
- 管理面板必须登录，默认地址为 http://127.0.0.1:8088。

## 使用

Windows 下载 [Release](https://github.com/sky22333/netboot/releases) 中的二进制，解压运行。

```text
--config     指定 pxe.toml
--data-dir   指定数据目录
--host       覆盖管理端监听主机
--port       覆盖管理端端口
--no-browser 禁止自动打开浏览器
```

不指定配置和数据目录时，以程序所在目录为工作目录，在其中创建 data。
首次打开页面创建管理员，配置网络和启动文件，再在运行概览启动服务。程序重启后需重新启动 PXE 服务。

Linux：

```sh
curl -fsSL -o netboot.sh https://raw.githubusercontent.com/sky22333/netboot/main/netboot.sh
chmod +x netboot.sh
./netboot.sh
```

Docker（Linux host 网络）：

```sh
docker run -d --name netboot --restart unless-stopped --network host -v "$PWD/data:/data" ghcr.io/sky22333/netboot
```

完整操作见 [使用文档](docs/使用文档.md)，构建见 [生产部署与编译](docs/生产部署与编译.md)，模块说明见 [项目结构](docs/docs.md)。
