#!/usr/bin/env bash
# Shared helpers for deploy scripts. Source this, don't execute it.

# Run a command with a deadline. Prefers GNU timeout, then gtimeout
# (Homebrew coreutils on macOS), then perl alarm. If none are available,
# runs the command without a deadline.
run_with_timeout() {
  local secs=$1; shift
  if command -v timeout >/dev/null 2>&1; then
    timeout "$secs" "$@"
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout "$secs" "$@"
  elif command -v perl >/dev/null 2>&1; then
    perl -e 'alarm shift; exec @ARGV' -- "$secs" "$@"
  else
    "$@"
  fi
}

# Fail fast unless oc can authenticate within a few seconds.
require_oc_login() {
  run_with_timeout 5 oc whoami &>/dev/null || {
    echo "ERROR: Not logged in to OpenShift. Run 'oc login' first." >&2
    exit 1
  }
}
