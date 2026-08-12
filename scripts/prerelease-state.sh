#!/usr/bin/env bash
# scripts/prerelease-state.sh — is the vendored native core a pre-release,
# and if so, how many commits into it are we?
#
# native/src/getversion.sh already computes both facts, but only as internal
# shell locals in a script whose CLI (-n/-g/-s/-i) emits C headers and
# filename strings, not something the release-version scripts can query
# directly. This helper re-derives the same two facts with the *exact* same
# logic, so this script and getversion.sh can never disagree about the
# pre-release state or the revision count:
#
#   pre-release detection — the sed + case from getversion.sh:74-79
#   revision count        — the rev-list from getversion.sh:151,
#                            against the same base_git_rev as getversion.sh:116
#
# Usage:
#   scripts/prerelease-state.sh --is-prerelease
#     Exit 0 if the core is a pre-release (no output), exit 1 if it is a
#     final release.
#   scripts/prerelease-state.sh --revs
#     Print the revision count since the previous release, e.g. 504.
#     Only valid while --is-prerelease; hard-errors otherwise.
#   scripts/prerelease-state.sh --suffix
#     Print the SemVer prerelease suffix, e.g. -pre.504, or an empty line
#     when the core is a final release.
#
# Note on padding: getversion.sh:181 also computes a zero-padded pre_revs3
# (e.g. "504" -> "504", but "99" -> "099") for its own filename/description
# strings. That padding must NEVER be used here: SemVer §9 forbids leading
# zeros in numeric prerelease identifiers ("pre.099" is invalid). This
# script only ever emits the unpadded count.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

CONFIGURE_AC="${REPO_ROOT}/native/configure.ac"

if [ ! -f "${CONFIGURE_AC}" ]; then
  echo "prerelease-state.sh: ${CONFIGURE_AC} not found" >&2
  exit 1
fi

# Same expression as native/src/getversion.sh:75-79.
x="$(sed -n 's|^smartmontools_release_date=\(.*\)$|\1|p' "${CONFIGURE_AC}")"
case "${x}" in
  20*) IS_PRERELEASE=false ;;
  \ \#*) IS_PRERELEASE=true ;;
  *)
    echo "prerelease-state.sh: ${CONFIGURE_AC}: unable to detect pre-release state" >&2
    exit 1
    ;;
esac

# Same base revision as native/src/getversion.sh:116 (the RELEASE_7_5 commit
# in this repo's ancestry).
BASE_GIT_REV="943adaeda55c2d534c722fe66c6b4613a782caa1"

revs() {
  if ! ${IS_PRERELEASE}; then
    echo "prerelease-state.sh: --revs requested but the core is a final release" >&2
    exit 1
  fi
  local x
  if ! x="$(git -C "${REPO_ROOT}" rev-list --count "${BASE_GIT_REV}..HEAD" 2>/dev/null)"; then
    echo "prerelease-state.sh: ${BASE_GIT_REV}: revision not found (shallow clone? fetch-depth: 0 is required)" >&2
    exit 1
  fi
  # Same valid band as native/src/getversion.sh:179.
  if [ "${x}" -le 0 ] || [ "${x}" -ge 5600 ]; then
    echo "prerelease-state.sh: ${x}: revision count out of the expected 0 < x < 5600 band" >&2
    exit 1
  fi
  echo "${x}"
}

case "${1:-}" in
  --is-prerelease)
    ${IS_PRERELEASE}
    ;;
  --revs)
    revs
    ;;
  --suffix)
    if ${IS_PRERELEASE}; then
      echo "-pre.$(revs)"
    else
      echo ""
    fi
    ;;
  *)
    echo "usage: $(basename "$0") --is-prerelease|--revs|--suffix" >&2
    exit 2
    ;;
esac
