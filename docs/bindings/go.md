# Go bindings

The Go bindings live at [`bindings/go/`](../../bindings/go/), tagged
independently as `bindings/go/vX.Y.Z`. Full API reference, usage examples,
and feature list are in [`bindings/go/README.md`](../../bindings/go/README.md)
and [`bindings/go/APIDOC.md`](../../bindings/go/APIDOC.md) — this page covers
only what's specific to how the binding fits into the monorepo.

## Install

```bash
go get github.com/dianlight/smartmontools-sdk/bindings/go/v8
```

The module path includes `bindings/go` because Go's module system resolves a
subdirectory module against the enclosing repository's tags prefixed with
that same path (`bindings/go/vX.Y.Z`) — this is how `native/`'s separate
`vX.Y.Z` release tags coexist with the Go module's own versioning in one
repository without colliding.

## Backend selection

Two independent implementations satisfy the same `Backend` interface:

| Backend | How it talks to the device | Requires |
|---|---|---|
| `ExecBackend` | Shells out to `smartctl -j ...` and parses JSON | `smartctl` on `PATH` (smartmontools installed) |
| `LibBackend` | Loads `native/capi/`'s wrapper via `purego`/`dlopen`, no CGO | The wrapper built in-tree — see below |

`ExecBackend` is the default and needs nothing beyond a system `smartctl`
install. `LibBackend` avoids process-spawn overhead and works without
`smartctl` installed, at the cost of building the wrapper yourself; a
`CompareBackend` runs both and diffs their output, useful when validating
that a `LibBackend` build matches `ExecBackend`'s established behaviour.

## Building the wrapper for `LibBackend`

There is no pre-built or downloadable wrapper artifact — it is always built
in-tree from `native/`, from the repository root:

```bash
cd native && ./autogen.sh --force && mkdir -p build && cd build
../configure --with-devel=yes --with-pic
make -C include && make -C lib libsmartmon.la
cd ../..
scripts/build-wrapper.sh
```

This produces `packaging/artifacts/lib/libsmartmon_go.{so,dylib}`. See
[../development/build-core.md](../development/build-core.md) for what each
step does and why `--with-pic` is required (without it, `libsmartmon.a`
contains non-PIC objects that cannot be linked into a shared library).

## Wrapper resolution order

`libbackend.New` looks for the wrapper in this order:

1. `libbackend.WithLibraryPath(path)` option, if given.
2. `SMARTMON_LIB_PATH` environment variable, if set and the file exists at
   that path. If set but the file is missing, a warning is logged and
   resolution falls through to step 3. If a *different* system-path copy
   also exists, a warning is logged (possible stale install) but the
   env-specified copy still wins.
3. Standard system search: the dynamic linker's default search path for
   `libsmartmon_go.so`/`.dylib`, then a fixed list of absolute paths
   (`/usr/local/lib`, `/usr/lib`, per-arch multiarch dirs on Linux,
   Homebrew's `/opt/homebrew/lib` on macOS).

Whichever library is loaded, `LibBackend.New` immediately calls
`smartmon_abi_version()` and rejects an incompatible wrapper before doing
anything else — see [../architecture/abi-contract.md](../architecture/abi-contract.md).
A wrapper built before the monorepo merge has no such symbol at all, so it
is rejected with a clear "rebuild with `scripts/build-wrapper.sh`" error
rather than silently producing wrong results.

## Coming from `smartmontools-go`

See [../migration/smartmontools-go-to-bindings-go.md](../migration/smartmontools-go-to-bindings-go.md)
for the full migration guide, and
[../migration/import-path-migration.md](../migration/import-path-migration.md)
for a copy-pasteable import-rewrite recipe.
