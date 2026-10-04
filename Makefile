.PHONY: build test lint e2e bench-hook blocks-test check-instructions

build:
	go build -trimpath -ldflags="-s -w -X main.version=$$(git describe --tags --always)" -o bin/caveman-blocks ./cmd/caveman-blocks

test:
	go test ./...

lint:
	test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...

# End-to-end: testscript scenarios under testdata/e2e run the real binary (phase 1).
e2e: build
	go test -tags e2e ./cmd/... -run TestE2E

# Hook latency budget: rule-1 no-op under 5 ms, full decision under 30 ms (docs/ARCHITECTURE.md).
bench-hook: build
	go test -tags e2e ./cmd/... -run TestHookBudget -v

blocks-test:
	python3 -m unittest blocks/test_blocks.py

check-instructions:
	test ! -e CLAUDE.md
