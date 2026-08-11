#!/usr/bin/env bash
# scripts/core-artifact-changed.sh — did the native-core build artifact
# actually change between two refs?
#
# This is the release-gating question, and it is deliberately stricter than
# ci-core.yml's `paths:` trigger filter: ci-core.yml also reacts to
# `.gitmodules` (cheap defensive rebuild-and-test after any submodule bump),
# but this gate must not, because nothing in the build reads native/upstream
# — a gitlink-only change can never change libsmartmon.a or the C ABI
# wrapper. Including it here would let a no-op re-vendoring produce a
# meaningless new release tag. Do not "fix" this to match ci-core.yml.
#
# Usage:
#   scripts/core-artifact-changed.sh <base-ref> [head-ref]
#     Exit 0 if any artifact-relevant path changed between base-ref and
#     head-ref (default: HEAD). Exit 1 otherwise.
#   scripts/core-artifact-changed.sh --list <base-ref> [head-ref]
#     Print the changed artifact-relevant paths (possibly none) and exit 0
#     unconditionally — for building tracking-issue bodies.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Paths that participate in the native-core build artifact
# (libsmartmon.a + the C ABI wrapper). Kept close to, but stricter than,
# ci-core.yml's `paths:` filter — see header comment above for why
# `.gitmodules` / native/upstream are excluded.
ARTIFACT_PATHS=(
  'native/configure.ac'
  'native/Makefile.am'
  'native/lib/**'
  'native/include/**'
  'native/src/**'
  'native/capi/**'
  'native/drivedb/drivedb.h'
  'scripts/build-wrapper.sh'
)

LIST_ONLY=false
if [ "${1:-}" = "--list" ]; then
  LIST_ONLY=true
  shift
fi

if [ "$#" -lt 1 ]; then
  echo "usage: $(basename "$0") [--list] <base-ref> [head-ref]" >&2
  exit 2
fi

BASE_REF="$1"
HEAD_REF="${2:-HEAD}"

CHANGED="$(git -C "${REPO_ROOT}" diff --name-only "${BASE_REF}" "${HEAD_REF}" -- "${ARTIFACT_PATHS[@]}")"

if $LIST_ONLY; then
  echo "${CHANGED}"
  exit 0
fi

if [ -n "${CHANGED}" ]; then
  exit 0
fi
exit 1
