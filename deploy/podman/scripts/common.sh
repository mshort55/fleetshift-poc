#!/usr/bin/env bash
# Shared helpers for deploy scripts. Source this, don't execute it.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DEPLOY_DIR="$(cd "$COMPOSE_DIR/.." && pwd)"
ROOT_DIR="$(cd "$DEPLOY_DIR/.." && pwd)"

compose() {
  if ! command -v docker-compose &>/dev/null; then
    echo "ERROR: docker-compose is not installed." >&2
    echo "  Install: brew install docker-compose" >&2
    echo "  (podman-compose is not compatible — this stack requires depends_on health checks)" >&2
    exit 1
  fi
  # COMPOSE_FILES is set by Taskfile as a space-separated string of -f flags.
  # shellcheck disable=SC2086
  podman compose ${COMPOSE_FILES} --env-file "$ROOT_DIR/.env" "$@"
}

detect_podman_socket() {
  if [ -z "${PODMAN_SOCKET:-}" ]; then
    PODMAN_SOCKET=$(podman info --format '{{.Host.RemoteSocket.Path}}' 2>/dev/null | sed 's|^unix://||' || echo "/run/user/$(id -u)/podman/podman.sock")
  fi
}

generate_password() {
  openssl rand -base64 32 | tr -dc 'a-zA-Z0-9' | head -c 16
}
