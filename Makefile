SHELL := /bin/sh
.DEFAULT_GOAL := help

GO ?= go
BIN_DIR := bin

GO_VERSION := $(shell tr -d '[:space:]' < go.version)
GOLANGCI_LINT_VERSION ?= v2.12.2
ACTIONLINT_VERSION ?= v1.7.12
GOVULNCHECK_VERSION ?= v1.6.0
GORELEASER_VERSION ?= v2.17.0

GOLANGCI_LINT ?= $(BIN_DIR)/golangci-lint
ACTIONLINT ?= $(BIN_DIR)/actionlint
GOVULNCHECK ?= $(BIN_DIR)/govulncheck
GORELEASER ?= $(BIN_DIR)/goreleaser

FUZZ_PACKAGE ?= ./diagnosis
FUZZ_TARGET ?= FuzzDecodeReport
FUZZ_TIME ?= 10s

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf '%s' dev)
COMMIT ?= $(shell commit_value=$$(git rev-parse --verify HEAD 2>/dev/null) && printf '%s' "$$commit_value" || printf '%s' unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -buildid= \
	-X github.com/ryancswallace/jobman-diagnose/internal/buildinfo.Version=$(VERSION) \
	-X github.com/ryancswallace/jobman-diagnose/internal/buildinfo.Commit=$(COMMIT) \
	-X github.com/ryancswallace/jobman-diagnose/internal/buildinfo.Date=$(BUILD_DATE)

.PHONY: help
help: ## Show available targets.
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_.-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: setup
setup: go-version-check tools download ## Install pinned tools and download modules.

.PHONY: go-version-check
go-version-check: ## Verify that the exact pinned Go toolchain is active.
	@actual="$$( $(GO) env GOVERSION 2>/dev/null || true )"; \
	expected='go$(GO_VERSION)'; \
	if [ "$$actual" != "$$expected" ]; then \
		echo "Go toolchain $$expected is required; active toolchain is $$actual." >&2; \
		exit 2; \
	fi

.PHONY: tools
tools: tool-golangci-lint tool-actionlint tool-govulncheck tool-goreleaser ## Install all pinned development tools.

.PHONY: tool-golangci-lint
tool-golangci-lint:
	@if ! $(GOLANGCI_LINT) version 2>/dev/null \
		| grep -Fq 'version $(patsubst v%,%,$(GOLANGCI_LINT_VERSION))'; then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION) into $(BIN_DIR)/"; \
		mkdir -p $(BIN_DIR); \
		GOBIN=$(abspath $(BIN_DIR)) $(GO) install \
			github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi

.PHONY: tool-actionlint
tool-actionlint:
	@if ! $(ACTIONLINT) -version 2>/dev/null \
		| grep -Fq '$(patsubst v%,%,$(ACTIONLINT_VERSION))'; then \
		echo "Installing actionlint $(ACTIONLINT_VERSION) into $(BIN_DIR)/"; \
		mkdir -p $(BIN_DIR); \
		GOBIN=$(abspath $(BIN_DIR)) $(GO) install \
			github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION); \
	fi

.PHONY: tool-govulncheck
tool-govulncheck:
	@if ! $(GOVULNCHECK) -version 2>/dev/null \
		| grep -Fq '$(GOVULNCHECK_VERSION)'; then \
		echo "Installing govulncheck $(GOVULNCHECK_VERSION) into $(BIN_DIR)/"; \
		mkdir -p $(BIN_DIR); \
		GOBIN=$(abspath $(BIN_DIR)) $(GO) install \
			golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION); \
	fi

.PHONY: tool-goreleaser
tool-goreleaser:
	@if ! $(GORELEASER) --version 2>/dev/null \
		| grep -Fq '$(patsubst v%,%,$(GORELEASER_VERSION))'; then \
		echo "Installing GoReleaser $(GORELEASER_VERSION) into $(BIN_DIR)/"; \
		mkdir -p $(BIN_DIR); \
		GOBIN=$(abspath $(BIN_DIR)) $(GO) install \
			github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION); \
	fi

.PHONY: download
download: ## Download and verify module content.
	$(GO) mod download
	$(GO) mod verify

.PHONY: mod-check
mod-check: ## Verify module files are tidy and downloaded content is intact.
	$(GO) mod verify
	$(GO) mod tidy -diff

.PHONY: format
format: tool-golangci-lint ## Format Go source.
	$(GOLANGCI_LINT) fmt

.PHONY: format-check
format-check: tool-golangci-lint ## Check Go formatting.
	$(GOLANGCI_LINT) fmt --diff

.PHONY: lint
lint: tool-golangci-lint ## Run static analysis against the Linux release target.
	GOOS=linux CGO_ENABLED=0 $(GOLANGCI_LINT) run ./...

.PHONY: workflow-check
workflow-check: tool-actionlint ## Validate GitHub Actions workflows.
	$(ACTIONLINT) .github/workflows/*.yml

.PHONY: vulncheck
vulncheck: tool-govulncheck ## Check reachable Go code for known vulnerabilities.
	$(GOVULNCHECK) ./...

.PHONY: test
test: ## Run race-enabled unit and compatibility tests.
	$(GO) test -race -shuffle=on ./...

.PHONY: fuzz
fuzz: ## Fuzz one decoder target for a bounded duration.
	$(GO) test -run '^$$' -fuzz '^$(FUZZ_TARGET)$$' -fuzztime '$(FUZZ_TIME)' $(FUZZ_PACKAGE)

.PHONY: coverage
coverage: ## Write an atomic coverage profile to coverage.txt.
	$(GO) test -race -shuffle=on -covermode=atomic -coverpkg=./... -coverprofile=coverage.txt ./...

.PHONY: evaluate
evaluate: ## Run the checked-in deterministic diagnosis quality corpus.
	$(GO) run ./devel/evaluate --corpus testdata/evaluation/manifest.json --summary

.PHONY: build
build: ## Build the companion binary.
	mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -mod=readonly -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/jobman-diagnose ./cmd/jobman-diagnose

.PHONY: cross-build
cross-build: ## Compile every supported release OS and architecture.
	@set -eu; \
	for target in linux/amd64 linux/arm64 linux/386 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 windows/386; do \
		goos=$$(printf '%s' "$$target" | cut -d/ -f1); \
		goarch=$$(printf '%s' "$$target" | cut -d/ -f2); \
		echo "building $$goos/$$goarch"; \
		GOOS=$$goos GOARCH=$$goarch CGO_ENABLED=0 $(GO) build -trimpath -mod=readonly ./...; \
	done

.PHONY: release-check
release-check: tool-goreleaser ## Validate the release configuration.
	$(GORELEASER) check

.PHONY: release-build
release-build: tool-goreleaser ## Compile every target declared to GoReleaser.
	$(GORELEASER) build --snapshot --clean

.PHONY: quick-check
quick-check: go-version-check mod-check format-check lint test build ## Run the fast local validation loop.

.PHONY: check
check: go-version-check mod-check format-check lint workflow-check vulncheck test evaluate cross-build release-check release-build build ## Run the complete local validation gate.
