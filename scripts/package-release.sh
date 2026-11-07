#!/usr/bin/env bash
set -e

# ==============================================================================
# VPS Auto Deploy - 本地 Release 打包发布脚本
# 用法:
#   ./scripts/package-release.sh [版本号，如 v1.0.0]
# ==============================================================================

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  # 优先尝试获取最新的 git tag，如果没有则使用 v1.0.0-dev
  VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "v1.0.0-dev")
fi

echo "=========================================================="
echo " 开始打包 Release 发布资产: ${VERSION}"
echo "=========================================================="

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DIST_DIR="$ROOT_DIR/dist/release"
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

# 1. 构建 Web 前端
echo "==> 1. 构建 Web 前端 UI (Vue 3 + Vite)..."
cd "$ROOT_DIR/web"
export PATH="$HOME/.local/bin:$HOME/.local/share/pnpm:$PATH"
if command -v pnpm &>/dev/null; then
  pnpm build
else
  npm run build
fi
cd "$ROOT_DIR"

# 2. 嵌入静态资源到服务端目录
echo "==> 2. 嵌入静态资源到 cmd/server/dist..."
rm -rf cmd/server/dist
cp -r web/dist cmd/server/dist

# 3. 多架构交叉编译与归档打包
ARCHS=("amd64" "arm64")

for ARCH in "${ARCHS[@]}"; do
  echo "==> 3. 正在交叉编译 Linux/${ARCH}..."
  STAGE_DIR="$DIST_DIR/vps-panel_${VERSION}_linux_${ARCH}"
  mkdir -p "$STAGE_DIR"

  CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build \
    -trimpath \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o "$STAGE_DIR/vps-panel" \
    ./cmd/server

  chmod +x "$STAGE_DIR/vps-panel"
  cp "$ROOT_DIR/scripts/install.sh" "$STAGE_DIR/install.sh"
  chmod +x "$STAGE_DIR/install.sh"

  echo "==> 4. 生成发布压缩包: linux_${ARCH}..."
  TAR_VERSION="$DIST_DIR/vps-panel_${VERSION}_linux_${ARCH}.tar.gz"
  TAR_LATEST="$DIST_DIR/vps-panel_linux_${ARCH}.tar.gz"

  # 打包包含版本号的 tar.gz
  tar -czvf "$TAR_VERSION" -C "$STAGE_DIR" vps-panel install.sh
  # 同时生成不含版本号的别名，便于脚本直接固定链接拉取
  cp -f "$TAR_VERSION" "$TAR_LATEST"

  # 保留一份单独的二进制（便于部分场景直接替换更新）
  cp -f "$STAGE_DIR/vps-panel" "$DIST_DIR/vps-panel-linux-${ARCH}"

  rm -rf "$STAGE_DIR"
done

# 4. 生成校验码文件
echo "==> 5. 生成 SHA-256 校验清单..."
cd "$DIST_DIR"
sha256sum vps-panel_${VERSION}_*.tar.gz vps-panel-linux-* > checksums.txt
cd "$ROOT_DIR"

echo "=========================================================="
echo " [✓] 打包完成！发布产物位于: $DIST_DIR"
echo "=========================================================="
ls -lh "$DIST_DIR"
echo "=========================================================="
echo "校验码预览 (checksums.txt):"
cat "$DIST_DIR/checksums.txt"
echo "=========================================================="
