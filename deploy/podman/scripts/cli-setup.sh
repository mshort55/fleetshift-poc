#!/usr/bin/env bash
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/common.sh"

# Configure fleetctl CLI for the current deployment. Called by 'task deploy:cli-setup'.
#
# Fetches OIDC discovery and writes auth config to ~/.config/fleetshift/auth.json.
# Uses OIDC_ISSUER_URL from .env if set (external mode), otherwise falls back
# to the local Keycloak instance.

# Env vars are set by Taskfile. For standalone usage, source .env manually.
if [ -z "${DEPLOY_MODE:-}" ]; then
  set -a; source "$ROOT_DIR/.env"; set +a
fi

if [ -n "${OIDC_ISSUER_URL:-}" ]; then
  ISSUER_URL="${OIDC_ISSUER_URL}"
else
  ISSUER_URL="http://${KC_HOSTNAME:-localhost}:${KC_HTTP_PORT:-8180}/auth/realms/fleetshift"
fi
DISCOVERY_URL="${ISSUER_URL}/.well-known/openid-configuration"

echo "==> Fetching OIDC discovery from ${DISCOVERY_URL}"
DISCOVERY=$(curl -sf "$DISCOVERY_URL") || {
  echo "ERROR: Could not reach OIDC provider at ${DISCOVERY_URL}" >&2
  echo "Is the stack running? Try 'task deploy:up' first." >&2
  exit 1
}

AUTH_ENDPOINT=$(echo "$DISCOVERY" | jq -r '.authorization_endpoint')
TOKEN_ENDPOINT=$(echo "$DISCOVERY" | jq -r '.token_endpoint')

CONFIG_DIR="${HOME}/.config/fleetshift"
CONFIG_FILE="${CONFIG_DIR}/auth.json"

mkdir -p "$CONFIG_DIR"

jq -n \
  --arg issuer "$ISSUER_URL" \
  --arg client "fleetshift-cli" \
  --arg auth "$AUTH_ENDPOINT" \
  --arg token "$TOKEN_ENDPOINT" \
  '{
    issuer_url: $issuer,
    client_id: $client,
    scopes: ["openid", "profile", "email"],
    authorization_endpoint: $auth,
    token_endpoint: $token,
    key_enrollment_client_id: "fleetshift-signing"
  }' > "$CONFIG_FILE"

echo "CLI config written to ${CONFIG_FILE}"
echo "Run 'fleetctl auth login' to authenticate."
