# Release process

`native/` and `bindings/go/` release on separate tag namespaces, separate
schedules, and separate GitHub Actions workflows — this independence is the
point of the monorepo restructure (see
[../architecture/dependency-rules.md](../architecture/dependency-rules.md)):
neither tier's release process depends on the other's.

## Continuous integration

| Workflow | Triggers on | What it does |
|---|---|---|
| `ci-core.yml` | PRs touching `native/**`, `.gitmodules`, `scripts/build-wrapper.sh` | Builds `libsmartmon.a` and the C ABI wrapper across all 7 release-matrix targets; uploads artifacts. No release step. |
| `ci-bindings-go.yml` | PRs touching `bindings/go/**`, `native/capi/**`, `native/drivedb/**`, `scripts/{build-wrapper,sync-drivedb-go}.sh` | Three jobs: `lint-and-unit` (`mise run ci` + coverage), `drivedb-drift` (re-runs `scripts/sync-drivedb-go.sh`, fails on any diff — this is what makes `native/drivedb/drivedb.h`'s "single source of truth" claim enforced rather than aspirational), `integration` (builds native + wrapper for `linux-amd64`, then runs `go test ./backends/lib/...` against it with `SMARTMON_INTEGRATION=1`). |

Both also gate on `native/upstream`'s submodule still resolving via
`git describe` for version detection.

## Releasing `native/`

Tag `vX.Y.Z` (or trigger `release-core.yml` manually) →

1. `prepare` job derives `VERSION` from the tag (or the latest `v*` tag, for
   manual dispatch).
2. `build` job runs the same 7-target matrix as `ci-core.yml`.
3. `release` job packages each target's artifacts (the static archive,
   headers, and the wrapper) into
   `libsmartmon-${VERSION}-${target}.tar.gz`, then creates/updates the GitHub
   release, marked `--latest`.

## Releasing `bindings/go`

Tag `bindings/go/vX.Y.Z` (or trigger `release-bindings-go.yml` manually) →

1. Derives `TAG`/`VERSION` from `GITHUB_REF_NAME`, or from the latest
   `bindings/go/v*` tag for manual dispatch.
2. Warms the Go module proxy:
   `GOPROXY=proxy.golang.org go list -m github.com/dianlight/smartmontools-sdk/bindings/go@${TAG}`.
3. Creates/updates the GitHub release, explicitly with `--latest=false` —
   deliberately not the repository's "latest" release, since `bindings/go`'s
   tags live in a separate namespace from `native/`'s and neither should
   shadow the other in the GitHub UI.

No binary artifacts are attached; Go modules distribute via the module
proxy, not release tarballs.

### `mise run release` (local tagging helper)

`bindings/go/mise.toml`'s `release` task automates picking the next tag:

- On `main`: a stable release (`bindings/go/vX.Y.Z`).
- On a branch with an open PR (checked via `gh pr list`): a prerelease,
  suffixed `-beta.N` (bumped from the current beta count on that branch).
- On a branch with no open PR: hard error — prereleases only make sense
  attached to a reviewable PR.

Supports `--dry-run` to preview the computed tag without pushing.

## When to bump the ABI version

A change to `native/capi/smartmon_c_api.h` requires a
`smartmon_abi_version()` bump per
[../architecture/abi-contract.md](../architecture/abi-contract.md)'s
three-tier rule:

- Adding a new symbol, with no existing symbol touched → bump **minor**.
- Removing, renaming, or changing the signature of any existing symbol →
  bump **major**.
- Anything else (internal behaviour change visible only through existing
  symbols' documented semantics) → bump **patch**, or nothing.

A `native/capi/` change that bumps major should be called out explicitly in
`bindings/go/CHANGELOG.md`'s next release entry, since it is a breaking
change for that binding even though the Go source code may not need to
change at all.

## Updating the drive database

`native/drivedb/drivedb.h` is refreshed automatically by
`update-submodule.yml`'s `check-drivedb` job: it reads the `VERSION:` comment
at the top of the local file to determine which upstream branch
(`drivedb/${VERSION}`) to compare against, and if the upstream copy differs,
opens a PR that updates `native/drivedb/drivedb.h` **and** regenerates
`bindings/go/backends/exec/{drivedb.h,drivedb_version.go}` via
`scripts/sync-drivedb-go.sh` in the same commit. No manual step is needed;
`ci-bindings-go.yml`'s drift check exists to catch a PR that updated one copy
without the other.
