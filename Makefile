# ============================================================================
# pmanage — Makefile
# Fleet-grade GPU / NPU / TPU process manager
# ============================================================================

APP_NAME    := pmanage
BIN_DIR     := bin
GO          := go
GOBIN       := $(shell $(GO) env GOPATH)/bin
NPM         := npm
VITE_PORT   ?= 9245
GOOS        ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
export PATH := $(GOBIN):$(PATH)

# CGO flags for macOS (Apple Silicon IOKit/IOReport)
CGO_CFLAGS  ?= -mmacosx-version-min=12.0
CGO_LDFLAGS ?= -mmacosx-version-min=12.0

# ============================================================================
# Help
# ============================================================================

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@echo ""
	@echo "pmanage — Fleet-grade GPU / NPU / TPU process manager"
	@echo ""
	@grep -E '^[a-zA-Z_/-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-28s\033[0m %s\n", $$1, $$2}'
	@echo ""

# ============================================================================
# Development
# ============================================================================

.PHONY: dev
dev: frontend-deps ## Run app in development mode (hot-reload)
	wails3 dev -config ./build/config.yml -port $(VITE_PORT)

.PHONY: run
run: build ## Build and run the production binary
	./$(BIN_DIR)/$(APP_NAME)

.PHONY: run-darwin
run-darwin: build-darwin ## Build and run on macOS
	./$(BIN_DIR)/$(APP_NAME)

.PHONY: run-linux
run-linux: build-linux ## Build and run on Linux
	./$(BIN_DIR)/$(APP_NAME)

.PHONY: run-windows
run-windows: build-windows ## Build and run on Windows
	./$(BIN_DIR)/$(APP_NAME).exe

# ============================================================================
# Build
# ============================================================================

.PHONY: build
build: build-$(GOOS) ## Build for current OS

.PHONY: build-darwin
build-darwin: frontend-build generate-icons ## Build for macOS
	CGO_ENABLED=1 GOOS=darwin GOARCH=$(shell uname -m) \
		CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		MACOSX_DEPLOYMENT_TARGET=12.0 \
		$(GO) build -tags production -trimpath -buildvcs=false -ldflags="-w -s" \
		-o "$(BIN_DIR)/$(APP_NAME)"

.PHONY: build-linux
build-linux: frontend-build ## Build for Linux
	CGO_ENABLED=1 GOOS=linux GOARCH=$(shell uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/') \
		$(GO) build -tags production -trimpath -buildvcs=false -ldflags="-w -s" \
		-o "$(BIN_DIR)/$(APP_NAME)"

.PHONY: build-windows
build-windows: frontend-build ## Build for Windows (cross-compile from macOS/Linux)
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
		$(GO) build -tags production -trimpath -buildvcs=false -ldflags="-w -s" \
		-o "$(BIN_DIR)/$(APP_NAME).exe"

.PHONY: build-dev
build-dev: frontend-build ## Build in development mode (debug symbols)
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(shell uname -m) \
		$(GO) build -buildvcs=false -gcflags=all="-l" \
		-o "$(BIN_DIR)/$(APP_NAME)"

.PHONY: build-universal
build-universal: build-darwin-amd64 build-darwin-arm64 ## Build macOS universal binary (arm64 + amd64)
	lipo -create -output "$(BIN_DIR)/$(APP_NAME)" \
		"$(BIN_DIR)/$(APP_NAME)-amd64" \
		"$(BIN_DIR)/$(APP_NAME)-arm64"
	rm -f "$(BIN_DIR)/$(APP_NAME)-amd64" "$(BIN_DIR)/$(APP_NAME)-arm64"

.PHONY: build-darwin-amd64
build-darwin-amd64: frontend-build
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
		CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		$(GO) build -tags production -trimpath -buildvcs=false -ldflags="-w -s" \
		-o "$(BIN_DIR)/$(APP_NAME)-amd64"

.PHONY: build-darwin-arm64
build-darwin-arm64: frontend-build
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
		CGO_CFLAGS="$(CGO_CFLAGS)" CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		$(GO) build -tags production -trimpath -buildvcs=false -ldflags="-w -s" \
		-o "$(BIN_DIR)/$(APP_NAME)-arm64"

# ============================================================================
# Server Mode
# ============================================================================

.PHONY: build-server
build-server: frontend-build ## Build server mode (headless, no GUI)
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(shell uname -m) \
		$(GO) build -tags server,production -trimpath -buildvcs=false -ldflags="-w -s" \
		-o "$(BIN_DIR)/$(APP_NAME)-server"

.PHONY: build-server-dev
build-server-dev: frontend-build ## Build server mode (development)
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(shell uname -m) \
		$(GO) build -tags server -buildvcs=false -gcflags=all="-l" \
		-o "$(BIN_DIR)/$(APP_NAME)-server"

.PHONY: run-server
run-server: build-server-dev ## Build and run server mode (development)
	./$(BIN_DIR)/$(APP_NAME)-server

# ============================================================================
# Frontend
# ============================================================================

.PHONY: frontend-deps
frontend-deps: ## Install frontend dependencies
	cd frontend && $(NPM) install

.PHONY: frontend-build
frontend-build: frontend-deps ## Build frontend (production)
	cd frontend && $(NPM) run build

.PHONY: frontend-build-dev
frontend-build-dev: frontend-deps ## Build frontend (development)
	cd frontend && $(NPM) run build:dev

.PHONY: frontend-dev
frontend-dev: frontend-deps ## Run frontend dev server
	cd frontend && $(NPM) run dev -- --port $(VITE_PORT) --strictPort

.PHONY: frontend-typecheck
frontend-typecheck: frontend-deps ## TypeScript type-check
	cd frontend && npx tsc --noEmit

# ============================================================================
# Testing
# ============================================================================

.PHONY: test
test: test-go test-frontend ## Run all tests (Go + frontend)

.PHONY: test-go
test-go: ## Run Go tests
	$(GO) test ./...

.PHONY: test-go-race
test-go-race: ## Run Go tests with race detector
	$(GO) test -race ./...

.PHONY: test-go-v
test-go-v: ## Run Go tests verbose
	$(GO) test -v ./...

.PHONY: test-hailo
test-hailo: ## Run Hailo-specific tests
	$(GO) test -v ./pkg/accelerator/... -run "Hailo"

.PHONY: test-accelerator
test-accelerator: ## Run all accelerator tests
	$(GO) test -v ./pkg/accelerator/...

.PHONY: test-frontend
test-frontend: frontend-deps ## Run frontend tests (vitest)
	cd frontend && npx vitest run

.PHONY: test-e2e
test-e2e: build ## Run end-to-end smoke test
	@if [ -f scripts/e2e_smoke.sh ]; then \
		bash scripts/e2e_smoke.sh; \
	else \
		echo "e2e_smoke.sh not found"; exit 1; \
	fi

# ============================================================================
# Code Quality
# ============================================================================

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: vet ## Run linters
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

.PHONY: fmt
fmt: ## Format Go code
	gofmt -s -w .

.PHONY: fmt-check
fmt-check: ## Check Go formatting
	gofmt -s -l .

.PHONY: tidy
tidy: ## Run go mod tidy
	$(GO) mod tidy

.PHONY: check
check: vet fmt-check ## Run all checks (vet + format check)
	@echo "All checks passed"

# ============================================================================
# Code Generation
# ============================================================================

.PHONY: generate-bindings
generate-bindings: tidy ## Generate Wails frontend bindings
	wails3 generate bindings -f -clean=true -ts -i

.PHONY: generate-icons
generate-icons: ## Generate app icons from build/appicon.png
	@if [ -f build/appicon.png ]; then \
		wails3 generate icons \
			-input build/appicon.png \
			-macfilename build/darwin/icons.icns \
			-windowsfilename build/windows/icon.ico \
			-iconcomposerinput build/appicon.icon \
			-macassetdir build/darwin; \
	else \
		echo "build/appicon.png not found, skipping icon generation"; \
	fi

# ============================================================================
# Packaging
# ============================================================================

.PHONY: package
package: build ## Package app bundle for current OS
	@case "$(GOOS)" in \
		darwin) $(MAKE) package-app ;; \
		linux) $(MAKE) package-app ;; \
		*) echo "Package target not implemented for $(GOOS)" ;; \
	esac

.PHONY: package-app
package-app: build-darwin ## Create .app bundle (macOS)
	mkdir -p "$(BIN_DIR)/$(APP_NAME).app/Contents/MacOS"
	mkdir -p "$(BIN_DIR)/$(APP_NAME).app/Contents/Resources"
	@if [ -f build/darwin/icons.icns ]; then \
		cp build/darwin/icons.icns "$(BIN_DIR)/$(APP_NAME).app/Contents/Resources/"; \
	fi
	@if [ -f build/darwin/Assets.car ]; then \
		cp build/darwin/Assets.car "$(BIN_DIR)/$(APP_NAME).app/Contents/Resources/"; \
	fi
	cp "$(BIN_DIR)/$(APP_NAME)" "$(BIN_DIR)/$(APP_NAME).app/Contents/MacOS/"
	cp build/darwin/Info.plist "$(BIN_DIR)/$(APP_NAME).app/Contents/"
	codesign --force --deep --sign - "$(BIN_DIR)/$(APP_NAME).app" 2>/dev/null || true
	@echo "Created $(BIN_DIR)/$(APP_NAME).app"

.PHONY: package-dmg
package-dmg: package-app ## Create .dmg installer (macOS)
	@if command -v wails3 >/dev/null 2>&1; then \
		wails3 tool package --format dmg \
			--name "$(APP_NAME)" \
			--out "$(BIN_DIR)" \
			--background "build/darwin/dmg-background.png" \
			--volume-icon "build/darwin/icons.icns" \
			--file-icon "build/darwin/dmg-file-icon.icns" \
			--window-width 540 \
			--window-height 380; \
	else \
		echo "wails3 not found — cannot create DMG"; exit 1; \
	fi

.PHONY: package-deb
package-deb: build-linux ## Create .deb package (Linux)
	@echo "Creating .deb package..."
	mkdir -p "$(BIN_DIR)/$(APP_NAME)_$(shell git describe --tags --always 2>/dev/null || echo '0.0.0')_$(shell uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64')/usr/bin"
	cp "$(BIN_DIR)/$(APP_NAME)" "$(BIN_DIR)/$(APP_NAME)_$(shell git describe --tags --always 2>/dev/null || echo '0.0.0')_$(shell uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64')/usr/bin/"
	@echo "Deb packaging stub — use fpm or dpkg-deb for full .deb creation"

# ============================================================================
# Docker
# ============================================================================

.PHONY: docker-build
docker-build: frontend-build ## Build Docker image for server mode
	docker build \
		--build-arg CGO_ENABLED=0 \
		--build-arg GO_IMAGE=golang:alpine \
		--build-arg RUNTIME_IMAGE=gcr.io/distroless/static-debian12 \
		-t $(APP_NAME):latest \
		-f build/docker/Dockerfile.server .

.PHONY: docker-run
docker-run: docker-build ## Build and run Docker container
	docker run --rm -p 8080:8080 $(APP_NAME):latest

.PHONY: docker-setup
docker-setup: ## Build cross-compilation Docker image
	docker build -t wails-cross -f build/docker/Dockerfile.cross build/docker/

# ============================================================================
# E2E / Smoke
# ============================================================================

.PHONY: e2e
e2e: test-e2e ## Alias for test-e2e

.PHONY: smoke
smoke: build test-go ## Quick smoke test (build + Go tests only)
	@echo "Smoke test passed"

# ============================================================================
# Clean
# ============================================================================

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf "$(BIN_DIR)"
	rm -rf frontend/dist
	rm -rf frontend/node_modules/.vite
	rm -rf frontend/bindings
	@echo "Cleaned build artifacts"

.PHONY: clean-all
clean-all: clean ## Remove everything including node_modules
	rm -rf frontend/node_modules
	@echo "Deep clean complete"

# ============================================================================
# Git Workflow
# ============================================================================

.PHONY: status
status: ## Show git status
	git status

.PHONY: diff
diff: ## Show uncommitted changes
	git diff

.PHONY: log
log: ## Show recent commits
	git log --oneline -15

.PHONY: push
push: ## Push to origin dev
	git push origin dev

.PHONY: pull
pull: ## Pull from origin dev
	git pull origin dev

# ============================================================================
# Info
# ============================================================================

.PHONY: info
info: ## Show project information
	@echo "App:        $(APP_NAME)"
	@echo "Go:         $(shell $(GO) version)"
	@echo "Node:       $(shell node --version 2>/dev/null || echo 'not found')"
	@echo "npm:        $(shell $(NPM) --version 2>/dev/null || echo 'not found')"
	@echo "Wails:      $(shell wails3 version 2>/dev/null || echo 'not found')"
	@echo "OS:         $(GOOS)"
	@echo "Arch:       $(shell uname -m)"
	@echo "Branch:     $(shell git branch --show-current 2>/dev/null || echo 'n/a')"
	@echo "Commit:     $(shell git rev-parse --short HEAD 2>/dev/null || echo 'n/a')"
