.PHONY: build test lint e2e bench-hook blocks-test blocks-lint blocks-verify check-instructions

BIN := bin/caveman-blocks
BIN_SRCS := $(shell find cmd internal blocks plugins -type f ! -name '*_test.go' ! -path 'blocks/tests/*') go.mod go.sum

$(BIN): $(BIN_SRCS)
	go build -trimpath -ldflags="-s -w -X main.version=$$(git describe --tags --always)" -o $(BIN) ./cmd/caveman-blocks

build:
	go build -trimpath -ldflags="-s -w -X main.version=$$(git describe --tags --always)" -o $(BIN) ./cmd/caveman-blocks

test:
	go test ./...

lint:
	test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	go vet ./...

# End-to-end: testscript scenarios under testdata/e2e run the test binary via testscript.Main.
e2e:
	go test -tags e2e ./cmd/... -run TestE2E

# Hook latency budget: rule-1 no-op under 5 ms, full decision under 30 ms (docs/ARCHITECTURE.md).
bench-hook: $(BIN)
	CAVEMAN_BLOCKS_BIN=$(CURDIR)/$(BIN) go test -tags e2e ./cmd/... -run TestHookBudget -v -count=1

blocks-test:
	python3 -m unittest blocks/tests/test_blocks.py

# Contributor check, not run in CI: ruff with defaults over the first-party blocks (CONTRIBUTING.md).
blocks-lint:
	@if command -v uvx >/dev/null 2>&1; then uvx ruff check blocks/; \
	elif command -v ruff >/dev/null 2>&1; then ruff check blocks/; \
	else echo "blocks-lint: skipped, neither uvx nor ruff is installed"; fi

# First-party blocks carry no stamp; --first-party passes each one whose example passes (docs/CI.md).
blocks-verify: $(BIN)
	./$(BIN) lint --first-party blocks
	./$(BIN) verify --check --all --first-party --fixtures-root blocks/fixtures --blocks-dir blocks

check-instructions:
	test -L CLAUDE.md && test "$$(readlink CLAUDE.md)" = "AGENTS.md"
