#!/usr/bin/env bash
set -e

# ==========================================================
# VPS Auto Deploy - 一键部署管理服务端脚本 (支持 Root 与非 Root 用户空间)
# ==========================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${GREEN}==========================================================${NC}"
echo -e "${GREEN}      开始部署 VPS Auto Deploy 服务端                     ${NC}"
echo -e "${GREEN}==========================================================${NC}"

# 1. 运行权限与环境检测
if [ "$EUID" -eq 0 ]; then
  IS_ROOT=1
  INSTALL_DIR="/usr/local/bin"
  CONFIG_DIR="/etc/vps-auto-deploy"
  SYSTEMD_DIR="/etc/systemd/system"
  SYSTEMD_FILE="$SYSTEMD_DIR/vps-panel.service"
  SYSTEMCTL_CMD="systemctl"
  JOURNALCTL_CMD="journalctl"
  WANTED_TARGET="multi-user.target"
  USER_LINE="User=root"
  echo -e "${BLUE}[*] 运行模式: 系统级 (Root 生产模式)${NC}"
else
  IS_ROOT=0
  INSTALL_DIR="$HOME/.local/bin"
  CONFIG_DIR="$HOME/.config/vps-auto-deploy"
  SYSTEMD_DIR="$HOME/.config/systemd/user"
  SYSTEMD_FILE="$SYSTEMD_DIR/vps-panel.service"
  SYSTEMCTL_CMD="systemctl --user"
  JOURNALCTL_CMD="journalctl --user"
  WANTED_TARGET="default.target"
  USER_LINE=""
  echo -e "${BLUE}[*] 运行模式: 用户空间模式 (非 Root / 当前用户: $USER)${NC}"
  # 开启用户持久化守护 (即使退出 shell 仍保持后台运行)
  loginctl enable-linger "$USER" 2>/dev/null || true
fi

# 2. 架构检测
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

mkdir -p "$CONFIG_DIR"
mkdir -p "$INSTALL_DIR"
mkdir -p "$SYSTEMD_DIR"

# 3. 安装二进制文件
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || pwd)"
FOUND_LOCAL=0

install_binary() {
  local src="$1"
  echo -e "${BLUE}[*] 正在安装二进制文件: $src -> $INSTALL_DIR/vps-panel...${NC}"
  cp -f "$src" "$INSTALL_DIR/vps-panel.tmp"
  chmod +x "$INSTALL_DIR/vps-panel.tmp"
  mv -f "$INSTALL_DIR/vps-panel.tmp" "$INSTALL_DIR/vps-panel"
  FOUND_LOCAL=1
}

# 优先探测解压到同级目录或本地编译产物
if [ -f "$SCRIPT_DIR/vps-panel" ]; then
  install_binary "$SCRIPT_DIR/vps-panel"
elif [ -f "$SCRIPT_DIR/bin/vps-panel" ]; then
  install_binary "$SCRIPT_DIR/bin/vps-panel"
elif [ -f "./vps-panel" ]; then
  install_binary "./vps-panel"
elif [ -f "./bin/vps-panel" ]; then
  install_binary "./bin/vps-panel"
fi

if [ "$FOUND_LOCAL" -eq 0 ]; then
  echo -e "${BLUE}[*] 未在本地找到二进制，正在从 GitHub Release 获取最新版本 (${GOARCH})...${NC}"
  LATEST_TAG=$(curl -fsSL https://api.github.com/repos/JayYang1991/vps-auto-deploy/releases/latest 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || echo "v1.0.0")
  
  TMP_DIR=$(mktemp -d)
  DOWNLOAD_OK=0

  # 1. 尝试下载 Release 压缩包 (首选格式)
  TAR_URLS=(
    "https://github.com/JayYang1991/vps-auto-deploy/releases/download/${LATEST_TAG}/vps-panel_${LATEST_TAG}_linux_${GOARCH}.tar.gz"
    "https://github.com/JayYang1991/vps-auto-deploy/releases/download/${LATEST_TAG}/vps-panel_linux_${GOARCH}.tar.gz"
  )

  for url in "${TAR_URLS[@]}"; do
    echo -e "${BLUE}[*] 尝试下载压缩包: $url ...${NC}"
    if curl -fsSL -L "$url" -o "$TMP_DIR/release.tar.gz" 2>/dev/null; then
      tar -zxvf "$TMP_DIR/release.tar.gz" -C "$TMP_DIR" >/dev/null 2>&1
      if [ -f "$TMP_DIR/vps-panel" ]; then
        install_binary "$TMP_DIR/vps-panel"
        DOWNLOAD_OK=1
        break
      fi
    fi
  done

  # 2. 若压缩包不存在，尝试下载裸二进制
  if [ "$DOWNLOAD_OK" -eq 0 ]; then
    BIN_URL="https://github.com/JayYang1991/vps-auto-deploy/releases/download/${LATEST_TAG}/vps-panel-linux-${GOARCH}"
    echo -e "${BLUE}[*] 尝试直接下载二进制文件: $BIN_URL ...${NC}"
    if curl -fsSL -L "$BIN_URL" -o "$TMP_DIR/vps-panel" 2>/dev/null; then
      install_binary "$TMP_DIR/vps-panel"
      DOWNLOAD_OK=1
    fi
  fi

  rm -rf "$TMP_DIR"

  if [ "$DOWNLOAD_OK" -eq 0 ]; then
    echo -e "${RED}[错误] 无法从 GitHub 下载可用发布版本，请检查网络连接或执行 ./build.sh 本地编译打包。${NC}"
    exit 1
  fi
fi

# 4. 创建 Systemd 服务文件
cat <<EOF > "$SYSTEMD_FILE"
[Unit]
Description=VPS Auto Deploy Control Plane Panel
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
$USER_LINE
WorkingDirectory=$CONFIG_DIR
ExecStart=$INSTALL_DIR/vps-panel --port 20888 --db $CONFIG_DIR/data.db
Restart=always
RestartSec=5s
LimitNOFILE=65535

[Install]
WantedBy=$WANTED_TARGET
EOF

# 5. 重载并启动服务
$SYSTEMCTL_CMD daemon-reload
$SYSTEMCTL_CMD enable vps-panel
$SYSTEMCTL_CMD restart vps-panel

sleep 2

# 6. 读取日志中的访问路径
echo -e "${GREEN}==========================================================${NC}"
echo -e "${GREEN} [✓] 部署完成！服务已在后台保持自启运行 (systemd: vps-panel)${NC}"
echo -e "${GREEN}==========================================================${NC}"
$JOURNALCTL_CMD -u vps-panel -n 16 --no-pager
echo -e "${GREEN}==========================================================${NC}"
echo -e "${YELLOW}常用命令:${NC}"
echo -e " 查看状态:   $SYSTEMCTL_CMD status vps-panel"
echo -e " 查看实时日志: $JOURNALCTL_CMD -u vps-panel -f"
echo -e " 重启服务:   $SYSTEMCTL_CMD restart vps-panel"
echo -e " 停止服务:   $SYSTEMCTL_CMD stop vps-panel"
echo -e " 重置密码:   $INSTALL_DIR/vps-panel --db $CONFIG_DIR/data.db --reset-password <新密码>"
echo -e "${GREEN}==========================================================${NC}"
