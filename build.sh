#!/usr/bin/env bash
set -e

echo "==> 1. Building Web UI (Vue 3 + Vite)..."
cd web
export PATH="$HOME/.local/share/pnpm:$HOME/.local/share/pnpm/bin:$HOME/.local/bin:$PATH"
pnpm build
cd ..

echo "==> 2. Embedding Web UI into Server..."
rm -rf cmd/server/dist
cp -r web/dist cmd/server/dist

echo "==> 3. Compiling Single Binary (bin/vps-panel)..."
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/vps-panel ./cmd/server

echo "==> Build finished successfully! Output: bin/vps-panel"
ls -lh bin/vps-panel

