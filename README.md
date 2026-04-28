# pxe

Go + Vue 3 的跨平台 PXE 网络启动管理服务。

## 当前实现

- 管理 Web UI：中文界面、登录认证、移动端抽屉导航、实时日志。
- 服务控制：启动、停止、重启 DHCP/ProxyDHCP、TFTP、HTTP Boot、SMB、Torrent。
- DHCP：ProxyDHCP 67/4011、完整 DHCP 地址池、租约、静态绑定、冲突探测。
- 启动文件选择：BIOS/UEFI/iPXE 自动识别，netboot.xyz 优先，可回退到自定义启动文件。
- TFTP：下载、可选上传、blksize/tsize、重试、并发限制、`netboot/...` 虚拟路径。
- HTTP Boot：文件服务、HEAD、Range、缓存校验头、`/dynamic.ipxe`、`/netboot/...` 虚拟路径。
- 文件管理：浏览、上传、创建目录、重命名、删除、生成 torrent。
- netboot.xyz：从官方地址下载常用启动文件，显示本地存在状态和 SHA256。
- 数据存储：纯 Go SQLite，默认 `data/pxe.db`。

## 开发构建

```powershell
npm ci --prefix web
npm run build --prefix web
go test ./...
go vet ./...
go build -o dist\pxe.exe .\cmd\pxe
```

Linux/macOS：

```bash
(cd web && npm ci && npm run build)
go test ./...
go vet ./...
go build -o dist/pxe ./cmd/pxe
```

## 运行

```powershell
.\dist\pxe.exe
```

不带参数启动时，程序会切换到可执行文件所在目录，并在当前目录创建 `data/`。常用参数：

```text
--config     指定 pxe.toml
--data-dir   指定数据目录
--host       覆盖管理端监听主机
--port       覆盖管理端端口
--no-browser 禁止自动打开浏览器
```

## 运行时目录

```text
data/
├─ pxe.toml
├─ pxe.db
├─ secret.key
├─ logs/pxe.log
├─ boot/
│  ├─ netboot/  # netboot.xyz 下载文件
│  ├─ tftp/     # 自定义 TFTP 文件，可为空
│  └─ http/     # 自定义 HTTP Boot 文件，可为空
├─ smb/
└─ exports/
```

`boot/netboot` 会通过 TFTP 的 `netboot/...` 和 HTTP 的 `/netboot/...` 暴露出来，不需要复制到 `boot/tftp` 或 `boot/http`。完全离线时请把 netboot.xyz 启动文件放在 `boot/netboot`，把 ISO/WIM/VHD/内核等大文件放在 `boot/http`。

更多内容见 [docs](./docs)。
