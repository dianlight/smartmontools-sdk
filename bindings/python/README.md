# smartmontools-sdk — Python bindings (planned)

Not yet implemented. See [../../docs/roadmap.md](../../docs/roadmap.md) for
the plan: Python bindings would consume `native/capi/smartmon_c_api.h`
directly (likely via `ctypes` or `cffi`), the same language-neutral C ABI
boundary that [`bindings/go/`](../go/)'s `LibBackend` loads with `purego`.

See [../../docs/architecture/dependency-rules.md](../../docs/architecture/dependency-rules.md)
for the one-way dependency rule any binding here must follow.
