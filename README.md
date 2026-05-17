## About Smartmontools SDK

This is a fork of [smartmontools/smartmontools](https://github.com/smartmontools/smartmontools)
with **libsmartctl** shared library support, maintained for the
[smartmontools-go](https://github.com/dianlight/smartmontools-go) project.

### What's different

- **`libsmartctl.so` / `libsmartctl.dylib`** — shared library target via `--enable-libsmartctl`
- **C API** (`src/libsmartctl.h`) — stable interface for FFI consumers (purego, CGO)
- **Weekly upstream sync** — automated rebase via `sync-upstream.yml`

### Building the shared library

```bash
./autogen.sh
./configure --enable-shared --disable-static --enable-libsmartctl \
  CFLAGS="-fPIC" CXXFLAGS="-fPIC -DBUILDING_LIBSMARTCTL"
make -j$(nproc)
```

The library will be at `src/.libs/libsmartctl.so` (Linux) or
`src/.libs/libsmartctl.dylib` (macOS).

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
In many cases, these utilities will provide advanced warning of disk degradation and failure.

Smartmontools was originally derived from the Linux [smartsuite package](https://sourceforge.net/projects/smartsuite/) and supports ATA/SATA, SCSI/SAS, and NVMe disks and also SCSI/SAS tape devices.
It should run on any modern Linux, FreeBSD, NetBSD, OpenBSD, Darwin (macOS), Solaris, Windows, Cygwin, OS/2, eComStation, or QNX system.
Smartmontools can also be run from one of many different Live CDs/DVDs.

## Important links
- [Project homepage](https://www.smartmontools.org/)
- [GitHub repository](https://github.com/smartmontools/smartmontools)
- [CI builds](https://github.com/smartmontools/smartmontools-builds/releases)
- [Project Releases](https://github.com/smartmontools/smartmontools/releases)


## Code Signing
This program uses free code signing provided by [SignPath.io](https://signpath.io) and a free code signing certificate by the [SignPath Foundation](https://signpath.org)

## License
Smartmontools uses [GNU GPL Version 2](https://www.gnu.org/licenses/gpl-2.0.html#SEC1) license. 
