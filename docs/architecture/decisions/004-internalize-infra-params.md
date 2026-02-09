# 004. Internalize Infrastructure Parameters

## Status
Accepted

## Context
The DAAP manifesto states:

> "Consumers do NOT interact with: instances, clusters, replication topology, storage layout. Infrastructure is an implementation detail."

However, the v0.2 API exposed `instances` and `storageSize` as user-supplied fields in `POST /databases`, and `dbname` (always `"app"`, a CNPG default) appeared in GET responses. This violated the product principle that infrastructure details should be invisible to consumers.

## Decision
Remove `instances`, `storageSize`, and `dbname` from the API surface entirely:

- **`instances` and `storageSize`**: No longer accepted in `POST /databases` request bodies. The platform sets these as internal defaults in the template builder (`instances=1`, `storageSize=1Gi`).
- **`dbname`**: Removed from the database schema (migration 003), the domain model, and all API responses. The database name is always `app` (CNPG convention) and consumers access it via the Kubernetes Secret referenced by `secretName`.

The API now accepts only `name`, `ownerTeam`, `purpose`, and `namespace` for database creation.

## Consequences
- **Positive**: API aligns with the manifesto — consumers focus on what they need (a database), not how it's provisioned.
- **Positive**: Reduces validation complexity (no instances/storageSize checks).
- **Positive**: Prevents consumers from making poor infrastructure choices (e.g., 10 replicas for a dev database).
- **Negative**: Platform operators who need to customize instances or storage per database must modify the template builder code or add a separate admin API in the future.
- **Neutral**: Existing databases with `dbname` in the schema will have the column dropped by migration 003.
