# smartmontools-sdk — Rust bindings (planned)

Not yet implemented. See [../../docs/roadmap.md](../../docs/roadmap.md) for
the plan: Rust bindings would consume `native/capi/smartmon_c_api.h` directly
(likely via `bindgen`-generated FFI), the same language-neutral C ABI
boundary that [`bindings/go/`](../go/)'s `LibBackend` loads with `purego`.

See [../../docs/architecture/dependency-rules.md](../../docs/architecture/dependency-rules.md)
for the one-way dependency rule any binding here must follow.
