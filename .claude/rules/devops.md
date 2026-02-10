---
globs:
  - "Dockerfile"
  - "docker-compose.*"
  - ".github/**"
  - "Makefile"
---

# DevOps Conventions (Go)

## Docker
- Use multi-stage builds (builder stage + production stage)
- Pin base image versions (e.g., `golang:1.22-alpine`, not `golang:latest`)
- Use `.dockerignore` to exclude unnecessary files
- Run as non-root user in production stage
- Order layers for maximum cache efficiency: copy `go.mod`/`go.sum` first, then `go mod download`, then copy source
- Final image: use `scratch` or `alpine` — no full OS images

## Docker Compose
- Use named volumes for persistent data (database)
- Define healthchecks for services
- Use environment variables for configuration, with defaults
- Separate service definitions for dev vs test environments

## Makefile / Scripts
- Provide standard targets: `setup`, `dev`, `test`, `lint`, `build`, `migrate`, `seed`
- Include `help` target that lists available commands
- Use `.PHONY` for non-file targets
- Scripts must be idempotent (safe to run multiple times)

## CI/CD (GitHub Actions)
- Run on: push to main, PR to main
- Steps: lint (`golangci-lint`), build, test
- Cache Go modules between runs (`actions/cache` with `go.sum` key)
- Fail fast on lint errors before running tests
- When the test job uses a database service container, ALWAYS run migrations before tests
- Every new config variable added to `config.go` must be documented in `.env.example`

## Local Development
- Use k3d for local Kubernetes clusters (not Kind, not Minikube)
- CNPG operator installed via Helm on k3d
- `make cluster-up` creates a k3d cluster
- `make cluster-down` tears down the k3d cluster

## Go Tooling
- Linter: `golangci-lint` (config in `.golangci.yml`)
- Formatter: `gofmt` / `goimports`
- Build: `go build ./...`
- Test: `go test ./...` with `-race` flag

## Environment Config
- Document all variables in `.env.example`
- Group variables by service (app, database, external APIs)
- Use sensible defaults for development
- Never commit `.env` files — only `.env.example`
