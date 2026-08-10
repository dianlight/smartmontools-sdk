# smartmontools-go Roadmap

This document describes the phased plan to evolve the library from an exec-based
wrapper around `smartctl` to a self-contained Go library with multiple pluggable
backends. Each phase is a separate minor-version series; all phases maintain full
backward compatibility with the existing public API.

---

## Current State (v0.2.x)

```
SmartClient (client.go)
    └── Commander interface  ←  execCommander (os/exec + smartctl binary)
```

- Single backend: exec smartctl, parse JSON output.
- Requires smartctl ≥ 7.0 installed on the host system.
- drivedb.h embedded for USB bridge lookup (manually updated).

---

## Phase 1 — Backend Abstraction (v0.3)

**Goal**: Extract a `Backend` interface. The current exec logic becomes `ExecBackend`.
Consumers can inject any backend via `WithBackend(...)`. Nothing visible changes
for existing users.

**Key deliverables**:
- `backend/backend.go` — `Backend` interface
- `backend/exec/exec.go` — `ExecBackend` (refactored from `client.go`)
- `client.go` refactored to delegate to `Backend`
- `WithBackend(Backend) ClientOption` added
- drivedb auto-update GitHub Actions workflow (ADR-004)
- `drivedb_version.go` generated constant

**Consumer impact**: None. `NewClient()` still uses `ExecBackend` by default.

---

## Phase 2 — Shadow Mode & Telemetry (v0.4)

**Goal**: Enable side-by-side execution of two backends for production validation.

**Key deliverables**:
- `backend/shadow/shadow.go` — `ShadowBackend`
- `ShadowMode` enum: `Disabled`, `Report`, `Fallback`, `Validate`
- `telemetry/telemetry.go` — `TelemetryReporter` interface + `DiffEvent`/`FallbackEvent`
- `telemetry/log/` — slog/tlog reporter
- `telemetry/otel/` — OpenTelemetry reporter (spans + metrics)
- `telemetry/webhook/` — HTTP webhook reporter
- Structural diff engine with numeric tolerance and field exclusions
- Sampling rate support (reduce overhead for high-frequency polling)

**Consumer impact**: New opt-in API. Existing code unchanged.

**Typical migration workflow during this phase**:
```
ShadowModeReport (primary=exec, secondary=new)
  → monitor DiffEvent.Diffs rate in production
  → when diff rate ≈ 0 → switch to ShadowModeFallback
  → when primary errors ≈ 0 → switch to new backend directly
```

---

## Phase 3 — Native ioctl Backend (v0.5)

**Goal**: Pure Go backend using kernel ioctl calls. No external binary. Linux-first.

**Key deliverables**:
- `backend/ioctl/ioctl_linux.go` — ATA SMART via `SG_IO` + NVMe via `NVME_IOCTL_ADMIN_CMD`
- `backend/ioctl/ata/` — ATA SMART attribute parser (512-byte data structure)
- `backend/ioctl/nvme/` — NVMe SMART/Health Information log page parser
- `backend/ioctl/ioctl_stub.go` — Unsupported platform stub returning `ErrUnsupported`
- `NewIoctlBackend(...) (Backend, error)` constructor
- Build tag isolation: `//go:build linux`
- drivedb parser extended to expose firmware quirks (beyond USB-only)

**Platform support at v0.5**:

| Protocol | Linux | FreeBSD | macOS | Windows |
|----------|-------|---------|-------|---------|
| ATA/SATA | ✅ | planned | planned | planned |
| NVMe | ✅ | planned | planned | planned |
| SCSI | planned | — | — | — |
| USB bridge | ✅ (via drivedb) | — | — | — |

**Consumer impact**: New opt-in backend. Exec backend unchanged and default.

---

## Phase 4 — purego Library Backend (v0.6) ✅ Implemented

**Goal**: Use smartmontools compiled as a shared library loaded at runtime via
[ebitengine/purego](https://github.com/ebitengine/purego) — no CGO required.

### Implementation — purego (Option A, implemented)

Loads the pre-built `libsmartmon_go.so` / `libsmartmon_go.dylib` wrapper at runtime.
Pure Go binary; no CGO.

**Delivered**:
- `backends/lib/lib.go` — `LibBackend` (purego dlopen + symbol binding)
- `backends/lib/csrc/smartmon_c_api.{h,cpp}` — thin C++ wrapper over `libsmartmon.a`
- `backends/lib/lib_stub.go` — unsupported-platform stub
- `lib.New(opts ...Option) (*LibBackend, error)` constructor

**Consumer impact**: Opt-in backend. Set `SMARTMON_LIB_PATH` or pass
`libbackend.WithLibraryPath`. See [README](../../README.md#libbackend-purego-ffi--linuxmacos)
for setup instructions.

---

## Phase 5 — Native Go Backend (v1.0)

**Goal**: Full reimplementation of SMART protocol layer in Go. Zero runtime
dependencies. The library becomes fully self-contained.

**Migration strategy**:

| Milestone | Scope |
|-----------|-------|
| v1.0-alpha | ATA SMART attributes (read-only) |
| v1.0-beta | NVMe SMART log pages |
| v1.0-rc | SCSI SMART passthrough |
| v1.0 | USB bridge negotiation (from drivedb) |
| v1.1 | ATA self-test execution |
| v1.2 | Windows (DeviceIoControl) |
| v1.3 | macOS (IOKit) |

The `NativeBackend` reuses:
- `drivedb.go` / `drivedb.h` for USB device type resolution.
- All existing types from `types.go` (zero API change).
- The `Backend` interface from Phase 1.

**Consumer impact**: `NativeBackend` becomes the new recommended default.
`ExecBackend` remains available but is documented as a legacy fallback.

---

## Phase 6 — Soft ExecBackend Deprecation (v1.x)

When `NativeBackend` reaches feature parity with `ExecBackend`:
- Deprecation notice added to `ExecBackend` godoc.
- `NewClient()` switches default to `NativeBackend` behind a build tag
  (opt-in, not forced).
- Documentation updated with migration guide.
- `ExecBackend` retained indefinitely as an opt-in for edge cases.

---

## Cross-Cutting Concerns

### drivedb.h Tracking (all phases)

Automated daily workflow detects upstream changes and opens a PR automatically.
See [ADR-004](./ADR-004-drivedb-autoupdate.md).

### Backward Compatibility

The `SmartClient` public interface is **frozen** from v0.2 onward. All additions
are additive (new constructors, new options, new optional interfaces). No existing
method signatures change.

### Build Tags

```
//go:build !noioctl   → enables IoctlBackend by default on Linux
//go:build noioctl    → disables IoctlBackend (use for containers without CAP_SYS_RAWIO)
//go:build cgo        → enables CGO LibBackend
```

### Permissions

| Backend | Minimum Privilege |
|---------|------------------|
| ExecBackend | Depends on smartctl install; typically root or `disk` group |
| IoctlBackend | `CAP_SYS_RAWIO` (Linux); root on macOS |
| LibBackend (purego) | Same as IoctlBackend |
| NativeBackend | Same as IoctlBackend |

### Testing Strategy

Each backend ships with:
1. Unit tests using synthetic ioctl/output fixtures.
2. Integration tests gated behind `//go:build integration` (require real hardware or
   device emulator).
3. Shadow mode tests that run ExecBackend vs. new backend on the same device and
   assert zero diffs.

---

## ADR Index

| ADR | Title | Status |
|-----|-------|--------|
| [ADR-001](./ADR-001-smart-access-approaches.md) | SMART Data Access Approaches | Proposed |
| [ADR-002](./ADR-002-multi-backend-architecture.md) | Multi-Backend Architecture | Proposed |
| [ADR-003](./ADR-003-shadow-mode-telemetry.md) | Shadow Mode and Telemetry | Proposed |
| [ADR-004](./ADR-004-drivedb-autoupdate.md) | Automated drivedb.h Tracking | Proposed |

---

## References

- [smartmontools upstream](https://github.com/smartmontools/smartmontools)
- [Linux kernel ATA documentation](https://www.kernel.org/doc/html/latest/driver-api/libata.html)
- [NVMe specification](https://nvmexpress.org/specifications/)
- [ebitengine/purego](https://github.com/ebitengine/purego)
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/instrumentation/go/)
