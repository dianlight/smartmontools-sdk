# Building the native core

`native/` uses the smartmontools project's original autotools build system
unchanged — nothing in the monorepo restructure touched `configure.ac`'s
build logic beyond adding `native/capi` as a subdirectory.

## Prerequisites

```bash
sudo apt-get install -y autoconf automake libtool   # Debian/Ubuntu
```

macOS: `brew install autoconf automake libtool`.

## Build

```bash
cd native
./autogen.sh --force
mkdir -p build && cd build
../configure --with-devel=yes --with-pic
make -C include
make -j"$(nproc)" -C lib libsmartmon.la
```

This produces `native/build/lib/.libs/libsmartmon.a`.

## Why `--with-pic` is required

Without it, `configure.ac`'s `LT_INIT([disable_shared])` produces a static
archive built from **non-PIC** object code. That archive links fine into
another static binary, but cannot be folded into a shared library — linking
it into one fails with:

```
relocation R_X86_64_32 against '.rodata' can not be used when making a shared object
```

`native/capi/`'s wrapper (see [../architecture/abi-contract.md](../architecture/abi-contract.md))
*is* a shared library, built by `scripts/build-wrapper.sh` — so
`libsmartmon.a` must contain PIC objects, which is exactly what
`--with-pic` requests from libtool. This was the actual blocker behind the
pre-monorepo SDK PR #15's CI, which was red on every target that attempted
this link.

## Building the C ABI wrapper

Once `libsmartmon.a` exists:

```bash
cd ../..   # back to repo root
scripts/build-wrapper.sh
```

This compiles `native/capi/smartmon_c_api.cpp` and links it against
`native/build/lib/.libs/libsmartmon.a` by **explicit archive path**, not
`-L… -lsmartmon` — so the resulting shared library records no `DT_NEEDED` on
an unshipped library and stays `dlopen`-safe as a single self-contained file.
Output: `packaging/artifacts/lib/libsmartmon_go.{so,dylib}`.

### Cross-compiling

`scripts/build-wrapper.sh --target <target>` supports the full release
matrix: `linux-amd64`, `linux-aarch64`, `linux-amd64-musl`,
`linux-aarch64-musl` (via `zig c++`), `darwin-amd64`, `darwin-aarch64` (via
osxcross), and `darwin-native` (a real macOS host, auto-detected — see the
script's `detect_host_target()`). `windows-amd64` is accepted but skipped:
the Go `LibBackend` is `//go:build linux || darwin` only, so there is
nothing to build a wrapper for on Windows.

Run with no `--target` to build for the host you're running on.

## Verifying the build

```bash
nm -D --defined-only packaging/artifacts/lib/libsmartmon_go.so | grep -c '^smartmon_'
# expect 12
```

The full contract check — symbol presence *and* `smartmon_abi_version()`
compatibility — is what `integration/core-go/run.sh` automates; see
[local-development.md](local-development.md).
