#!/usr/bin/env bash
# cluster-setup.sh — Create a local k3d cluster and install CNPG operator.
# Idempotent: safe to run multiple times.
set -euo pipefail

CLUSTER_NAME="${K3D_CLUSTER:-daap-local}"
CNPG_VERSION="${CNPG_VERSION:-1.25.0}"
CNPG_RELEASE_BRANCH="$(echo "$CNPG_VERSION" | cut -d. -f1-2)"

echo "==> Setting up local development cluster..."

# 1. Create k3d cluster (idempotent)
if k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME} "; then
  echo "k3d cluster '${CLUSTER_NAME}' already exists. Skipping creation."
else
  echo "Creating k3d cluster '${CLUSTER_NAME}'..."
  k3d cluster create "${CLUSTER_NAME}" --wait
  echo "k3d cluster '${CLUSTER_NAME}' created."
fi

# 2. Install CNPG operator (idempotent)
if kubectl get deployment -n cnpg-system cnpg-controller-manager >/dev/null 2>&1; then
  echo "CNPG operator already installed. Skipping installation."
else
  echo "Installing CNPG operator v${CNPG_VERSION}..."
  kubectl apply --server-side -f \
    "https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-${CNPG_RELEASE_BRANCH}/releases/cnpg-${CNPG_VERSION}.yaml"
  echo "Waiting for CNPG operator to be ready..."
  kubectl wait --for=condition=Available deployment/cnpg-controller-manager \
    -n cnpg-system --timeout=120s
  echo "CNPG operator installed and ready."
fi

echo "==> Local development cluster is ready."
echo "    Cluster:  ${CLUSTER_NAME}"
echo "    CNPG:     v${CNPG_VERSION}"
echo ""
echo "    Run 'kubectl get pods -n cnpg-system' to verify."
