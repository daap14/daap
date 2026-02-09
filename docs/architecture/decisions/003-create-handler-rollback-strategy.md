# 003. Create Handler Rollback Strategy

## Status
Accepted

## Context
The `POST /databases` handler performs two sequential operations:
1. Insert a database record in PostgreSQL (status = "provisioning")
2. Create CNPG Cluster and Pooler resources in Kubernetes

If step 2 fails (K8s unavailable, invalid manifest, RBAC error, etc.), the database record remains in "provisioning" status with no backing Kubernetes resources. The reconciler will then endlessly attempt to check a non-existent cluster, logging warnings every cycle.

This document evaluates three strategies for handling this failure mode.

---

## Option A: Roll back the DB record on K8s failure

**Approach**: If `ApplyCluster` or `ApplyPooler` fails, immediately soft-delete (or hard-delete) the database record before returning the error response.

**Pros**:
- Clean state: no orphaned records in the database
- Simple to reason about — the create is atomic from the caller's perspective
- No changes needed to the reconciler
- The user can retry the exact same request immediately

**Cons**:
- Partial K8s state: if the Cluster was created but the Pooler fails, the Cluster resource is orphaned in K8s (need cleanup logic)
- The record's UUID is lost — audit trail / correlation is harder
- Hard-delete breaks the soft-delete convention; soft-delete leaves a "ghost" record
- Two-phase cleanup is complex: must also attempt `DeleteCluster` if only the Pooler failed

---

## Option B: Mark the record as "error" on K8s failure

**Approach**: If `ApplyCluster` or `ApplyPooler` fails, update the database record to `status = "error"` before returning the error response. Optionally store the error reason.

**Pros**:
- Record persists for audit / debugging (user can see what was attempted)
- Consistent with the existing status lifecycle (provisioning → error is already valid)
- The reconciler already handles "error" status (especially with the new drift-detection)
- No partial-delete complexity — the record simply reflects reality
- User can DELETE the errored record to retry cleanly, or an admin can investigate

**Cons**:
- Errored records accumulate unless the user explicitly deletes them
- The reconciler would re-check errored records (already implemented via Suggestion 7), but since no K8s resource exists it will keep logging warnings
- Slightly more complex response — 500 with a record that exists but is in "error" state

---

## Option C: Let the reconciler handle the "cluster not found" case

**Approach**: Leave the record in "provisioning" and make the reconciler resilient to missing clusters. When `GetClusterStatus` returns a NotFound error, the reconciler marks the record as "error" automatically.

**Pros**:
- Zero changes to the Create handler — simplest code change
- The reconciler is already the single source of truth for status transitions
- Handles other edge cases too (e.g., someone manually deletes the K8s resource)
- Eventually consistent — the record will be corrected within one reconciler interval

**Cons**:
- Delayed error detection: the user gets a 500 but the record stays "provisioning" for up to `RECONCILER_INTERVAL` seconds before being marked "error"
- The 500 response from Create already signals failure, but the GET response shows "provisioning" — inconsistent
- Requires distinguishing "NotFound" from other K8s errors in the reconciler

---

## Decision

**Option B** — mark the database record as "error" in the Create handler when K8s resource creation fails.

The handler calls `UpdateStatus(id, {Status: "error"})` before returning the 500 response. This gives immediate visibility to the API consumer and keeps the status lifecycle consistent.

## Consequences

### Positive
- Immediate error visibility for the API consumer
- Clean status lifecycle with no orphaned "provisioning" records
- Record persists for audit/debugging — user can inspect and DELETE to retry
- No partial-delete complexity (no need to roll back the DB record)

### Negative
- Errored records accumulate and require manual cleanup or a TTL-based garbage collector
- Partial K8s state possible: if the Cluster was created but the Pooler fails, the Cluster remains in K8s while the record is marked "error" — the user must DELETE to trigger K8s cleanup
