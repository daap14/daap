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
CNPG_VERSION         := 1.25.1
GOLANGCI_LINT_VERSION := v2.8.0
AIR                  := $(shell go env GOPATH)/bin/air
GOLANGCI_LINT        := $(shell go env GOPATH)/bin/golangci-lint
VACUUM               := $(shell go env GOPATH)/bin/vacuum
DATABASE_URL         ?= postgres://daap:daap@localhost:5432/daap?sslmode=disable
MIGRATIONS_DIR       := migrations

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
	@if ! command -v migrate >/dev/null 2>&1; then \
		echo "Installing golang-migrate..."; \
		$(GO) install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest; \
	else \
		echo "golang-migrate already installed."; \
	fi
	@echo "Setup complete."

# ——————————————————————————————————————————————
# Development
# ——————————————————————————————————————————————
.PHONY: dev
dev: ## Start dev server with hot reload (air)
	$(AIR)

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

.PHONY: lint-openapi
lint-openapi: ## Lint OpenAPI spec
	@if ! command -v $(VACUUM) >/dev/null 2>&1; then \
		echo "Installing vacuum..."; \
		$(GO) install github.com/daveshanley/vacuum@latest; \
	fi
	$(VACUUM) lint api/openapi.yaml

.PHONY: fmt
fmt: ## Format Go source files
	gofmt -w .
	goimports -w .

# ——————————————————————————————————————————————
# Database Migrations
# ——————————————————————————————————————————————
.PHONY: migrate
migrate: ## Run all pending database migrations
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

.PHONY: migrate-rollback
migrate-rollback: ## Rollback the last database migration
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1

.PHONY: migrate-status
migrate-status: ## Show current migration version
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" version

.PHONY: migrate-create
migrate-create: ## Create a new migration pair (usage: make migrate-create NAME=description)
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-create NAME=description"; exit 1; fi
	@NEXT=$$(printf "%03d" $$(( $$(ls -1 $(MIGRATIONS_DIR)/*.up.sql 2>/dev/null | wc -l) + 1 ))); \
	touch $(MIGRATIONS_DIR)/$${NEXT}_$(NAME).up.sql; \
	touch $(MIGRATIONS_DIR)/$${NEXT}_$(NAME).down.sql; \
	echo "Created $(MIGRATIONS_DIR)/$${NEXT}_$(NAME).up.sql"; \
	echo "Created $(MIGRATIONS_DIR)/$${NEXT}_$(NAME).down.sql"

.PHONY: seed
seed: ## Seed database with sample data
	@psql "$(DATABASE_URL)" -f scripts/seed.sql

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
	CNPG_VERSION=$(CNPG_VERSION) bash scripts/cluster-setup.sh

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
