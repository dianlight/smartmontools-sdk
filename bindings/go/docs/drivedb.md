# USB Bridge Device Type Database (drivedb.h)

## Overview

The library embeds the standard `drivedb.h` file from the [smartmontools project](https://github.com/smartmontools/smartmontools), which contains a comprehensive database of drive and USB bridge configurations. This database is used to automatically handle "Unknown USB bridge" errors by providing appropriate device type parameters without manual intervention.

## Purpose

When smartctl encounters an unknown USB bridge, it cannot automatically determine which protocol to use for communication. The `drivedb.h` file provides a mapping of USB vendor:product IDs to device types, allowing the library to:

1. Automatically detect the correct device type for known USB bridges
2. Avoid unnecessary fallback attempts
3. Provide faster access to SMART data for common USB storage adapters
4. Reduce error messages and improve user experience

## Data Source

The `drivedb.h` file is the **official drive database** from the smartmontools project:

- **Repository**: https://github.com/smartmontools/smartmontools
- **File**: `lib/drivedb.h`
- **Maintained by**: The smartmontools community and maintainers
- **Updated regularly**: With new drive models, USB bridges, and firmware fixes

## File Format

The `drivedb.h` file uses a C struct format. USB entries have:

```c
{ "USB: <device>; <bridge>",
  "<vendor_id>:<product_id>",  // e.g., "0x152d:0x0578"
  "<firmware_regexp>",          // Firmware version filter (optional)
  "<warning_message>",          // Warning for users (optional)
  "-d <device_type>"            // Device type: sat, usbjmicron, etc.
}
```

### USB Entry Fields

- **modelfamily**: String starting with "USB:" describing the device and bridge
- **modelregexp**: USB vendor:product ID in hex format (may contain regex patterns)
- **firmwareregexp**: Optional firmware version filter
- **warningmsg**: Optional warning message
- **presets**: Device type parameter (e.g., "-d sat", "-d usbjmicron")

## How It Works

1. **Initialization**: When a client is created (`NewClient()` or `NewClientWithPath()`), the embedded `drivedb.h` file is parsed and USB entries are loaded into an in-memory cache.

2. **Detection**: When smartctl reports an "Unknown USB bridge" error, the library extracts the USB vendor:product ID from the error message.

3. **Lookup**: The extracted ID is checked against the prepopulated cache.

4. **Fallback**:
   - If found in drivedb.h, the corresponding device type is used immediately
   - If not found, the library tries `-d sat` as a default fallback
   - Successful attempts are cached by device path for future calls

## Examples

### Example 1: Known USB Bridge (JMicron JMS578)

```go
client, _ := smartmontools.NewClient()

// Device with JMicron JMS578 bridge (0x152d:0x0578)
// This USB bridge is in the standard drivedb.h
info, err := client.GetSMARTInfo("/dev/disk/by-id/usb-Device-xxx")

// The library automatically:
// 1. Detects "Unknown USB bridge [0x152d:0x0578]" error
// 2. Finds it in drivedb.h with device type "sat"
// 3. Retries with -d sat
// 4. Returns SMART info successfully
```

### Example 2: Unknown USB Bridge (Not in Database)

```go
client, _ := smartmontools.NewClient()

// Device with a USB bridge not in drivedb.h
info, err := client.GetSMARTInfo("/dev/disk/by-id/usb-NewBridge_xxx")

// The library automatically:
// 1. Detects "Unknown USB bridge [0xXXXX:0xYYYY]" error
// 2. Doesn't find it in drivedb.h
// 3. Falls back to trying -d sat (common for most USB-SATA bridges)
// 4. If successful, caches the device type for this device path
```

## Regex Pattern Expansion

The library automatically expands common regex patterns in USB product IDs:

- **Character classes**: `0x152d:0x05(7[789]|80)` expands to:
  - `0x152d:0x0577`
  - `0x152d:0x0578`
  - `0x152d:0x0579`
  - `0x152d:0x0580`

- **Alternatives**: Multiple alternatives separated by `|` are each expanded

This allows the single drivedb.h entry to match multiple similar devices efficiently.

## Updating the Database

This module is part of the `smartmontools-sdk` monorepo, where `native/drivedb/drivedb.h`
is the **single source of truth**. The copy embedded here at
`backends/exec/drivedb.h`, and its provenance file `backends/exec/drivedb_version.go`, are
**generated artefacts** — never hand-edit either one.

### Automated Updates

- The repository-root **`.github/workflows/update-submodule.yml`**'s `check-drivedb` job polls
  the matching `drivedb/<version>` branch on the upstream smartmontools repository daily. When
  it detects a change, it updates `native/drivedb/drivedb.h`, runs
  [`scripts/sync-drivedb-go.sh`](../../../scripts/sync-drivedb-go.sh) to refresh this module's
  copy, and opens a single PR containing all three files so the two copies are never out of
  sync even transiently.
- **`ci-bindings-go.yml`** re-runs `scripts/sync-drivedb-go.sh` on every PR touching
  `bindings/go/**` or `native/drivedb/**` and fails the build if it produces a diff — this is
  what makes "single source of truth" enforced rather than aspirational.

Each PR includes the full upstream diff for review before merging.

### Manual Update

To refresh this module's copy after `native/drivedb/drivedb.h` changes (e.g. a manual edit at
the repo root), run from the repository root:

```bash
scripts/sync-drivedb-go.sh
```

This copies `native/drivedb/drivedb.h` into `backends/exec/drivedb.h` and regenerates
`backends/exec/drivedb_version.go` by parsing the source file's own `VERSION:` line — no
network access or external state is needed.

### Embedded Version Constants

The file `backends/exec/drivedb_version.go` (generated by `scripts/sync-drivedb-go.sh`)
exposes the upstream provenance of the embedded file:

```go
import smartmontools "github.com/dianlight/smartmontools-sdk/bindings/go"

log.Info("drivedb loaded",
    "commit", smartmontools.DrivedbUpstreamCommit,
    "date",   smartmontools.DrivedbUpstreamDate,
)
```

> **Note**: `drivedb_version.go` is generated automatically. Do not edit it by hand.



The device type cache is protected by a `sync.RWMutex`, making it safe for concurrent access from multiple goroutines.

## Logging

The library provides debug and info logs when using the drivedb:

- **Info**: When a USB bridge is found in drivedb.h
- **Info**: When successfully accessing a device with the determined device type
- **Debug**: Number of USB entries loaded from drivedb.h
- **Debug**: When caching device types

Enable debug logging to see detailed information about USB bridge detection and caching.

## Benefits of Using Standard drivedb.h

1. **Official**: Uses the same database as the smartmontools project
2. **Comprehensive**: Includes hundreds of USB bridges from worldwide community contributions
3. **Up-to-date**: Regularly updated with new devices and fixes
4. **Tested**: Extensively tested by the smartmontools community
5. **Zero Configuration**: Works out of the box after library installation

## Related Functions

- `loadDrivedbAddendum()`: Parses and loads the embedded drivedb.h file
- `extractUSBIDs()`: Extracts and expands USB vendor:product IDs from regex patterns
- `extractUSBBridgeID()`: Extracts USB vendor:product ID from error messages
- `getCachedDeviceType()`: Retrieves device type from cache
- `setCachedDeviceType()`: Stores device type in cache

## See Also

- [smartmontools GitHub repository](https://github.com/smartmontools/smartmontools)
- [smartmontools drivedb.h](https://github.com/smartmontools/smartmontools/blob/master/lib/drivedb.h)
- [smartctl man page](https://www.smartmontools.org/browser/trunk/smartmontools/smartctl.8.in)
- [smartmontools drive database format](https://www.smartmontools.org/browser/trunk/smartmontools/drivedb.h)
