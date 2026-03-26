#!/bin/bash
set -e

echo "=========================================="
echo "欢迎使用 BetterLan 云端节点一键安装程序"
echo "=========================================="
echo ""

URL_AMD64="https://github.com/KDronin/BetterLAN-server/releases/download/v1.0.0/betterlan-server-linux-amd64"
URL_ARM64="https://github.com/KDronin/BetterLAN-server/releases/download/v1.0.0/betterlan-server-linux-arm64"
URL_386="https://github.com/KDronin/BetterLAN-server/releases/download/v1.0.0/betterlan-server-linux-386"

ARCH=$(uname -m)
case "$ARCH" in
    x86_64)
        DOWNLOAD_URL=$URL_AMD64
        ARCH_NAME="amd64 (x86_64)"
        ;;
    aarch64 | armv8l)
        DOWNLOAD_URL=$URL_ARM64
        ARCH_NAME="arm64 (aarch64)"
        ;;
    i386 | i686)
        DOWNLOAD_URL=$URL_386
        ARCH_NAME="386 (x86 32-bit)"
        ;;
    *)
        echo "错误: 不支持的系统架构 ($ARCH)。当前仅支持 amd64, arm64, 386。"
        exit 1
        ;;
esac

echo "[+] 识别到您的系统架构为: $ARCH_NAME"
echo ""

read -p "请输入节点绑定的 IP 地址 [默认 0.0.0.0]: " USER_IP </dev/tty
if [ -z "$USER_IP" ]; then
    USER_IP="0.0.0.0"
fi

read -p "请输入节点端口 [默认 45678]: " USER_PORT </dev/tty
if [ -z "$USER_PORT" ]; then
    USER_PORT=45678
fi

echo ""
echo "配置信息确认:"
echo "绑定 IP: $USER_IP"
echo "绑定端口: $USER_PORT"
echo "正在部署..."
echo ""

INSTALL_DIR="/opt/betterlan-server"
mkdir -p $INSTALL_DIR
cd $INSTALL_DIR

cat > config.json << EOF
{
  "ip": "$USER_IP",
  "port": $USER_PORT
}
EOF
echo "[+] 配置文件 config.json 生成完毕"

echo "[+] 正在下载服务端程序 ($ARCH_NAME)..."
if ! wget -qO betterlan-server "$DOWNLOAD_URL"; then
    echo "下载失败！请检查网络。"
    exit 1
fi

chmod +x betterlan-server
echo "[+] 权限设置完毕"

echo "[+] 正在配置 Systemd 守护进程..."
cat > /etc/systemd/system/betterlan.service << EOF
[Unit]
Description=BetterLan Relay Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/betterlan-server
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable betterlan >/dev/null 2>&1
systemctl restart betterlan

echo "=========================================="
echo "安装完成！"
echo "服务已在后台静默运行，并已设置开机自启。"
echo ""
echo "查看状态: systemctl status betterlan"
echo "查看日志: journalctl -u betterlan -f"
echo "停止服务: systemctl stop betterlan"
echo "重启服务: systemctl restart betterlan"
echo "卸载节点: rm -rf $INSTALL_DIR /etc/systemd/system/betterlan.service && systemctl daemon-reload"
echo "=========================================="
