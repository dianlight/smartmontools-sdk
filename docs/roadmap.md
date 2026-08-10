# Roadmap

## Why this monorepo exists

Issue #17 restructured `smartmontools-sdk` from a two-repository split
(native core + Go bindings) into a single repository with a strict one-way
dependency: `bindings/*` → `native/capi/` → `native/`. See
[architecture/dependency-rules.md](architecture/dependency-rules.md) for the
circular-dependency problem this solved. The same structure is designed to
scale to more than one language binding without repeating that problem.

## Current state

- `native/` — the C++ core, vendored from upstream smartmontools.
- `native/capi/` — the C ABI boundary, versioned via `smartmon_abi_version()`.
- `bindings/go/` — complete, tagged `bindings/go/v0.5.0` and up. Two backend
  implementations (`ExecBackend`, `LibBackend`) plus a `CompareBackend` for
  validating agreement between them.

## Planned: additional language bindings

`bindings/python/` and `bindings/rust/` exist today as placeholders. Both
would consume exactly the same `native/capi/smartmon_c_api.h` boundary that
`bindings/go/`'s `LibBackend` does — the C ABI was designed language-neutral
specifically so this is additive work, not a redesign:

- **`bindings/python/`** — likely `ctypes` or `cffi` against the same
  wrapper shared library, mirroring `LibBackend`'s `dlopen` + ABI-version
  check approach rather than CGO/native-extension compilation.
- **`bindings/rust/`** — likely a `bindgen`-generated FFI layer over
  `smartmon_c_api.h` directly, since Rust's FFI story doesn't need the
  `dlopen`-at-runtime indirection Go's no-CGO constraint required.

Neither has an implementation yet; whichever lands first should validate
that `native/capi/`'s existing 12-symbol surface is genuinely sufficient
for a second consumer, since `bindings/go/` is currently the only thing
that's ever exercised it in anger.

## Longer-term: vendoring strategy

`native/lib`, `native/include`, and `native/src` are currently a flat
overlay copy of upstream smartmontools sources (see
[architecture/repository-layout.md](architecture/repository-layout.md#vendoring-strategy-overlay-not-patches)),
not a patch series applied against the `native/upstream` submodule checkout.
Restructuring this into a `native/patches/` tier — patches applied on top of
a real upstream checkout, making upstream diffs and rebases explicit — was
identified during the issue #17 restructure but explicitly deferred: it's a
question about how the vendored fork is maintained, orthogonal to the
repository-layout change issue #17 addressed.

## Explicitly out of scope

- Archiving the old `smartmontools-go` repository — it stays live as a
  read-write transition repo (see
  [migration/smartmontools-go-to-bindings-go.md](migration/smartmontools-go-to-bindings-go.md))
  until downstream consumers have migrated.
