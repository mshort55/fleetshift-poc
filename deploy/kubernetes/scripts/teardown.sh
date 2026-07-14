#!/usr/bin/env bash
set -euo pipefail

# ------------------------------------------------------------------
# FleetShift Kubernetes Teardown
#
# Removes all FleetShift resources from an OpenShift cluster.
# Called by 'task kubernetes:teardown'.
#
# Steps:
#   1. Deletes Kustomize-managed resources (deployment, services, routes, etc.)
#   2. Deletes the fleetshift namespace
#
# Prerequisites:
#   - 'oc' CLI installed
#   - Logged into an OpenShift cluster (oc login)
#
# Usage:
#   ./teardown.sh
# ------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K8S_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
# shellcheck source=../../scripts/common.sh
source "$(cd "$SCRIPT_DIR/../../scripts" && pwd)/common.sh"
NAMESPACE="fleetshift"

echo "=== FleetShift Kubernetes Teardown ==="

# --- Preconditions ---
command -v oc >/dev/null 2>&1 || { echo "ERROR: 'oc' CLI not found."; exit 1; }
require_oc_login

if ! oc get namespace "${NAMESPACE}" &>/dev/null; then
  echo "Namespace '${NAMESPACE}' not found. Nothing to tear down."
  exit 0
fi

echo "Removing resources via Kustomize..."
oc delete -k "${K8S_DIR}" --ignore-not-found=true

echo "Deleting namespace..."
oc delete namespace "${NAMESPACE}" --ignore-not-found=true

echo ""
echo "=== Teardown Complete ==="
