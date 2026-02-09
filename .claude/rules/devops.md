---
globs:
  - "Dockerfile"
  - "docker-compose.*"
  - ".github/**"
  - "Makefile"
---

# DevOps Conventions

## Docker
- Use multi-stage builds (dev stage + prod stage)
- Pin base image versions (e.g., `node:20.11-alpine`, not `node:latest`)
- Use `.dockerignore` to exclude unnecessary files
- Run as non-root user in production stage
- Order layers for maximum cache efficiency (deps before code)

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
- Steps: install, lint, typecheck, build, test
- Cache dependencies between runs
- Fail fast on lint/type errors before running tests
- Use matrix strategy for multiple runtime versions if needed
- When the test job uses a database service container, ALWAYS run migrations before tests
- Every new config variable added to config.go must be documented in `.env.example`

## Environment Config
- Document all variables in `.env.example`
- Group variables by service (app, database, external APIs)
- Use sensible defaults for development
- Never commit `.env` files — only `.env.example`
