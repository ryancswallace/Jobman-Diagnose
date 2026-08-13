SHELL := /bin/sh
.DEFAULT_GOAL := help

GO ?= go
BIN_DIR := bin

GO_VERSION := $(shell tr -d '[:space:]' < go.version)
GOLANGCI_LINT_VERSION ?= v2.12.2
ACTIONLINT_VERSION ?= v1.7.12
GOVULNCHECK_VERSION ?= v1.6.0
GORELEASER_VERSION ?= v2.17.0
SYFT_VERSION ?= v1.46.0
CSPELL_VERSION ?= 10.0.1

DOCKER ?= docker
DOCKER_PROGRESS ?= plain

GOLANGCI_LINT ?= $(BIN_DIR)/golangci-lint
ACTIONLINT ?= $(BIN_DIR)/actionlint
GOVULNCHECK ?= $(BIN_DIR)/govulncheck
GORELEASER ?= $(BIN_DIR)/goreleaser
SYFT ?= $(BIN_DIR)/syft
SYFT_VERSION_FILE := $(BIN_DIR)/.syft-$(SYFT_VERSION)

FUZZ_PACKAGE ?= ./diagnosis
FUZZ_TARGET ?= FuzzDecodeReport
FUZZ_TIME ?= 10s
COVERAGE_MIN ?= 90

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

.PHONY: all ci
all: check ## Run the complete local validation gate.
ci: check ## Alias for the complete continuous-integration gate.

.PHONY: versions
versions: ## Show the selected project and development-tool versions.
	@printf 'Go:              %s\n' '$(GO_VERSION)'
	@printf 'golangci-lint:   %s\n' '$(GOLANGCI_LINT_VERSION)'
	@printf 'actionlint:      %s\n' '$(ACTIONLINT_VERSION)'
	@printf 'govulncheck:     %s\n' '$(GOVULNCHECK_VERSION)'
	@printf 'GoReleaser:     %s\n' '$(GORELEASER_VERSION)'
	@printf 'Syft:           %s\n' '$(SYFT_VERSION)'
	@printf 'cspell:         %s\n' '$(CSPELL_VERSION)'

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
tools: tool-golangci-lint tool-actionlint tool-govulncheck tool-goreleaser tool-syft ## Install all pinned development tools.

.PHONY: tool-golangci-lint
tool-golangci-lint:
	@set -eu; \
	if ! $(GOLANGCI_LINT) version 2>/dev/null \
		| grep -Fq 'version $(patsubst v%,%,$(GOLANGCI_LINT_VERSION))'; then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION) into $(BIN_DIR)/"; \
		mkdir -p $(BIN_DIR); \
		GOBIN=$(abspath $(BIN_DIR)) $(GO) install \
			github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi

.PHONY: tool-actionlint
tool-actionlint:
	@set -eu; \
	if ! $(ACTIONLINT) -version 2>/dev/null \
		| grep -Fq '$(patsubst v%,%,$(ACTIONLINT_VERSION))'; then \
		echo "Installing actionlint $(ACTIONLINT_VERSION) into $(BIN_DIR)/"; \
		mkdir -p $(BIN_DIR); \
		GOBIN=$(abspath $(BIN_DIR)) $(GO) install \
			github.com/rhysd/actionlint/cmd/actionlint@$(ACTIONLINT_VERSION); \
	fi

.PHONY: tool-govulncheck
tool-govulncheck:
	@set -eu; \
	if ! $(GOVULNCHECK) -version 2>/dev/null \
		| grep -Fq '$(GOVULNCHECK_VERSION)'; then \
		echo "Installing govulncheck $(GOVULNCHECK_VERSION) into $(BIN_DIR)/"; \
		mkdir -p $(BIN_DIR); \
		GOBIN=$(abspath $(BIN_DIR)) $(GO) install \
			golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION); \
	fi

.PHONY: tool-goreleaser
tool-goreleaser:
	@set -eu; \
	if ! $(GORELEASER) --version 2>/dev/null \
		| grep -Fq '$(patsubst v%,%,$(GORELEASER_VERSION))'; then \
		echo "Installing GoReleaser $(GORELEASER_VERSION) into $(BIN_DIR)/"; \
		mkdir -p $(BIN_DIR); \
		GOBIN=$(abspath $(BIN_DIR)) $(GO) install \
			github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION); \
	fi

.PHONY: tool-syft
tool-syft:
	@set -eu; \
	if ! test -x '$(SYFT)' || ! test -f '$(SYFT_VERSION_FILE)'; then \
		echo "Installing Syft $(SYFT_VERSION) into $(BIN_DIR)/"; \
		mkdir -p $(BIN_DIR); \
		GOBIN=$(abspath $(BIN_DIR)) $(GO) install \
			github.com/anchore/syft/cmd/syft@$(SYFT_VERSION); \
		touch '$(SYFT_VERSION_FILE)'; \
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

.PHONY: shellcheck
shellcheck: ## Statically analyze repository shell scripts.
	@if command -v shellcheck >/dev/null 2>&1; then \
		find devel examples -type f -name '*.sh' -print0 | xargs -0 shellcheck; \
	elif $(DOCKER) info >/dev/null 2>&1; then \
		$(DOCKER) run --rm -v '$(CURDIR):/mnt:ro' \
			koalaman/shellcheck-alpine:v0.11.0 \
			$$(find devel examples -type f -name '*.sh' -print); \
	else \
		echo 'shellcheck requires shellcheck or a running Docker daemon.' >&2; \
		exit 2; \
	fi

.PHONY: vulncheck
vulncheck: tool-govulncheck ## Check reachable Go code for known vulnerabilities.
	$(GOVULNCHECK) ./...

.PHONY: test
test: ## Run race-enabled unit and compatibility tests.
	$(GO) test -race -shuffle=on ./...

.PHONY: e2etest
e2etest: ## Test assembled binaries selected by JOBMAN_E2E_BINARY and JOBMAN_DIAGNOSE_E2E_BINARY.
	@test -n "$$JOBMAN_E2E_BINARY" || { echo 'JOBMAN_E2E_BINARY is required.' >&2; exit 2; }
	@test -n "$$JOBMAN_DIAGNOSE_E2E_BINARY" || { echo 'JOBMAN_DIAGNOSE_E2E_BINARY is required.' >&2; exit 2; }
	$(GO) test -race -shuffle=on ./tests/e2e

.PHONY: fuzz
fuzz: ## Fuzz one decoder target for a bounded duration.
	$(GO) test -run '^$$' -fuzz '^$(FUZZ_TARGET)$$' -fuzztime '$(FUZZ_TIME)' $(FUZZ_PACKAGE)

.PHONY: coverage
coverage: ## Write an atomic coverage profile to coverage.txt.
	$(GO) test -race -shuffle=on -covermode=atomic -coverpkg=./... -coverprofile=coverage.txt ./...

.PHONY: coverage-check
coverage-check: coverage ## Enforce the aggregate statement coverage floor.
	$(GO) tool cover -func=coverage.txt | awk -v minimum='$(COVERAGE_MIN)' -f devel/check-coverage.awk

.PHONY: docs-check
docs-check: ## Verify repository-relative links in Markdown documentation.
	@if git --no-pager grep -nI -E '[[:blank:]]+$$' -- '*.md'; then \
		echo 'Markdown files contain trailing whitespace.' >&2; \
		exit 1; \
	fi
	$(GO) run ./devel/docscheck -root .

.PHONY: spellcheck
spellcheck: ## Spell-check the repository with a pinned cspell version.
	@if command -v cspell >/dev/null 2>&1 \
		&& [ "$$(cspell --version)" = '$(CSPELL_VERSION)' ]; then \
		cspell lint --dot .; \
	elif command -v npx >/dev/null 2>&1; then \
		npx --yes cspell@$(CSPELL_VERSION) lint --dot .; \
	elif $(DOCKER) info >/dev/null 2>&1; then \
		$(DOCKER) build --progress=$(DOCKER_PROGRESS) \
			--file Dockerfile.cspell \
			--build-arg CSPELL_VERSION=$(CSPELL_VERSION) \
			--output type=cacheonly .; \
	else \
		echo 'cspell requires cspell $(CSPELL_VERSION), npx, or a running Docker daemon.' >&2; \
		exit 2; \
	fi

.PHONY: docs
docs: docs-check spellcheck ## Validate authored documentation.

.PHONY: evaluate
evaluate: evaluation-fixtures-check ## Run the checked-in deterministic diagnosis quality corpus.
	$(GO) run ./devel/evaluate --corpus testdata/evaluation/manifest.json --summary

.PHONY: gen-evaluation-fixtures
gen-evaluation-fixtures: ## Regenerate synthetic nonsecret diagnostic evaluation evidence.
	$(GO) run ./devel/evaluationfixtures \
		-output testdata/evaluation/evidence \
		-manifest testdata/evaluation/manifest.json

.PHONY: evaluation-fixtures-check
evaluation-fixtures-check: ## Verify checked-in synthetic evidence matches its generator.
	@set -eu; \
	temporary=$$(mktemp -d "$${TMPDIR:-/tmp}/jobman-diagnose-fixtures.XXXXXXXXXX"); \
	trap 'rm -rf "$$temporary"' EXIT HUP INT TERM; \
	$(GO) run ./devel/evaluationfixtures \
		-output "$$temporary/evidence" \
		-manifest "$$temporary/manifest.json"; \
	diff -ru testdata/evaluation/evidence "$$temporary/evidence"; \
	diff -u testdata/evaluation/manifest.json "$$temporary/manifest.json"

.PHONY: build
build: ## Build the companion binary.
	mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -mod=readonly -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/jobman-diagnose ./cmd/jobman-diagnose

.PHONY: install
install: ## Install the companion with the active Go toolchain.
	$(GO) install -trimpath -mod=readonly -ldflags '$(LDFLAGS)' ./cmd/jobman-diagnose

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
release-check: tool-goreleaser release-metadata-check ## Validate the release configuration and metadata.
	$(GORELEASER) check

.PHONY: release-metadata-check
release-metadata-check: ## Verify changelog and citation metadata against the latest stable tag.
	./devel/check-release-metadata.sh

.PHONY: release-build
release-build: tool-goreleaser ## Compile every target declared to GoReleaser.
	$(GORELEASER) build --snapshot --clean

.PHONY: snapshot
snapshot: tool-goreleaser tool-syft ## Build a complete local release snapshot without publishing.
	PATH='$(abspath $(BIN_DIR))':$$PATH \
		$(GORELEASER) release --snapshot --clean --parallelism 1 --skip=sign
	./devel/check-release.sh dist

.PHONY: package-smoke
package-smoke: ## Install snapshot packages in pinned Debian, Fedora, and Alpine containers.
	./devel/package-smoke.sh dist

.PHONY: clean
clean: ## Remove build, release, and test artifacts.
	rm -rf $(BIN_DIR) dist coverage.txt

.PHONY: quick-check
quick-check: go-version-check mod-check format-check lint shellcheck test docs build ## Run the fast local validation loop.

.PHONY: check
check: go-version-check mod-check format-check lint workflow-check shellcheck vulncheck coverage-check evaluate docs cross-build release-check release-build build ## Run the complete local validation gate.
