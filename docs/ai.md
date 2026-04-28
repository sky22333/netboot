# pxe 项目说明与维护规范

## 项目定位

`pxe` 是一个使用 Go 后端 + Vue 3 Web UI 实现的跨平台 PXE 网络启动管理服务。它面向 Windows、Linux、macOS、OpenWrt、Armbian 等环境，目标是以单二进制方式提供 DHCP/ProxyDHCP、TFTP、HTTP Boot、动态 iPXE 菜单、客户端管理、文件管理和 netboot.xyz 辅助下载能力。

## 当前技术栈

后端：

- Go 1.25+。
- Gin 管理 API 和静态前端托管。
- `log/slog` 结构化日志。
- 纯 Go SQLite：`modernc.org/sqlite`，方便 CGO 关闭和交叉编译。
- TOML 启动配置：`github.com/pelletier/go-toml/v2`。
- SSE 实时事件流。
- Go `embed` 打包 Vue 构建产物。

前端：

- Vue 3。
- TypeScript。
- Vite。
- Tailwind CSS。
- Vue Router。
- Pinia。
- lucide-vue-next。
- 中文界面，风格接近 shadcn/ui：中性色、细边框、轻阴影、8px 左右圆角。

## 目录结构

```text
pxe/
├─ cmd/pxe/                 # 程序入口
├─ internal/
│  ├─ app/                  # 应用装配、服务生命周期
│  ├─ config/               # pxe.toml 启动配置
│  ├─ dhcp/                 # DHCP、ProxyDHCP、租约和启动文件响应
│  ├─ httpboot/             # HTTP Boot、Range、dynamic.ipxe、netboot 虚拟路径
│  ├─ ipxe/                 # iPXE 脚本生成
│  ├─ netboot/              # netboot.xyz 下载
│  ├─ observability/        # 事件总线、实时日志
│  ├─ platform/             # 权限和平台诊断
│  ├─ pxeopt/               # PXE Option 43
│  ├─ smb/                  # Windows SMB 辅助
│  ├─ storage/              # SQLite、模型、默认配置
│  ├─ tftp/                 # TFTP RRQ/WRQ、blksize/tsize
│  ├─ torrent/              # torrent 生成和 tracker
│  └─ web/                  # Gin API、认证、前端 embed
├─ web/                     # Vue 前端源码
├─ docs/                    # 中文部署、离线、维护文档
├─ go.mod
└─ README.md
```

## 运行时文件结构

```text
data/
├─ pxe.toml                 # 启动配置
├─ pxe.db                   # 少量结构化数据
├─ secret.key               # cookie/session 签名密钥
├─ logs/pxe.log             # 结构化文本日志
├─ boot/
│  ├─ netboot/              # netboot.xyz 下载文件
│  ├─ tftp/                 # 自定义 TFTP 文件，可为空
│  └─ http/                 # 自定义 HTTP Boot 文件和镜像，可为空
├─ smb/
└─ exports/
```

设计原则：

- 大文件、镜像、启动资源保存在文件系统。
- 数据库只保存配置、菜单、客户端、账号、下载记录和少量事件。
- `boot/netboot` 通过 TFTP 的 `netboot/...` 和 HTTP 的 `/netboot/...` 暴露，避免复制文件。
- `boot/tftp` 和 `boot/http` 可以为空，用户只在需要自定义文件时放入内容。

## 启动链路

推荐默认链路：

```text
客户端 PXE
-> 现有 DHCP 分配 IP
-> pxe ProxyDHCP 返回 next-server 和 filename
-> TFTP 下载 netboot.xyz.kpxe 或 netboot.xyz.efi
-> 进入 netboot.xyz 或自定义启动菜单
```

完整 DHCP 链路：

```text
客户端 PXE
-> pxe 完整 DHCP 分配 IP、网关、DNS、租约和启动文件
-> TFTP 下载启动文件
-> HTTP Boot 或 netboot.xyz 继续引导
```

关键实现：

- ProxyDHCP 同时监听 UDP 4011 和 67，用于兼容不同 PXE/iPXE 固件。
- 对 DHCP DISCOVER 同时发送兼容的 OFFER 和 ACK，提升老旧固件兼容性。
- 响应目标包含 `255.255.255.255:68`、按通告 IP/子网掩码计算的定向广播和必要时的客户端单播。
- BIOS 优先使用 `netboot/netboot.xyz.kpxe`，UEFI 优先使用 `netboot/netboot.xyz.efi`。
- 如果 netboot 文件不存在，回退到服务配置中的默认启动文件。

## 配置说明

`pxe.toml` 只保存启动前必须知道的信息：

```toml
[data]
dir = "./data"

[admin]
admin_addr = "127.0.0.1:8088"

[database]
path = "./data/pxe.db"

[security]
secret_file = "./data/secret.key"

[logging]
level = "info"
format = "text"
```

常规服务配置保存在数据库中，通过 Web UI 管理。

重要字段：

- 监听 IP：`0.0.0.0` 表示监听所有网卡，适合接收 DHCP 广播。
- 通告 IP：客户端访问 TFTP/HTTP 的服务器地址，必须是客户端可达的网卡 IP。
- ProxyDHCP 模式：不分配 IP，地址池、网关、DNS 不生效。
- 完整 DHCP 模式：会分配 IP，必须配置正确的地址池、网关、DNS 和子网掩码。

## Web UI

页面：

- 仪表盘。
- 服务配置。
- 客户端。
- 启动菜单。
- 文件管理。
- netboot.xyz。
- 操作菜单。
- 用户。
- 日志。
- 系统诊断。

交互要求：

- 全中文。
- 移动端使用抽屉导航。
- 日志通过 SSE 实时更新；仪表盘和日志页共用 `web/src/lib/eventLog.ts`，按事件 ID 升序显示，最多保留最近 1000 条，避免重复 SSE、乱序刷新和无限内存增长。
- 文件管理必须做路径限制和危险操作确认。
- 完整 DHCP、删除、上传、外部下载等高风险操作必须明确提示。

## 安全要求

- 默认管理端监听 `127.0.0.1`。
- 首次使用创建管理员账号。
- 远程管理必须启用认证。
- Cookie 使用 HttpOnly 和 SameSite。
- 文件访问必须限制在配置根目录内。
- TFTP 上传默认关闭。
- 日志不得记录密码、token、session。

## 构建与发布

本地生产构建：

```bash
cd pxe
(cd web && npm ci && npm run build)
go test ./...
go vet ./...
go build -trimpath -ldflags="-s -w" -o dist/pxe ./cmd/pxe
```

Windows：

```powershell
cd pxe
npm ci --prefix web
npm run build --prefix web
go test ./...
go vet ./...
go build -trimpath -ldflags="-s -w" -o dist\pxe.exe .\cmd\pxe
```

GitHub Actions：

- `.github/workflows/release.yml` 手动触发。
- 构建 Windows、Linux、macOS 多平台二进制。
- 上传 zip/tar.gz

## 离线自托管

完全离线时：

1. 把 `netboot.xyz.kpxe`、`netboot.xyz-undionly.kpxe`、`netboot.xyz.efi` 放到 `data/boot/netboot`。
2. 把 ISO/WIM/IMG/VHD、Linux 内核、initrd 和自动安装配置放到 `data/boot/http`。
3. 在启动菜单中添加本地镜像路径，例如 `images/winpe.iso`。
4. 不依赖公网 URL 的菜单才是真正离线可用菜单。

注意：netboot.xyz 自身的在线菜单可能继续访问公网。完全离线部署应使用自定义本地菜单和本地镜像。

## 开发规范

- API 层只处理请求和响应，业务逻辑放内部模块。
- 协议解析和业务选择逻辑分离。
- 新增配置必须同步默认值、校验、UI、文档。
- 新增 API 必须使用统一响应结构和中文错误提示。
- 新增前端页面必须复用现有设计风格。
- 文件路径和 URL 参数必须校验。
- 不提交 `data/`、`dist/`、`node_modules/`、数据库、日志、密钥和临时文件。

## 维护规范

- 修改 DHCP/TFTP/HTTP Boot 逻辑时，必须考虑 BIOS、UEFI、iPXE 和老旧 PXE 固件兼容性。
- 修改数据库结构时必须保证幂等迁移或兼容旧表。
- 修复 bug 时优先补测试或可复现日志。
- 重构只在明确范围内进行，不混入无关功能。
- 删除文件或接口前确认没有 UI、API、服务启动或流水线仍在使用。
- 发布前更新 README、docs 和本文件。

## 质量门禁

```bash
go test ./...
go vet ./...
npm run typecheck --prefix web
npm run build --prefix web
```

协议相关建议逐步补充：

- DHCP Options 53、54、60、66、67、77、93、97。
- PXE Option 43。
- iPXE 脚本 golden test。
- TFTP RRQ/WRQ。
- HTTP Range。
- 文件路径越界防护。

## 已知风险

- VirtualBox 桥接 Wi-Fi、部分交换机和 Windows 防火墙可能影响 DHCP/ProxyDHCP 广播。
- 完整 DHCP 模式可能与路由器 DHCP 冲突，生产环境必须谨慎使用。
- TFTP 依赖 UDP，丢包、MTU、blksize 会影响稳定性。
- netboot.xyz 下载依赖外网，离线环境需要提前准备文件。
