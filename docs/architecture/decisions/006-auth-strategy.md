# 006. Auth Strategy

## Status
Accepted

## Context
DAAP is a platform-internal Database as a Service API. Starting with v0.4, we need authentication and authorization to control who can manage platform resources (teams, users) and who can operate on databases. The system must support:

- A **superuser** identity that bootstraps the platform (creates teams and users) but cannot perform business operations.
- **Teams** with hardcoded roles (`platform` or `product`) that determine access scope.
- **Users** belonging to teams, each authenticated by an API key.
- **Ownership scoping**: product-role users see only their team's databases.

We need to decide on the authentication mechanism, key format and storage, authorization model, and middleware architecture.

### Options Considered

| Option | Pros | Cons |
|--------|------|------|
| **API keys with bcrypt** | Simple, stateless, no token lifecycle, sufficient for platform-internal use | No automatic expiry, no standard protocol |
| JWT (self-issued) | Stateless verification, standard format, can embed claims | Requires token refresh flow, key management for signing, over-engineered for internal API |
| OAuth2 / OIDC | Industry standard, supports external IdPs | Requires authorization server infrastructure, extreme overhead for a platform-internal service with < 100 users |

### Hashing: bcrypt vs argon2id

| Option | Pros | Cons |
|--------|------|------|
| **bcrypt (cost=12)** | Built into `golang.org/x/crypto/bcrypt`, well-understood, single-function API, sufficient for high-entropy keys | Not memory-hard (weaker against GPU attacks on low-entropy passwords) |
| argon2id | Memory-hard, recommended for user passwords | Requires tuning memory/time/parallelism params, `golang.org/x/crypto/argon2` has a lower-level API requiring manual salt management |

API keys are 32 random bytes (256 bits of entropy) — far above the threshold where bcrypt's lack of memory-hardness matters. Argon2id's advantages apply to low-entropy user-chosen passwords, not to cryptographically random keys. bcrypt with cost=12 provides a clean, battle-tested solution with minimal code.

## Decision

### 1. API Key Authentication
Use API keys passed in the `X-API-Key` request header. Keys are validated against bcrypt hashes stored in the database. No tokens, no sessions, no external identity provider.

### 2. Key Format
Generate keys as: 32 cryptographically random bytes -> base64url encoding -> prepend `daap_` prefix. Total length is approximately 47 characters.

```
daap_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2
```

The `daap_` prefix makes keys identifiable in logs, configs, and secret scanners. It also enables grep-based detection of accidentally committed keys.

### 3. Key Prefix for Lookup
Store the first 8 characters of the full key (including the `daap_` prefix) as `api_key_prefix` in the users table, in plaintext. This enables efficient database lookup: on each request, extract the prefix, query for matching active users, then bcrypt-compare the full key against each candidate.

Without the prefix, authentication would require loading all active users and bcrypt-comparing against each one — O(n) bcrypt operations per request, which is prohibitively slow at cost=12.

With the prefix, the lookup narrows to at most 1-2 candidates (prefix collisions are statistically rare for 8-character prefixes over a small user population).

A partial index on `api_key_prefix WHERE revoked_at IS NULL` ensures the lookup only scans active users.

### 4. Hashing
bcrypt with cost=12. The `golang.org/x/crypto/bcrypt` package is already an indirect dependency via client-go and is promoted to a direct dependency. Cost=12 provides ~250ms hash time on modern hardware — acceptable for the authentication-per-request pattern since the prefix lookup keeps the number of bcrypt comparisons to ~1.

### 5. User Model
Three entities in the database:

- **Superuser**: A single user row with `is_superuser = TRUE` and `team_id = NULL`. Enforced by a partial unique index (`CREATE UNIQUE INDEX ... WHERE is_superuser = TRUE`). Auto-bootstrapped on first startup when the users table is empty. Can only manage teams and users. Cannot access business endpoints (databases).

- **Teams**: Named groups with a `role` column constrained to `platform` or `product` via `CHECK`. Created by the superuser. Cannot be deleted if they still have active (non-revoked) users (`ON DELETE RESTRICT` on the FK).

- **Users**: Belong to exactly one team via `team_id` FK. Each user has a unique API key (prefix + bcrypt hash). Created by the superuser. Soft-revoked by setting `revoked_at` (not hard-deleted, for audit trail).

### 6. Superuser Bootstrap
On server startup, after running migrations, the auth service checks `CountAll()` on the users table:
- If the table is empty: create a superuser with a generated API key. Log the raw key once via `slog.Info("Superuser API key created", "key", rawKey)`. The key is never stored in plaintext and is only displayed this one time.
- If the table is not empty: no-op. This makes the bootstrap idempotent on subsequent restarts.

### 7. Middleware Stack Order
```
RequestID -> Recovery -> Auth -> AuthZ -> Logger -> Handler
```

- **RequestID**: Assigns a UUID to every request for tracing.
- **Recovery**: Catches panics and returns 500 instead of crashing.
- **Auth**: Extracts `X-API-Key`, resolves to an `Identity` (user + team + role), stores in context. Missing/invalid key -> 401.
- **AuthZ**: Per-route-group middleware that checks the identity's role/superuser status against the required permission. Unauthorized -> 403.
- **Logger**: Logs the request with the resolved identity context (user ID, team, role).

Auth middleware is not applied to public routes (`/health`, `/openapi.json`), which live in a separate Chi route group.

### 8. Authorization Matrix

| Caller | `/teams` | `/users` | `/databases` | `/health`, `/openapi.json` |
|--------|----------|----------|--------------|---------------------------|
| **Superuser** | Full CRUD | Full CRUD | **403 Forbidden** | Public |
| **Platform user** | 403 Forbidden | 403 Forbidden | Full access (all databases) | Public |
| **Product user** | 403 Forbidden | 403 Forbidden | Own team's databases only | Public |
| **Unauthenticated** | 401 | 401 | 401 | Public |

Implemented via two AuthZ middlewares:
- `RequireSuperuser()`: Applied to `/teams` and `/users` route groups. Rejects non-superuser identities with 403.
- `RequireRole("platform", "product")`: Applied to `/databases` route group. Rejects the superuser (who has no role) and any identity whose team role is not in the allowed list with 403.

### 9. Ownership Scoping
Product-role users are scoped to their own team's databases:
- **List** (`GET /databases`): The handler auto-injects an `owner_team` filter matching the user's team name. Platform users see all databases.
- **Get/Update/Delete** (`GET/PATCH/DELETE /databases/{id}`): If the database's `ownerTeam` does not match the user's team name, return **404 Not Found** (not 403). This prevents information leakage about the existence of other teams' resources.
- **Create** (`POST /databases`): Product users' `ownerTeam` is auto-set to their team name. If explicitly provided and mismatched, return 403. Platform users can set any `ownerTeam`.

## Consequences

### Positive
- Simple, stateless authentication with no external infrastructure dependencies.
- bcrypt + high-entropy keys provide strong security with minimal code complexity.
- The prefix lookup pattern keeps authentication performant (single DB query + ~1 bcrypt comparison per request).
- The superuser bootstrap eliminates the chicken-and-egg problem of creating the first privileged identity.
- Ownership scoping at the handler level reuses the existing `ListFilter.OwnerTeam` field — no database schema changes needed for the databases table.
- 404 (not 403) for non-owned resources follows security best practices for information leakage prevention.

### Negative
- API keys have no automatic expiry or rotation mechanism. Keys must be manually revoked. TTL-based expiry is deferred to a future iteration.
- bcrypt comparison on every request adds ~250ms latency. Mitigated by the prefix lookup keeping comparisons to ~1 per request, but there is no caching layer. A future iteration could add an in-memory cache with short TTL.
- The superuser key is logged to stdout on first startup. In production, log aggregation must be secured. An alternative (writing to a file or K8s Secret) adds complexity without clear benefit for an initial implementation.
- No audit logging of authentication events (successful logins, failed attempts, revocations). Deferred to v0.9 (Security Defaults).

### Neutral
- `golang.org/x/crypto/bcrypt` is already an indirect dependency via client-go. This ADR promotes it to a direct dependency — no new transitive dependencies are introduced.
- The `X-API-Key` header is a non-standard header. Standard alternatives (`Authorization: Bearer`) were considered but `X-API-Key` is more explicit about the authentication mechanism and avoids confusion with OAuth bearer tokens.
- One API key per user. Multiple keys per user are not supported in v0.4 but could be added later by extracting keys into a separate table.
