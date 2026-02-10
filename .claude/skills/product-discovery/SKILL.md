---
description: Run product discovery — parallel research, user interviews, v1 vision, and iteration roadmap
user-invocable: true
disable-model-invocation: true
tools:
  - Read
  - Glob
  - Grep
---

# /product-discovery

Orchestrate the product team to go from research to a v1 roadmap.

## Prerequisites
- Product manifesto exists at `docs/MANIFESTO.md`
- At least v0.1 has been built (codebase context needed for strategist and PM)

## Steps

### 1. Spawn 3 product-researcher agents in parallel

Create a team and spawn 3 instances of `product-researcher` simultaneously, each with a different research brief:

**Researcher A — DBaaS Platforms**:
> Research existing Database-as-a-Service platforms: Neon, PlanetScale, Aiven, CrunchyBridge, AWS RDS, GCP Cloud SQL, Azure Database, Heroku Postgres. Focus on features, APIs, self-service capabilities, tier/pricing models, and abstraction level. Write your findings to `docs/research/dbaas-platforms.md`.

**Researcher B — IDP & Developer Experience**:
> Research Internal Developer Platforms and developer experience patterns: Backstage, Humanitec, Port, Kratix, and custom IDPs. Focus on how developers consume platform services, self-service patterns, golden paths, API design, and onboarding UX. Write your findings to `docs/research/idp-developer-experience.md`.

**Researcher C — Platform Operations & SRE**:
> Research platform operations and SRE patterns for managed databases: backup/restore practices, observability stacks, compliance and security patterns, multi-tenancy, Kubernetes database operators (CNPG, Zalando Postgres Operator, KubeDB, Percona). Write your findings to `docs/research/platform-operations.md`.

### 2. Wait for all research to complete

Verify all 3 files exist:
- `docs/research/dbaas-platforms.md`
- `docs/research/idp-developer-experience.md`
- `docs/research/platform-operations.md`

Briefly review each for completeness before proceeding.

### 3. Spawn product-strategist

Spawn the `product-strategist` agent:
> Read all research documents in `docs/research/`, the manifesto, and the current project state. Follow your two-phase workflow: first define the responsibility model (interview the user), then define the v1 vision (interview the user again).

**Important**: The strategist will send you questions for the user. Relay these to the user and relay the answers back. Do not answer on behalf of the user.

Wait for:
- `docs/product/responsibility-model.md`
- `docs/product/v1-vision.md`

### 4. Spawn product-manager

Spawn the `product-manager` agent:
> Read the v1 vision, responsibility model, existing iteration specs, ADRs, and current codebase. Produce the iteration roadmap at `docs/product/v1-roadmap.md`.

Wait for:
- `docs/product/v1-roadmap.md`

### 5. Review with user

Present the roadmap to the user for review. If changes are needed:
- For roadmap ordering/scope changes: re-spawn the product-manager with feedback
- For vision changes: re-spawn the product-strategist, then the PM
- For research gaps: re-spawn a researcher with a specific brief

## Output
- `docs/research/dbaas-platforms.md` — DBaaS market research
- `docs/research/idp-developer-experience.md` — IDP and developer experience research
- `docs/research/platform-operations.md` — Platform operations research
- `docs/product/responsibility-model.md` — Responsibility matrix
- `docs/product/v1-vision.md` — V1 feature definition
- `docs/product/v1-roadmap.md` — Iteration roadmap to v1
