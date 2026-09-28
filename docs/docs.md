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
| internal/tftp | 文件传送和可选上传 |
| internal/httpboot | HTTP 文件传送、Range、健康报告 |
| internal/netboot | netboot.xyz 固件下载 |
| internal/smb | Windows 系统共享 |
| internal/observability | SSE 事件和日志 |
| internal/booturl、netutil、command、platform | 地址、网络、命令输出解码和平台工具 |
| web/src | Vue 管理页面 |
| embed.ipxe | 编译进自定义固件的启动脚本 |

## 状态与配置

TOML 仅包含 data、admin、database 三部分启动配置。服务设置保存在 SQLite settings 表。新数据库直接建立当前结构，不提供旧版本升级迁移或配置兼容逻辑。

应用启动先打开数据库与管理 Web，协议服务通过仪表盘显式启动。配置改变后需重启协议服务。启停操作串行执行；所有监听创建成功才报告启动完成，任一模块失败回滚此次启动。SMB 保存运行时配置供停止使用。下载固件与选择启动文件是独立动作。

普通 PXE 客户端按架构获得配置的固件；iPXE 客户端获得 HTTP /boot.ipxe。TFTP 与 HTTP 只读取实际文件，不生成动态启动脚本。固件自己的安装菜单属于固件内容，不受管理页面控制。

## API

管理 API 前缀为 /api/v1，初始化、登录入口之外的业务 API 需要会话认证。会话只存令牌 SHA-256 摘要，服务端有效期 24 小时；退出撤销当前会话，改密和删用户撤销关联会话。初始化通过原子 SQL 保证只创建一个首位管理员。提供状态、诊断、配置、服务启停、客户端、用户、文件、日志和固件下载。列表结果使用 []。

HTTP Boot 文件端口独立于管理端口，不要求浏览器登录，面向 PXE 网络。健康报告入口为 /client/report。

## DHCP 数据模型

clients.ip 只存管理员静态保留地址，observed_ip 只存最近观测地址；未知地址不会覆盖有效观测值。leases 保存带到期时间的 OFFER/ACK，地址唯一，并在事务内检查静态保留、基础设施地址与其他租约。OFFER 保留 60 秒，ACK 使用配置租期，DECLINE 隔离 10 分钟。重启服务不会丢失租约。

持久日志文件保留当前 10 MiB 和一个 10 MiB 备份；SQLite events 保留最近 5000 条，内存事件最多 1000 条。

## 构建

先运行 npm ci --prefix web 和 npm run build --prefix web，生成 internal/web/dist，随后 Go embed 将其嵌入应用。应用、Docker 和 iPXE 固件分别由独立工作流构建。

### Web 文件上传

`POST /api/v1/files/upload?root=http&path=images/example.iso` 要求登录、`Content-Type: application/octet-stream` 和准确的 `Content-Length`，请求体直接为文件内容。旧 multipart 接口已移除。上限由启动配置 `admin.max_upload_bytes` 控制，默认 32 GiB；列表接口返回 `max_upload_bytes` 供 UI 展示。上传直接写入目标目录的保留临时文件，完成并同步后原子创建目标硬链接，不覆盖同名文件；两个并发槽位限制磁盘压力。文本编辑上限仍为 1 MiB，固件下载仍为 64 MiB。
