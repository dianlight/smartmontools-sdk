#!/usr/bin/env bash
# integration/core-go/run.sh — end-to-end contract test between native/ and
# bindings/go/, the thing neither smartmontools-go nor smartmontools-sdk
# could test on their own before the monorepo merge.
#
# Verifies, in order:
#   1. native/ builds libsmartmon.a with --with-pic.
#   2. scripts/build-wrapper.sh links a shared library against it.
#   3. The wrapper exports exactly the 12 smartmon_* symbols the Go binding
#      expects, and smartmon_abi_version() satisfies the major/minor contract
#      bindings/go/backends/lib/lib.go enforces at runtime.
#   4. The Go lib backend can dlopen the wrapper, call smartmon_init() and
#      smartmon_scan_devices(), and get back well-formed JSON.
#   5. bindings/go/backends/exec/drivedb.h has no drift from
#      native/drivedb/drivedb.h (delegates to scripts/sync-drivedb-go.sh).
#
# Usage: integration/core-go/run.sh
# Optional: SMARTMON_SDK_DEVICE=/dev/sdX to also exercise device-dependent
# assertions (GetSMARTInfo/CheckHealth) against a real device.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

log() { echo "[core-go] $*" >&2; }

# --- 1. Build the native core -----------------------------------------------
log "building native/ (--with-pic)"
# Matches ci-core.yml/ci-bindings-go.yml/release-core.yml: version detection
# in native/src/getversion.sh errors out without a full git history (e.g. a
# shallow clone or the native/upstream submodule not fully checked out).
export SMARTMONTOOLS_TEST_BUILD=1
cd "${REPO_ROOT}/native"
./autogen.sh --force
mkdir -p build
cd build
../configure --with-devel=yes --with-pic
make -C include
make -j"$(nproc)" -C lib libsmartmon.la

ARCHIVE="${REPO_ROOT}/native/build/lib/.libs/libsmartmon.a"
test -f "${ARCHIVE}"

# --- 2. Build the C ABI wrapper ---------------------------------------------
log "building the C ABI wrapper"
"${REPO_ROOT}/scripts/build-wrapper.sh"

case "$(uname -s)" in
  Darwin) WRAPPER="${REPO_ROOT}/packaging/artifacts/lib/libsmartmon_go.dylib" ;;
  *)      WRAPPER="${REPO_ROOT}/packaging/artifacts/lib/libsmartmon_go.so" ;;
esac
test -f "${WRAPPER}"

# --- 3. Symbol and ABI contract ---------------------------------------------
log "checking exported symbol contract"
EXPECTED_SYMBOLS=(
  smartmon_abi_version
  smartmon_abort_selftest
  smartmon_check_health
  smartmon_cleanup
  smartmon_disable_smart
  smartmon_enable_smart
  smartmon_free_string
  smartmon_get_smart_data
  smartmon_init
  smartmon_last_error
  smartmon_run_selftest
  smartmon_scan_devices
)

case "$(uname -s)" in
  Darwin) EXPORTED="$(nm -gU "${WRAPPER}" | awk '{print $NF}' | sed 's/^_//')" ;;
  *)      EXPORTED="$(nm -D --defined-only "${WRAPPER}" | awk '{print $NF}')" ;;
esac

FOUND_COUNT=0
for sym in "${EXPECTED_SYMBOLS[@]}"; do
  if ! grep -qx "${sym}" <<<"${EXPORTED}"; then
    echo "[core-go] FAIL: missing expected symbol: ${sym}" >&2
    exit 1
  fi
  FOUND_COUNT=$((FOUND_COUNT + 1))
done
log "found all ${FOUND_COUNT} expected symbols"

log "checking smartmon_abi_version() against the Go binding's requirement"
ABI_CHECK_SRC="${SCRIPT_DIR}/abi_check.c"
ABI_CHECK_BIN="${REPO_ROOT}/native/build/abi_check"
# dlopen()/dlsym() live in libSystem on macOS, so there is no separate libdl
# to link there; -ldl only applies to glibc/musl targets.
DL_LIBS=()
[ "$(uname -s)" = "Darwin" ] || DL_LIBS=(-ldl)
"${CXX:-cc}" -o "${ABI_CHECK_BIN}" "${ABI_CHECK_SRC}" -I"${REPO_ROOT}/native/capi" "${DL_LIBS[@]}"
# sed -nE instead of grep -oP: -P (PCRE) is a GNU grep extension, absent on
# BSD/macOS grep.
ABI_MAJOR_REQUIRED="$(sed -nE 's/.*abiMajorRequired = ([0-9]+).*/\1/p' "${REPO_ROOT}/bindings/go/backends/lib/lib.go")"
ABI_MINOR_REQUIRED="$(sed -nE 's/.*abiMinorRequired = ([0-9]+).*/\1/p' "${REPO_ROOT}/bindings/go/backends/lib/lib.go")"
"${ABI_CHECK_BIN}" "${WRAPPER}" "${ABI_MAJOR_REQUIRED}" "${ABI_MINOR_REQUIRED}"

# --- 4. Go lib backend: dlopen + scan + JSON shape --------------------------
log "running the Go lib-backend integration tests"
cd "${REPO_ROOT}/bindings/go"
export SMARTMON_INTEGRATION=1
export SMARTMON_LIB_PATH="${WRAPPER}"
go test -failfast ./backends/lib/...

if [ -n "${SMARTMON_SDK_DEVICE:-}" ]; then
  log "device-dependent assertions enabled for ${SMARTMON_SDK_DEVICE}"
else
  log "SMARTMON_SDK_DEVICE not set; device-dependent assertions skipped"
fi

# --- 5. drivedb single source of truth --------------------------------------
log "checking drivedb.h has no drift"
"${REPO_ROOT}/scripts/sync-drivedb-go.sh"
git -C "${REPO_ROOT}" diff --exit-code -- \
  bindings/go/backends/exec/drivedb.h \
  bindings/go/backends/exec/drivedb_version.go

log "all core-go integration checks passed"
