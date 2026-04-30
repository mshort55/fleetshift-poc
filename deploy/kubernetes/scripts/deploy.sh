#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K8S_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
NAMESPACE="fleetshift"

echo "=== FleetShift Kubernetes Deployment ==="
echo "Manifests: ${K8S_DIR}"
echo ""

# --- Preconditions ---
command -v oc >/dev/null 2>&1 || { echo "ERROR: 'oc' CLI not found."; exit 1; }
timeout 5 oc whoami &>/dev/null || { echo "ERROR: Not logged in to OpenShift. Run 'oc login' first."; exit 1; }

# --- Apply manifests ---
echo "Applying Kustomize manifests..."
oc apply -k "${K8S_DIR}"

# --- Wait for PostgreSQL ---
echo "Waiting for PostgreSQL to be ready..."
oc wait -n "${NAMESPACE}" \
  statefulset/postgres \
  --for=jsonpath='{.status.readyReplicas}'=1 \
  --timeout=120s

# --- Force image import ---
echo "Importing images from quay.io..."
oc import-image fleetshift-server:latest -n "${NAMESPACE}" --confirm 2>/dev/null || true
oc import-image fleetshift-web:latest -n "${NAMESPACE}" --confirm 2>/dev/null || true

# --- Ensure triggers ---
echo "Setting image triggers..."
oc set triggers deployment/fleetshift-server -n "${NAMESPACE}" \
  --from-image=fleetshift-server:latest -c fleetshift-server 2>/dev/null || true
oc set triggers deployment/fleetshift-server -n "${NAMESPACE}" \
  --from-image=fleetshift-web:latest -c web-builder --containers=web-builder 2>/dev/null || true

# --- Wait for server ---
echo "Waiting for fleetshift-server to be ready..."
oc wait -n "${NAMESPACE}" \
  deployment/fleetshift-server \
  --for=condition=Available \
  --timeout=300s

# --- Auth setup ---
echo "Checking auth-setup job..."
JOB_STATUS=$(oc get job auth-setup -n "${NAMESPACE}" -o jsonpath='{.status.succeeded}' 2>/dev/null || echo "")
if [ "${JOB_STATUS}" = "1" ]; then
  echo "Auth setup already completed."
else
  echo "Deleting previous auth-setup job (if any) and re-running..."
  oc delete job auth-setup -n "${NAMESPACE}" --ignore-not-found=true
  oc apply -k "${K8S_DIR}" -l "batch.kubernetes.io/job-name=auth-setup" 2>/dev/null || \
    oc apply -f "${K8S_DIR}/auth-setup/job.yaml" -n "${NAMESPACE}"
  echo "Waiting for auth-setup to complete..."
  oc wait -n "${NAMESPACE}" job/auth-setup --for=condition=Complete --timeout=120s || \
    echo "WARNING: Auth setup did not complete. Check logs: oc logs -n ${NAMESPACE} job/auth-setup"
fi

# --- Summary ---
echo ""
echo "=== Deployment Complete ==="
HTTP_ROUTE=$(oc get route fleetshift -n "${NAMESPACE}" -o jsonpath='{.spec.host}' 2>/dev/null || echo "<pending>")
GRPC_ROUTE=$(oc get route fleetshift-grpc -n "${NAMESPACE}" -o jsonpath='{.spec.host}' 2>/dev/null || echo "<pending>")
echo "  Frontend + API: https://${HTTP_ROUTE}"
echo "  gRPC:           ${GRPC_ROUTE}:443"
echo ""
echo "  Force image update:  oc import-image fleetshift-server:latest -n ${NAMESPACE} --confirm"
echo "  View logs:           oc logs -n ${NAMESPACE} deployment/fleetshift-server"
echo "  Tear down:           ${SCRIPT_DIR}/teardown.sh"
