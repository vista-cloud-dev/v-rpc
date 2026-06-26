# v-rpc — the `v rpc` domain (RPC Broker debug tap). Build conventions inherited
# from go-cli-template: static (CGO_ENABLED=0), -trimpath, version stamped via
# -ldflags, cross-compile matrix, lint, test, schema.

BIN     ?= v-rpc                     # the v rpc domain CLI (standalone)
PKG     := github.com/vista-cloud-dev/v-rpc
# Version is stamped into the shared clikit module.
LDPKG   := github.com/vista-cloud-dev/clikit
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%d)
LDFLAGS := -s -w -X $(LDPKG).Version=$(VERSION) -X $(LDPKG).Commit=$(COMMIT) -X $(LDPKG).Date=$(DATE)

# Static, no-libc, reproducible.
GOFLAGS := -trimpath
export CGO_ENABLED := 0

PLATFORMS := linux/amd64 linux/arm64 darwin/arm64 windows/amd64

.PHONY: all check build run lint test tidy schema dist install clean

# Where `make install` drops the binary (override: make install BINDIR=~/scripts/bin).
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

all: lint test build

# The pre-commit gate (mirrors CI): lint + race tests + build.
check: lint test build

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BIN) .

run: build
	./dist/$(BIN) $(ARGS)

lint:
	golangci-lint run ./...

# -race requires cgo, so enable it for tests only; build/dist stay static.
test:
	CGO_ENABLED=1 go test $(GOFLAGS) -race -cover ./...

tidy:
	go mod tidy

schema: build
	./dist/$(BIN) schema

dist:
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		echo "  $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch go build $(GOFLAGS) -ldflags "$(LDFLAGS)" \
			-o dist/$(BIN)-$$os-$$arch$$ext . ; \
	done

# Install the static binary onto PATH. Co-locate the m-<engine> driver in the
# same BINDIR and v-rpc auto-finds it (driver-contract §4) — no M_<ENGINE>_BIN.
install: build
	@mkdir -p "$(BINDIR)"
	install -m 0755 dist/$(BIN) "$(BINDIR)/$(BIN)"
	@echo "installed $(BIN) -> $(BINDIR)/$(BIN)"
	@echo "tip: also put m-ydb / m-iris in $(BINDIR) so v-rpc locates the driver automatically."

clean:
	rm -f dist/$(BIN) dist/$(BIN)-* *.test
