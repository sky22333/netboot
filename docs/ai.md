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
├─ embed.ipxe               # iPXE 固件内置脚本
├─ .github/workflows/
│  ├─ release.yml           # 应用本体多平台发布
│  └─ build-boot.yml        # iPXE 固件构建
├─ cmd/pxe/                 # 程序入口
├─ internal/
│  ├─ app/                  # 应用装配、服务生命周期
│  ├─ bootmenu/             # UEFI/iPXE 菜单 timeout 等共用逻辑
│  ├─ command/              # 服务器命令输出解码
│  ├─ config/               # pxe.toml 启动配置
│  ├─ dhcp/                 # DHCP、ProxyDHCP、租约和启动文件响应
│  ├─ httpboot/             # HTTP Boot、Range、dynamic.ipxe、netboot 虚拟路径
│  ├─ ipxe/                 # iPXE 脚本生成
│  ├─ netboot/              # netboot.xyz 下载
│  ├─ netutil/              # 广播地址、网卡广播等网络工具
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
-> TFTP 下载 BIOS/UEFI 可执行 iPXE 文件
-> 进入本项目 iPXE 动态菜单
-> 执行本地 boot.ipxe、netboot.xyz 或其他 HTTP 脚本
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
- ProxyDHCP 的 DISCOVER 返回 OFFER，REQUEST 返回 ACK，避免向 DISCOVER 发送不合规 ACK。
- 响应目标包含 `255.255.255.255:68`、按通告 IP/子网掩码计算的定向广播和必要时的客户端单播。
- BIOS 启动文件优先级为 `netboot/netboot.xyz.kpxe`、`netboot/netboot.xyz-undionly.kpxe`、服务配置里的 BIOS 默认启动文件。
- UEFI 启动文件按架构选择：IA32 使用 `boot_files.uefi_ia32`，x64 优先 `netboot/netboot.xyz.efi` 再回退 `boot_files.uefi_x64`，ARM32 使用 `boot_files.uefi_arm32`，ARM64 优先 `netboot/netboot.xyz-arm64.efi` 再回退 `boot_files.uefi_arm64`。其中 IA32/ARM32 默认留空，表示需要用户自备。
- 不再提供 BIOS 原生菜单；老式 BIOS 默认加载可执行 iPXE 文件后进入 iPXE 动态菜单。
- iPXE 动态菜单默认第一项为 `Run boot.ipxe`，执行 `data/boot/http/boot.ipxe`。
- UEFI 原生菜单和 iPXE 动态菜单共用 `internal/bootmenu` 计算 timeout；启用随机等待时每次生成菜单都会得到 0 到配置秒数之间的随机值。
- 下发给 PXE/iPXE 客户端的菜单标题和菜单项使用英文/ASCII，避免固件控制台乱码。
- netboot.xyz 下载完成后会按需生成 `data/boot/tftp/local-vars.ipxe`；文件已存在时不覆盖。该脚本提供英文菜单，可从公网镜像或通告 IP 对应的内网 HTTP 路径启动 Debian 12 和 Alpine Linux，内网地址会自动使用当前 HTTP Boot 端口。

## iPXE 固件构建链路

仓库根目录的 `embed.ipxe` 是给 iPXE 固件编译使用的内置脚本，不是运行时动态生成脚本。当前脚本行为：

- 设置 `menu-timeout=60000`。
- 设置公网镜像 `https://mirrors.tuna.tsinghua.edu.cn`。
- 菜单包含 `Public Install Debian 12`、`Public Install Alpine Linux`、`iPXE Shell` 和 `Continue netboot.xyz`。
- Debian 和 Alpine 项直接从公网镜像加载 kernel/initrd。
- 启动失败后进入 iPXE shell，退出项执行 `exit`。

`.github/workflows/build-boot.yml` 是独立的 iPXE 固件构建流水线：

- 手动触发，可选 `tag_name`；非空时会发布到 GitHub Release。
- 下载 iPXE v2.0.0 源码。
- 修改 `src/config/general.h`，开启 `DOWNLOAD_PROTO_HTTPS`。
- 不传入 `TRUST=` 自定义 CA bundle，使用 iPXE 默认公共 CA/crosscert 信任机制；公网 HTTPS 优先走该机制，私有 CA 或完全离线 HTTPS 需要单独拆分并注入证书链。
- 复制仓库根目录 `embed.ipxe`，构建时通过 `EMBED=embed.ipxe` 注入。
- 输出 `undionly.kpxe`、`ipxe-x86_64.efi`、`ipxe-arm64.efi`。

这条链路只负责生成第一阶段 iPXE 固件，不会启动服务，也不会自动写入 `data/boot/netboot`。产物需要由部署者按客户端架构放到运行时目录，或改名为当前 DHCP 选文件逻辑会下发的文件名。

需要明确区分三类脚本：

- `embed.ipxe`：编译进固件，固件启动后立即执行；修改后必须重新构建固件。
- `data/boot/tftp/local-vars.ipxe`：netboot.xyz 页面按需生成的本地变量/钩子脚本，已存在时不覆盖。
- `/dynamic.ipxe`：HTTP Boot 或管理 Web 在运行时生成的动态菜单脚本，依赖数据库菜单、客户端和服务配置。

当前 ARM64 状态：流水线能构建 `ipxe-arm64.efi`，运行时 netboot.xyz 下载列表包含 `netboot.xyz-arm64.efi`，DHCP 代码会把 option 93 的 ARM64 UEFI 架构值映射到 `uefi_arm64`，不会再把 ARM64 客户端导向 x64 的 `netboot.xyz.efi`。

## PXE 客户端网络交互细节

本节按当前代码实现描述客户端实际会看到的网络行为，主要用于排查固件兼容性问题。入口在 `internal/app.StartServices`，协议处理集中在 `internal/dhcp`、`internal/tftp`、`internal/httpboot` 和 `internal/ipxe`。

### 服务启动和监听端口

启用服务时，`internal/app` 从 SQLite 读取服务配置，然后按开关启动协议服务：

- HTTP Boot：监听 `settings.HTTPBoot.Addr`，通常是 TCP 80 或自定义端口。
- TFTP：监听 `settings.Server.ListenIP:69`。
- Torrent tracker：监听 `settings.Torrent.Addr`。
- 完整 DHCP：监听 `settings.Server.ListenIP:67`。
- ProxyDHCP：同时监听 `settings.Server.ListenIP:67` 和 `settings.Server.ListenIP:4011`。

ProxyDHCP 同时监听 67 和 4011 是为了兼容不同固件。有些 PXE 固件只在普通 DHCP 交换中读取启动信息，有些会继续向 ProxyDHCP 4011 请求启动信息。复杂网络中如果 DHCP relay 或防火墙没有转发 4011，只有同网段客户端最容易成功。

### DHCP 请求解析

`internal/dhcp.buildResponse` 只处理 DHCP message type 1 和 3：

- `1`：Discover。
- `3`：Request。
- `4` 和 `7`：Decline/Release，会释放完整 DHCP 租约，不返回启动响应。

代码会解析以下关键 options：

- Option 53：DHCP message type。
- Option 50：客户端请求的 IP。
- Option 54：DHCP server identifier；完整 DHCP 模式下如果 Request 指向别的 DHCP server，会忽略。
- Option 60：vendor class；包含 `PXEClient` 或 `iPXE` 会影响识别。
- Option 77：user class；iPXE 常见值为 `iPXE`。
- Option 93：PXE client architecture，用于区分 BIOS、UEFI IA32、UEFI x64、UEFI ARM32、UEFI ARM64。
- Option 97：client machine identifier，PXE 客户端常带。
- Option 175：iPXE encapsulated options；存在时按 iPXE 处理，并可检测 HTTP 等 feature。

如果不是 PXE/iPXE 客户端，并且完整 DHCP 模式下 `non_pxe_action=network_only`，代码只返回普通网络配置，不返回启动文件。ProxyDHCP 模式下普通 DHCP 客户端会被忽略，避免影响非启动设备。

### 完整 DHCP 交互

完整 DHCP 模式会维护内存租约池：

```text
DHCPDISCOVER -> 分配/预留地址 -> DHCPOFFER
DHCPREQUEST  -> 确认地址       -> DHCPACK
```

完整 DHCP 响应包含：

- `yiaddr`：分配给客户端的 IP。
- `siaddr`：启动服务器 IP，默认是通告 IP，菜单项选择时可使用菜单项服务器 IP。
- Option 1：子网掩码。
- Option 3：网关。
- Option 6：DNS。
- Option 51/58/59：租约、续租、重新绑定时间。
- Option 54：server identifier。
- Option 60：`PXEClient`，仅启动响应包含。
- Option 66：TFTP server name，有 bootfile 时写入。
- Option 67：bootfile name，有 bootfile 时写入。
- BOOTP header file 字段：同样写入 bootfile，但超过 127 字节会截断；短 TFTP 文件名最稳。

完整 DHCP 适合隔离网络。如果同网段已有路由器 DHCP，同时启用完整 DHCP 会导致地址冲突或客户端随机选择 DHCP server。

### ProxyDHCP 交互

ProxyDHCP 不分配 IP，只提供启动信息：

```text
客户端从现有 DHCP 获取 IP
客户端 PXE 请求启动信息
本程序返回 next-server、bootfile、PXE options
```

当前实现中：

- Discover 返回 DHCPOFFER。
- Request 返回 DHCPACK。
- 不再对同一个 Discover 额外发送 ACK。
- port 67 的 proxy 响应会尝试广播到客户端端口 68。
- port 4011 的 proxy 响应会优先回发到请求来源地址。

响应目标由 `sendResponse` 组合：

- `255.255.255.255:68`。
- 根据通告 IP 和子网掩码计算出的定向广播，例如 `192.168.1.255:68`。
- 如果请求包中有 `ciaddr` 或 option 50，则尝试单播到 `clientIP:68`。
- ProxyDHCP 下还会回发到 UDP `remote`。

这种多目标发送提高了同网段兼容性，但实际是否能到达客户端仍取决于操作系统广播权限、防火墙、交换机、DHCP relay 和固件监听行为。

### 架构识别和启动文件选择

架构识别由 option 93 决定：

```text
缺失或 0  -> bios
6         -> uefi_ia32
7 或 9    -> uefi_x64
10        -> uefi_arm32
11        -> uefi_arm64
其他      -> bios
```

启动文件选择在 `executableBootFile`：

- UEFI IA32：下发 `settings.BootFiles.UEFIIA32`；默认留空时不会伪造不存在的文件名。
- UEFI x64：如果 `data/boot/netboot/netboot.xyz.efi` 存在，优先下发 `netboot/netboot.xyz.efi`，否则回退到 `settings.BootFiles.UEFIX64`。
- UEFI ARM32：下发 `settings.BootFiles.UEFIARM32`；默认留空时不会伪造不存在的文件名。
- UEFI ARM64：如果 `data/boot/netboot/netboot.xyz-arm64.efi` 存在，优先下发 `netboot/netboot.xyz-arm64.efi`，否则回退到 `settings.BootFiles.UEFIARM64`。
- BIOS：优先 `netboot/netboot.xyz.kpxe`，再 `netboot/netboot.xyz-undionly.kpxe`，最后 `settings.BootFiles.BIOS`。

启动文件候选由 `bootFileCandidates` 统一维护，DHCP 主流程只调用 `executableBootFile`，避免多处重复判断。

### iPXE 二阶段识别

代码把以下情况识别为 iPXE：

- Option 77 包含 `iPXE`。
- Option 60 包含 `iPXE`。
- Option 175 存在。

识别为 iPXE 后，如果 option 175 中检测到 feature `0x13`，会返回：

```text
http://通告IP[:HTTP端口]/dynamic.ipxe?bootfile=ipxemenu
```

如果没有检测到 HTTP feature，则回退到普通可执行启动文件，避免向不支持 HTTP 的 iPXE 下发 HTTP chain。这个策略偏保守；如果某些 iPXE 构建实际支持 HTTP 但没有正确上报 feature，可能不会进入 HTTP 动态菜单。

iPXE 客户端状态会写为 `ipxe`，普通 PXE 启动阶段写为 `pxe`。客户端表中的状态来自 DHCP 请求，不代表系统已经成功启动。

### UEFI 原生 PXE 菜单

UEFI 原生菜单使用 PXE Option 43，生成逻辑在 `internal/pxeopt`。当前行为：

- 只对 UEFI 非 iPXE 客户端考虑原生菜单。
- 如果请求 option 43 中包含 selected type，会按菜单项 `pxe_type` 找到启动项并返回其 `boot_file`。
- 菜单项的 `server_ip` 会写入 `siaddr` 和 option 66。
- 默认 UEFI 菜单项为 `iPXE UEFI x64 -> ipxe-x86_64.efi`，以及 `Boot Local Disk -> 空 boot_file`。
- UEFI 原生菜单默认关闭；完整 DHCP 在默认状态下直接按架构下发 EFI。
- 完整 DHCP 模式且手动启用 UEFI 原生菜单时，会返回 Option 43 菜单。
- ProxyDHCP 模式下不返回 UEFI 原生菜单，而是直接下发启动文件。

实际经验上，UEFI 固件对 PXE Option 43 菜单支持差异较大。项目默认不把原生 UEFI 菜单作为主要兼容路径；更稳的做法是直接按架构加载 iPXE，再使用 HTTP 动态菜单。

### TFTP 第一阶段

TFTP 由 `internal/tftp` 提供：

- 初始监听 UDP 69。
- 每个 RRQ/WRQ 使用临时 UDP 端口继续传输。
- 支持 `blksize` 和 `tsize` OACK。
- 如果客户端不确认 OACK，会回退到标准 512 字节块。
- `blksize` 最大压到 1428，降低 MTU 分片风险。
- 读取路径限制在 TFTP root；`netboot/...` 会映射到 netboot 下载目录。

TFTP 适合传第一阶段小文件，如 `.kpxe`、`.efi`、`.ipxe`。大镜像应走 HTTP Boot。老旧固件可能对 OACK、临时端口、丢包重传更敏感，排障时可降低 block size 或回退默认设置。

TFTP 中存在虚拟 iPXE 脚本：

- 请求 `boot.ipxe`、`dynamic.ipxe`、`ipxemenu.ipxe` 时，如果实际文件不存在，代码会生成脚本。
- 脚本会优先 chain HTTP `/dynamic.ipxe?bootfile=ipxemenu`。
- HTTP 不可用时回退尝试 TFTP；ARM64 iPXE 优先 `netboot/netboot.xyz-arm64.efi`，其他 UEFI 使用 `netboot/netboot.xyz.efi`，BIOS 使用 `netboot/netboot.xyz.kpxe` 或 `netboot/netboot.xyz-undionly.kpxe`。
- 最后尝试本地磁盘启动。

### HTTP Boot 和动态 iPXE

HTTP Boot 服务由 `internal/httpboot` 提供：

- `/dynamic.ipxe`：生成 iPXE 动态脚本。
- `/client/report`：接收客户端健康报告。
- `/netboot/...`：映射到 netboot 下载目录。
- 其他路径：读取 HTTP Boot root。
- 支持 `Range` 时使用 `http.ServeContent`，适合内核、initrd 和大文件。

管理 Web 也暴露 `/dynamic.ipxe`，但实际客户端通常通过 HTTP Boot 服务访问。动态脚本由 `internal/ipxe.Generator` 生成：

- 空 bootfile 或 `ipxemenu`：生成 iPXE 菜单。
- `%dynamicboot%=boot.ipxe`：转换为 HTTP `/dynamic.ipxe?bootfile=boot.ipxe`。
- 相对路径：从 HTTP Boot root 通过 HTTP chain。
- `http://` 或 `https://`：直接 chain。
- 空 boot file：尝试本地磁盘 `sanboot --drive 0x80`。

这不是原生 UEFI HTTP Boot 模式。项目仍以 TFTP 加载第一阶段 iPXE/EFI 文件，再由 iPXE 使用 HTTP。

### 常见固件失败点

- Secure Boot 开启：DHCP/TFTP 正常，但未签名的 iPXE/netboot EFI 可能被固件拒绝。
- UEFI IA32/ARM32：代码能识别并有独立配置字段，但默认不下载 netboot.xyz 对应文件；需要自行准备匹配架构的 EFI。
- 只支持原生 UEFI HTTP Boot 的环境：项目没有 DHCP 下发 HTTP URL 的独立模式。
- Wi-Fi PXE：大多数家用无线网卡固件不支持预启动 PXE。
- USB 网卡 PXE：依赖 BIOS/UEFI 对该 USB 网卡的内置支持，兼容性弱于板载有线网卡。
- ProxyDHCP 支持不完整的固件：可尝试完整 DHCP 模式验证。
- 跨 VLAN：需要 DHCP relay/ip helper 正确转发 67、68、4011，并允许 TFTP/HTTP 到服务端。
- 防火墙：Windows、Linux nftables/iptables、OpenWrt 防火墙都可能阻断 UDP 67/69/4011 或 TCP HTTP Boot 端口。

### 维护建议

- 修改 DHCP 响应前，先确认 option 53、54、60、66、67、77、93、97、175 的语义。
- 修改 boot file 选择时，必须考虑 BIOS、UEFI IA32、UEFI x64、UEFI ARM32、UEFI ARM64 的文件架构匹配，并保持 `bootFileCandidates`、前端配置类型和文档一致。
- 修改 `embed.ipxe` 或 iPXE 固件产物名称时，必须同步运行时文档，避免部署者把固件内置菜单、netboot.xyz 本地钩子和 `/dynamic.ipxe` 误认为同一条链路。
- 不要把 UEFI Option 43 菜单当作唯一菜单能力；iPXE 动态菜单是主路径。
- 增加新固件兼容策略时，应补 DHCP 响应 golden test 或抓包说明。
- 排查客户端不响应时，优先抓包看客户端是否收到 `yiaddr/siaddr/filename/option66/option67`，以及是否继续发 TFTP RRQ。

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
- 文件管理覆盖 HTTP Boot、TFTP 和 netboot.xyz 三个目录；目录列表只读取当前目录，不递归扫描。在线编辑仅允许小型 UTF-8 文本文件，镜像、固件和压缩包不读取到浏览器。
- 文件管理必须做路径限制和危险操作确认。
- 完整 DHCP、删除、上传、外部下载等高风险操作必须明确提示。
- 客户端列表操作按钮需要保持横向文字显示；紧凑表格使用固定操作列和不换行按钮。
- netboot.xyz 页面展示来源、保存位置、本地钩子、每个启动文件状态和下载结果。
- 操作菜单模板由后端根据服务器平台生成；命令输出通过 `internal/command` 解码，Windows GBK/GB18030 输出需要正确显示中文。

## 安全要求

- 默认管理端监听 `127.0.0.1`。
- 首次使用创建管理员账号。
- 用户名限制为 3-32 位 ASCII 字符，只允许字母、数字、点、下划线、短横线和 `@`。
- 登录失败按来源 IP 和用户名限流，10 分钟内失败 10 次会锁定 10 分钟。
- 远程管理必须启用认证。
- Cookie 使用 HttpOnly 和 SameSite。
- 服务配置保存必须保留 `security`、`boot_files`、`netboot_xyz` 等完整 section，避免页面外调用截断配置导致认证关闭或启动文件丢失。
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
mkdir -p dist
go build -trimpath -ldflags="-s -w" -o dist/pxe ./cmd/pxe
```

Windows：

```powershell
cd pxe
npm ci --prefix web
npm run build --prefix web
go test ./...
go vet ./...
New-Item -ItemType Directory -Force -Path dist | Out-Null
go build -trimpath -ldflags="-s -w" -o dist\pxe.exe .\cmd\pxe
```

生产构建必须保留 `-trimpath -ldflags="-s -w"`，用于减小单文件体积并避免暴露本地构建路径。

GitHub Actions：

- `.github/workflows/release.yml` 手动触发。
- 构建 Windows、Linux、macOS 多平台二进制。
- 上传 zip/tar.gz。
- `.github/workflows/build-boot.yml` 手动构建 iPXE 固件，输出 `undionly.kpxe`、`ipxe-x86_64.efi`、`ipxe-arm64.efi`。

## 离线自托管

完全离线时：

1. 老式 BIOS 推荐把官方 `undionly.kpxe` 改名为 `netboot.xyz-undionly.kpxe`，放到 `data/boot/netboot`。
2. 把 `boot.ipxe`、Linux 内核、initrd 和自动安装配置放到 `data/boot/http`。
3. iPXE 菜单使用 `%dynamicboot%=boot.ipxe` 执行本地启动脚本，默认显示为 `Run boot.ipxe`。
4. 不依赖公网 URL 的脚本才是真正离线可用脚本。

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
- 修改 Wake-on-LAN 时必须保持多目标发送策略：UDP 9/7、全局广播、定向广播、客户端单播和本机网卡广播。
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
