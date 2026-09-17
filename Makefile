.PHONY: build dev test web-install web-build installer-check

web-install:
	cd web && npm ci

web-build:
	cd web && npm run build

build: web-build
	mkdir -p bin
	go build -o bin/wirely ./cmd/wirely

dev:
	go run ./cmd/wirely

installer-check:
	sh -n ./install.sh
	./install.sh --help >/dev/null

test: installer-check
	go test ./cmd/... ./internal/...
	cd web && npm run lint
	cd web && npm run build
