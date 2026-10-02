VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null)
PKG     := github.com/eureka-corp/md-cli/internal/buildinfo
LDFLAGS := -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Time=$(shell date +%s)
INSTALL_DIR ?= $(HOME)/.local/bin
LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

.DEFAULT_GOAL := build
.PHONY: build install test lint check snapshot

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o md .

install: build
	install -d $(INSTALL_DIR) && install -m 0755 md $(INSTALL_DIR)/md

test:
	go test -race ./...

lint:
	$(LINT) run

check: test lint

snapshot:
	go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean
