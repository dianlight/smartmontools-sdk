# Architecture overview

`smartmontools-sdk` is a monorepo with three layers, each depending only on the
one below it:

```
bindings/go, bindings/python, bindings/rust   (language bindings)
                    │
                    ▼
              native/capi/                     (stable C ABI)
                    │
                    ▼
                 native/                        (C++ core, vendored from upstream)
```

The dependency arrow points one way, always. `native/` has no knowledge of any
binding; `native/capi/` has no knowledge of Go, Python, or Rust; a binding may
depend on `native/capi/`'s symbols but never reaches into `native/`'s C++
internals directly. This rule is what issue #17 restructured the repository to
enforce — see [dependency-rules.md](dependency-rules.md) for why the previous
two-repository layout violated it.

## `native/` — the C++ core

A vendored copy of [smartmontools](https://www.smartmontools.org/)'s device
and SMART/health-monitoring logic, built with the project's original autotools
build system (`autogen.sh` / `configure` / `make`). It produces a static
library, `libsmartmon.a`, built with `--with-pic` so its object code can be
folded into a shared library one layer up. `native/upstream` is a git submodule
pinned to a specific smartmontools release, used only for version-string
detection (`git describe`) — nothing compiles from it.

## `native/capi/` — the C ABI boundary

A thin `extern "C"` wrapper (`smartmon_c_api.cpp`/`.h`) around the C++ core:
device scanning, SMART data retrieval, health checks, self-tests. This is the
only language-neutral surface any binding is allowed to call. It is built into
a shared library (`libsmartmon_go.{so,dylib}`) by `scripts/build-wrapper.sh`,
deliberately without libtool — see that script's header comment for why.

The boundary carries an explicit version contract: `smartmon_abi_version()`
returns a `(major<<16)|(minor<<8)|patch` triple that every binding checks
before calling anything else. See
[abi-contract.md](abi-contract.md) for the compatibility rules this encodes.

## `bindings/*` — language bindings

Each binding directory is an independently versioned, independently tagged
consumer of the C ABI. `bindings/go/` is implemented today, tagged
`bindings/go/vX.Y.Z`; `bindings/python/` and `bindings/rust/` are placeholders
for future work against the same boundary (see [roadmap.md](../roadmap.md)).
A binding may link the wrapper directly (as `bindings/go`'s `LibBackend` does
via `purego`/`dlopen`) or shell out to a separate `smartctl` process (as
`bindings/go`'s `ExecBackend` does) — both are valid ways to consume the same
underlying SMART data, and `bindings/go` ships both so callers can choose.

## Independent release tracks

`native/` and each `bindings/*` release on separate tag namespaces
(`vX.Y.Z` for the core, `bindings/go/vX.Y.Z` for the Go module) and separate
GitHub Actions workflows. They are unified only by the ABI contract: a
binding release states the core major/minor version it requires, and
`smartmon_abi_version()` enforces that requirement at load time rather than at
build time. This is what makes partially-independent releases safe — see
[abi-contract.md](abi-contract.md).
