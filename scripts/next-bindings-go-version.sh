#!/usr/bin/env bash
# scripts/next-bindings-go-version.sh — compute the next bindings/go release tag.
#
# Two independent inputs decide the version, and exactly one applies per run:
#
#   (default / --core-version X.Y.Z)  Mirror the core version literally (D3):
#     a core release always produces a bindings release at the identical
#     version, even with zero Go source changes. With no --core-version given,
#     this shells out to next-core-version.sh itself so both halves of the
#     cascade agree without either side hardcoding the other's arithmetic.
#
#   --bump patch|minor   Standalone bindings release, independent of any core
#     release (e.g. a Go-only bug fix). Bumps the newest existing
#     bindings/go/v* stable tag, mirroring the arithmetic in
#     bindings/go/mise.toml's `release` task so the local dev helper and CI
#     never disagree. Never bumps major — a major bump changes the Go module
#     import path and is a deliberate, human-driven decision, not something
#     this script should do on its own.
#
# In every mode, before printing anything, this script hard-fails if the
# computed version's major component disagrees with the /vN suffix on the
# module path in bindings/go/go.mod. That check is Go's semantic import
# versioning rule made mechanical: an import-path/major mismatch produces a
# module nothing can `go get`, and this must never reach a tag push.
#
# Usage:
#   scripts/next-bindings-go-version.sh                      # mirror core (auto)
#   scripts/next-bindings-go-version.sh --core-version 8.0.0  # mirror core (explicit)
#   scripts/next-bindings-go-version.sh --bump patch|minor    # standalone bump
#   scripts/next-bindings-go-version.sh --current             # newest existing tag
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

GO_MOD="${REPO_ROOT}/bindings/go/go.mod"

if [ ! -f "${GO_MOD}" ]; then
  echo "next-bindings-go-version.sh: ${GO_MOD} not found" >&2
  exit 1
fi

# --- parse args ---------------------------------------------------------
MODE="mirror"
CORE_VERSION=""
BUMP=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --current)
      MODE="current"
      shift
      ;;
    --core-version)
      MODE="mirror"
      CORE_VERSION="${2:?--core-version requires a value}"
      shift 2
      ;;
    --bump)
      MODE="bump"
      BUMP="${2:?--bump requires patch or minor}"
      shift 2
      ;;
    *)
      echo "next-bindings-go-version.sh: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

CURRENT_TAG="$(git -C "${REPO_ROOT}" tag -l 'bindings/go/v*' --sort=-version:refname | { grep -v -- '-' || true; } | head -n1)"

if [ "${MODE}" = "current" ]; then
  echo "${CURRENT_TAG}"
  exit 0
fi

# --- compute the candidate version --------------------------------------
if [ "${MODE}" = "mirror" ]; then
  if [ -z "${CORE_VERSION}" ]; then
    CORE_TAG="$("${SCRIPT_DIR}/next-core-version.sh")"
    CORE_VERSION="${CORE_TAG#v}"
  fi
  VERSION="${CORE_VERSION}"
else
  if [ "${BUMP}" != "patch" ] && [ "${BUMP}" != "minor" ]; then
    echo "next-bindings-go-version.sh: --bump must be 'patch' or 'minor', got '${BUMP}'" >&2
    exit 2
  fi
  if [ -z "${CURRENT_TAG}" ]; then
    CURRENT_VERSION="0.0.0"
  else
    CURRENT_VERSION="${CURRENT_TAG#bindings/go/v}"
  fi
  MAJOR="${CURRENT_VERSION%%.*}"
  REST="${CURRENT_VERSION#*.}"
  MINOR="${REST%%.*}"
  PATCH="${REST#*.}"
  if [[ ! "${MAJOR}" =~ ^[0-9]+$ ]] || [[ ! "${MINOR}" =~ ^[0-9]+$ ]] || [[ ! "${PATCH}" =~ ^[0-9]+$ ]]; then
    echo "next-bindings-go-version.sh: ${CURRENT_TAG}: unexpected tag shape" >&2
    exit 1
  fi
  case "${BUMP}" in
    minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
    patch) PATCH=$((PATCH + 1)) ;;
  esac
  VERSION="${MAJOR}.${MINOR}.${PATCH}"
fi

# --- enforce Go semantic import versioning ------------------------------
MODULE_PATH="$(sed -n 's|^module[[:space:]]\+||p' "${GO_MOD}" | head -n1)"
if [ -z "${MODULE_PATH}" ]; then
  echo "next-bindings-go-version.sh: ${GO_MOD}: module path not found" >&2
  exit 1
fi

VERSION_MAJOR="${VERSION%%.*}"
if [[ ! "${VERSION_MAJOR}" =~ ^[0-9]+$ ]]; then
  echo "next-bindings-go-version.sh: ${VERSION}: invalid version" >&2
  exit 1
fi

if [[ "${MODULE_PATH}" =~ /v([0-9]+)$ ]]; then
  MODULE_MAJOR="${BASH_REMATCH[1]}"
else
  # Go omits the /vN suffix for major 0 and 1.
  MODULE_MAJOR="1"
fi

if [ "${VERSION_MAJOR}" -ge 2 ] && [ "${VERSION_MAJOR}" != "${MODULE_MAJOR}" ]; then
  echo "next-bindings-go-version.sh: computed version v${VERSION} (major ${VERSION_MAJOR}) disagrees with module path '${MODULE_PATH}' (major ${MODULE_MAJOR})." >&2
  echo "next-bindings-go-version.sh: Go semantic import versioning requires the module path to end in /v${VERSION_MAJOR}. Update bindings/go/go.mod (and all importers) before releasing." >&2
  exit 1
fi
if [ "${VERSION_MAJOR}" -le 1 ] && [[ "${MODULE_PATH}" =~ /v[0-9]+$ ]]; then
  echo "next-bindings-go-version.sh: computed version v${VERSION} (major ${VERSION_MAJOR}) should not have a /vN module path suffix, but module path is '${MODULE_PATH}'." >&2
  exit 1
fi

echo "bindings/go/v${VERSION}"
