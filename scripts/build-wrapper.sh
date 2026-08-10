#!/usr/bin/env bash
# scripts/build-wrapper.sh — build the native/capi/ C ABI shared library.
#
# Compiles native/capi/smartmon_c_api.cpp against a prebuilt libsmartmon.a
# and links a shared library that Go's purego-based lib backend can dlopen.
# Deliberately does not use libtool: the release matrix already hand-drives
# CXX/CXXFLAGS per target (zig for musl, osxcross for darwin), and asking
# libtool to emit a non-PIC static archive *and* a PIC shared object across
# 7 cross targets from the same sources is disproportionate to what's needed
# here. The archive is linked by explicit path, not -L/-lsmartmon, so the
# result is self-contained for dlopen and records no DT_NEEDED on a library
# that is never shipped.
#
# Usage:
#   scripts/build-wrapper.sh [--target NAME] [--archive PATH] [--out PATH]
#
# With no --target, auto-detects the host (uname -s/-m) and builds natively.
# --target selects one of the release-matrix cross configurations below;
# pass the same names used in .github/workflows/ci-core.yml.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

CAPI_DIR="${REPO_ROOT}/native/capi"
INCLUDE_DIR="${REPO_ROOT}/native/include"
BUILD_INCLUDE_DIR="${REPO_ROOT}/native/build/include"
ARCHIVE="${REPO_ROOT}/native/build/lib/.libs/libsmartmon.a"
OUT=""
TARGET=""

while [ $# -gt 0 ]; do
  case "$1" in
    --target) TARGET="$2"; shift 2 ;;
    --archive) ARCHIVE="$2"; shift 2 ;;
    --out) OUT="$2"; shift 2 ;;
    --capi-dir) CAPI_DIR="$2"; shift 2 ;;
    --include-dir) INCLUDE_DIR="$2"; shift 2 ;;
    --build-include-dir) BUILD_INCLUDE_DIR="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,20p' "${BASH_SOURCE[0]}"
      exit 0
      ;;
    *)
      echo "build-wrapper.sh: unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

if [ ! -f "${CAPI_DIR}/smartmon_c_api.cpp" ]; then
  echo "build-wrapper.sh: ${CAPI_DIR}/smartmon_c_api.cpp not found" >&2
  exit 1
fi
if [ ! -f "${ARCHIVE}" ]; then
  echo "build-wrapper.sh: ${ARCHIVE} not found — build native/ first (see native/README.md)" >&2
  exit 1
fi

# uname-based host auto-detection, ported from the Go repo's deleted
# setup-lib-backend.sh (recovered from git history commit f8a7f9a8).
detect_host_target() {
  local os_raw arch_raw os_tag arch_tag
  os_raw="$(uname -s)"
  arch_raw="$(uname -m)"
  case "${os_raw}" in
    Linux) os_tag="linux" ;;
    Darwin) os_tag="darwin" ;;
    *)
      echo "build-wrapper.sh: unsupported host OS: ${os_raw}" >&2
      exit 1
      ;;
  esac
  case "${arch_raw}" in
    x86_64|amd64) arch_tag="amd64" ;;
    aarch64|arm64) arch_tag="aarch64" ;;
    *)
      echo "build-wrapper.sh: unsupported host arch: ${arch_raw}" >&2
      exit 1
      ;;
  esac
  echo "${os_tag}-${arch_tag}"
}

if [ -z "${TARGET}" ]; then
  TARGET="$(detect_host_target)"
fi

# CXX_ARR is the compiler invocation as an argv prefix (may be multiple
# words, e.g. "zig c++ -target ..."), never a string re-parsed by the shell.
CXX_ARR=()
LINK_FLAGS=()
EXTRA_CXXFLAGS=()
LIBEXT="so"
SHARED_FLAG="-shared"

case "${TARGET}" in
  linux-amd64)
    CXX_ARR=(g++)
    LINK_FLAGS=(-lstdc++ -lm)
    ;;
  linux-aarch64)
    CXX_ARR=(aarch64-linux-gnu-g++)
    LINK_FLAGS=(-lstdc++ -lm)
    ;;
  linux-amd64-musl)
    CXX_ARR=(zig c++ -target x86_64-linux-musl)
    LINK_FLAGS=()  # zig bundles libc++; do not add -lstdc++
    ;;
  linux-aarch64-musl)
    CXX_ARR=(zig c++ -target aarch64-linux-musl)
    LINK_FLAGS=()
    ;;
  darwin-amd64)
    # osxcross cross-compile, as run in the CI container.
    CXX_ARR=(o64-clang++)
    EXTRA_CXXFLAGS=(-arch x86_64 -stdlib=libc++)
    LINK_FLAGS=(-framework CoreFoundation -framework IOKit)
    SHARED_FLAG="-dynamiclib"
    LIBEXT="dylib"
    ;;
  darwin-aarch64)
    CXX_ARR=(o64-clang++)
    EXTRA_CXXFLAGS=(-arch arm64 -stdlib=libc++)
    LINK_FLAGS=(-framework CoreFoundation -framework IOKit)
    SHARED_FLAG="-dynamiclib"
    LIBEXT="dylib"
    ;;
  darwin-native)
    # Native build on an actual Mac (not osxcross). The Go side only ever
    # needs this for local development; release artifacts come from the
    # darwin-amd64/darwin-aarch64 osxcross targets above.
    CXX_ARR=(c++)
    EXTRA_CXXFLAGS=(-stdlib=libc++)
    LINK_FLAGS=(-framework CoreFoundation -framework IOKit)
    SHARED_FLAG="-dynamiclib"
    LIBEXT="dylib"
    # SDK/libc++ header detection, ported from the fix in commit fc6eeccc.
    MACOS_SDK="$(xcrun --show-sdk-path 2>/dev/null || true)"
    if [ -n "${MACOS_SDK}" ] && [ -d "${MACOS_SDK}/usr/include/c++/v1" ]; then
      EXTRA_CXXFLAGS+=(-I"${MACOS_SDK}/usr/include/c++/v1" -isysroot "${MACOS_SDK}")
    fi
    ;;
  windows-amd64)
    echo "build-wrapper.sh: windows-amd64 skipped — Go lib backend is linux/darwin only" >&2
    exit 0
    ;;
  *)
    echo "build-wrapper.sh: unknown target: ${TARGET}" >&2
    echo "known targets: linux-amd64 linux-aarch64 linux-amd64-musl linux-aarch64-musl darwin-amd64 darwin-aarch64 darwin-native windows-amd64" >&2
    exit 2
    ;;
esac

# host darwin auto-detection maps to darwin-native, not the osxcross targets.
if [ "${TARGET}" = "darwin-amd64" ] || [ "${TARGET}" = "darwin-aarch64" ]; then
  if [ "$(uname -s)" = "Darwin" ] && [ -z "${SMARTMON_FORCE_OSXCROSS:-}" ]; then
    exec "${BASH_SOURCE[0]}" --target darwin-native --archive "${ARCHIVE}" --out "${OUT}" \
      --capi-dir "${CAPI_DIR}" --include-dir "${INCLUDE_DIR}" --build-include-dir "${BUILD_INCLUDE_DIR}"
  fi
fi

if [ -z "${OUT}" ]; then
  OUT="${REPO_ROOT}/packaging/artifacts/lib/libsmartmon_go.${LIBEXT}"
fi
mkdir -p "$(dirname "${OUT}")"

echo "build-wrapper.sh: target=${TARGET} out=${OUT}" >&2

"${CXX_ARR[@]}" \
  -std=c++17 -fPIC -O2 \
  -I"${INCLUDE_DIR}" -I"${BUILD_INCLUDE_DIR}" \
  "${EXTRA_CXXFLAGS[@]}" \
  "${SHARED_FLAG}" \
  -o "${OUT}" \
  "${CAPI_DIR}/smartmon_c_api.cpp" \
  "${ARCHIVE}" \
  "${LINK_FLAGS[@]}"

echo "build-wrapper.sh: wrote ${OUT}" >&2
