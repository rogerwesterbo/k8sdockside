# Makefile for k8sdockside
# inspired by kubebuilder.io

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Basic colors
BLACK=\033[0;30m
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[0;33m
BLUE=\033[0;34m
PURPLE=\033[0;35m
CYAN=\033[0;36m
WHITE=\033[0;37m

# Text formatting
BOLD=\033[1m
UNDERLINE=\033[4m
RESET=\033[0m

APP_NAME ?= k8sdockside
FRONTEND_DIR ?= frontend

## Location to install tool dependencies to. Kept under bin/ (already gitignored),
## but in its own subdir so `make clean` can drop build output without nuking tools.
LOCALBIN ?= $(shell pwd)/bin/tools
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GOSEC ?= $(LOCALBIN)/gosec
GOVULNCHECK ?= $(LOCALBIN)/govulncheck

# Use the Go toolchain version declared in go.mod when building tools
GO_VERSION := $(shell awk '/^go /{print $$2}' go.mod)
GO_TOOLCHAIN := go$(GO_VERSION)
GOSEC_VERSION ?= latest
GOLANGCI_LINT_VERSION ?= latest
GOVULNCHECK_VERSION ?= latest

# Keep the wails3 CLI on the exact version this module depends on. Lazily
# evaluated so it only runs when a wails target is actually invoked.
WAILS_VERSION = $(shell go list -m -f '{{.Version}}' github.com/wailsapp/wails/v3)

# build/ios is `package main` whose main() sits behind `//go:build ios`, so a
# plain `go build ./...` fails to link it on Linux. Filter it out.
GO_PKGS = $(shell go list ./... | grep -v '/build/ios')

##@ Help
.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build
.PHONY: dev
dev: ## Run the app in development mode with hot reload (wails3 dev).
	@printf "$(CYAN)Starting wails3 dev...$(RESET)\n"
	@wails3 dev

.PHONY: build
build: ## Build the production desktop app into bin/ (wails3 build).
	@printf "$(CYAN)Building $(APP_NAME)...$(RESET)\n"
	@wails3 build
	@printf "$(GREEN)✓ Build complete: $(BOLD)bin/$(APP_NAME)$(RESET)\n"

.PHONY: build-go
build-go: ## Compile the Go packages only (skips build/ios, no frontend build).
	@printf "$(CYAN)Building Go packages...$(RESET)\n"
	@go build $(GO_PKGS)
	@printf "$(GREEN)✓ Go build complete$(RESET)\n"

.PHONY: build-frontend
build-frontend: ## Build the frontend into frontend/dist.
	@printf "$(CYAN)Building frontend...$(RESET)\n"
	@cd $(FRONTEND_DIR) && npm run build
	@printf "$(GREEN)✓ Frontend built: $(BOLD)$(FRONTEND_DIR)/dist$(RESET)\n"

.PHONY: generate
generate: ## Regenerate the TypeScript bindings from the Go services.
	@printf "$(CYAN)Generating bindings...$(RESET)\n"
	# -i must match build/Taskfile.yml's generate:bindings, which reruns on every
	# build. Without it this target emits model classes and non-null slices, and
	# the next build silently replaces them with interfaces and nullable ones.
	@wails3 generate bindings -clean=true -ts -i
	@printf "$(GREEN)✓ Bindings generated$(RESET)\n"

.PHONY: clean
clean: clean-frontend ## Clean build artifacts, caches and frontend output.
	@printf "$(YELLOW)Cleaning build artifacts...$(RESET)\n"
	@rm -rf bin/$(APP_NAME) bin/$(APP_NAME).exe .task
	@rm -f coverage.out coverage.html bench.cpu bench.mem
	@go clean -testcache
	@printf "$(GREEN)✓ Clean complete$(RESET)\n"

.PHONY: clean-frontend
clean-frontend: ## Remove frontend/dist, node_modules and the vite cache.
	@printf "$(YELLOW)Cleaning frontend...$(RESET)\n"
	@rm -rf $(FRONTEND_DIR)/dist $(FRONTEND_DIR)/node_modules $(FRONTEND_DIR)/.vite $(FRONTEND_DIR)/node_modules/.vite
	@printf "$(GREEN)✓ Frontend cleaned$(RESET)\n"

.PHONY: clean-tools
clean-tools: ## Remove the locally installed tools in bin/tools.
	@printf "$(YELLOW)Removing $(LOCALBIN)...$(RESET)\n"
	@rm -rf $(LOCALBIN)
	@printf "$(GREEN)✓ Tools removed$(RESET)\n"

##@ Code sanity
.PHONY: fmt
fmt: ## Run go fmt against code.
	@printf "$(CYAN)Running go fmt...$(RESET)\n"
	@go fmt ./...
	@printf "$(GREEN)✓ Code formatted$(RESET)\n"

.PHONY: vet
vet: ## Run go vet against code.
	@printf "$(CYAN)Running go vet...$(RESET)\n"
	@go vet ./...
	@printf "$(GREEN)✓ Vet complete$(RESET)\n"

.PHONY: fix
fix: ## Run go fix against code.
	@printf "$(CYAN)Running go fix...$(RESET)\n"
	@go fix ./...
	@printf "$(GREEN)✓ Fix complete$(RESET)\n"

.PHONY: lint
lint: golangci-lint ## Run golangci-lint against the Go code.
	@printf "$(CYAN)Running golangci-lint...$(RESET)\n"
	@$(GOLANGCI_LINT) run --timeout 5m ./...
	@printf "$(GREEN)✓ Lint complete$(RESET)\n"

.PHONY: lint-frontend
lint-frontend: ## Type-check the Svelte frontend (svelte-check).
	@printf "$(CYAN)Running svelte-check...$(RESET)\n"
	@cd $(FRONTEND_DIR) && npm run check
	@printf "$(GREEN)✓ Frontend check complete$(RESET)\n"

##@ Tests
.PHONY: test
test: ## Run unit tests.
	@printf "$(CYAN)Running unit tests...$(RESET)\n"
	@go test -v $(GO_PKGS) -coverprofile coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@printf "$(GREEN)✓ Tests complete - coverage report: $(BOLD)coverage.html$(RESET)\n"

.PHONY: bench
bench: ## Run benchmarks (override with BENCH=<regex>, PKG=<package pattern>, COUNT=<n>)
	@bench_regex=$${BENCH:-.}; \
	pkg_pattern=$${PKG:-.}; \
	count=$${COUNT:-1}; \
	printf "$(CYAN)Running benchmarks: $(RESET)regex=$${bench_regex} packages=$${pkg_pattern} count=$${count}\n"; \
	go test -run=^$$ -bench=$${bench_regex} -benchmem -count=$${count} $${pkg_pattern}; \
	printf "$(GREEN)✓ Benchmarks complete$(RESET)\n"

##@ Dependencies
.PHONY: deps
deps: ## Download and verify Go dependencies, and install frontend deps.
	@printf "$(CYAN)Downloading Go dependencies...$(RESET)\n"
	@go mod download
	@go mod verify
	@go mod tidy
	@printf "$(CYAN)Installing frontend dependencies...$(RESET)\n"
	@cd $(FRONTEND_DIR) && npm install
	@printf "$(GREEN)✓ Dependencies ready!$(RESET)\n"

.PHONY: update-deps
update-deps: update-deps-go update-deps-frontend ## Update Go and frontend dependencies.
	@printf "$(GREEN)$(BOLD)✓ All dependencies updated!$(RESET)\n"

.PHONY: update-deps-go
update-deps-go: ## Update Go dependencies.
	@printf "$(CYAN)Updating Go dependencies...$(RESET)\n"
	@go get -u ./...
	@go mod tidy
	@printf "$(GREEN)✓ Go dependencies updated!$(RESET)\n"

.PHONY: update-deps-frontend
update-deps-frontend: ## Update frontend dependencies (minor/patch only; TARGET=latest for majors).
	@target=$${TARGET:-minor}; \
	printf "$(CYAN)Updating frontend dependencies (target=$${target})...$(RESET)\n"; \
	cd $(FRONTEND_DIR) && \
	if command -v ncu >/dev/null 2>&1; then \
		ncu --target $${target} -u; \
	else \
		printf "$(YELLOW)ncu not found, using npx npm-check-updates...$(RESET)\n"; \
		npx --yes npm-check-updates --target $${target} -u; \
	fi; \
	npm install
	@printf "$(GREEN)✓ Frontend dependencies updated!$(RESET)\n"

##@ Tools
.PHONY: golangci-lint
golangci-lint: | $(LOCALBIN) ## Download golangci-lint locally if necessary.
	@$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: install-wails
install-wails: ## Install the wails3 CLI at the version required by go.mod.
	@printf "$(CYAN)Installing wails3 $(WAILS_VERSION)...$(RESET)\n"
	@go install github.com/wailsapp/wails/v3/cmd/wails3@$(WAILS_VERSION)
	@printf "$(GREEN)✓ wails3 $(WAILS_VERSION) installed$(RESET)\n"

.PHONY: install-security-scanner
install-security-scanner: $(GOSEC) ## Install gosec security scanner locally (static analysis for security issues)
$(GOSEC): | $(LOCALBIN)
	@set -e; printf "$(CYAN)Installing gosec $(GOSEC_VERSION)...$(RESET)\n"; \
	if ! GOBIN=$(LOCALBIN) go install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION) 2>/dev/null; then \
		printf "$(YELLOW)Primary install failed, attempting fallback to @main...$(RESET)\n"; \
		if ! GOBIN=$(LOCALBIN) go install github.com/securego/gosec/v2/cmd/gosec@main; then \
			printf "$(RED)✗ gosec installation failed$(RESET)\n"; \
			exit 1; \
		fi; \
	fi; \
	printf "$(GREEN)✓ gosec installed at $(BOLD)$(GOSEC)$(RESET)\n"; \
	chmod +x $(GOSEC)

.PHONY: install-govulncheck
install-govulncheck: $(GOVULNCHECK) ## Install govulncheck locally (vulnerability scanner for Go)
$(GOVULNCHECK): | $(LOCALBIN)
	@set -e; echo "Attempting to install govulncheck $(GOVULNCHECK_VERSION)"; \
	if ! GOBIN=$(LOCALBIN) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) 2>/dev/null; then \
		echo "Primary install failed, attempting install from @latest (compatibility fallback)"; \
		if ! GOBIN=$(LOCALBIN) go install golang.org/x/vuln/cmd/govulncheck@latest; then \
			echo "govulncheck installation failed for versions $(GOVULNCHECK_VERSION) and @latest"; \
			exit 1; \
		fi; \
	fi; \
	echo "govulncheck installed at $(GOVULNCHECK)"; \
	chmod +x $(GOVULNCHECK)

##@ Security
# gosec skips build/: the Wails scaffold's own iOS/Android dependency installers
# live there and trip G204/G702, which would fail every scan on code we don't own.
.PHONY: audit
audit: gosec govulncheck npm-audit ## Run all security scans (gosec + govulncheck + npm audit).
	@printf "$(GREEN)$(BOLD)✓ Audit complete$(RESET)\n"

.PHONY: gosec
gosec: install-security-scanner ## Run gosec security scan (fails on findings)
	@printf "$(CYAN)Running gosec...$(RESET)\n"
	@$(GOSEC) -exclude-dir=build ./...
	@printf "$(GREEN)✓ gosec complete$(RESET)\n"

.PHONY: govulncheck
govulncheck: install-govulncheck ## Run govulncheck vulnerability scan (fails on findings)
	@printf "$(CYAN)Running govulncheck...$(RESET)\n"
	@$(GOVULNCHECK) ./...
	@printf "$(GREEN)✓ govulncheck complete$(RESET)\n"

.PHONY: npm-audit
npm-audit: ## Run npm audit against the frontend dependencies.
	@printf "$(CYAN)Running npm audit...$(RESET)\n"
	@cd $(FRONTEND_DIR) && npm audit
	@printf "$(GREEN)✓ npm audit complete$(RESET)\n"

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
printf "$(CYAN)Downloading $${package}...$(RESET)\n" ;\
rm -f $(1) || true ;\
GOTOOLCHAIN=$(GO_TOOLCHAIN) GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
printf "$(GREEN)✓ Installed $(BOLD)$(1)-$(3)$(RESET)\n" ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
