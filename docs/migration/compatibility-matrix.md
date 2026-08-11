# Compatibility matrix

| old `smartmontools-go` | `native/` release | `bindings/go` release | C ABI (`smartmon_abi_version`) | Notes |
|---|---|---|---|---|
| `v0.4.1` | 8.0 (unreleased as a tag) | — | 1.0 (undeclared — symbol did not yet exist) | Last pre-monorepo release. Its wrapper was built by fetching `smartmon_c_api.{cpp,h}` cross-repo from a pinned SDK tag; that fetch is what issue #17 eliminated. |
| — | `v8.0.0` | `bindings/go/v8.0.0` | 1.0 | First monorepo release, under the shared-version cascade (see [../development/release-process.md](../development/release-process.md)). Module path change, relicense to GPL-2.0-or-later, `smartmon_abi_version()` introduced, wrapper now built in-tree from `native/`. Supersedes the `bindings/go/v0.5.0` heading once carried in `bindings/go/CHANGELOG.md` — that version was never actually tagged (`git tag -l 'bindings/go/*'` was empty), so there is no `v0.5.0` release to migrate from. |

Each subsequent core release adds one row here, always with a `bindings/go`
release at the identical version — see the release cascade linked above.

## Footnote: the `v7.5` tag and release are mislabelled

A `v7.5` tag and GitHub release exist in this repository, but neither is part
of the version scheme above. The tag points at an unrelated repo-sync merge
commit, and its release's asset tarballs are named `libsmartmon-7.5-*` while
the binaries inside self-report `8.0-496-gc3204fe3d087` — a leftover from a
now-fixed bug where the old build workflow derived the version from the
`native/upstream` submodule's `git describe` instead of this repo's own
history. `native/src/getversion.sh` now hard-errors on any version below
`8.0` or with more than two components, so a `v7.5`-shaped tag can no longer
even be built from this tree. Deleting the mislabelled tag and release is
recommended but is a destructive action requiring explicit maintainer
approval — it is not performed automatically by any workflow in this repo.

## How to read this table

- **`native/` release** is the vendored smartmontools core's tag under the
  scheme in [../development/release-process.md](../development/release-process.md)
  (`v<AC_INIT>.<N>`) — independent of the Go module's own historical version
  numbers, but from `v8.0.0` onward identical to the `bindings/go` release
  next to it by construction.
- **C ABI** is `smartmon_abi_version()`'s `major.minor` — the number a
  `LibBackend` build actually checks at load time, per
  [../architecture/abi-contract.md](../architecture/abi-contract.md). This
  remains the number that matters for runtime compatibility; it can advance
  on its own schedule even though the release tags now move in lockstep — see
  [../architecture/dependency-rules.md](../architecture/dependency-rules.md#what-this-buys).
- `v0.4.1`'s ABI is listed as "undeclared" rather than "1.0" because
  `smartmon_abi_version()` did not exist yet — there was no runtime-checkable
  version at all, only the unsynchronized `SMARTMONTOOLS_GO_REF: v0.4.1`
  string pin described in
  [smartmontools-go-to-bindings-go.md](smartmontools-go-to-bindings-go.md).

This table grows one row per `native/` release, and correspondingly one
`bindings/go` release, plus a note whenever the C ABI requirement actually
changes — see
[../development/release-process.md](../development/release-process.md) for
when a release is expected to bump the ABI major/minor it requires.
