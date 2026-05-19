BINARY      := kungfu
BIN_DIR     := bin
MODULE      := github.com/mjcurry/kungfu
LDFLAGS_PKG := $(MODULE)/internal/cli

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X $(LDFLAGS_PKG).version=$(VERSION) \
	-X $(LDFLAGS_PKG).commit=$(COMMIT) \
	-X $(LDFLAGS_PKG).date=$(DATE)

.PHONY: build test lint install clean

## build: compile the binary to ./bin/kungfu with version metadata
build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/kungfu

## test: run the full test suite
test:
	go test ./...

## lint: vet the code and verify gofmt is clean
lint:
	go vet ./...
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needs to be run on:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

## install: install kungfu into GOBIN
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/kungfu

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR)
