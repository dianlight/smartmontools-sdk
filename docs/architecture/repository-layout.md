# Repository layout

```
native/                  C++ core (vendored smartmontools, autotools build)
  upstream/               git submodule → smartmontools/smartmontools.git,
                           used ONLY for `git describe` version detection;
                           nothing compiles from this checkout
  include/, lib/, src/    the vendored source tree itself (a copy, not a
                           patch series — see "vendoring strategy" below)
  drivedb/drivedb.h        single source of truth for the drive database
  capi/                    the C ABI boundary (smartmon_c_api.{cpp,h})
  build/                   configure/make output (gitignored)

bindings/
  go/                     Go bindings, tagged bindings/go/vX.Y.Z
    docs/                  Go-specific docs (drivedb.md, ADRs, roadmap) —
                            distinct from the top-level docs/ tree you're
                            reading now, which covers the whole monorepo
  python/                 placeholder — see docs/roadmap.md
  rust/                   placeholder — see docs/roadmap.md

scripts/
  build-wrapper.sh         builds native/capi/ into a shared library
                            (libsmartmon_go.{so,dylib}) for the 7-target
                            release matrix
  sync-drivedb-go.sh        regenerates bindings/go/backends/exec/drivedb.h
                            from native/drivedb/drivedb.h — the enforcement
                            mechanism behind "single source of truth"

integration/
  core-go/                 end-to-end contract test: builds native/ + the
                            capi wrapper, checks the exported symbol set and
                            smartmon_abi_version(), then runs bindings/go's
                            lib-backend tests against the freshly built
                            wrapper — this is what neither smartmontools-go
                            nor smartmontools-sdk could test standalone

packaging/
  artifacts/               build output staged for release tarballs
                            (gitignored)

docs/                     you are here — monorepo-wide documentation
.github/workflows/        see docs/development/release-process.md
```

## Vendoring strategy: overlay, not patches

`native/lib`, `native/include`, and `native/src` are a **copy** of
smartmontools sources, not a patch series applied to `native/upstream`. The
submodule pin exists solely so `native/autogen.sh`/`configure` can run
`git describe` against it for version strings — no build step reads source
files out of `native/upstream`. This means upgrading the vendored core is a
manual re-copy-and-diff exercise, not a `git merge`. Restructuring this into
a `native/patches/` overlay against a real upstream checkout is tracked as
future work (see [../roadmap.md](../roadmap.md)) and was explicitly kept out
of scope for the issue #17 restructure — it's a separate question about how
the fork is maintained, not about repository layout.

## Directory ownership

| Path | Owner concern | Changes independently of |
|---|---|---|
| `native/` (excluding `capi/`) | Vendored C++ core | everything above it |
| `native/capi/` | C ABI boundary | the bindings that consume it, as long as `smartmon_abi_version()`'s contract holds |
| `bindings/go/` | Go-specific concerns: `purego` bindings, `exec` fallback, shadow-mode telemetry | `bindings/python/`, `bindings/rust/` |
| `scripts/` | Cross-cutting build/sync tooling used by more than one tier | — |
| `integration/` | Cross-tier contracts that no single tier can assert alone | — |

This ownership table is what [dependency-rules.md](dependency-rules.md)'s
one-way rule protects: a change to `bindings/go/` should never require a
change to `native/capi/`, and a change to `native/capi/` should only require
a *version bump*, never a rewrite, in a binding that already checks
`smartmon_abi_version()`.
