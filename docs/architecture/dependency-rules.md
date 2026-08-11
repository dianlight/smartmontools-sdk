# Dependency rules

## The rule

Dependencies point one way only:

```
bindings/*  →  native/capi/  →  native/
```

- `native/` never references anything under `bindings/` or `native/capi/`.
- `native/capi/` never references anything under `bindings/`, and exposes
  only the 12 symbols declared in `native/capi/smartmon_c_api.h`
  (`smartmon_abi_version`, `smartmon_init`, `smartmon_cleanup`,
  `smartmon_scan_devices`, `smartmon_get_smart_data`, `smartmon_check_health`,
  `smartmon_enable_smart`, `smartmon_disable_smart`, `smartmon_run_selftest`,
  `smartmon_abort_selftest`, `smartmon_free_string`, `smartmon_last_error`).
- A binding (`bindings/go/`, and future `bindings/python/`,
  `bindings/rust/`) may call those 12 symbols, or shell out to a separate
  `smartctl` process, but never links against `native/`'s C++ objects
  directly or parses its internal headers.

Nothing in this repository is exempt: CI, release tooling, and the drivedb
sync script all respect the same direction (see
[repository-layout.md](repository-layout.md) for where each script lives).

## Why this was not the case before

Before the issue #17 restructure, `smartmontools-sdk` (the native core) and
`smartmontools-go` (the Go bindings) were two repositories, and each named
the *other* as authoritative for the same artifact:

- `smartmontools-sdk` PR #15 fetched `smartmon_c_api.{cpp,h}` **from** the Go
  repository at a pinned tag (`SMARTMONTOOLS_GO_REF: v0.4.1`) and compiled it
  into `libsmartmon_go.so`.
- `smartmontools-go` PR #39 documented the SDK repository as the
  authoritative *builder* of that same wrapper, pinned to the same tag.

That is a cycle: the C ABI shim's source of truth lived in the Go repo, but
the Go repo's build instructions pointed back at the SDK repo to produce it.
Both PRs duplicated the version pin `v0.4.1` as an unsynchronized string
constant with no mechanism keeping the two copies aligned. Neither PR could
merge without cementing the loop, which is what forced issue #17.

Two structural problems compounded the cycle:

1. **No language-neutral home for the C ABI shim.** It lived inside the Go
   repository, so any future `bindings/python/` or `bindings/rust/` would
   have had to depend on the Go repo too, not on the native core directly —
   turning a one-to-many relationship into a chain.
2. **No version contract at the boundary.** Nothing detected a mismatch
   between the wrapper's actual C ABI and what a binding expected. A binding
   could load a stale or newer wrapper and get silently wrong behaviour.

## How the one-way rule fixes both

Moving the C ABI shim into `native/capi/` — inside the same repository as
`native/`, and with zero knowledge of any binding — removes the *possibility*
of the cycle: there is no other repository left for it to point back to.

The version contract closes the second gap. `smartmon_abi_version()` returns
a `(major<<16)|(minor<<8)|patch` triple; a binding checks it before calling
anything else, using three-tier semver:

- **major** must match exactly — an incompatible break.
- **minor** must be `>=` what the binding was built against — additive only.
- **patch** is unchecked — affects neither symbols nor semantics.

This matters because of a fact about `purego` (used by `bindings/go`'s
`LibBackend`) that PR #39 got wrong: `purego.RegisterLibFunc` binds by
**symbol name only** and does not validate function signatures. A *missing*
symbol is detectable and produces a clear load error. A symbol that exists
but has a **changed signature** binds successfully and produces undefined
behaviour at the first call — there is no "silent fallback to the exec
backend," there is memory corruption or a wrong result. `smartmon_abi_version()`
exists specifically because signature mismatches are otherwise invisible
until they crash. See [abi-contract.md](abi-contract.md) for the full policy.

## What this buys

- `native/capi/` can gain new symbols (minor bump) without touching a single
  binding.
- The runtime contract is the ABI check, not the release cascade. Since
  `release-core.yml` now always tags a paired `bindings/go/vX.Y.Z` on every
  core release (see
  [../development/release-process.md](../development/release-process.md)),
  `native/` and `bindings/go/` share a release *version* — but a binding still
  only trusts `smartmon_abi_version()` at load time, never the tag it shipped
  under. A binding built against an older `native/capi/` keeps working against
  a newer wrapper as long as the ABI major matches and the minor is `>=`,
  regardless of how far the two version numbers have since diverged in a
  fork or a vendored copy.
- A future `bindings/python/` or `bindings/rust/` depends on `native/capi/`
  exactly the same way `bindings/go/` does, with no dependency on Go at all.
