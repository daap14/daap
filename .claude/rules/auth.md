---
globs: ["**/auth/**/*.go", "**/middleware/auth*.go", "**/middleware/authz*.go", "**/handler/team.go", "**/handler/user.go"]
---

# Authentication and Authorization Conventions

## Middleware Chain
- Public routes (`/health`, `/openapi.json`) are in a separate Chi group with no auth middleware
- Authenticated routes use `middleware.Auth(authService)` to resolve `X-API-Key` to an `Identity`
- Superuser-only routes add `middleware.RequireSuperuser()` after auth
- Business routes add `middleware.RequireRole("platform", "product")` after auth — this also blocks the superuser

## Identity Context
- Always use `middleware.GetIdentity(ctx)` to retrieve the authenticated identity in handlers
- In tests, use `middleware.WithIdentity(ctx, identity)` to inject identity without running auth middleware
- Never access identity from headers directly in handlers — the middleware is the single source of truth

## Ownership Scoping
- Product users are filtered by their team name on list endpoints (auto-inject `owner_team` filter)
- Product users get 404 (not 403) for resources owned by other teams — prevents information leakage
- Platform users see all resources with no ownership restrictions
- Ownership checks must happen before any mutation (check, then act)

## Superuser Constraints
- The superuser has no team and no role (`TeamID`, `TeamName`, `Role` are all nil)
- The superuser cannot access business endpoints (`/databases`) — `RequireRole` rejects nil role
- The superuser cannot be revoked — `DELETE /users/{id}` returns 403 for the superuser
- Only one superuser can exist (enforced by partial unique index on `is_superuser`)

## API Key Handling
- Raw API keys are returned only at user creation time — never again
- Never log or store the plaintext key beyond the bootstrap log message
- Never return `api_key_hash` in API responses — only `apiKeyPrefix` for identification
- Key format: `daap_` prefix + base64url-encoded 32 random bytes
- Lookup: extract first 8 chars as prefix, query `idx_users_api_key_prefix`, bcrypt-compare candidates

## Validation
- Team `role` must be exactly `"platform"` or `"product"` — reject all other values
- User `teamId` must reference an existing team — return 404 if not found
- Team names are unique — return 409 `DUPLICATE_NAME` on conflict
- Cannot delete a team with active (non-revoked) users — return 409 `TEAM_HAS_USERS`
