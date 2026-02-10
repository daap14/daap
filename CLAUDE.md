# DAAP — AI-Assisted Product Development

## Project Overview
DAAP (Developer AI-Assisted Product) transforms a product manifesto into a working API/backend through iterative releases with feedback loops. Claude operates as the entire development team via agent teams.

## Current State
- **Phase**: v0.2 released, ready for v0.3 planning
- **Current Iteration**: v0.3 (next)
- **Last Release**: v0.2.0 — Database Lifecycle (CRUD, CNPG provisioning, reconciler)
- **Tech Stack**: Go (Chi router, client-go, CNPG Go API types, pgx — see ADRs 001, 002)

## Tech Stack
- **Language**: Go 1.22+
- **HTTP Router**: Chi or Gin (decided in ADR 001)
- **K8s Client**: client-go + CNPG Go API types
- **Details**: See `docs/architecture/decisions/001-tech-stack.md`

## Key Commands
```bash
make setup         # Initial project setup
make dev           # Start dev server with hot reload
make test          # Run all tests (unit)
make test-integration  # Run integration tests (requires K8s)
make lint          # Run linter
make build         # Build production artifacts
make migrate       # Run database migrations
make seed          # Seed database with sample data
make cluster-up    # Create local K8s cluster (k3d)
make cluster-down  # Tear down local K8s cluster
make cnpg-install  # Install CNPG operator on local cluster
```

## Project Structure
```
daap/
├── CLAUDE.md              # This file — project context
├── .claude/               # Agent teams, skills, hooks, rules
│   ├── settings.json      # Agent teams + hooks config
│   ├── agents/            # Dev team: architect, dev, reviewer, local-devops
│   │                      # Product team: product-researcher, product-strategist, product-manager
│   ├── skills/            # 9 skill playbooks
│   ├── rules/             # File-specific conventions
│   └── hooks/             # Quality gate scripts
├── .github/               # Issue templates, PR template
├── docs/
│   ├── MANIFESTO.md       # Product vision (user writes this)
│   ├── product/           # V1 vision, responsibility model, roadmap
│   ├── research/          # Market research and analysis
│   ├── iterations/        # vX.Y.md specs and plans
│   ├── architecture/      # ADRs in decisions/
│   └── feedback/          # Per-iteration feedback logs
├── internal/              # Production code (Go packages)
├── cmd/                   # Entry points (server)
├── migrations/            # Database migrations
└── tests/                 # Test files (unit + integration)
```

## Conventions

### Git
- **Commits**: Conventional Commits — `type(scope): description`
- **Branches**: `feat/description`, `fix/description`, `chore/description`
- **PRs**: Always target `master`, one PR per task, reviewed before merge

### Code
- See `.claude/rules/` for file-specific conventions
- Composition over inheritance
- No hardcoded secrets — use environment variables
- Parameterized queries only — no string concatenation for SQL
- 80% test coverage target

### API
- RESTful design with consistent response envelope: `{data, error, meta}`
- Input validation on all endpoints
- See `.claude/rules/api-design.md`

## Agent Team Roles

### File Ownership Boundaries
| Agent | Owns | Cannot Touch |
|-------|------|-------------|
| **architect** | `docs/architecture/`, `docs/iterations/`, `*.schema.*`, `*.openapi.*` | `internal/`, `cmd/`, `tests/`, infra files |
| **dev** | `internal/`, `cmd/`, `migrations/`, `tests/`, `*_test.go`, `README.md`, `.claude/rules/`, `go.mod`, `go.sum` | `docs/` (except README.md), infra files |
| **reviewer** | `.claude/rules/` (from review patterns) | Production code, tests, infra files — reviews via GitHub PR comments only |
| **local-devops** | `Dockerfile`, `docker-compose.*`, `.github/workflows/`, `Makefile`, `scripts/`, `.env.example`, `.golangci.yml` | `internal/`, `cmd/`, `tests/`, `docs/` |
| **product-researcher** | `docs/research/` | Everything else |
| **product-strategist** | `docs/product/` | Everything else |
| **product-manager** | `docs/product/` | Everything else |

### Dev Workflow (per task)
1. Dev creates feature branch + implements code + tests + docs
2. Dev creates PR on GitHub
3. Dev waits for CI to pass (fixes failures if needed)
4. Once CI green: dev notifies reviewer
5. Reviewer adds comments on the PR
6. Dev applies suggestions, pushes, notifies reviewer to re-review
7. Repeat 5-6 until approved → merge

### Learning Loop
- Reviewer and dev capture reusable patterns as `.claude/rules/` files
- Rules prevent the same review comment from being repeated
- Dev updates `README.md` when API endpoints change

### Lead (you) orchestrates:
- `/product-discovery` — research → responsibility model → v1 vision → roadmap
- `/breakdown-manifesto` — manifesto → iteration specs + GH issues
- `/plan-iteration` — iteration spec → technical task plan
- `/review-code` — PR review on GitHub
- `/create-release` — tag + changelog + GH release
- `/incorporate-feedback` — feedback → requirement changes

## Architecture Decisions
See `docs/architecture/decisions/` for ADRs. Format: `NNN-title.md`

## Iteration Lifecycle
1. Write manifesto → 2. Breakdown → 3. Tech stack ADR → 4. Dev env setup → 5. Plan iteration → 6. Execute (agent team) → 7. Review → 8. Release → 9. Feedback → 10. Incorporate → Loop to 5
