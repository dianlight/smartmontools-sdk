# Local development

One-command bootstrap and the end-to-end contract check, for working across
the `native/` → `native/capi/` → `bindings/go/` boundary locally.

## Prerequisites

```bash
sudo apt-get install -y autoconf automake libtool   # native/ build
```

Go and (for local musl cross-builds) Zig are managed by `mise` — see
`bindings/go/mise.toml`. `mise install` from `bindings/go/` picks up the
pinned Go version.

## Full local build

```bash
# 1. Native core
cd native && ./autogen.sh --force && mkdir -p build && cd build
../configure --with-devel=yes --with-pic
make -C include && make -j"$(nproc)" -C lib libsmartmon.la
cd ../..

# 2. C ABI wrapper
scripts/build-wrapper.sh

# 3. Go bindings
cd bindings/go && mise run ci
```

See [build-core.md](build-core.md) for what each native-core step does.

## Running the end-to-end contract test

`integration/core-go/run.sh` is the test that neither the old
`smartmontools-sdk` nor `smartmontools-go` repository could run standalone —
it exercises the full boundary in one script:

```bash
integration/core-go/run.sh
```

In order, it: builds `native/` with `--with-pic`; builds the wrapper via
`scripts/build-wrapper.sh`; asserts all 12 expected `smartmon_*` symbols are
exported (`nm -D`/`nm -gU`); compiles `integration/core-go/abi_check.c` and
runs it against the wrapper to confirm `smartmon_abi_version()` satisfies the
major/minor `bindings/go/backends/lib/lib.go` requires (read directly out of
the source via `grep -oP`, so the check can never silently drift from the
real requirement); runs `go test ./backends/lib/...` against the freshly
built wrapper with `SMARTMON_INTEGRATION=1`; and finally re-runs
`scripts/sync-drivedb-go.sh` and asserts no drift in
`bindings/go/backends/exec/drivedb.h`.

Optionally set `SMARTMON_SDK_DEVICE=/dev/sdX` to also exercise
device-dependent assertions (`GetSMARTInfo`/`CheckHealth`) against a real
device; without it, those assertions are skipped and the script logs that
explicitly rather than silently.

## Working on just the Go bindings

If you aren't changing `native/` or `native/capi/`, you don't need to build
either — `ExecBackend`-only work just needs `smartctl` installed, and
`mise run ci` / `mise run test` from `bindings/go/` is enough. Only
`LibBackend` work, or an ABI-affecting change, requires the full sequence
above.

## Common pitfalls

- **Skip `--with-pic`ing on `configure`** and the wrapper link fails with
  `relocation R_X86_64_32 against '.rodata'` — see
  [build-core.md](build-core.md#why---with-pic-is-required).
- **`mise` is scoped per directory** — running `mise run ci` from the repo
  root rather than `bindings/go/` will not find the Go-specific tasks;
  `bindings/go/mise.toml` only applies inside that directory.
- **`git diff --exit-code` is blind to untracked files.** If
  `scripts/sync-drivedb-go.sh` ever needs to *create* a new file rather than
  update an existing one, a drift check built purely on `git diff` would
  pass even though something changed — worth remembering if the sync
  script's output files are ever restructured.
