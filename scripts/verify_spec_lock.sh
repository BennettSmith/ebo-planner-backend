#!/usr/bin/env bash
set -euo pipefail

SPEC_LOCK_FILE="${SPEC_LOCK_FILE:-spec.lock}"

err() {
  printf "ERROR: %s\n" "$1" >&2
}

if [[ ! -f "${SPEC_LOCK_FILE}" ]]; then
  err "${SPEC_LOCK_FILE} is missing. Create it at the repo root with the spec tag this service implements (e.g. v1.2.3)."
  exit 1
fi

spec_version="$(tr -d '[:space:]' < "${SPEC_LOCK_FILE}" | head -n 1 || true)"
if [[ -z "${spec_version}" ]]; then
  err "${SPEC_LOCK_FILE} is empty. Set it to the spec tag this service implements (e.g. v1.2.3)."
  exit 1
fi

if ! printf "%s" "${spec_version}" | grep -Eq '^v?[0-9]+\.[0-9]+\.[0-9]+$'; then
  err "${SPEC_LOCK_FILE} must contain a SemVer tag like v1.2.3 (or 1.2.3). Found: '${spec_version}'"
  exit 1
fi

printf "OK: %s pins spec version %s\n" "${SPEC_LOCK_FILE}" "${spec_version}"


