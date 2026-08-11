#!/usr/bin/env bash
# scripts/next-core-version.sh — compute the next native-core release tag.
#
# The core version scheme is v<AC_INIT>.<N>, where <AC_INIT> is the
# smartmontools package version from native/configure.ac (the same value
# native/src/getversion.sh embeds into every build) and <N> is a per-generation
# counter that resets whenever AC_INIT changes. This makes the tag fully
# computable from the tree and comparable with `sort -V`.
#
# AC_INIT is scraped with the exact same sed expression as
# native/src/getversion.sh:70, so the two can never disagree about what the
# current package version is.
#
# Usage:
#   scripts/next-core-version.sh            # print the next tag, e.g. v8.0.0
#   scripts/next-core-version.sh --current  # print the newest existing tag
#                                            # for the current AC_INIT generation,
#                                            # or nothing if none exists
#   scripts/next-core-version.sh --acinit   # print the bare AC_INIT value, e.g. 8.0
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

# Newest existing tag for this AC_INIT generation, e.g. v8.0.3 for ACINIT=8.0.
CURRENT="$(git -C "${REPO_ROOT}" tag -l "v${ACINIT}.*" --sort=-v:refname | head -n1)"

if [ "${1:-}" = "--current" ]; then
  echo "${CURRENT}"
  exit 0
fi

if [ -z "${CURRENT}" ]; then
  echo "v${ACINIT}.0"
  exit 0
fi

N="${CURRENT##*.}"
if [[ ! "${N}" =~ ^[0-9]+$ ]]; then
  echo "next-core-version.sh: ${CURRENT}: unexpected tag shape" >&2
  exit 1
fi
echo "v${ACINIT}.$((N + 1))"
