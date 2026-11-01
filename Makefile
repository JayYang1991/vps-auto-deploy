.PHONY: all build build-web build-server clean

all: build

build-web:
	@echo "==> Building Web UI..."
	cd web && pnpm build

build-server:
	@echo "==> Embedding Web UI into Server..."
	rm -rf cmd/server/dist
	cp -r web/dist cmd/server/dist
	@echo "==> Building Go Binary (vps-panel)..."
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/vps-panel ./cmd/server

build: build-web build-server
	@echo "==> Build complete: bin/vps-panel"

clean:
	rm -rf bin cmd/server/dist web/dist

