# 001. Tech Stack and Project Layout

## Status
Accepted

## Context
DAAP is a Database as a Service platform that exposes databases (backed by CNPG on Kubernetes) through a REST API. Iteration v0.1 requires a walking skeleton: a Go API server that proves connectivity to Kubernetes and the CNPG operator, with a health endpoint, response envelope, dev tooling, and CI.

We need to decide on:
- HTTP framework/router
- Kubernetes client approach and CNPG integration
- Configuration management
- Logging
- Testing framework and tooling
- Project directory layout and key dependencies

## Decision

### Language: Go 1.22+
Go is the natural choice for Kubernetes-native tooling. The CNPG operator is written in Go, and client-go is the canonical Kubernetes client. Go 1.22 brings enhanced routing patterns in `net/http` and improved toolchain management.

### HTTP Router: Chi (`github.com/go-chi/chi/v5`)
Chi was selected over Gin and the standard library router.

| Option | Pros | Cons |
|--------|------|------|
| **Chi** | Lightweight, idiomatic `net/http` compatible, excellent middleware chaining, zero external deps, context-based | Smaller community than Gin |
| Gin | Large community, rich ecosystem, fast | Non-standard interfaces (`gin.Context`), heavier, opinionated |
| `net/http` (1.22) | Zero deps, new pattern matching | No middleware chaining, manual route grouping, more boilerplate |

Chi wins because it composes standard `http.Handler` interfaces, which means middleware and handlers are portable and testable without framework coupling. For a Kubernetes-native service, staying close to the standard library is more important than Gin's convenience features.

### Kubernetes Client: client-go + CNPG Go API types
- `k8s.io/client-go` for Kubernetes API access (both in-cluster via ServiceAccount and out-of-cluster via kubeconfig).
- `github.com/cloudnative-pg/cloudnative-pg/api/v1` for typed CNPG custom resource definitions (Cluster, Backup, ScheduledBackup, etc.).
- Version pinning: CNPG API types and client-go versions must be compatible. Pin to CNPG v1.25.x (latest stable) and the corresponding client-go version it depends on.

### Configuration: `github.com/kelseyhightower/envconfig`
Chosen over Viper and manual `os.Getenv`:

| Option | Pros | Cons |
|--------|------|------|
| **envconfig** | Minimal, struct-based, zero-config, env-var-native | No file-based config |
| Viper | File + env + flags, widely used | Heavy, complex, many transitive deps |
| Manual `os.Getenv` | Zero deps | Verbose, no validation, no struct mapping |

envconfig maps environment variables directly to a typed Go struct with validation tags. For a Kubernetes-deployed service, environment variables are the standard configuration mechanism (12-factor app). File-based config is not needed.

Config struct (in `internal/config/`):
```go
type Config struct {
    Port           int    `envconfig:"PORT" default:"8080"`
    LogLevel       string `envconfig:"LOG_LEVEL" default:"info"`
    KubeconfigPath string `envconfig:"KUBECONFIG_PATH" default:""`
    Namespace      string `envconfig:"NAMESPACE" default:"default"`
    Version        string `envconfig:"VERSION" default:"dev"`
}
```

### Logging: `log/slog` (standard library)
Go 1.21+ includes `log/slog` for structured logging. No external dependency needed. Supports JSON output, log levels, and structured key-value pairs out of the box. Sufficient for v0.1; can be revisited if more advanced features (e.g., log rotation, sampling) are needed later.

### Testing: standard `testing` + `github.com/stretchr/testify`
- `testing` package for test execution and benchmarks.
- `testify` for assertions (`assert`, `require`) and mocking (`mock`). This is the de facto standard in the Go ecosystem.
- No test framework beyond this for v0.1.

### Hot Reload: `github.com/air-verse/air`
`air` watches for file changes and rebuilds/restarts the binary automatically. Used only in development (`make dev`). Not a Go module dependency; installed as a tool.

### Linting: golangci-lint
`golangci-lint` aggregates multiple linters. Start with a permissive config for v0.1 and tighten in later iterations. Key linters enabled: `govet`, `errcheck`, `staticcheck`, `gosimple`, `unused`, `ineffassign`, `gofmt`, `goimports`.

### Project Layout

```
daap/
├── cmd/
│   └── server/
│       └── main.go              # Entry point: config, wiring, server start
├── internal/
│   ├── api/
│   │   ├── handler/
│   │   │   └── health.go        # Health endpoint handler
│   │   ├── middleware/
│   │   │   ├── envelope.go      # Response envelope middleware
│   │   │   └── requestid.go     # Request ID middleware
│   │   ├── response/
│   │   │   └── envelope.go      # Response envelope types
│   │   └── router.go            # Chi router setup, route registration
│   ├── config/
│   │   └── config.go            # Config struct + loading via envconfig
│   └── k8s/
│       └── client.go            # K8s client init, CNPG typed client, health check
├── tests/
│   ├── unit/                    # Unit tests mirroring internal/ structure
│   ├── integration/             # Integration tests (real HTTP, mocked K8s)
│   └── fixtures/                # Test data and fixtures
├── docs/                        # Architecture, iterations, feedback
├── scripts/                     # Helper scripts (cluster setup, etc.)
├── .github/workflows/           # CI pipeline
├── Dockerfile                   # Multi-stage build
├── docker-compose.yml           # Local dev services
├── Makefile                     # Standard targets
├── .golangci.yml                # Linter config
├── .env.example                 # Environment variable documentation
├── go.mod                       # Go module definition
└── go.sum                       # Dependency checksums
```

Key layout decisions:
- `cmd/server/` for the single entry point. Additional commands (CLI tools, migrations) can be added under `cmd/` later.
- `internal/` enforces Go's access control: packages here cannot be imported by external modules.
- `internal/api/` groups all HTTP-layer code (handlers, middleware, response types, router).
- `internal/k8s/` isolates Kubernetes client logic from the API layer.
- `tests/` is separate from `internal/` to follow the project convention in `.claude/rules/testing.md`. Tests use `_test` package suffix for black-box testing where appropriate.

### Key Dependencies (go.mod)

| Module | Version | Purpose |
|--------|---------|---------|
| `github.com/go-chi/chi/v5` | v5.1.x | HTTP router |
| `k8s.io/client-go` | v0.31.x | Kubernetes API client |
| `k8s.io/apimachinery` | v0.31.x | K8s API types and utilities |
| `github.com/cloudnative-pg/cloudnative-pg` | v1.25.x | CNPG CRD types |
| `github.com/kelseyhightower/envconfig` | v1.2.x | Config from env vars |
| `github.com/stretchr/testify` | v1.9.x | Test assertions and mocks |
| `github.com/google/uuid` | v1.6.x | UUID generation for request IDs |

## Consequences

### Positive
- Chi's `net/http` compatibility means handlers and middleware are framework-independent and easy to test.
- envconfig keeps configuration simple and Kubernetes-native (no config files to mount).
- `log/slog` removes a dependency while providing structured logging.
- The project layout separates concerns clearly and follows Go conventions (`cmd/`, `internal/`).
- client-go + CNPG types give fully typed access to Kubernetes and CNPG resources.

### Negative
- Chi has a smaller ecosystem than Gin; some middleware may need to be written manually.
- envconfig does not support config files, which may be limiting if non-K8s deployment targets are needed later (unlikely given the product scope).
- Pinning CNPG and client-go versions tightly creates an upgrade coupling: CNPG major version bumps will require coordinated client-go upgrades.

### Neutral
- The test directory structure (`tests/` separate from source) differs from the common Go pattern of co-located `_test.go` files but follows the project's testing convention.
- `air` for hot reload is a dev-only tool and does not affect the production build.
