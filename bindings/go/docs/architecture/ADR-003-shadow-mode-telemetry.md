# ADR-003: Shadow Mode and Telemetry

## Status

Proposed

## Context

Before switching from the exec backend to any new backend (ioctl, lib, native Go),
we need a mechanism to validate that the new backend produces **equivalent results**
to the proven exec backend in real production workloads. Offline unit tests against
mock data cannot cover all device models, firmware revisions, and edge cases seen in
the wild.

A **shadow mode** runs two backends in parallel on every call. The primary backend's
result is returned to the caller; the secondary backend's result is compared
asynchronously and any differences are reported via a **telemetry reporter**. This
provides:

- Continuous regression detection during the migration period.
- Quantitative data on backend parity before promoting the secondary to primary.
- Automatic fault isolation: if the secondary backend errors, the primary result
  is still returned cleanly.

---

## Decision

### ShadowBackend

`ShadowBackend` is itself a `Backend` implementation. It wraps a *primary* and a
*secondary* backend and a `TelemetryReporter`. The caller receives the primary
result in all cases; secondary execution and diffing happen in a background goroutine
to avoid adding latency to the hot path.

```go
// ShadowMode controls how the shadow backend responds to differences.
type ShadowMode int

const (
    // ShadowModeDisabled — secondary is not called. Equivalent to primary alone.
    ShadowModeDisabled ShadowMode = iota
    // ShadowModeReport — secondary runs async; differences are reported; primary result returned.
    ShadowModeReport
    // ShadowModeFallback — primary result returned; if primary errors, secondary is tried.
    ShadowModeFallback
    // ShadowModeValidate — both run; error returned to caller if results diverge beyond tolerance.
    ShadowModeValidate
)
```

#### Execution Flow (ShadowModeReport)

```
caller ──▶ ShadowBackend.GetSMARTInfo(ctx, device)
                │
                ├── (sync)  primary.GetSMARTInfo(ctx, device) → primaryResult
                │
                ├── (async goroutine, budget = ctx deadline - 200ms)
                │       secondary.GetSMARTInfo(ctx, device) → secondaryResult
                │       diff(primaryResult, secondaryResult) → []FieldDiff
                │       reporter.ReportDiff(...)
                │
                └── return primaryResult to caller   ← no added latency
```

#### Execution Flow (ShadowModeFallback)

```
caller ──▶ ShadowBackend.GetSMARTInfo(ctx, device)
                │
                ├── primary.GetSMARTInfo(ctx, device)
                │       ├── OK  → return primaryResult
                │       └── ERR → secondary.GetSMARTInfo(ctx, device)
                │                       ├── OK  → reporter.ReportFallback; return secondaryResult
                │                       └── ERR → return combined error
```

#### Execution Flow (ShadowModeValidate)

```
caller ──▶ ShadowBackend.GetSMARTInfo(ctx, device)
                │
                ├── (concurrent) primary.GetSMARTInfo  ─┐
                ├── (concurrent) secondary.GetSMARTInfo ─┤→ both results
                │                                        │
                ├── diff(primaryResult, secondaryResult)  │
                │       ├── within tolerance → return primary
                │       └── diverges        → reporter.ReportDiff; return error
```

---

### TelemetryReporter Interface

```go
// DiffEvent contains the results of a shadow comparison for one operation.
type DiffEvent struct {
    Operation     string          // "GetSMARTInfo", "ScanDevices", …
    DevicePath    string
    PrimaryName   string          // backend name
    SecondaryName string
    PrimaryResult any             // *SMARTInfo, []Device, bool, …
    SecondaryResult any
    Diffs         []FieldDiff     // nil when results are equal
    PrimaryErr    error
    SecondaryErr  error
    PrimaryLatency   time.Duration
    SecondaryLatency time.Duration
    Timestamp     time.Time
}

// FieldDiff describes a single field that differs between backends.
type FieldDiff struct {
    Field     string // JSON path, e.g. "nvme_smart_health_information_log.temperature"
    Primary   any
    Secondary any
}

// FallbackEvent is emitted when the primary backend failed and secondary was used.
type FallbackEvent struct {
    Operation  string
    DevicePath string
    PrimaryErr error
    Timestamp  time.Time
}

// TelemetryReporter receives shadow comparison events.
// Implementations must be safe for concurrent use.
type TelemetryReporter interface {
    // ReportDiff is called after every shadow comparison (even when Diffs is empty).
    ReportDiff(ctx context.Context, event DiffEvent)
    // ReportFallback is called when the primary backend failed and secondary was used.
    ReportFallback(ctx context.Context, event FallbackEvent)
}
```

Bundled reporters:

| Reporter | Package | Description |
|---|---|---|
| `LogReporter` | `telemetry/log` | Writes structured events via `slog` / `tlog` |
| `OtelReporter` | `telemetry/otel` | Emits OpenTelemetry spans + metrics |
| `WebhookReporter` | `telemetry/webhook` | POSTs JSON events to an HTTP endpoint |
| `MultiReporter` | `telemetry` | Fan-out to multiple reporters |
| `NopReporter` | `telemetry` | Discards all events (testing) |

---

### Diffing Strategy

SMART data contains fields that legitimately differ between calls (e.g. power-on
hours, temperature, raw sensor readings that tick every second). The diff engine
must account for **tolerance** on numeric fields.

```go
// DiffOptions configures how two SMARTInfo values are compared.
type DiffOptions struct {
    // IgnoreFields lists JSON paths to skip entirely.
    IgnoreFields []string
    // NumericTolerances maps JSON paths to allowed absolute delta.
    // e.g. {"temperature.current": 2, "power_on_time.hours": 0}
    NumericTolerances map[string]float64
    // IgnoreSmartctlMetadata skips the "smartctl" metadata block
    // (version, messages, exit_status) which differs between backends.
    IgnoreSmartctlMetadata bool
}

// DefaultDiffOptions returns sensible defaults for production use.
func DefaultDiffOptions() DiffOptions {
    return DiffOptions{
        IgnoreSmartctlMetadata: true,
        NumericTolerances: map[string]float64{
            "temperature.current":                   2,
            "nvme_smart_health_information_log.temperature": 2,
            "power_on_time.hours":                   0,
        },
    }
}
```

Structural diff uses `reflect.DeepEqual` after applying tolerances and ignoring
excluded paths. The output is a flat list of `FieldDiff` entries with JSON-path
field names.

---

### Automatic Telemetry Sampling

For high-frequency polling workloads, reporting every comparison would be noisy.
`ShadowBackend` supports a **sampling rate** (default 100%, configurable):

```go
type ShadowBackendOptions struct {
    Mode          ShadowMode
    Reporter      TelemetryReporter
    DiffOptions   DiffOptions
    // SampleRate is the fraction of calls on which the secondary is invoked (0.0–1.0).
    // Default 1.0 (every call). Set to 0.1 to shadow 10% of calls.
    SampleRate    float64
    // AsyncTimeout is the maximum time budget given to the secondary call.
    // Default: half of the remaining context deadline, capped at 5s.
    AsyncTimeout  time.Duration
}
```

---

### Usage Example

```go
primary, _ := exec.NewExecBackend()
secondary, _ := ioctl.NewIoctlBackend()

reporter := telemetry.NewMultiReporter(
    log.NewLogReporter(slog.Default()),
    otel.NewOtelReporter(otel.DefaultMeterProvider()),
)

shadow := shadow.NewShadowBackend(primary, secondary, reporter,
    shadow.ShadowBackendOptions{
        Mode:       shadow.ShadowModeReport,
        SampleRate: 0.25, // shadow 25% of calls
        DiffOptions: shadow.DefaultDiffOptions(),
    },
)

client, _ := smartmontools.NewClient(
    smartmontools.WithBackend(shadow),
)
```

During the v0.4→v0.5 transition period, consumers run `ShadowModeReport` in
production. When the `DiffEvent.Diffs` rate drops to zero over a monitoring
window, they switch to `ShadowModeFallback`, then eventually to the new backend
alone.

---

## Consequences

### Positive
- Zero-risk validation of new backends in production environments.
- Quantitative parity data before promoting a backend.
- Primary result always returned; secondary failures are non-fatal.
- Pluggable reporters allow integration with existing observability stacks
  (OpenTelemetry, Prometheus via OTel bridge, custom webhooks).

### Negative
- Shadow mode uses 2× resources (CPU, I/O, memory) when both backends execute.
- Async goroutines require careful context management to avoid leaks.
- Diffing complex nested structs adds a small CPU cost.

### Neutral
- `ShadowModeDisabled` makes `ShadowBackend` a zero-overhead passthrough,
  so it can remain configured in code paths that are not currently migrating.

---

## References

- [ADR-002: Multi-Backend Architecture](./ADR-002-multi-backend-architecture.md)
- [ebitengine/purego](https://github.com/ebitengine/purego)
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/instrumentation/go/)
- [ROADMAP](./ROADMAP.md)
