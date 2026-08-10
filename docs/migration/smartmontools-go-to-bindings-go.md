# Migrating from `smartmontools-go` to `bindings/go`

The standalone [`smartmontools-go`](https://github.com/dianlight/smartmontools-go)
repository has moved into this monorepo, at `bindings/go/`, as part of
[issue #17](https://github.com/dianlight/smartmontools-sdk/issues/17). This
page maps what moved, what changed, and what was deleted.

## Why

`smartmontools-go` and `smartmontools-sdk` had drifted into a circular
build-time dependency: the SDK fetched `smartmon_c_api.{cpp,h}` from the Go
repo at a pinned tag to build a wrapper library, while the Go repo documented
the SDK as the authoritative builder of that same wrapper, pinned to the same
tag. Each repository named the other as the source of truth for one artifact.
Merging the native core (`native/`), the C ABI boundary (`native/capi/`), and
the Go bindings (`bindings/go/`) into one repository with a one-way dependency
removes the cycle at its root. See
[architecture/dependency-rules.md](../architecture/dependency-rules.md) for
the full rationale.

## What moved, and where

| Old (`smartmontools-go`) | New (`smartmontools-sdk`) |
|---|---|
| Repository root | `bindings/go/` |
| `go.mod` module path `github.com/dianlight/smartmontools-go` | `github.com/dianlight/smartmontools-sdk/bindings/go` |
| Bare `vX.Y.Z` tags | `bindings/go/vX.Y.Z` tags |
| `LICENSE` (GPL-3.0-only) | Relicensed to GPL-2.0-or-later; see root [`COPYING`](../../COPYING) |
| `backends/lib/`'s `smartmon_c_api.{cpp,h}` fetched cross-repo from a pinned `smartmontools-go` tag | `native/capi/smartmon_c_api.{cpp,h}`, built in-tree |
| `backends/exec/drivedb.h` updated by its own `drivedb-fetch.yml` + Renovate datasource | Generated from `native/drivedb/drivedb.h` (the monorepo's single source of truth) by `scripts/sync-drivedb-go.sh` |
| `scripts/setup-lib-backend.sh` (referenced by a `//go:generate` directive but never committed) | `scripts/build-wrapper.sh` |
| `.github/workflows/ci.yml`, `release.yml` | `.github/workflows/ci-bindings-go.yml`, `release-bindings-go.yml` |

Everything else — `types/`, `examples/`, `backends/exec/`, `backends/shadow/`,
the `Client` orchestrator, ADRs under `docs/architecture/` — moved unchanged
in content, only relocated under `bindings/go/`.

## What was deleted

- The cross-repo `curl` fetch of `smartmon_c_api.{cpp,h}` from
  `raw.githubusercontent.com/dianlight/smartmontools-go` — this fetch *was*
  the circular dependency.
- `SMARTMONTOOLS_GO_REF` and `artifacts/lib/smartmontools-go-version`, the
  unsynchronized version-pin plumbing that motivated issue #17 in the first
  place. With both sides in one repository, the git SHA is the handshake.
- All "download a pre-built wrapper from a smartmontools-sdk release" prose
  in `README.md`, `doc.go`, `backends/lib/shared.go`, and the example
  `main.go` files — the wrapper is now built in-tree from `native/`, never
  downloaded.
- `drivedb-fetch.yml` and its Renovate custom datasource/regex manager —
  superseded by `native/drivedb/drivedb.h` plus the drift check in
  `ci-bindings-go.yml`.

## What you need to do

1. Update your `go.mod` and imports — see
   [import-path-migration.md](import-path-migration.md) for the exact old →
   new mapping and a `sed` recipe.
2. If you build the `libsmartmon_go` wrapper yourself rather than using a
   distro package, switch from downloading a `smartmontools-sdk` release
   tarball to building it in-tree: see
   [../development/build-core.md](../development/build-core.md) and
   `bindings/go/README.md`'s "LibBackend prerequisites" section.
3. Re-vendor your licence notice: this code is now GPL-2.0-or-later, not
   GPL-3.0-only.

No public Go API changed as part of this move — `Client`, `Backend`,
`ExecBackend`, `LibBackend`, and all `types.*` shapes are identical. The
`smartmon_abi_version()` check added to `LibBackend.New` (see
[../architecture/abi-contract.md](../architecture/abi-contract.md)) is the one
new runtime behaviour: a pre-monorepo wrapper build, which has no such symbol,
is now rejected with a clear error instead of silently binding a mismatched
signature.

See [compatibility-matrix.md](compatibility-matrix.md) for exact version
correspondences.
