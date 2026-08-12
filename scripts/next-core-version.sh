#!/usr/bin/env bash
# scripts/next-core-version.sh — compute the next native-core release tag.
#
# The core version scheme is v<AC_INIT>.<N>[-pre.<revs>], where <AC_INIT> is
# the smartmontools package version from native/configure.ac (the same
# value native/src/getversion.sh embeds into every build), <N> is a
# per-generation counter that resets whenever AC_INIT changes, and the
# -pre.<revs> suffix is present exactly while native/configure.ac's
# smartmontools_release_date is commented out (see scripts/prerelease-state.sh)
# — i.e. while upstream has not yet shipped a final AC_INIT release. <revs>
# is the commit count since the previous release, the same number
# getversion.sh embeds into its own "pre-8.0-504" build strings, so a tag's
# -pre.504 and the shipped binary's pre-8.0-504 always agree.
#
# <N> only ever advances when a FINAL tag (no -pre suffix) is cut for this
# generation — every prerelease build along the way targets the same <N>,
# differing only by <revs>, and graduates to the plain v<AC_INIT>.<N> once
# upstream sets a release date. This is why the "current" lookup below is
# split into a stable-only view (drives the counter) and an any-tag view
# (needed by callers, e.g. the artifact-diff gate, that must see the truth
# regardless of prerelease status).
#
# v8.0.0 and v8.0.1 are permanently burned in the Go module checksum
# database (see docs/development/release-process.md): v8.0.0 was published
# prematurely, and v8.0.1 exists only as a retraction tombstone with no code
# changes. Neither must ever be re-minted, even if their git tags are later
# deleted — hence the explicit floor below rather than relying solely on tag
# presence.
#
# AC_INIT is scraped with the exact same sed expression as
# native/src/getversion.sh:70, so the two can never disagree about what the
# current package version is.
#
# Usage:
#   scripts/next-core-version.sh
#     Print the next tag, e.g. v8.0.2 or v8.0.2-pre.504.
#   scripts/next-core-version.sh --current
#     Print the newest existing STABLE (non-prerelease) tag for the current
#     AC_INIT generation, or nothing if none exists. This is what feeds the
#     artifact-diff gate (scripts/core-artifact-changed.sh) — it must diff
#     against a real released tag, never a prerelease.
#   scripts/next-core-version.sh --current-any
#     Print the newest existing tag of ANY kind (prereleases included) for
#     the current AC_INIT generation, or nothing if none exists.
#   scripts/next-core-version.sh --acinit
#     Print the bare AC_INIT value, e.g. 8.0.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

CONFIGURE_AC="${REPO_ROOT}/native/configure.ac"

if [ ! -f "${CONFIGURE_AC}" ]; then
  echo "next-core-version.sh: ${CONFIGURE_AC} not found" >&2
  exit 1
fi

# Same expression as native/src/getversion.sh:70.
ACINIT="$(sed -n 's|^AC_INIT[^,]*, *\[\([0-9.]*\)\] *,.*$|\1|p' "${CONFIGURE_AC}")"
if [ -z "${ACINIT}" ]; then
  echo "next-core-version.sh: ${CONFIGURE_AC}: package version not found" >&2
  exit 1
fi

MAJOR="${ACINIT%%.*}"
MINOR="${ACINIT#*.}"
if [ "${MAJOR}.${MINOR}" != "${ACINIT}" ] || [[ ! "${MAJOR}" =~ ^[0-9]+$ ]] || [[ ! "${MINOR}" =~ ^[0-9]+$ ]]; then
  echo "next-core-version.sh: ${ACINIT}: expected a two-component X.Y version" >&2
  exit 1
fi
if [ "${MAJOR}" -lt 8 ]; then
  echo "next-core-version.sh: ${ACINIT}: package versions below 8.0 are not supported (see native/src/getversion.sh)" >&2
  exit 1
fi

if [ "${1:-}" = "--acinit" ]; then
  echo "${ACINIT}"
  exit 0
fi

# All existing tags for this AC_INIT generation, e.g. v8.0.0, v8.0.1,
# v8.0.2-pre.504, v8.0.2 for ACINIT=8.0. `tag -l` here is a glob, not a
# regex, so "v${ACINIT}.*" matches any suffix including a -pre.N one.
ALL_TAGS="$(git -C "${REPO_ROOT}" tag -l "v${ACINIT}.*")"

# Stable-only newest, by true SemVer precedence (NOT `sort -V`, which ranks
# a prerelease above its own release — see scripts/semver-sort.sh header).
# ACINIT is guaranteed dash-free by the two-component check above, so
# filtering on '-' cannot false-positive on the ACINIT portion of the tag.
CURRENT="$(printf '%s\n' "${ALL_TAGS}" | grep -v -- '-' | "${SCRIPT_DIR}/semver-sort.sh" --newest || true)"

if [ "${1:-}" = "--current" ]; then
  echo "${CURRENT}"
  exit 0
fi

if [ "${1:-}" = "--current-any" ]; then
  printf '%s\n' "${ALL_TAGS}" | "${SCRIPT_DIR}/semver-sort.sh" --newest || true
  exit 0
fi

# v8.0.0 (premature) and v8.0.1 (retraction tombstone) are permanently
# burned in the Go checksum database — never re-mint them, even though
# their tags could in principle be deleted later.
FLOOR=0
if [ "${ACINIT}" = "8.0" ]; then
  FLOOR=2
fi

if [ -z "${CURRENT}" ]; then
  N="${FLOOR}"
else
  # Strip any -pre.<revs> suffix BEFORE extracting the counter. A
  # suffix-naive parser (the pre-existing `N="${CURRENT##*.}"`) would read
  # "504" out of "v8.0.2-pre.504" and pass the `^[0-9]+$` guard, silently
  # minting "v8.0.505" — a real, previously-unguarded bug.
  CORE="${CURRENT%%-*}"
  N="${CORE##*.}"
  if [[ ! "${N}" =~ ^[0-9]+$ ]]; then
    echo "next-core-version.sh: ${CURRENT}: unexpected tag shape" >&2
    exit 1
  fi
  N=$((N + 1))
  if [ "${N}" -lt "${FLOOR}" ]; then
    N="${FLOOR}"
  fi
fi

echo "v${ACINIT}.${N}$("${SCRIPT_DIR}/prerelease-state.sh" --suffix)"
