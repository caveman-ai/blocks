.PHONY: build test lint e2e bench-hook check-instructions

build:
	go build -trimpath -ldflags="-s -w -X main.version=$$(git describe --tags --always)" -o bin/caveman-blocks ./cmd/caveman-blocks

test:
	go test ./...

lint:
	test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...

e2e: build
	@echo "e2e: fixture repo tests land in phase 1 (docs/ROADMAP.md)"; exit 0

bench-hook: build
	@echo "bench-hook: latency budget test lands in phase 1"; exit 0

check-instructions:
	test ! -e CLAUDE.md
