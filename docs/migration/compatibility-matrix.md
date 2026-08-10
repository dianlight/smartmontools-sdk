# Compatibility matrix

| old `smartmontools-go` | new `bindings/go` | `native/` (libsmartmon) | C ABI (`smartmon_abi_version`) | Notes |
|---|---|---|---|---|
| `v0.4.1` | — | 8.0 | 1.0 (undeclared — symbol did not yet exist) | Last pre-monorepo release. Its wrapper was built by fetching `smartmon_c_api.{cpp,h}` cross-repo from a pinned SDK tag; that fetch is what issue #17 eliminated. |
| — | `bindings/go/v0.5.0` | 8.0 | 1.0 | First monorepo release. Module path change, relicense to GPL-2.0-or-later, `smartmon_abi_version()` introduced, wrapper now built in-tree from `native/`. |

## How to read this table

- **`native/` (libsmartmon)** is the vendored smartmontools core version
  (`AC_INIT` in `native/configure.ac`), independent of the Go module's own
  version number.
- **C ABI** is `smartmon_abi_version()`'s `major.minor` — the number a
  `LibBackend` build actually checks at load time, per
  [../architecture/abi-contract.md](../architecture/abi-contract.md). This is
  the number that matters for compatibility, not the `native/` or
  `bindings/go` release numbers, which can advance independently of each
  other and of the ABI.
- `v0.4.1`'s ABI is listed as "undeclared" rather than "1.0" because
  `smartmon_abi_version()` did not exist yet — there was no runtime-checkable
  version at all, only the unsynchronized `SMARTMONTOOLS_GO_REF: v0.4.1`
  string pin described in
  [smartmontools-go-to-bindings-go.md](smartmontools-go-to-bindings-go.md).

This table will grow one row per `bindings/go/vX.Y.Z` release whose C ABI
requirement changes — see
[../development/release-process.md](../development/release-process.md) for
when a release is expected to bump the ABI major/minor it requires.
