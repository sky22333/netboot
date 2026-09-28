# 项目结构与启动流程

## 模块

| 路径 | 职责 |
|---|---|
| cmd/pxe | 命令行参数、信号与浏览器打开 |
| internal/config | TOML 启动配置、运行目录 |
| internal/app | 管理 Web 和协议服务生命周期 |
| internal/web | 登录认证、管理 API、前端托管 |
| internal/storage | SQLite 数据、服务配置、客户端与账号 |
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

应用启动先打开数据库与管理 Web，协议服务通过仪表盘显式启动。配置改变后需重启协议服务。下载固件与选择启动文件是独立动作。

普通 PXE 客户端按架构获得配置的固件；iPXE 客户端获得 HTTP /boot.ipxe。TFTP 与 HTTP 只读取实际文件，不生成动态启动脚本。固件自己的安装菜单属于固件内容，不受管理页面控制。

## API

管理 API 前缀为 /api/v1，初始化、登录入口之外的业务 API 需要会话认证。提供状态、诊断、配置、服务启停、客户端、用户、文件、日志和固件下载。列表结果使用 []。

HTTP Boot 文件端口独立于管理端口，不要求浏览器登录，面向 PXE 网络。健康报告入口为 /client/report。

## 构建

先运行 npm ci --prefix web 和 npm run build --prefix web，生成 internal/web/dist，随后 Go embed 将其嵌入应用。应用、Docker 和 iPXE 固件分别由独立工作流构建。
