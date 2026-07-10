# AnotherMUD — developer Makefile
#
# Conventions:
#   - All Go binaries live under cmd/<name>/ and build into ./bin/<name>.
#   - Targets are phony unless they produce a file artifact in ./bin or ./dist.

BINARY      := anothermud
CMD_PKG     := ./cmd/$(BINARY)
BIN_DIR     := bin
DIST_DIR    := dist
PKGS        := ./...

GO          ?= go
GOFLAGS     ?=
LDFLAGS     ?= -s -w
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# World selection for `run` / `watch`. A boot loads ONE world pack (plus its
# dependency closure); empty = the binary's own defaults (the starter-world
# demo). Override directly (`make run WORLD_PACKS=wot WORLD_START_ROOM=wot:the-green`)
# or use the `*-wot` convenience targets below.
WORLD_PACKS      ?=
WORLD_START_ROOM ?=
# Recursive (=) so target-specific overrides (e.g. run-wot) resolve at recipe time.
RUN_ENV           = ANOTHERMUD_PACKS=$(WORLD_PACKS) ANOTHERMUD_START_ROOM=$(WORLD_START_ROOM)

# Cross-compile matrix for `make release`.
RELEASE_TARGETS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	windows/amd64

.DEFAULT_GOAL := help

## help: show this message
.PHONY: help
help:
	@awk 'BEGIN {FS = ":.*?## "} /^## / {sub(/^## /, "", $$0); printf "  %s\n", $$0} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

## build: compile the main binary into ./bin/$(BINARY)
.PHONY: build
build:
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS) -X main.version=$(VERSION)" -o $(BIN_DIR)/$(BINARY) $(CMD_PKG)

## run: run the main binary (the starter-world demo by default)
.PHONY: run
run:
	$(RUN_ENV) $(GO) run $(CMD_PKG)

## run-wot: run the Wheel of Time world (content/wot)
.PHONY: run-wot
run-wot: WORLD_PACKS := wot
run-wot: WORLD_START_ROOM := wot:the-green
run-wot: run

## run-shadowrun: run the Shadowrun world (content/shadowrun — starts in Downtown)
.PHONY: run-shadowrun
run-shadowrun: WORLD_PACKS := shadowrun
run-shadowrun: WORLD_START_ROOM := shadowrun:downtown-plaza
run-shadowrun: run

## watch: live-reload — rebuild + restart on any .go/.yaml/.lua change (needs air)
.PHONY: watch
watch:
	@air="$$(command -v air 2>/dev/null || true)"; \
	[ -n "$$air" ] || air="$$($(GO) env GOPATH)/bin/air"; \
	if [ ! -x "$$air" ]; then \
		echo "air not installed. Install it with:"; \
		echo "  go install github.com/air-verse/air@latest"; \
		exit 1; \
	fi; \
	echo "live reload: edit + save -> rebuild + restart (~1s). Reconnect; saves persist."; \
	$(RUN_ENV) "$$air"

## watch-wot: live-reload the Wheel of Time world (content/wot)
.PHONY: watch-wot
watch-wot: WORLD_PACKS := wot
watch-wot: WORLD_START_ROOM := wot:the-green
watch-wot: watch

## watch-shadowrun: live-reload the Shadowrun world (content/shadowrun — starts in Downtown)
.PHONY: watch-shadowrun
watch-shadowrun: WORLD_PACKS := shadowrun
watch-shadowrun: WORLD_START_ROOM := shadowrun:downtown-plaza
watch-shadowrun: watch

## worlddoc: render world documentation for every world pack to docs/world/
.PHONY: worlddoc
worlddoc:
	$(GO) run ./cmd/worlddoc -pack all
	@echo "open docs/world/index.html in a browser"

## test: run all tests
.PHONY: test
test:
	$(GO) test $(GOFLAGS) -race -count=1 $(PKGS)

## cover: run tests with coverage and write coverage.out
.PHONY: cover
cover:
	$(GO) test $(GOFLAGS) -race -count=1 -coverprofile=coverage.out -covermode=atomic $(PKGS)
	@$(GO) tool cover -func=coverage.out | tail -n 1

## cover-html: open coverage report in browser
.PHONY: cover-html
cover-html: cover
	$(GO) tool cover -html=coverage.out

## bench: run benchmarks
.PHONY: bench
bench:
	$(GO) test -run=^$$ -bench=. -benchmem $(PKGS)

## fmt: format and goimports-style cleanup via gofmt
.PHONY: fmt
fmt:
	$(GO) fmt $(PKGS)

## vet: run go vet
.PHONY: vet
vet:
	$(GO) vet $(PKGS)

## tidy: tidy go.mod / go.sum
.PHONY: tidy
tidy:
	$(GO) mod tidy

## lint: run golangci-lint (requires it on PATH)
.PHONY: lint
lint:
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not installed"; exit 1; }
	golangci-lint run $(PKGS)

## check: fmt + vet + test (the gate to run before committing)
.PHONY: check
check: fmt vet test

## release: cross-compile $(BINARY) for the release matrix into ./dist
.PHONY: release
release:
	@mkdir -p $(DIST_DIR)
	@for target in $(RELEASE_TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		out="$(DIST_DIR)/$(BINARY)-$(VERSION)-$$os-$$arch$$ext"; \
		echo "build $$out"; \
		GOOS=$$os GOARCH=$$arch $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS) -X main.version=$(VERSION)" -o $$out $(CMD_PKG) || exit 1; \
	done

## clean: remove build artifacts
.PHONY: clean
clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) coverage.out
