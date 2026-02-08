---
description: Set up the complete development environment for the project
user-invocable: false
tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
  - Grep
---

# Setup Dev Environment

Set up Docker, development scripts, linting/formatting, and tooling based on the chosen tech stack.

## Prerequisites
- Tech stack ADR exists at `docs/architecture/decisions/001-tech-stack.md`
- Project directory structure exists

## Steps

### 1. Read the tech stack decision
- Read `docs/architecture/decisions/001-tech-stack.md`
- Identify: language, framework, database, package manager, runtime version

### 2. Create Dockerfile
Multi-stage build with:
- **dev stage**: includes dev dependencies, hot reload, debug tools
- **prod stage**: minimal image, only production dependencies
- Pin base image versions (never use `latest`)
- Run as non-root user in prod stage
- Optimize layer caching (copy lockfile first, then install, then copy code)

### 3. Create docker-compose.yml
Services:
- **app**: builds from Dockerfile, mounts source as volume (dev), exposes ports
- **db**: database service with named volume for persistence, healthcheck
- Any additional services needed (Redis, etc.)
- Environment variables from `.env` file

### 4. Create .env.example
Document all required environment variables:
```
# App
APP_PORT=3000
NODE_ENV=development

# Database
DB_HOST=db
DB_PORT=5432
DB_NAME=daap_dev
DB_USER=postgres
DB_PASSWORD=postgres

# Add more as needed
```

### 5. Create Makefile
Standard targets:
```makefile
.PHONY: help setup dev test lint build migrate seed clean

help: ## Show this help
setup: ## Initial project setup (install deps, create DB)
dev: ## Start development server with hot reload
test: ## Run all tests
lint: ## Run linter
build: ## Build production artifacts
migrate: ## Run database migrations
seed: ## Seed database with sample data
clean: ## Remove build artifacts and containers
```

### 6. Configure linter/formatter
Based on the tech stack, set up the appropriate tools:
- **Node.js/TypeScript**: Biome (preferred) or ESLint + Prettier
- **Python**: Ruff
- **Go**: built-in `go fmt` + `golangci-lint`
- **Rust**: `rustfmt` + `clippy`

### 7. Create additional tooling
- `.editorconfig` for consistent editor settings
- `.dockerignore` to exclude unnecessary files from Docker context
- Runtime version file (`.nvmrc`, `.tool-versions`, etc.)

### 8. Verify
Run the following to validate the setup:
```bash
docker compose build
docker compose up -d
make test  # or equivalent
docker compose down
```

## Output
- `Dockerfile`
- `docker-compose.yml`
- `.env.example`
- `Makefile` (or `scripts/` directory)
- Linter/formatter configuration files
- `.editorconfig`, `.dockerignore`
- Verification that `docker compose up` and `make test` work
