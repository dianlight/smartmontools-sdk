# Release process

`native/` and `bindings/go/` release on separate tag namespaces and separate
GitHub Actions workflows, but they are **version- and cadence-coupled**: a
core release always produces a paired bindings release at the identical
version, even with zero Go source changes. The independence that survives
from the original monorepo restructure (see
[../architecture/dependency-rules.md](../architecture/dependency-rules.md))
is architectural, not release-cadence — `bindings/go/` never needs a code
change to react to a `native/capi/` change within the same ABI major/minor,
but it does always get a new tag when `native/` gets one.

## Continuous integration

| Workflow | Triggers on | What it does |
|---|---|---|
| `ci-core.yml` | PRs touching `native/**`, `.gitmodules`, `scripts/build-wrapper.sh` | Builds `libsmartmon.a` and the C ABI wrapper across all 7 release-matrix targets; uploads artifacts. No release step. |
| `ci-bindings-go.yml` | PRs touching `bindings/go/**`, `native/capi/**`, `native/drivedb/**`, `scripts/{build-wrapper,sync-drivedb-go}.sh` | Three jobs: `lint-and-unit` (`mise run ci` + coverage), `drivedb-drift` (re-runs `scripts/sync-drivedb-go.sh`, fails on any diff — this is what makes `native/drivedb/drivedb.h`'s "single source of truth" claim enforced rather than aspirational), `integration` (builds native + wrapper for `linux-amd64`, then runs `go test ./backends/lib/...` against it with `SMARTMON_INTEGRATION=1`). |

Both workflows need `fetch-depth: 0` and `SMARTMONTOOLS_TEST_BUILD=1`, because
`native/src/getversion.sh` derives the version string by counting commits
from a hardcoded `base_git_rev` in *this repo's own history* (see
[repository-layout.md](../architecture/repository-layout.md#vendoring-strategy-this-repo-is-the-fork))
— it never runs `git describe` against anything, and it hard-errors without
enough git history to count from.

## Core version numbering

The core version scheme is `v<AC_INIT>.<N>[-pre.<revs>]`, where `<AC_INIT>` is
the smartmontools package version from `native/configure.ac` (scraped by
`scripts/next-core-version.sh` with the exact same expression
`native/src/getversion.sh` uses, so the two can never disagree), `<N>` is a
per-generation counter that resets whenever `AC_INIT` changes, and the
optional `-pre.<revs>` suffix is a SemVer prerelease identifier carrying the
same commit count `getversion.sh` embeds in its own build string (`revs` is
`git rev-list --count` from a fixed `base_git_rev`, computed once by
`scripts/prerelease-state.sh` and shared by both version scripts so they can
never disagree on it either). This makes the tag fully computable from the
tree and, via `scripts/semver-sort.sh`, correctly orderable — plain `sort -V`
and `git tag --sort=-v:refname` both rank a `-pre.N` suffix *higher* than the
release it precedes, the opposite of SemVer §11.3, so neither is used for
this scheme.

**The suffix is present exactly when `native/configure.ac`'s
`smartmontools_release_date` is commented out** — i.e. exactly when
`native/src/getversion.sh` itself considers the tree a pre-release. There is
no separate flag to keep in sync: `scripts/prerelease-state.sh
--is-prerelease` reads the same field. The suffix disappears — the tree
"graduates" to a final release — the first time upstream sets that field for
the `8.0` generation; no repo-side action is needed beyond re-vendoring that
commit.

`native/src/getversion.sh` hard-errors on any version below `8.0` or with
more than two components, so this scheme's floor is `v8.0.0`. The historical
tag `v7.5` predates the scheme entirely and is mislabelled — see
[compatibility-matrix.md](../migration/compatibility-matrix.md)'s footnote.

### The `v8.0.0` / `v8.0.1` tombstone

`v8.0.0` and `bindings/go/v8.0.0` were published as **final** releases on
2026-08-11 while upstream smartmontools 8.0 was (and still is) unreleased —
the release tooling at the time didn't yet check
`smartmontools_release_date` before minting a version. Both are permanently
part of the Go module checksum database and the GitHub release history;
neither can be recalled, only worked around. `v8.0.1` /
`bindings/go/v8.0.1` are stable (non-prerelease) tags containing no code
changes beyond a `retract` block in `bindings/go/go.mod` naming both `v8.0.0`
and themselves. This is not a mistake to clean up — do not delete either tag.

The reason a *stable* tombstone is required, not just a `retract` entry
somewhere: Go's `@latest` query prefers the newest release over any
prerelease, even one that outranks it in raw SemVer precedence, and it only
consults a version's `retract` directives once that version is already the
`@latest` candidate. A `retract` block published only inside a prerelease's
`go.mod` is therefore never read by a plain `go get`. Publishing `v8.0.1` as
a stable release makes it (temporarily) the `@latest` candidate, whose
`go.mod` retractions then exclude both it and `v8.0.0` from consideration —
at which point `@latest` falls through to the newest `-pre.N` release, which
is what should have been published all along:

```
go get .../v8@latest
  -> resolves v8.0.1 (highest stable release)
  -> reads its go.mod, sees v8.0.0 and v8.0.1 both retracted
  -> excludes both, re-resolves among what remains
  -> lands on the newest v8.0.2-pre.N        <- the intended outcome
```

This has a real limit: anyone who already pinned `@v8.0.0` explicitly before
this fix sees nothing different — retraction only changes what an *upgrade*
resolves to (`go list -m -u` will now show a warning), it cannot rewrite or
break an existing lockfile. That is the ceiling of what is achievable once a
version has been published; it is why the tombstone matters far more than
deleting the tag, which only tidies the GitHub UI and has no effect on
`go get` at all (`proxy.golang.org` mirrors a version independently of
whether its origin tag still exists).

### Consuming a prerelease

While the core carries a `-pre.<revs>` suffix, a plain `go get
github.com/dianlight/smartmontools-sdk/bindings/go/v8` (no explicit
`@version`) does **not** select it — Go's default `@latest` query still
prefers a release over a prerelease in general, it only falls through past
`v8.0.0`/`v8.0.1` because those specific versions retract themselves (see
above). Pin the exact version to get the prerelease intentionally:

```
go get github.com/dianlight/smartmontools-sdk/bindings/go/v8@v8.0.2-pre.504
```

The native core's release page and the bindings' release notes both state
this explicitly for every prerelease build (see `release-core.yml` and
`release-bindings-go.yml`'s "Create or update release" steps).

## The release cascade

```
upstream smartmontools releases RELEASE_8_1
  │
  ├─ update-submodule.yml (check-update, daily 06:00 UTC)
  │    git merge RELEASE_8_1  →  PR with the real source diff
  │    (conflicts → draft PR listing conflicting paths; human resolves)
  │      merge to main
  │        └─ tag-core-release.yml (push: main)
  │             gate: did the artifact actually change? (scripts/core-artifact-changed.sh)
  │             PASS → compute next tag (scripts/next-core-version.sh) → v8.1.0
  │             push tag; gh workflow run release-core.yml -f version=8.1.0
  │               └─ release-core.yml: prepare → build (7 targets) → release
  │                    └─ dispatch-bindings → gh workflow run release-bindings-go.yml
  │                         └─ tag bindings/go/v8.1.0 → release, notes cross-ref core
  │
  ├─ update-submodule.yml (check-drivedb)
  │    drivedb.h differs → PR (both copies, via scripts/sync-drivedb-go.sh)
  │      merge → gate PASS (drivedb.h participates in the build) → v8.1.1 → bindings/go/v8.1.1
  │
  └─ safety net: a merge touching only .gitmodules/native/upstream
       → gate FAIL → no tag, no release, tracking issue opened instead
```

No state is persisted anywhere between runs: when `AC_INIT` moves to `8.1`,
`next-core-version.sh` sees no `v8.1.*` tags yet and emits `v8.1.0` on its
own.

### Why dispatch, not tag-push

Tags and branches pushed with `secrets.GITHUB_TOKEN` do **not** fire
`on: push:` workflows — GitHub suppresses events authored by its own Actions
token to prevent runaway recursion. This is why `tag-core-release.yml` pushes
the tag and then explicitly runs `gh workflow run release-core.yml`, and why
`release-core.yml`'s final job does the same for `release-bindings-go.yml`,
instead of relying on `release-bindings-go.yml`'s `on: push: tags:` trigger.
**No PAT or GitHub App is required** — `GITHUB_TOKEN` can call the
workflow-dispatch API given `permissions: actions: write`, which every
workflow in this cascade declares. The one hard constraint this imposes:
`gh workflow run` can only dispatch a workflow file that exists on the
**default branch**, so a workflow file added in a PR can't be exercised via
dispatch until that PR merges.

### The artifact-changed gate

`scripts/core-artifact-changed.sh` is `tag-core-release.yml`'s safety net: if
a merge to `main` doesn't touch any path that participates in the
`libsmartmon.a` build (source, headers, the C ABI wrapper, `drivedb.h`, the
wrapper build script), no tag and no release are created — instead a
tracking issue is opened (or updated, if one is already open) explaining
why, with a `force: true` escape hatch via manual dispatch. This makes a
no-op release structurally impossible even if a merge somehow only bumps the
`native/upstream` gitlink without a real source change. The allowlist is
deliberately *stricter* than `ci-core.yml`'s `paths:` trigger filter — see the
script's header comment for why `.gitmodules`/`native/upstream` are excluded
there but not here.

### Re-vendoring upstream

`update-submodule.yml`'s `check-update` job fetches
`smartmontools/smartmontools.git`'s tags, finds the newest `RELEASE_*` tag not
already an ancestor of `main`, and attempts `git merge` on a fresh
`update-smartmontools-<version>` branch:

- **Clean merge** → pushed, and a normal PR is opened carrying the real
  source diff (including any `AC_INIT` bump, which is precisely what makes
  the tag scheme above mint a new generation once merged).
- **Conflicts** → pushed as-is with conflict markers committed, and a
  **draft** PR is opened listing the conflicting paths. Nothing is
  auto-resolved. This is the expected common case: this repo has
  deliberately deleted most of the smartctl/smartd CLI sources under
  `native/src/` that upstream still carries, so most upstream releases
  conflict on exactly those paths. A human resolves the branch and merges
  once clean.

Either way, the `native/upstream` submodule gitlink is bumped to the merged
tag in the same commit — kept for audit continuity only, since nothing in the
build reads it (see
[repository-layout.md](../architecture/repository-layout.md#vendoring-strategy-this-repo-is-the-fork)).
Once the resulting PR merges to `main`, the cascade above takes over
automatically.

## Releasing `native/`

`tag-core-release.yml` pushes `vX.Y.Z` and dispatches this workflow (or
trigger it manually) →

1. `prepare` job derives `VERSION` from the `workflow_dispatch` input, the
   pushed tag, or (for a dispatch with neither) the newest tag for the
   *current* `AC_INIT` generation only — this is what prevents a bare manual
   dispatch from accidentally republishing a stale generation's tag.
2. `build` job runs the same 7-target matrix as `ci-core.yml`.
3. `release` job packages each target's artifacts (the static archive,
   headers, and the wrapper) into
   `libsmartmon-${VERSION}-${target}.tar.gz`, then creates/updates the GitHub
   release, marked `--latest`, with notes carrying the upstream `AC_INIT`,
   the `getversion.sh` string, the C ABI version, and the paired bindings tag.
4. `dispatch-bindings` job runs only if this job's own `dry_run` input was
   false, and dispatches `release-bindings-go.yml` with `core_version` set —
   this is expectation 2 (bindings release on every core release).

All side-effecting steps are gated behind `dry_run` (default `true` on manual
dispatch); a dry run prints what would happen and creates nothing.

## Releasing `bindings/go`

Tag `bindings/go/vX.Y.Z`, a dispatch from `release-core.yml`'s
`dispatch-bindings` job, or a direct manual trigger →

1. Derives `TAG`/`VERSION` in priority order: explicit `version` input →
   `GITHUB_REF_NAME` (tag push) → computed from `core_version` (the cascade
   path — mirrors the core version literally, per the `/v8` module path
   below) → a standalone `patch`/`minor` bump → the newest existing tag.
2. Creates the tag if it doesn't exist yet (the cascade path always needs
   this, since nothing tagged it before dispatch); idempotent on a real tag
   push.
3. Warms the Go module proxy:
   `GOPROXY=proxy.golang.org go list -m github.com/dianlight/smartmontools-sdk/bindings/go/v8@${TAG}`
   — non-fatal; the module is still fetchable on first consumer request even
   if this step fails.
4. Creates/updates the GitHub release, explicitly with `--latest=false` —
   deliberately not the repository's "latest" release, since `bindings/go`'s
   tags live in a separate namespace from `native/`'s and neither should
   shadow the other in the GitHub UI. Release notes cross-reference the
   paired core release when `core_version` was set.

No binary artifacts are attached; Go modules distribute via the module
proxy, not release tarballs.

### The `/v8` module path

`bindings/go/go.mod`'s module path is
`github.com/dianlight/smartmontools-sdk/bindings/go/v8` — the `/v8` suffix
exists to satisfy Go's semantic import versioning rule (any major version
≥ 2 requires a `/vN` path suffix), and the literal `8` is chosen to match the
native core's current `AC_INIT` major so that "bindings version = core
version" (the design goal) is satisfiable at all. This was free to adopt
because `bindings/go` had never been released under any tag before this
scheme (`git tag -l 'bindings/go/*'` was empty) — there was no existing
consumer import path to break.

**This is a recurring cost, not a one-time one.** The next time upstream
smartmontools bumps its major version (`8.0 → 9.0`), this module path must
move to `/v9`, forcing every consumer to update their import path again —
`scripts/next-bindings-go-version.sh` hard-fails if the computed major ever
disagrees with the `/vN` suffix actually in `go.mod`, specifically so this
can never be published silently mismatched.

### `mise run release` (local tagging helper)

`bindings/go/mise.toml`'s `release` task is for **standalone** bindings-only
releases (a Go-only fix that doesn't warrant waiting for a core release) — it
delegates its version arithmetic to `scripts/next-bindings-go-version.sh` so
the local helper and CI never disagree:

- On `main`: a stable release (`bindings/go/vX.Y.Z`, patch or minor bump only
  — see below).
- On a branch with an open PR (checked via `gh pr list`): a prerelease,
  suffixed `-beta.N` (bumped from the current beta count on that branch).
- On a branch with no open PR: hard error — prereleases only make sense
  attached to a reviewable PR.

**This entire task hard-errors up front while the core itself is a
`-pre.<revs>` prerelease** (see the tombstone section above), before either
branch above is even evaluated — checked via
`scripts/prerelease-state.sh --is-prerelease`. Two independent reasons this
is a hard block rather than something to route around: there is no correct
version for a standalone bindings bump to target on `main` while the
mainline series is itself a moving target (`next-bindings-go-version.sh
--bump` refuses for the same reason); and on a branch, the `-beta.N`
computation above reads the *nearest reachable* tag via `git describe
--abbrev=0`, not the true newest by SemVer precedence, so it would happily
mint `-pre.<revs+1>` from an unreviewed branch and hijack the mainline
prerelease channel. Nesting the branch channel under the mainline one (e.g.
`-pre.<revs>.beta.N`) does not fix this — SemVer §11.4.4 ranks *more*
identifier fields higher when preceding fields are equal, so that scheme
would make PR builds outrank mainline, the opposite of the intended fix.
Standalone bindings releases resume once the core graduates to a final
release.

Major bumps are **not** offered here: they change the module's `/vN` import
path and require the manual steps in the "`/v8` module path" section above,
not a routine tagging command. Supports `--dry-run` to preview the computed
tag without pushing.

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
without the other. Because `drivedb.h` participates in the native build
artifact, merging this PR passes the artifact-changed gate above and
triggers a real core (and paired bindings) release.

## Secrets and permissions

No secret beyond the built-in `secrets.GITHUB_TOKEN` is required anywhere in
this cascade. Each workflow declares the minimum `permissions:` it needs at
the job level (job-level `permissions:` replaces, not merges with, the
workflow-level block, so every job that needs `actions: write` to dispatch
another workflow declares it explicitly rather than inheriting it).

## Testing the cascade

Every side-effecting step in `tag-core-release.yml`, `release-core.yml`, and
`release-bindings-go.yml` is gated behind a `dry_run` input (default `true`
on manual dispatch) — a dry run prints the computed version and what would
happen, and creates or pushes nothing, not even a tracking issue. Exercise
the full chain with `dry_run: true` at each stage before ever running one for
real; see the workflows' own `workflow_dispatch.inputs` descriptions for the
exact fields.
