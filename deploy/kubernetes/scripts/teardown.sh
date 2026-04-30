#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K8S_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
NAMESPACE="fleetshift"

echo "=== FleetShift Kubernetes Teardown ==="

# --- Preconditions ---
command -v oc >/dev/null 2>&1 || { echo "ERROR: 'oc' CLI not found."; exit 1; }
timeout 5 oc whoami &>/dev/null || { echo "ERROR: Not logged in to OpenShift. Run 'oc login' first."; exit 1; }

if ! oc get namespace "${NAMESPACE}" &>/dev/null; then
  echo "Namespace '${NAMESPACE}' not found. Nothing to tear down."
  exit 0
fi

MODE="${1:-kustomize}"

case "${MODE}" in
  kustomize)
    echo "Removing resources via Kustomize..."
    oc delete -k "${K8S_DIR}" --ignore-not-found=true
    echo "Deleting namespace..."
    oc delete namespace "${NAMESPACE}" --ignore-not-found=true
    ;;
  namespace)
    echo "Deleting entire namespace (all resources)..."
    oc delete namespace "${NAMESPACE}"
    ;;
  *)
    echo "Usage: $0 [kustomize|namespace]"
    echo "  kustomize (default): delete Kustomize resources, then namespace"
    echo "  namespace: delete entire namespace at once"
    exit 1
    ;;
esac

echo ""
echo "=== Teardown Complete ==="
