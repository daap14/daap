---
globs: ["**/reconciler/**/*.go"]
---

# Reconciler Conventions

## Status Coverage
- The reconciler must watch ALL active database statuses, not just "provisioning"
- "ready" databases must be monitored for drift (cluster becoming unhealthy)
- "error" databases must be checked for recovery (cluster becoming healthy again)
- Only "deleted" databases should be excluded from reconciliation

## Idempotency
- Status updates must be idempotent: don't update a record to "ready" if it's already "ready"
- Check `db.Status != targetStatus` before issuing an update to avoid unnecessary DB writes

## Credential Handling
- The reconciler must NEVER read or store plaintext credentials from Kubernetes Secrets
- Store only the K8s Secret reference name (e.g., `{cluster-name}-app`)
- Consumers retrieve credentials directly from K8s when needed
