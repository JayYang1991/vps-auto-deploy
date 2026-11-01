#!/usr/bin/env bash
set -e

# ==========================================================
# VPS Auto Deploy - Ubuntu 一键安装与配置脚本
# ==========================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}[错误] 请使用 root 权限执行此脚本: sudo bash $0${NC}"
  exit 1
fi

echo -e "${GREEN}==========================================================${NC}"
echo -e "${GREEN}      开始安装 VPS Auto Deploy 面板 (Ubuntu)              ${NC}"
echo -e "${GREEN}==========================================================${NC}"

# 1. 架构检测
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)
    GOARCH="amd64"
    ;;
  aarch64|arm64)
    GOARCH="arm64"
    ;;
  *)
    echo -e "${RED}[错误] 不受支持的 CPU 架构: $ARCH${NC}"
    exit 1
    ;;
esac

INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/vps-auto-deploy"
SYSTEMD_FILE="/etc/systemd/system/vps-panel.service"

mkdir -p "$CONFIG_DIR"
mkdir -p "$INSTALL_DIR"

# 2. 安装二进制文件
# 若当前目录下存在本地编译好的 bin/vps-panel 或 ./vps-panel 则直接使用，否则从 GitHub Release 下载
if [ -f "./bin/vps-panel" ]; then
  echo -e "${BLUE}[*] 发现本地 bin/vps-panel，执行安装...${NC}"
  cp ./bin/vps-panel "$INSTALL_DIR/vps-panel"
elif [ -f "./vps-panel" ]; then
  echo -e "${BLUE}[*] 发现本地 vps-panel，执行安装...${NC}"
  cp ./vps-panel "$INSTALL_DIR/vps-panel"
else
  echo -e "${BLUE}[*] 正在从 GitHub Release 下载最新版本 (${GOARCH})...${NC}"
  LATEST_TAG=$(curl -fsSL https://api.github.com/repos/vps-auto-deploy/vps-panel/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || echo "v1.0.0")
  DOWNLOAD_URL="https://github.com/vps-auto-deploy/vps-panel/releases/download/${LATEST_TAG}/vps-panel-linux-${GOARCH}"
  curl -fsSL "$DOWNLOAD_URL" -o "$INSTALL_DIR/vps-panel" || {
    echo -e "${RED}[错误] 下载失败，请检查网络或确认 Release 是否发布。${NC}"
    exit 1
  }
fi

chmod +x "$INSTALL_DIR/vps-panel"

# 3. 创建 Systemd 服务文件
cat <<EOF > "$SYSTEMD_FILE"
[Unit]
Description=VPS Auto Deploy Control Plane Panel
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=$CONFIG_DIR
ExecStart=$INSTALL_DIR/vps-panel --port 20888 --db $CONFIG_DIR/data.db
Restart=always
RestartSec=5s
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

# 4. 重载并启动服务
systemctl daemon-reload
systemctl enable vps-panel
systemctl restart vps-panel

sleep 2

# 5. 读取日志中的访问路径
echo -e "${GREEN}==========================================================${NC}"
echo -e "${GREEN} [✓] 安装完成！服务已在后台保持自启运行 (systemd: vps-panel)${NC}"
echo -e "${GREEN}==========================================================${NC}"
journalctl -u vps-panel -n 15 --no-pager
echo -e "${GREEN}==========================================================${NC}"
echo -e "${YELLOW}常用命令:${NC}"
echo -e " 查看状态:   systemctl status vps-panel"
echo -e " 查看实时日志: journalctl -u vps-panel -f"
echo -e " 重启服务:   systemctl restart vps-panel"
echo -e " 重置密码:   $INSTALL_DIR/vps-panel --db $CONFIG_DIR/data.db --reset-password <新密码>"
echo -e "${GREEN}==========================================================${NC}"

