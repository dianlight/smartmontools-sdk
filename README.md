## About Smartmontools SDK

This repository produces prebuilt static libraries (`libsmartmon.a`) from
[smartmontools/smartmontools](https://github.com/smartmontools/smartmontools)
for multiple platforms and architectures.

The upstream smartmontools source is included as a **git submodule** — not
forked. An SDK overlay (`include/`, `lib/`) wraps the library build targets
used by consumers such as
[smartmontools-go](https://github.com/dianlight/smartmontools-go).

### Supported Platforms

| OS | Architecture |
|----|-------------|
| Linux | amd64, aarch64 |
| macOS (Darwin) | amd64, aarch64 |
| Windows | amd64 |

### How It Works

1. **Submodule** — `smartmontools/` points to a specific upstream release tag.
2. **Build workflow** — triggered on PR merge or manual dispatch, compiles
   `libsmartmon.a` for all 5 platform/arch targets.
3. **Release** — artifacts are published as GitHub releases tagged with the
   upstream version (e.g. `v8.0`). Each release contains one `.tar.gz` per
   target with `lib/libsmartmon.a` and `include/smartmon/*.h`.
4. **Automated updates** — a daily workflow checks for new upstream release
   tags and opens a PR to update the submodule.

### Using a prebuilt release

Download the appropriate archive from the
[releases page](https://github.com/dianlight/smartmontools-sdk/releases),
then add the library and headers to your build:

```bash
tar -xzf libsmartmon-<version>-<target>.tar.gz -C /usr/local
# lib/libsmartmon.a → /usr/local/lib/libsmartmon.a
# include/smartmon/ → /usr/local/include/smartmon/
```

Link against it with `-lsmartmon` (and `-lstdc++` for C++ symbol resolution).

### Building locally

Requires: autoconf, automake, libtool, a C++11 compiler.

```bash
git clone --recurse-submodules https://github.com/dianlight/smartmontools-sdk.git
cd smartmontools-sdk
./autogen.sh
mkdir build && cd build
../configure --with-devel=yes
make -C include
make -j$(nproc) -C lib libsmartmon.la
```

The static library will be at `build/lib/.libs/libsmartmon.a` and the public
headers under `include/smartmon/`.

### Public headers

All public headers are installed under `include/smartmon/`:

| Header | Description |
|--------|-------------|
| `dev_interface.h` | Core device abstraction (open, identify, passthrough) |
| `atacmds.h` | ATA/SATA command set |
| `nvmecmds.h` | NVMe command set |
| `scsicmds.h` (via lib) | SCSI/SAS command set |
| `json.h` | JSON output builder |
| `utility.h` | Logging, string helpers |
| `smartmon_defs.h` | Common macros and type definitions |

---

## About Smartmontools

The smartmontools package implements the **Self-Monitoring, Analysis and
Reporting Technology** (SMART) protocol for ATA/SATA, SCSI/SAS and NVMe
storage devices. This SDK exposes the core library (`libsmartmon`) so that
other programs can query device health without spawning a subprocess.

## Links

- [Smartmontools homepage](https://www.smartmontools.org/)
- [Upstream repository](https://github.com/smartmontools/smartmontools)
- [Smartmontools releases](https://github.com/smartmontools/smartmontools/releases)
- [smartmontools-go](https://github.com/dianlight/smartmontools-go) — Go bindings

## License

Smartmontools uses [GNU GPL Version 2](https://www.gnu.org/licenses/gpl-2.0.html#SEC1) license.
