#!/usr/bin/env bash
set -euo pipefail

CHANGELOG_FILE="${CHANGELOG_FILE:-CHANGELOG.md}"

err() {
  printf "ERROR: %s\n" "$1" >&2
}

if [[ ! -f "${CHANGELOG_FILE}" ]]; then
  err "${CHANGELOG_FILE} is missing. Create it at the repo root."
  exit 1
fi

unreleased_heading="## [Unreleased]"

if ! grep -Fxq "${unreleased_heading}" "${CHANGELOG_FILE}"; then
  err "${CHANGELOG_FILE} must contain the exact heading: ${unreleased_heading}"
  exit 1
fi

required_subheadings=(
  "### Added"
  "### Changed"
  "### Deprecated"
  "### Removed"
  "### Fixed"
  "### Security"
)

missing=()
for h in "${required_subheadings[@]}"; do
  if ! awk -v unreleased="${unreleased_heading}" -v wanted="${h}" '
    $0 == unreleased { in_unreleased=1; next }
    in_unreleased && $0 ~ /^## \[/ { in_unreleased=0 }
    in_unreleased && $0 == wanted { found=1 }
    END { exit(found ? 0 : 1) }
  ' "${CHANGELOG_FILE}"; then
    missing+=("${h}")
  fi
done

if (( ${#missing[@]} > 0 )); then
  err "${CHANGELOG_FILE} is missing required subheadings under ${unreleased_heading}:"
  for h in "${missing[@]}"; do
    printf "  - %s\n" "${h}" >&2
  done
  err "Add the headings (they can be empty) so releases can be cut consistently."
  exit 1
fi

printf "OK: %s looks valid.\n" "${CHANGELOG_FILE}"


