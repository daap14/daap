#!/usr/bin/env bash
# cluster-teardown.sh — Delete the local k3d cluster.
# Idempotent: safe to run multiple times.
set -euo pipefail

CLUSTER_NAME="${K3D_CLUSTER:-daap-local}"

if k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME} "; then
  echo "Deleting k3d cluster '${CLUSTER_NAME}'..."
  k3d cluster delete "${CLUSTER_NAME}"
  echo "k3d cluster '${CLUSTER_NAME}' deleted."
else
  echo "k3d cluster '${CLUSTER_NAME}' does not exist. Nothing to do."
fi
