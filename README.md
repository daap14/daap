# DAAP

AI-assisted product development workflow. Transforms a product manifesto into a working API/backend through iterative releases with feedback loops.

## Getting Started

1. Write your product vision in `docs/MANIFESTO.md`
2. Run `/breakdown-manifesto` to create iteration specs and GitHub issues
3. Discuss tech stack with Claude to produce an ADR
4. Run `/plan-iteration docs/iterations/v0.1.md` to create the first technical plan
5. Create an agent team to start building

## Project Structure

See `CLAUDE.md` for full project context and conventions.

## API Documentation

The OpenAPI 3.1 specification is available at runtime:

```
GET /openapi.json
```

The spec source file is at `api/openapi.yaml`. It is embedded into the binary at build time and served as JSON.

## Authentication

### Domain Model

DAAP uses API key authentication with role-based access control:

- **Superuser**: A single admin identity, auto-created on first startup. Manages teams and users but cannot access business endpoints (tiers, databases).
- **Teams**: Groups of users with an assigned role (`platform` or `product`).
- **Users**: Belong to a team. Each user has a unique API key for authentication.

### Bootstrap

On first startup (empty `users` table), the server creates a superuser and logs the API key once:

```
INFO Superuser API key created key=daap_...
```

This key is never shown again. Store it securely.

### Making Authenticated Requests

Pass the API key in the `X-API-Key` header:

```bash
curl -H "X-API-Key: daap_..." http://localhost:8080/teams
```

### Roles and Permissions

| Caller | `/teams` | `/users` | `/blueprints` | `/tiers` | `/databases` | `/health`, `/openapi.json` |
|---|---|---|---|---|---|
| Superuser | Full access | Full access | No access (403) | No access (403) | No access (403) | Public |
| Platform user | No access (403) | No access (403) | Full CRUD | Full CRUD | Full access (all databases) | Public |
| Product user | No access (403) | No access (403) | Read-only | Read-only (redacted) | Own team's databases only | Public |
| Unauthenticated | 401 | 401 | 401 | 401 | 401 | Public |

### Public Endpoints

The following endpoints require no authentication:

- `GET /health` -- server health check
- `GET /openapi.json` -- OpenAPI specification

## API Endpoints

### Teams (superuser-only)

| Method | Path | Description |
|---|---|---|
| `POST` | `/teams` | Create a team |
| `GET` | `/teams` | List all teams |
| `DELETE` | `/teams/{id}` | Delete a team |

### Users (superuser-only)

| Method | Path | Description |
|---|---|---|
| `POST` | `/users` | Create a user (returns API key once) |
| `GET` | `/users` | List all users (metadata only) |
| `DELETE` | `/users/{id}` | Revoke a user |

### Blueprints

Blueprints define infrastructure templates — multi-document YAML manifests with Go template placeholders. Each blueprint is bound to a provider (e.g., `cnpg`). Platform users manage blueprints; product users can read them.

| Method | Path | Description | Access |
|---|---|---|---|
| `POST` | `/blueprints` | Create a blueprint | Platform only |
| `GET` | `/blueprints` | List all blueprints | Platform / Product |
| `GET` | `/blueprints/{id}` | Get a blueprint by ID | Platform / Product |
| `DELETE` | `/blueprints/{id}` | Delete a blueprint | Platform only |

A blueprint cannot be deleted while tiers reference it (returns 409 `BLUEPRINT_HAS_TIERS`).

### Tiers

Tiers link a blueprint to operational policies (destruction strategy, backup). Creating a tier requires a `blueprintName` referencing an existing blueprint. Platform users manage tiers; product users see only a summary (id, name, description).

| Method | Path | Description | Access |
|---|---|---|---|
| `POST` | `/tiers` | Create a tier | Platform only |
| `GET` | `/tiers` | List all tiers | Platform (full) / Product (summary) |
| `GET` | `/tiers/{id}` | Get a tier by ID | Platform (full) / Product (summary) |
| `PATCH` | `/tiers/{id}` | Update a tier | Platform only |
| `DELETE` | `/tiers/{id}` | Delete a tier | Platform only |

Product users receive a redacted response with only `id`, `name`, and `description`. Platform users see all fields including `blueprintId`, `blueprintName`, `destructionStrategy`, and `backupEnabled`.

A tier cannot be deleted while databases reference it (returns 409 `TIER_HAS_DATABASES`).

### Databases (platform/product roles)

| Method | Path | Description |
|---|---|---|
| `POST` | `/databases` | Create a database |
| `GET` | `/databases` | List databases |
| `GET` | `/databases/{id}` | Get a database by ID |
| `PATCH` | `/databases/{id}` | Update a database |
| `DELETE` | `/databases/{id}` | Delete a database |

Creating a database requires a `tier` name (e.g., `"tier": "standard"`). The tier's linked blueprint determines the infrastructure manifests applied via the provider.

## Development

```bash
make setup    # Initial project setup
make dev      # Start dev server
make test     # Run tests
make lint     # Run linter
```

## License

TBD
