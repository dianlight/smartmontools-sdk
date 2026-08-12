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
#     bindings/go/v* STABLE tag, mirroring the arithmetic in
#     bindings/go/mise.toml's `release` task so the local dev helper and CI
#     never disagree. Never bumps major — a major bump changes the Go module
#     import path and is a deliberate, human-driven decision, not something
#     this script should do on its own. Hard-errors while the core is a
#     pre-release (scripts/prerelease-state.sh --is-prerelease): a standalone
#     Go-only release has no correct version to bump to while the mainline
#     series itself is still a -pre.N moving target — see
#     docs/development/release-process.md.
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
#   scripts/next-bindings-go-version.sh --current              # newest existing
#     tag of ANY kind (prereleases included) — used by callers that need "is
#     there already something released", e.g. release-bindings-go.yml's
#     already-released fallback, which must not skip past an existing
#     prerelease just because it isn't stable.
#   scripts/next-bindings-go-version.sh --current-stable       # newest existing
#     STABLE tag only — used internally by --bump, which must never bump
#     from a prerelease.
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
    --current-stable)
      MODE="current-stable"
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

# All existing bindings/go tags, of any kind (prereleases included).
ALL_TAGS="$(git -C "${REPO_ROOT}" tag -l 'bindings/go/v*')"

# Newest tag of any kind, by true SemVer precedence (NOT `--sort=-version:refname`,
# which ranks a prerelease above its own release — see scripts/semver-sort.sh
# header). This is what "already released" callers (e.g.
# release-bindings-go.yml's fallback) must see: an existing prerelease still
# counts as "already released", so it must not be filtered out here.
CURRENT_TAG="$(printf '%s\n' "${ALL_TAGS}" | "${SCRIPT_DIR}/semver-sort.sh" --prefix 'bindings/go/' --newest || true)"

if [ "${MODE}" = "current" ]; then
  echo "${CURRENT_TAG}"
  exit 0
fi

# Newest STABLE (non-prerelease) tag only — the base --bump must bump from.
CURRENT_STABLE_TAG="$(printf '%s\n' "${ALL_TAGS}" | grep -v -- '-' | "${SCRIPT_DIR}/semver-sort.sh" --prefix 'bindings/go/' --newest || true)"

if [ "${MODE}" = "current-stable" ]; then
  echo "${CURRENT_STABLE_TAG}"
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
  # A standalone Go-only bump has no correct target while the mainline core
  # series is itself a moving -pre.N target — refuse rather than guess.
  if "${SCRIPT_DIR}/prerelease-state.sh" --is-prerelease; then
    echo "next-bindings-go-version.sh: --bump is not allowed while the core is a pre-release (see docs/development/release-process.md)." >&2
    echo "next-bindings-go-version.sh: a standalone bindings-only release has no correct version to target until the core graduates to a final release." >&2
    exit 1
  fi
  if [ -z "${CURRENT_STABLE_TAG}" ]; then
    CURRENT_VERSION="0.0.0"
  else
    CURRENT_VERSION="${CURRENT_STABLE_TAG#bindings/go/v}"
  fi
  # Strip any -pre.<revs> suffix BEFORE extracting MAJOR/MINOR/PATCH. Can
  # only matter if CURRENT_STABLE_TAG somehow still carried one (it can't,
  # by construction of the filter above), but keep the guard cheap and
  # explicit rather than relying on that invariant silently.
  CURRENT_VERSION="${CURRENT_VERSION%%-*}"
  MAJOR="${CURRENT_VERSION%%.*}"
  REST="${CURRENT_VERSION#*.}"
  MINOR="${REST%%.*}"
  PATCH="${REST#*.}"
  if [[ ! "${MAJOR}" =~ ^[0-9]+$ ]] || [[ ! "${MINOR}" =~ ^[0-9]+$ ]] || [[ ! "${PATCH}" =~ ^[0-9]+$ ]]; then
    echo "next-bindings-go-version.sh: ${CURRENT_STABLE_TAG}: unexpected tag shape" >&2
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
