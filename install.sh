#!/bin/bash
INSTALL_DIR="/opt/betterlan-server"
SERVICE_FILE="/etc/systemd/system/betterlan.service"
DOWNLOAD_URL="https://github.com/KDronin/BetterLAN-server/releases/download/v0.0.1/betterlan-server" 

echo "=========================================="
echo "欢迎使用 BetterLan 云端节点一键安装程序"
echo "=========================================="
echo ""

read -p "输入节点绑定的 IP 地址 [默认 0.0.0.0]: " USER_IP
USER_IP=${USER_IP:-0.0.0.0}

read -p "输入自定义节点端口 [默认 45678]: " USER_PORT
USER_PORT=${USER_PORT:-45678}

echo ""
echo "配置信息确认: "
echo "绑定 IP: $USER_IP"
echo "绑定端口: $USER_PORT"
echo "正在为您执行自动化部署..."
echo ""

mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR" || exit

cat > config.json << EOF
{
  "ip": "$USER_IP",
  "port": $USER_PORT
}
EOF
echo "[+] 配置文件 config.json 生成完毕"

echo "[+] 正在下载服务端程序..."
curl -s -o betterlan-server -L "$DOWNLOAD_URL"

if [ ! -s betterlan-server ]; then
    echo "下载失败！请检查网络或通知作者。"
    exit 1
fi

chmod +x betterlan-server
echo "[+] 服务端权限设置完毕"

echo "[+] 正在配置 Systemd 守护进程..."
cat > "$SERVICE_FILE" << EOF
[Unit]
Description=BetterLan Relay Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/betterlan-server
Restart=on-failure
RestartSec=3
StandardOutput=syslog
StandardError=syslog

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable betterlan.service
systemctl restart betterlan.service

echo "=========================================="
echo "安装完成！"
echo "服务已在后台静默运行，并已设置开机自启。"
echo ""
echo "您可以使用以下命令管理服务："
echo "查看状态: systemctl status betterlan"
echo "查看日志: journalctl -u betterlan -f"
echo "停止服务: systemctl stop betterlan"
echo "重启服务: systemctl restart betterlan"
echo "=========================================="
