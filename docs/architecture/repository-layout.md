# Repository layout

```
native/                  C++ core (autotools build); this repo's own history
                           is a real continuation of upstream smartmontools'
                           history via git merge — see "vendoring strategy"
                           below
  upstream/               git submodule → smartmontools/smartmontools.git;
                           vestigial audit trail only, nothing compiles from
                           or reads version info out of this checkout
  include/, lib/, src/    the vendored source tree — merged in from upstream
                           release tags, not copy-pasted
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

## Vendoring strategy: this repo *is* the fork

`native/lib`, `native/include`, and `native/src` are **not** a copy of
smartmontools sources sitting alongside a separate upstream checkout — this
repository's own git history *is* a continuation of upstream smartmontools'
history. `git log` shows a real two-parent merge of `tags/RELEASE_7_5` (and
7.4, 7.3, 7.2, 7.1 before it), and hundreds of commits authored by upstream
maintainer Christian Franke land in `native/lib|include|src` after that
merge — including the commit that bumped `AC_INIT` to the `8.0` this repo
currently builds. The `lib/`/`include/`/`src/`/`drivedb/` layout under
`native/` is itself upstream's own post-7.5 directory restructure, not
something invented here.

**`native/upstream` is vestigial.** Nothing in the build reads it: no
`configure`/`autogen.sh` step runs `git describe` against it, and version
strings are derived entirely from this repo's own history (see
`native/src/getversion.sh`'s hardcoded `base_git_rev`, which is the commit
SHA of `RELEASE_7_5` *in this repo's ancestry*). The submodule pin only
matters as an audit trail of which upstream tag was last merged, updated by
`update-submodule.yml`. Removing it entirely is tracked as follow-up work
(see [../roadmap.md](../roadmap.md)).

**Upgrading the vendored core is a real `git merge`**, not a manual
re-copy-and-diff exercise — `update-submodule.yml`'s `check-update` job
merges the newest untracked upstream `RELEASE_*` tag's tree into the index
with `git merge`, then records the result as a **single native commit** (via
`git write-tree` + `git commit-tree` with `main` as the only parent). It
deliberately does *not* commit a two-parent merge: the upstream tag commit
never enters this repo's history, and GitHub refuses `gh pr create` when a
head commit is attributed to a repository the `GITHUB_TOKEN` integration has
no access to ("Resource not accessible by integration
(createPullRequest)"). The resulting PR carries the same tree — and therefore
the same diff — a two-parent merge would have produced. Because this repo has
deliberately deleted the smartctl/smartd CLI sources upstream still carries
under `src/` (and restructured directories upstream itself later also
restructured), a merge of a new upstream release frequently conflicts on
exactly those paths; when it does, the workflow opens a draft PR with the
conflict list instead of guessing at a resolution. See
[release-process.md](../development/release-process.md#re-vendoring-upstream)
for the merge→tag→release cascade this triggers.

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
