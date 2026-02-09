# DAAP — Database as a Service Platform
# Run `make help` for available targets

.DEFAULT_GOAL := help

# ——————————————————————————————————————————————
# Variables
# ——————————————————————————————————————————————
APP_NAME             := daap
GO                   := go
GOFLAGS              := -v
BINARY               := bin/$(APP_NAME)
K3D_CLUSTER          := daap-local
CNPG_VERSION         := 1.25.0
GOLANGCI_LINT_VERSION := v2.8.0
GOLANGCI_LINT        := $(shell go env GOPATH)/bin/golangci-lint

# ——————————————————————————————————————————————
# Help
# ——————————————————————————————————————————————
.PHONY: help
help: ## Show this help
	@printf "\nUsage: make <target>\n\nTargets:\n"
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@printf "\n"

# ——————————————————————————————————————————————
# Setup
# ——————————————————————————————————————————————
.PHONY: setup
setup: ## Install project dependencies and dev tools
	$(GO) mod download
	$(GO) install github.com/air-verse/air@latest
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)
	@if ! command -v k3d >/dev/null 2>&1; then \
		echo "Installing k3d..."; \
		curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash; \
	else \
		echo "k3d already installed: $$(k3d version | head -1)"; \
	fi
	@echo "Setup complete."

# ——————————————————————————————————————————————
# Development
# ——————————————————————————————————————————————
.PHONY: dev
dev: ## Start dev server with hot reload (air)
	air

.PHONY: build
build: ## Build production binary
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(BINARY) ./cmd/server

.PHONY: run
run: build ## Build and run the binary
	./$(BINARY)

# ——————————————————————————————————————————————
# Quality
# ——————————————————————————————————————————————
.PHONY: test
test: ## Run all unit tests
	$(GO) test $(GOFLAGS) ./... -count=1

.PHONY: test-integration
test-integration: ## Run integration tests (requires K8s)
	$(GO) test $(GOFLAGS) -tags=integration ./tests/integration/... -count=1

.PHONY: test-coverage
test-coverage: ## Run tests with coverage report
	$(GO) test ./... -coverprofile=coverage.out -count=1
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: lint
lint: ## Run golangci-lint
	@if ! command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || ! $(GOLANGCI_LINT) version 2>&1 | grep -q "$(patsubst v%,%,$(GOLANGCI_LINT_VERSION))"; then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION); \
	fi
	$(GOLANGCI_LINT) run ./...

.PHONY: fmt
fmt: ## Format Go source files
	gofmt -w .
	goimports -w .

# ——————————————————————————————————————————————
# Kubernetes (local)
# ——————————————————————————————————————————————
.PHONY: cluster-up
cluster-up: ## Create local k3d cluster (idempotent)
	@if k3d cluster list 2>/dev/null | grep -q "^$(K3D_CLUSTER) "; then \
		echo "k3d cluster '$(K3D_CLUSTER)' already exists."; \
	else \
		k3d cluster create $(K3D_CLUSTER) --wait; \
		echo "k3d cluster '$(K3D_CLUSTER)' created."; \
	fi

.PHONY: cluster-down
cluster-down: ## Delete local k3d cluster (idempotent)
	@if k3d cluster list 2>/dev/null | grep -q "^$(K3D_CLUSTER) "; then \
		k3d cluster delete $(K3D_CLUSTER); \
		echo "k3d cluster '$(K3D_CLUSTER)' deleted."; \
	else \
		echo "k3d cluster '$(K3D_CLUSTER)' does not exist."; \
	fi

.PHONY: cnpg-install
cnpg-install: ## Install CNPG operator on local cluster (idempotent)
	@if kubectl get deployment -n cnpg-system cnpg-controller-manager >/dev/null 2>&1; then \
		echo "CNPG operator already installed."; \
	else \
		kubectl apply --server-side -f \
			https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-$(shell echo $(CNPG_VERSION) | cut -d. -f1-2)/releases/cnpg-$(CNPG_VERSION).yaml; \
		echo "Waiting for CNPG operator to be ready..."; \
		kubectl wait --for=condition=Available deployment/cnpg-controller-manager \
			-n cnpg-system --timeout=120s; \
		echo "CNPG operator installed and ready."; \
	fi

# ——————————————————————————————————————————————
# Docker
# ——————————————————————————————————————————————
.PHONY: docker-build
docker-build: ## Build Docker production image
	docker build --target prod -t $(APP_NAME):latest .

.PHONY: docker-dev
docker-dev: ## Start dev environment via Docker Compose
	docker compose up --build

.PHONY: docker-down
docker-down: ## Stop Docker Compose services
	docker compose down

# ——————————————————————————————————————————————
# Cleanup
# ——————————————————————————————————————————————
.PHONY: clean
clean: ## Remove build artifacts and coverage files
	rm -rf bin/ coverage.out coverage.html
