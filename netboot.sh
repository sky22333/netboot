#!/bin/sh

set -eu

BIN_NAME="pxe"
REPO_URL="https://github.com/sky22333/netboot"
LATEST_DOWNLOAD_URL="$REPO_URL/releases/latest/download"
INSTALL_DIR="/usr/local/bin"
BIN_PATH="$INSTALL_DIR/$BIN_NAME"
APP_DIR="/opt/netboot"
DATA_DIR="$APP_DIR/data"
TMP_DIR="/tmp/netboot-install"
SERVICE_NAME="netboot"
SERVICE_USER="root"
ADMIN_HOST="0.0.0.0"
ADMIN_PORT="8088"

info() { printf '%s\n' "提示：$*"; }
ok() { printf '%s\n' "完成：$*"; }
warn() { printf '%s\n' "注意：$*"; }
err() { printf '%s\n' "错误：$*" >&2; }

need_root() {
    if [ "$(id -u)" != "0" ]; then
        err "请使用 root 运行此脚本"
        exit 1
    fi
}

pause() {
    printf '%s' "按回车继续..."
    read _ans || true
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

detect_pkg_manager() {
    if command_exists apk; then
        printf '%s\n' "apk"
    elif command_exists apt-get; then
        printf '%s\n' "apt"
    elif command_exists dnf; then
        printf '%s\n' "dnf"
    elif command_exists yum; then
        printf '%s\n' "yum"
    else
        printf '%s\n' "unknown"
    fi
}

install_packages() {
    pm="$(detect_pkg_manager)"
    case "$pm" in
        apk)
            apk add --no-cache ca-certificates tar curl >/dev/null
            ;;
        apt)
            apt-get update
            DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates tar curl
            ;;
        dnf)
            dnf install -y ca-certificates tar curl
            ;;
        yum)
            yum install -y ca-certificates tar curl
            ;;
        *)
            warn "未识别包管理器，请确认已安装 ca-certificates、tar、curl 或 wget"
            ;;
    esac
}

detect_init_system() {
    if command_exists systemctl && [ -d /run/systemd/system ]; then
        printf '%s\n' "systemd"
    elif command_exists rc-service && command_exists rc-update; then
        printf '%s\n' "openrc"
    else
        printf '%s\n' "unknown"
    fi
}

detect_asset() {
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)
            printf '%s\n' "pxe-linux-amd64.tar.gz"
            ;;
        aarch64|arm64)
            printf '%s\n' "pxe-linux-arm64.tar.gz"
            ;;
        armv7l|armv7|armhf)
            printf '%s\n' "pxe-linux-arm-armv7.tar.gz"
            ;;
        *)
            err "不支持的架构：$arch"
            exit 1
            ;;
    esac
}

download_file() {
    url="$1"
    out="$2"
    if command_exists curl; then
        curl -fL --connect-timeout 20 --retry 3 -o "$out" "$url"
    elif command_exists wget; then
        wget -O "$out" "$url"
    else
        err "缺少下载工具 curl 或 wget"
        return 1
    fi
}

download_release_file() {
    name="$1"
    out="$2"
    download_file "$LATEST_DOWNLOAD_URL/$name" "$out"
}

stop_service_if_exists() {
    init="$(detect_init_system)"
    case "$init" in
        systemd)
            if systemctl list-unit-files "$SERVICE_NAME.service" >/dev/null 2>&1; then
                systemctl stop "$SERVICE_NAME.service" >/dev/null 2>&1 || true
            fi
            ;;
        openrc)
            if [ -f "/etc/init.d/$SERVICE_NAME" ]; then
                rc-service "$SERVICE_NAME" stop >/dev/null 2>&1 || true
            fi
            ;;
    esac
}

write_systemd_service() {
    cat >"/etc/systemd/system/$SERVICE_NAME.service" <<EOF
[Unit]
Description=Netboot PXE Management Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$APP_DIR
ExecStart=$BIN_PATH --data-dir $DATA_DIR --host $ADMIN_HOST --port $ADMIN_PORT --no-browser
Restart=always
RestartSec=5
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME.service"
}

write_openrc_service() {
    cat >"/etc/init.d/$SERVICE_NAME" <<EOF
#!/sbin/openrc-run

name="Netboot PXE Management Service"
description="Netboot PXE Management Service"
command="$BIN_PATH"
command_args="--data-dir $DATA_DIR --host $ADMIN_HOST --port $ADMIN_PORT --no-browser"
command_user="$SERVICE_USER"
directory="$APP_DIR"
pidfile="/run/$SERVICE_NAME.pid"
command_background="yes"
supervisor="supervise-daemon"
respawn_delay="5"
respawn_max="0"

depend() {
    need net
    after firewall
}
EOF
    chmod +x "/etc/init.d/$SERVICE_NAME"
    rc-update add "$SERVICE_NAME" default
}

install_service() {
    init="$(detect_init_system)"
    case "$init" in
        systemd)
            write_systemd_service
            ok "已安装 systemd 服务并设为开机自启"
            ;;
        openrc)
            write_openrc_service
            ok "已安装 OpenRC 服务并设为开机自启"
            ;;
        *)
            err "未识别系统服务管理器，仅支持 systemd 和 OpenRC"
            exit 1
            ;;
    esac
}

start_service() {
    init="$(detect_init_system)"
    case "$init" in
        systemd)
            systemctl start "$SERVICE_NAME.service"
            ;;
        openrc)
            rc-service "$SERVICE_NAME" start
            ;;
        *)
            err "未识别系统服务管理器"
            return 1
            ;;
    esac
    ok "服务已启动，管理地址：http://本机IP:$ADMIN_PORT"
}

stop_service() {
    init="$(detect_init_system)"
    case "$init" in
        systemd)
            systemctl stop "$SERVICE_NAME.service"
            ;;
        openrc)
            rc-service "$SERVICE_NAME" stop
            ;;
        *)
            err "未识别系统服务管理器"
            return 1
            ;;
    esac
    ok "服务已停止"
}

status_service() {
    init="$(detect_init_system)"
    case "$init" in
        systemd)
            systemctl status "$SERVICE_NAME.service" --no-pager || true
            ;;
        openrc)
            rc-service "$SERVICE_NAME" status || true
            ;;
        *)
            err "未识别系统服务管理器"
            return 1
            ;;
    esac
}

remove_service() {
    init="$(detect_init_system)"
    case "$init" in
        systemd)
            systemctl stop "$SERVICE_NAME.service" >/dev/null 2>&1 || true
            systemctl disable "$SERVICE_NAME.service" >/dev/null 2>&1 || true
            rm -f "/etc/systemd/system/$SERVICE_NAME.service"
            systemctl daemon-reload
            ;;
        openrc)
            rc-service "$SERVICE_NAME" stop >/dev/null 2>&1 || true
            rc-update del "$SERVICE_NAME" default >/dev/null 2>&1 || true
            rm -f "/etc/init.d/$SERVICE_NAME"
            ;;
        *)
            warn "未识别系统服务管理器，跳过服务卸载"
            ;;
    esac
}

install_binary() {
    asset="$(detect_asset)"
    mkdir -p "$TMP_DIR" "$INSTALL_DIR" "$APP_DIR" "$DATA_DIR"
    rm -rf "$TMP_DIR"/*

    info "下载最新版本：$asset"
    download_release_file "$asset" "$TMP_DIR/$asset"

    info "解压程序"
    tar -xzf "$TMP_DIR/$asset" -C "$TMP_DIR"
    if [ ! -f "$TMP_DIR/pxe" ]; then
        err "压缩包中未找到 pxe 程序"
        exit 1
    fi
    cp "$TMP_DIR/pxe" "$BIN_PATH"
    chmod 0755 "$BIN_PATH"
    ok "程序已安装到 $BIN_PATH"
}

install_firmware() {
    mkdir -p "$APP_DIR"
    for fw in undionly.kpxe ipxe-x86_64.efi ipxe-arm64.efi; do
        info "下载启动固件：$fw"
        if download_release_file "$fw" "$APP_DIR/$fw"; then
            chmod 0644 "$APP_DIR/$fw"
        else
            warn "固件下载失败：$fw"
        fi
    done
    ok "启动固件已保存到 $APP_DIR，请按需复制或在面板中配置使用"
}

do_install() {
    need_root
    info "开始安装 netboot"
    install_packages
    stop_service_if_exists
    install_binary
    install_firmware
    install_service
    start_service
}

do_update() {
    need_root
    info "开始更新 netboot"
    install_packages
    stop_service_if_exists
    install_binary
    install_firmware
    install_service
    start_service
    ok "更新完成"
}

do_uninstall() {
    need_root
    printf '%s' "是否同时删除数据目录 $DATA_DIR？输入 y 确认："
    read confirm || confirm=""
    remove_service
    rm -f "$BIN_PATH"
    case "$confirm" in
        y|Y)
            rm -rf "$APP_DIR"
            ok "程序和数据已卸载"
            ;;
        *)
            ok "程序已卸载，数据已保留：$DATA_DIR"
            ;;
    esac
}

print_menu() {
    printf '\n'
    printf '%s\n' "netboot Linux 管理菜单"
    printf '%s\n' "1) 安装"
    printf '%s\n' "2) 启动"
    printf '%s\n' "3) 停止"
    printf '%s\n' "4) 更新"
    printf '%s\n' "5) 卸载"
    printf '%s\n' "6) 查看状态"
    printf '%s\n' "0) 退出"
    printf '%s' "请选择："
}

main_menu() {
    while :; do
        print_menu
        read choice || exit 0
        case "$choice" in
            1)
                do_install
                pause
                ;;
            2)
                need_root
                start_service
                pause
                ;;
            3)
                need_root
                stop_service
                pause
                ;;
            4)
                do_update
                pause
                ;;
            5)
                do_uninstall
                pause
                ;;
            6)
                status_service
                pause
                ;;
            0)
                exit 0
                ;;
            *)
                warn "无效选择"
                ;;
        esac
    done
}

case "${1:-menu}" in
    install)
        do_install
        ;;
    start)
        need_root
        start_service
        ;;
    stop)
        need_root
        stop_service
        ;;
    update)
        do_update
        ;;
    uninstall)
        do_uninstall
        ;;
    status)
        status_service
        ;;
    menu|"")
        main_menu
        ;;
    *)
        printf '%s\n' "用法：sh $0 [menu|install|start|stop|update|uninstall|status]"
        exit 1
        ;;
esac
