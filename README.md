# smartmontools-sdk

A monorepo SDK for [smartmontools](https://www.smartmontools.org/)'s
**Self-Monitoring, Analysis and Reporting Technology** (SMART) protocol
support — a vendored C++ core, a stable C ABI, and language bindings built
on top of it, all in one repository with a strict one-way dependency:

```
bindings/go, bindings/python, bindings/rust   (language bindings)
                    │
                    ▼
              native/capi/                     (stable C ABI)
                    │
                    ▼
                 native/                        (C++ core, vendored from upstream)
```

See [docs/architecture/overview.md](docs/architecture/overview.md) for the
full picture, and
[docs/architecture/dependency-rules.md](docs/architecture/dependency-rules.md)
for why the dependency only ever points one way.

## Repository layout

| Path | What it is |
|---|---|
| [`native/`](native/) | The vendored C++ core, built with autotools into `libsmartmon.a` |
| [`native/capi/`](native/capi/) | The C ABI boundary (`smartmon_c_api.{cpp,h}`) that every binding calls |
| [`bindings/go/`](bindings/go/) | Go bindings — implemented, tagged `bindings/go/vX.Y.Z` |
| [`bindings/python/`](bindings/python/), [`bindings/rust/`](bindings/rust/) | Planned — see [docs/roadmap.md](docs/roadmap.md) |
| [`scripts/`](scripts/) | Cross-cutting build and sync tooling (`build-wrapper.sh`, `sync-drivedb-go.sh`) |
| [`integration/`](integration/) | End-to-end contract tests spanning `native/` and a binding |
| [`packaging/`](packaging/) | Staged release artifacts |
| [`docs/`](docs/) | The documentation you're browsing now |

Full directory-by-directory ownership: [docs/architecture/repository-layout.md](docs/architecture/repository-layout.md).

## Using a prebuilt release

Native-core release tarballs are published per target on the
[releases page](https://github.com/dianlight/smartmontools-sdk/releases),
tagged `vX.Y.Z`:

```bash
tar -xzf libsmartmon-<version>-<target>.tar.gz -C /usr/local
# lib/libsmartmon.a       → /usr/local/lib/libsmartmon.a
# include/smartmon/       → /usr/local/include/smartmon/
# lib/libsmartmon_go.*     → the C ABI wrapper shared library
```

Link the static library with `-lsmartmon -lstdc++`. The Go bindings are
distributed separately via the Go module proxy — see
[bindings/go/README.md](bindings/go/README.md).

## Building locally

```bash
sudo apt-get install -y autoconf automake libtool   # Debian/Ubuntu

cd native
./autogen.sh --force
mkdir -p build && cd build
../configure --with-devel=yes --with-pic
make -C include
make -j"$(nproc)" -C lib libsmartmon.la
```

`--with-pic` matters: without it, `libsmartmon.a` contains non-PIC objects
that cannot be linked into `native/capi/`'s shared-library wrapper. See
[docs/development/build-core.md](docs/development/build-core.md) for the
full explanation, cross-compilation targets, and how to build the wrapper
itself. For the complete local dev loop, including the end-to-end
`native/` ↔ Go contract test, see
[docs/development/local-development.md](docs/development/local-development.md).

## Public headers

Installed under `include/smartmon/` by the native build:

| Header | Description |
|---|---|
| `dev_interface.h` | Core device abstraction (open, identify, passthrough) |
| `atacmds.h` | ATA/SATA command set |
| `nvmecmds.h` | NVMe command set |
| `scsicmds.h` | SCSI/SAS command set |
| `json.h` | JSON output builder |
| `utility.h` | Logging, string helpers |
| `smartmon_defs.h` | Common macros and type definitions |

The C ABI wrapper's own header, `native/capi/smartmon_c_api.h`, is the
12-symbol surface any language binding depends on — see
[docs/architecture/abi-contract.md](docs/architecture/abi-contract.md).

## Migrating from the old two-repository layout

If you previously depended on the standalone `smartmontools-go` repository,
see [docs/migration/smartmontools-go-to-bindings-go.md](docs/migration/smartmontools-go-to-bindings-go.md)
for what moved and [docs/migration/import-path-migration.md](docs/migration/import-path-migration.md)
for a copy-pasteable import-rewrite recipe.

## About smartmontools

The smartmontools package implements the SMART protocol for ATA/SATA,
SCSI/SAS and NVMe storage devices. This SDK vendors its core library
(`libsmartmon`) so other programs — from any language, via `native/capi/` —
can query device health without spawning a subprocess, while still
supporting the exec-a-subprocess approach for consumers that prefer it (see
[bindings/go/README.md](bindings/go/README.md)'s `ExecBackend`).

## Links

- [Smartmontools homepage](https://www.smartmontools.org/)
- [Upstream repository](https://github.com/smartmontools/smartmontools)
- [Smartmontools releases](https://github.com/smartmontools/smartmontools/releases)

## License

GPL-2.0-or-later. See [COPYING](COPYING).
