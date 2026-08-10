# ADR-002: Multi-Backend Architecture

## Status

Accepted

## Context

The current library executes an external `smartctl` binary via `os/exec` and parses
its JSON output. While reliable and well-tested, this approach has a hard runtime
dependency on an installed smartmontools package, which:

- Prevents use in minimal containers, embedded systems, or environments without
  package managers.
- Adds process-spawn overhead to every SMART operation.
- Ties every release to a specific minimum smartctl version.

The `Commander` interface already abstracts command construction, but it operates at
the "shell command" level rather than at the "SMART operation" level. A higher-level
**Backend** abstraction is needed to support alternative implementations without
breaking the public API (`SmartClient`).

## Decision

Introduce a `Backend` interface that defines SMART operations at the semantic level
(not at the CLI argument level). The existing `Client` becomes a thin orchestrator
that delegates to whichever backend is configured. The public `SmartClient` interface
is unchanged.

### Backend Interface

```go
// Backend is the low-level interface for accessing SMART data.
// Implementations include ExecBackend, IoctlBackend, LibBackend, and ShadowBackend.
type Backend interface {
    // Name returns a human-readable identifier for the backend.
    Name() string

    // Version returns the underlying tool/library version string, if available.
    Version(ctx context.Context) (string, error)

    // ScanDevices discovers available storage devices.
    ScanDevices(ctx context.Context) ([]Device, error)

    // GetSMARTInfo returns full SMART information for a device.
    GetSMARTInfo(ctx context.Context, devicePath string) (*SMARTInfo, error)

    // CheckHealth returns true when the SMART overall-health assessment passes.
    CheckHealth(ctx context.Context, devicePath string) (bool, error)

    // GetDeviceInfo returns raw key/value device information.
    GetDeviceInfo(ctx context.Context, devicePath string) (map[string]any, error)

    // RunSelfTest starts a SMART self-test of the given type.
    RunSelfTest(ctx context.Context, devicePath string, testType string) error

    // IsSMARTSupported returns the SMART support status for a device.
    IsSMARTSupported(ctx context.Context, devicePath string) (*SmartSupport, error)

    // EnableSMART enables SMART on a device.
    EnableSMART(ctx context.Context, devicePath string) error

    // DisableSMART disables SMART on a device.
    DisableSMART(ctx context.Context, devicePath string) error

    // AbortSelfTest aborts a running SMART self-test.
    AbortSelfTest(ctx context.Context, devicePath string) error

    // Close releases any resources held by the backend.
    Close() error
}
```

### Implemented Layout

```text
smartmontools/
├── client.go              # public facade and client orchestration
├── backend.go             # public aliases for backend interfaces
├── commander.go           # public aliases for command interfaces
├── types.go               # public aliases for domain types
├── exec_compat.go         # backward-compatible ExecBackend wrappers
├── internal/types/        # shared types, interfaces, constants, helpers
└── backends/exec/         # smartctl-backed backend implementation
```

This layout breaks the circular dependency between the root package and the
exec backend by moving shared types and interfaces into `internal/types`. The
root package depends on `internal/types` and `backends/exec`, while
`backends/exec` depends only on `internal/types` and external packages.

### Backend Selection via ClientOption

```go
// WithBackend replaces the default ExecBackend with a custom backend.
func WithBackend(b Backend) ClientOption { … }

// NewExecBackend creates the default exec-based backend.
func NewExecBackend(opts ...ExecBackendOption) (Backend, error) { … }

// NewIoctlBackend creates the native ioctl-based backend (Linux/BSD only).
func NewIoctlBackend(opts ...IoctlBackendOption) (Backend, error) { … }

// NewLibBackend creates a CGO or purego FFI backend.
func NewLibBackend(libraryPath string, opts ...LibBackendOption) (Backend, error) { … }

// NewShadowBackend wraps two backends for side-by-side comparison.
func NewShadowBackend(primary, secondary Backend, reporter TelemetryReporter, mode ShadowMode) Backend { … }
```

Default: `NewClient()` continues to use `ExecBackend` when no `WithBackend` option is
supplied, preserving full backward compatibility.

---

## Backend Options

### ShadowMode

```go
type ShadowMode int

const (
    // ShadowModeDisabled uses only the primary backend (no shadow).
    ShadowModeDisabled ShadowMode = iota
    // ShadowModeReport runs both backends and reports differences but returns primary result.
    ShadowModeReport
    // ShadowModeFallback uses primary; falls back to secondary on primary error.
    ShadowModeFallback
    // ShadowModeValidate runs both; returns an error if results differ beyond tolerance.
    ShadowModeValidate
)
```

---

## Evaluated Backend Options

### Option A — ExecBackend (current, v0.x)

Wraps the existing `Commander`/`execCommander` logic. The current implementation is
moved into an `ExecBackend` struct that implements `Backend`.

| | |
|---|---|
| **Complexity** | Low — refactor only |
| **Dependencies** | `smartctl` binary |
| **Platform** | All (Linux, macOS, Windows) |
| **Performance** | Low (process spawn per call) |
| **Status** | Implement in v0.3 |

### Option B — IoctlBackend (native Go, v0.5+)

Uses `golang.org/x/sys/unix` to issue ATA PASS-THROUGH commands via `SG_IO` (SATA)
and NVMe admin commands via `NVME_IOCTL_ADMIN_CMD`.

```
┌──────────────────┐
│  IoctlBackend    │
│  (pure Go)       │
└────────┬─────────┘
         │ syscall.Syscall / unix.IoctlRetInt
         ▼
┌──────────────────┐
│  Kernel Driver   │
│  sd / nvme       │
└────────┬─────────┘
         │ hardware
         ▼
┌──────────────────┐
│  Storage Device  │
└──────────────────┘
```

ATA SMART read sequence (Linux):
```
SG_IO ioctl → ATA PASS-THROUGH (16) CDB → ATA READ DATA (0xD0)
```

NVMe SMART sequence (Linux):
```
NVME_IOCTL_ADMIN_CMD → Get Log Page (0x02) → SMART / Health Information log
```

Platform support matrix:

| Platform | ATA/SATA | NVMe | Notes |
|----------|----------|------|-------|
| Linux | `SG_IO` + `HDIO_DRIVE_CMD` | `NVME_IOCTL_ADMIN_CMD` | Full support |
| FreeBSD | `ATA_IDENTIFY_DATA` ioctl | `/dev/nvme*` | Medium effort |
| macOS | IOKit `IOATASMARTInterface` | IOKit `IONVMeFamily` | High effort |
| Windows | `DeviceIoControl` + `IOCTL_*` | NVMe WMI / IOCTL | High effort |

Build tags isolate platform-specific code:
```
backend/ioctl/ioctl_linux.go    //go:build linux
backend/ioctl/ioctl_freebsd.go  //go:build freebsd
backend/ioctl/ioctl_stub.go     //go:build !linux && !freebsd
```

| | |
|---|---|
| **Complexity** | Medium-high (protocol knowledge required) |
| **Dependencies** | `golang.org/x/sys/unix`; root/CAP_SYS_RAWIO |
| **Platform** | Linux first; BSD/macOS/Windows later |
| **Performance** | High (no process spawn) |
| **Status** | Implement in v0.5 |

### Option C — LibBackend via CGO (v0.6+)

Compile smartmontools source and expose a thin C API shim, then use CGO to call it.

```c
// libsmartctl_api.h — C shim over C++ smartmontools
int smartctl_scan_devices(char*** out_names, char*** out_types, int* out_count);
int smartctl_get_smart_json(const char* device, const char* type, char** out_json, int* out_len);
void smartctl_free_json(char* json);
```

Advantages:
- Reuses 100% of smartmontools device-quirk handling.
- No protocol reimplementation.

Disadvantages:
- Requires C++ compiler in build environment.
- Cross-compilation is complex.
- CGO disables some Go tooling optimisations.
- Shared library must be distributed or statically linked.

| | |
|---|---|
| **Complexity** | High (C++ build chain, CGO) |
| **Dependencies** | CGO enabled; smartmontools source or `.so` |
| **Platform** | Linux/macOS first |
| **Performance** | High |
| **Status** | Evaluate in v0.6 |

### Option D — LibBackend via purego/FFI (v0.6+)

Use [ebitengine/purego](https://github.com/ebitengine/purego) to dlopen a pre-built
`libsmartctl.so` / `libsmartctl.dylib` at runtime without CGO.

```go
import "github.com/ebitengine/purego"

var smartctlGetSmartJSON func(device, dtype *byte, outJSON **byte, outLen *int32) int32

lib, _ := purego.Dlopen("libsmartctl.so", purego.RTLD_LAZY)
purego.RegisterLibFunc(&smartctlGetSmartJSON, lib, "smartctl_get_smart_json")
```

Advantages over CGO:
- No CGO: pure Go binary, easier cross-compilation.
- Dynamic loading; library can be updated independently.

Disadvantages:
- Still requires pre-built shared library on target system.
- Runtime loading failures are harder to surface at build time.

| | |
|---|---|
| **Complexity** | Medium |
| **Dependencies** | `ebitengine/purego`; libsmartctl.so installed |
| **Platform** | Linux/macOS; Windows with DLL |
| **Performance** | High |
| **Status** | Preferred over CGO in v0.6 |

#### Library Build Strategies for Creating `libsmartctl.so`

Smartmontools is a C++ CLI application — not a library. `smartctl.cpp` has `main()`,
global state, and direct stdout/stderr output. To produce a `libsmartctl.so` for
purego/FFI loading, the C++ source must be compiled into a shared library. The
following strategies address this with varying tradeoffs in maintainability,
solidness, and platform reach.

| Strategy | Maintainability | Solidness | Platform | Effort |
|----------|----------------|-----------|----------|--------|
| **D1** Patch-and-Build Pipeline | ★★★ High | ★★★ High | Linux + cross | Medium |
| **D3** CGO Static Wrapper | ★★☆ Medium | ★★★ High | Linux first | Medium |
| **D4** Upstream Contribution | ★★★★★ (once merged) | ★★★ High | All | Very High |

---

##### D1: Patch-and-Build Pipeline

Maintain a versioned patch set that adds a shared library target to smartmontools's
autotools build, without maintaining a permanent fork.

**Patch layout:**
```
patches/
├── v7.5/
│   ├── 0001-add-libsmartctl-h.patch
│   ├── 0002-add-libsmartctl-cpp.patch
│   ├── 0003-modify-makefile-am.patch
│   └── 0004-exclude-main-on-lib-build.patch
├── v7.6/
│   └── ...
├── apply.sh
└── generate.sh
```

**C API header** (`libsmartctl.h`):
```c
#ifndef LIBSMARTCTL_H
#define LIBSMARTCTL_H

#define LIBSMARTCTL_VERSION "1.0.0"

typedef struct smartctl_ctx smartctl_ctx;

int smartctl_init(smartctl_ctx** ctx);
int smartctl_set_option(smartctl_ctx* ctx, const char* key, const char* value);
int smartctl_scan_devices(smartctl_ctx* ctx, char** out_json);
int smartctl_get_smart_data(smartctl_ctx* ctx, const char* device, char** out_json);
int smartctl_check_health(smartctl_ctx* ctx, const char* device);
void smartctl_free_string(char* s);
void smartctl_destroy(smartctl_ctx* ctx);

#endif
```

**Wrapper** (`libsmartctl.cpp`) reuses existing `smartctl.cpp` internals via
`#include` and guards `main()` with a preprocessor flag:
```cpp
#include "libsmartctl.h"
#include "smartctl.h"
#include "ataprint.h"
#include "nvmeprint.h"
#include "scsiprint.h"
#include "dev_interface.h"
```

**Key patch** (`0004-exclude-main-on-lib-build.patch`) — wraps `main()`:
```diff
--- a/src/smartctl.cpp
+++ b/src/smartctl.cpp
@@ -1,3 +1,5 @@
+#ifndef BUILDING_LIBSMARTCTL
+
 // … existing code unchanged …
 int main(int argc, char** argv) { … }
+
+#endif /* !BUILDING_LIBSMARTCTL */
```

**CI workflow** (in `smartmontools-go` repo, runs weekly):
```yaml
name: build-libsmartctl

on:
  schedule:
    - cron: '0 2 * * 0'   # weekly upstream check
  workflow_dispatch:

jobs:
  check-version:
    outputs:
      version: ${{ steps.resolve.outputs.version }}
    runs-on: ubuntu-latest
    steps:
      - id: resolve
        run: |
          LATEST=$(curl -s https://api.github.com/repos/smartmontools/smartmontools/releases/latest \
            | jq -r .tag_name)
          echo "version=$LATEST" >> "$GITHUB_OUTPUT"

  build:
    needs: check-version
    strategy:
      matrix:
        arch: [amd64, arm64]
    runs-on: ${{ matrix.arch == 'arm64' && 'ubuntu-24.04-arm' || 'ubuntu-latest' }}
    steps:
      - uses: actions/checkout@v4
      - run: |
          git clone --depth 1 --branch ${{ needs.check-version.outputs.version }} \
            https://github.com/smartmontools/smartmontools.git src
          cd src
          ../patches/apply.sh ${{ needs.check-version.outputs.version }}
          ./autogen.sh
          ./configure --enable-shared --disable-static \
            CFLAGS="-fPIC" CXXFLAGS="-fPIC -DBUILDING_LIBSMARTCTL"
          make -j$(nproc)
          mkdir -p ../dist
          cp src/.libs/libsmartctl.so* ../dist/
      - uses: actions/upload-artifact@v4
        with:
          name: libsmartctl-${{ needs.check-version.outputs.version }}-linux-${{ matrix.arch }}
          path: dist/

  release:
    if: startsWith(github.ref, 'refs/tags/')
    needs: [check-version, build]
    uses: softprops/action-gh-release@v2
    with:
      files: ./*/libsmartctl-*
```

**Patch evolution:** When a new tag appears, `apply.sh` tries the existing patches.
If they fail (upstream changed too much), a maintainer runs `generate.sh` which
creates fresh patches via `git diff`. CI catches failures automatically and files
an issue via `actions/github-script`.

| | |
|---|---|
| **Maintainability** | High — conflicts detected in CI; regeneration is a single script call |
| **Solidness** | High — same compiled C++ as smartctl; no source changes besides guarded `main()` |
| **Platform** | Linux (amd64 + arm64); macOS and FreeBSD via matrix expansion |
| **Status** | Retained for reference — patches in `patches/` directory |

---

##### D3: CGO Static Wrapper

Build smartmontools into a static archive (`.a`) and link via CGO into a Go wrapper
package. The result is a single self-contained binary with no runtime `.so`.

```
┌──────────────────┐     ┌───────────────────────────┐
│  libsmartctl.a    │────>│  go build -tags=cgo        │
│  (static C++)     │     │  backends/lib/cgo_wrapper  │
└──────────────────┘     └───────────────┬─────────────┘
                                         ▼
                                  self-contained binary
```

**Go wrapper** (`backends/lib/libsmartctl_cgo.go`):
```go
//go:build cgo

package lib

// #cgo LDFLAGS: -L${SRCDIR}/libsmartctl -lsmartctl -lstdc++ -lpthread
// #cgo CPPFLAGS: -DBUILDING_LIBSMARTCTL
// #include "libsmartctl.h"
import "C"
import "unsafe"

type Context struct{ ptr *C.smartctl_ctx }

func Init() (*Context, error) {
    var ctx *C.smartctl_ctx
    if rc := C.smartctl_init(&ctx); rc != 0 {
        return nil, ErrInit
    }
    return &Context{ptr: ctx}, nil
}

func (c *Context) GetSMARTData(device string) (string, error) {
    cdev := C.CString(device)
    defer C.free(unsafe.Pointer(cdev))
    var out *C.char
    if rc := C.smartctl_get_smart_data(c.ptr, cdev, &out); rc != 0 {
        return "", ErrDevice
    }
    defer C.smartctl_free_string(out)
    return C.GoString(out), nil
}
```

**CI** cross-compiles the static library per target:
```yaml
- name: Build libsmartctl.a for ${{ matrix.target }}
  run: |
    git clone --depth 1 --branch ${{ env.SMART_VERSION }} \
      https://github.com/smartmontools/smartmontools.git
    cd smartmontools
    ./autogen.sh && ./configure --host=${{ matrix.toolchain }} \
      --enable-static --disable-shared CXXFLAGS="-fPIC -DBUILDING_LIBSMARTCTL"
    make -j$(nproc)
    cp src/.libs/libsmartctl.a ../dist/
```

| | |
|---|---|
| **Maintainability** | Medium — same patch approach as D1, but CGO disables pure-Go cross-compile paths |
| **Solidness** | High — fully statically linked; no runtime `.so` |
| **Platform** | Linux (good cross-compile); macOS/Windows feasible via toolchain matrix |
| **Performance** | Highest — direct CGO call, no FFI marshalling |
| **Tradeoff** | CGO required; Go's `-race`, tinygo, wasm targets are unavailable |
| **Status** | Good option when `.so` distribution is undesirable |

---

##### D4: Upstream-First Contribution (Long-Term Goal)

Add `--enable-libsmartctl` directly to the upstream smartmontools project.
Once merged, all downstream patch maintenance is eliminated.

Required changes to upstream:
1. **`configure.ac`** — add `AC_ARG_ENABLE([libsmartctl], ...)`
2. **`src/Makefile.am`** — conditional `libsmartctl_la_LDFLAGS = -module -shared`
3. **`src/libsmartctl.h`** — public C API header
4. **`src/libsmartctl.cpp`** — API implementation (guarded against `main()`)
5. **`smartctl.cpp`** — guard `main()` as in D1

| | |
|---|---|
| **Maintainability** | ★★★★★ — zero downstream maintenance once merged |
| **Solidness** | Highest — maintained by smartmontools project |
| **Platform** | All upstream-supported platforms |
| **Status** | Begin discussion after D1/D2 validation; target v1.0+ |

---

**Recommendation:** **D1 (Patch-and-Build Pipeline)** is the active strategy for
building `libsmartctl.so`. Pursue **D4 (Upstream Contribution)** independently as a
long-term goal.

### Option E — Native Go Port (v1.0+)

Full reimplementation of smartmontools SMART protocol layer in Go:
- ATA SMART attribute parsing
- NVMe log page parsing
- SCSI SMART passthrough
- drivedb.h already embedded (in use today)

This is the longest path but produces a fully self-contained library with zero runtime
dependencies. It is the **long-term strategic goal**.

Migration strategy:
1. Start with ATA attribute parsing (most widely used).
2. Add NVMe admin command support.
3. Add SCSI passthrough.
4. Add USB bridge detection (leverage existing drivedb parser).
5. Platform-specific kernel interface per build tag.

| | |
|---|---|
| **Complexity** | Very high |
| **Dependencies** | None (pure Go) |
| **Platform** | Incremental, Linux first |
| **Performance** | Very high |
| **Status** | Begin in v1.0 |

---

## Migration Strategy

The migration is **soft** — each phase introduces a new backend without removing
the old one. All phases ship the exec backend as a stable fallback.

| Release | New Capability | Backward Compatible |
|---------|---------------|---------------------|
| v0.3 | Backend interface + ExecBackend refactor + `WithBackend` option | ✅ Yes |
| v0.4 | ShadowBackend + TelemetryReporter | ✅ Yes |
| v0.5 | IoctlBackend (Linux) | ✅ Yes |
| v0.6 | LibBackend (purego) | ✅ Yes |
| v1.0 | NativeBackend (full Go port, ATA+NVMe) | ✅ Yes |
| v1.x | Deprecate ExecBackend (optional dependency) | ⚠️ Non-breaking |

At each release, consumers can opt into the new backend via `WithBackend(...)`,
while existing code continues to work unchanged with `NewClient()`.

---

## Consequences

### Positive
- Public `SmartClient` interface is **unchanged** — zero breaking changes.
- New backends can be developed and tested in isolation.
- Shadow mode (ADR-003) enables safe production validation before switching.
- The library becomes embeddable in minimal environments.

### Negative
- More code to maintain per backend.
- IoctlBackend requires ongoing protocol knowledge.
- CGO/purego backends require distributing a shared library.

### Neutral
- `Commander` interface is retained for backward compatibility but becomes
  an implementation detail of `ExecBackend` only.

---

## Directory Layout

```
smartmontools-go/
├── backend/
│   ├── backend.go          # Backend interface + ShadowMode types
│   ├── exec/
│   │   └── exec.go         # ExecBackend (refactored from client.go)
│   ├── ioctl/
│   │   ├── ioctl_linux.go  # Linux ioctl implementation
│   │   ├── ioctl_stub.go   # Stub for unsupported platforms
│   │   └── ata/            # ATA SMART protocol parsing
│   │       └── attributes.go
│   ├── lib/
│   │   └── lib.go          # purego / CGO FFI backend
│   └── shadow/
│       └── shadow.go       # ShadowBackend
├── telemetry/
│   ├── telemetry.go        # TelemetryReporter interface
│   ├── log/                # slog/tlog reporter
│   └── otel/               # OpenTelemetry reporter
├── client.go               # SmartClient orchestrator (thin)
├── types.go                # Shared types (unchanged)
└── …
```

---

## References

- [smartmontools source](https://github.com/smartmontools/smartmontools)
- [Linux kernel ATA commands](https://www.kernel.org/doc/html/latest/driver-api/libata.html)
- [NVMe specification](https://nvmexpress.org/specifications/)
- [ebitengine/purego](https://github.com/ebitengine/purego)
- [golang.org/x/sys/unix](https://pkg.go.dev/golang.org/x/sys/unix)
- [ADR-001: SMART Data Access Approaches](./ADR-001-smart-access-approaches.md)
- [ADR-003: Shadow Mode and Telemetry](./ADR-003-shadow-mode-telemetry.md)
- [ROADMAP](./ROADMAP.md)
