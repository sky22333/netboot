# 项目结构与启动流程

## 模块

| 路径 | 职责 |
|---|---|
| cmd/pxe | 命令行参数、信号与浏览器打开 |
| internal/config | TOML 启动配置、运行目录 |
| internal/app | 管理 Web 和协议服务生命周期 |
| internal/web | 登录认证、管理 API、前端托管 |
| internal/storage | SQLite 数据、服务配置、静态绑定、观测地址、持久租约与账号 |
| internal/filetree | 共享文件路径命名空间，实际 IO 使用 os.Root 限制目录边界 |
| internal/dhcp | DHCP 地址分配、ProxyDHCP、按架构下发固件 |
| internal/tftp | 只读文件传送与选项协商 |
| internal/httpboot | HTTP 文件传送与 Range |
| internal/firmware | 项目与 netboot.xyz 固件目录、流式下载 |
| internal/smb | Windows 系统共享 |
| internal/observability | SSE 事件和日志 |
| internal/booturl、netutil、command、platform | 地址、网络、命令输出解码和平台工具 |
| web/src | Vue 管理页面 |
| embed.ipxe | 编译进自定义固件的启动脚本 |

## 状态与配置

TOML 仅包含 data、admin、database 三部分启动配置。服务设置保存在 SQLite settings 表。新数据库直接建立当前结构，不提供旧版本升级迁移或配置兼容逻辑。

应用启动先打开数据库与管理 Web，协议服务通过运行概览显式启动。配置改变后需重启协议服务。启停操作串行执行；所有监听创建成功才报告启动完成，任一模块失败回滚此次启动。SMB 仅支持 Windows，保存运行时配置供停止使用，并通过系统共享描述标记所属数据目录；再次启动会清理本实例遗留共享，不接管其他程序的共享。下载固件与选择启动文件是独立动作。

DHCP/ProxyDHCP 仅服务本地子网，不支持 DHCP Relay；每个请求只选一个响应目标，初始请求广播、续租按客户端地址单播。普通 PXE 客户端按架构获得配置的固件；已经运行的 iPXE 自行决定后续启动流程。完整 DHCP 模式仅提供地址等网络配置，不提供启动文件或 next-server；ProxyDHCP 不响应 iPXE 的第二阶段请求，但保留客户端观测记录。TFTP 与 HTTP 只读取实际文件，不生成动态启动脚本。固件自己的安装菜单属于固件内容，不受管理页面控制。

## API

管理 API 前缀为 /api/v1，初始化、登录入口之外的业务 API 需要会话认证。会话只存令牌 SHA-256 摘要，服务端有效期 24 小时；退出撤销当前会话，改密和删用户撤销关联会话。初始化通过原子 SQL 保证只创建一个首位管理员。提供状态、诊断、配置、服务启停、客户端、用户、文件、日志和固件下载。列表结果使用 []。

HTTP Boot 文件端口独立于管理端口，不要求浏览器登录，面向 PXE 网络。仅提供 GET/HEAD 文件读取，不接收设备报告。

## DHCP 数据模型

clients.ip 只存管理员静态保留地址，observed_ip 只存最近观测地址；未知地址不会覆盖有效观测值。leases 保存带到期时间的 OFFER/ACK，地址唯一，并在事务内检查静态保留、基础设施地址与其他租约。OFFER 保留 60 秒，ACK 使用配置租期，DECLINE 隔离 10 分钟。重启服务不会丢失租约。

持久日志文件保留当前 10 MiB 和一个 10 MiB 备份；SQLite events 保留最近 5000 条，内存事件最多 1000 条。事件统一生成 ID，历史列表和 SSE 使用相同格式与 ID；数据库读取失败直接报错。

## 构建

先运行 npm ci --prefix web 和 npm run build --prefix web，生成 internal/web/dist，随后 Go embed 将其嵌入应用。应用、Docker 和 iPXE 固件分别由独立工作流构建。

### Web 文件上传

`POST /api/v1/files/upload?root=http&path=images/example.iso` 要求登录、`Content-Type: application/octet-stream` 和准确的 `Content-Length`，请求体直接为文件内容。旧 multipart 接口已移除。上限由启动配置 `admin.max_upload_bytes` 控制，默认 32 GiB；列表接口返回 `max_upload_bytes` 供 UI 展示。上传直接写入目标目录的保留临时文件，完成并同步后原子创建目标硬链接，不覆盖同名文件；两个并发槽位限制磁盘压力。文本编辑上限仍为 1 MiB，固件下载仍为 64 MiB。

### 管理界面

Vue 3.5.43 + shadcn-vue 2.8.2 生成的 Reka Nova 组件，运行时依赖 Reka UI 2.10.5、Tailwind CSS 4.3.3。组件源码位于 `web/src/components/ui`，仅保留使用的组件；CLI 不进入项目依赖。主题使用本地系统字体，状态样式采用 Tailwind 原生属性选择器，无外部字体或 CSS 请求。导航与业务布局保持稳定，页面动态导入。

确认操作统一经过 `confirm.ts` 和 AlertDialog，页面刷新通过 `pageRefresh.ts` 等待实际请求；成功反馈使用 Sonner，错误保留在页面。文本编辑和配置修改离开页面前确认。日志使用有界浅响应数组及帧批处理，离开日志/概览页面断开 SSE。

依赖版本与 lockfile 固定。TypeScript 7.0.2 无法被当前 vue-tsc 3.3.11 加载，使用最新兼容稳定版本 6.0.3，不修改第三方包规避不兼容。

VueUse 使用 Reka UI 声明兼容的稳定版本 14.4.0，复用同一份运行时，避免同时打包两个主版本。

## 固件下载

`GET /api/v1/firmware` 只读取本地状态；`POST /api/v1/firmware/download` 接受来源 `project` 或 `netboot` 与目录中允许的文件名数组 `files`。项目固件直接通过 `https://github.com/sky22333/netboot/releases/latest/download/<文件名>` 下载并跟随重定向，不请求 GitHub API。两种来源统一保存到 TFTP 根目录，启动配置直接使用文件名。HTTP Boot 仅提供 HTTP 根目录内的文件。全局单任务、逐文件流式写入，单文件上限 64 MiB；完成长度检查与磁盘同步后替换同名文件，失败或取消清理临时文件并保留原文件。不会自动修改启动配置。

### 文件写入与账号操作

文本新建不覆盖已有文件；编辑使用读取时的内容版本，冲突返回 409，需重新读取再修改。文本保存和 Web 上传均先写同目录临时文件，完成长度检查与同步后再发布。文件及目录重命名使用系统原子“不覆盖”操作，目标已存在则失败；不支持该操作的文件系统直接报错，不降级覆盖。

批量设备创建使用一个事务，任一记录失败则整批回滚。账号列表可修改密码，修改后撤销该账号全部会话。
