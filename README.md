## About Smartmontools SDK

This repository produces prebuilt static libraries (`libsmartmon.a`) from
[smartmontools/smartmontools](https://github.com/smartmontools/smartmontools)
for multiple platforms and architectures.

The upstream smartmontools source is included as a **git submodule** — not
forked. An SDK overlay (`include/`, `lib/`, `src/`) adds the library build
targets and the `libsmartctl` C API wrapper used by
[smartmontools-go](https://github.com/dianlight/smartmontools-go).

### Supported Platforms

| OS | Architecture |
|----|-------------|
| Linux | amd64, aarch64 |
| macOS (Darwin) | amd64, aarch64 |
| Windows | amd64, aarch64 |

### How It Works

1. **Submodule** — `smartmontools/` points to a specific upstream release tag.
2. **Build workflow** — triggered on PR merge or manual dispatch, compiles
   `libsmartmon.a` for all 6 platform/arch targets.
3. **Release** — artifacts are published as GitHub releases tagged with the
   upstream version (e.g. `v8.0`).
4. **Automated updates** — a daily workflow checks for new upstream release
   tags and opens a PR to update the submodule.

### Building locally

```bash
git clone --recurse-submodules https://github.com/dianlight/smartmontools-sdk.git
cd smartmontools-sdk
./autogen.sh
mkdir build && cd build
../configure --with-devel=yes
make -C include
make -j$(nproc) -C lib
```

The static library will be at `build/lib/.libs/libsmartmon.a`.

### Building the shared library (libsmartctl)

```bash
./autogen.sh
./configure --enable-shared --disable-static --enable-libsmartctl \
  CFLAGS="-fPIC" CXXFLAGS="-fPIC -DBUILDING_LIBSMARTCTL"
make -j$(nproc)
```

### C API

See `src/libsmartctl.h` for the full API. Key functions:

| Function | Description |
|----------|-------------|
| `smartctl_init()` | Create execution context |
| `smartctl_scan_devices()` | Enumerate storage devices (JSON) |
| `smartctl_get_smart_data()` | Get full SMART data (JSON) |
| `smartctl_check_health()` | Overall health assessment |
| `smartctl_run_selftest()` | Start a self-test |
| `smartctl_enable_smart()` / `smartctl_disable_smart()` | Toggle SMART |
| `smartctl_abort_selftest()` | Abort running test |
| `smartctl_destroy()` | Free context |

---

## About Smartmontools

The smartmontools package contains two utility programs (`smartctl` and `smartd`)
to control and monitor storage systems using the **Self-Monitoring, Analysis and
Reporting Technology System** (SMART) built into most modern ATA/SATA, SCSI/SAS and NVMe disks.

## Links

- [Smartmontools homepage](https://www.smartmontools.org/)
- [Upstream repository](https://github.com/smartmontools/smartmontools)
- [Smartmontools releases](https://github.com/smartmontools/smartmontools/releases)

## License

Smartmontools uses [GNU GPL Version 2](https://www.gnu.org/licenses/gpl-2.0.html#SEC1) license.
