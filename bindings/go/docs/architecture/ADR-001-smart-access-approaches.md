# ADR-001: SMART Data Access Approaches

## Status

Accepted (Option 1 default, Option 3 implemented as LibBackend)

## Context

The smartmontools-go library needs a reliable and efficient way to access SMART (Self-Monitoring, Analysis, and Reporting Technology) data from storage devices. This document evaluates different architectural approaches for implementing SMART data access in a Go library.

Currently, the library uses a command-line wrapper approach (executing the external `smartctl` binary), but there are several alternative approaches that could provide different trade-offs in terms of performance, portability, complexity, and maintenance.

## Decision Options

We evaluate four distinct approaches for accessing SMART data from storage devices:

### Option 1: smartctl Command Wrapper (Current Implementation)

Execute the external `smartctl` binary and parse its JSON output.

#### Architecture

```
┌─────────────────┐
│  Go Application │
└────────┬────────┘
         │ os/exec
         ▼
┌─────────────────┐
│  smartctl CLI   │
│   (--json)      │
└────────┬────────┘
         │ ioctl/ATA/NVMe
         ▼
┌─────────────────┐
│  Storage Device │
└─────────────────┘
```

#### Implementation Details

```go
// Current implementation approach
type Client struct {
    smartctlPath string
    commander    Commander
}

func (c *Client) GetSMARTInfo(devicePath string) (*SMARTInfo, error) {
    cmd := c.commander.Command(c.smartctlPath, "--json", "-a", devicePath)
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }
    
    var info SMARTInfo
    if err := json.Unmarshal(output, &info); err != nil {
        return nil, err
    }
    return &info, nil
}
```

#### Advantages

1. **Simplicity**: Straightforward implementation using standard library
2. **Maintenance**: Leverages well-tested smartmontools codebase
3. **Portability**: Works across Linux, macOS, Windows (where smartctl is available)
4. **Feature completeness**: All smartctl features available immediately
5. **No native dependencies**: Pure Go (no CGO)
6. **Stability**: smartmontools handles device-specific quirks and edge cases
7. **JSON parsing**: Native JSON output from smartctl 7.0+

#### Disadvantages

1. **Performance overhead**: Process creation and execution overhead for each call
2. **External dependency**: Requires smartctl to be installed on the system
3. **Version dependency**: Requires smartctl 7.0+ for JSON output
4. **Limited error handling**: Limited control over low-level errors
5. **No real-time monitoring**: Each call is independent; no persistent connection
6. **Binary location**: Need to locate smartctl binary (PATH or custom path)

#### Use Cases

- Server monitoring applications that check devices periodically
- Desktop applications where smartctl is typically available
- Cross-platform tools that need maximum compatibility
- Applications that don't require high-frequency polling

#### Code Example

```go
package main

import (
    "fmt"
    "log"
    "github.com/dianlight/smartmontools-sdk/bindings/go/v8"
)

func main() {
    // Create client with default smartctl path
    client, err := smartmontools.NewClient()
    if err != nil {
        log.Fatal(err)
    }
    
    // Or specify custom path
    // client := smartmontools.NewClientWithPath("/usr/local/sbin/smartctl")
    
    // Get SMART info
    info, err := client.GetSMARTInfo("/dev/sda")
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Model: %s\n", info.ModelName)
    fmt.Printf("Temperature: %d°C\n", info.Temperature.Current)
}
```

---

### Option 2: Direct ioctl Access

Directly call Linux ioctl system calls to communicate with storage devices.

#### Architecture

```
┌─────────────────┐
│  Go Application │
└────────┬────────┘
         │ syscall.Syscall/unix.Ioctl
         ▼
┌─────────────────┐
│  Kernel Driver  │
│  (sd, nvme)     │
└────────┬────────┘
         │ hardware protocol
         ▼
┌─────────────────┐
│  Storage Device │
└─────────────────┘
```

#### Implementation Details

```go
// Example ioctl-based implementation (Linux-specific)
import (
    "syscall"
    "unsafe"
    "golang.org/x/sys/unix"
)

const (
    // SMART ioctl commands (ATA)
    HDIO_DRIVE_CMD = 0x031f
    WIN_SMART      = 0xb0
    SMART_READ_VALUES = 0xd0
)

type SmartIoctlClient struct {
    devicePath string
}

func (c *SmartIoctlClient) readSmartData() ([]byte, error) {
    fd, err := unix.Open(c.devicePath, unix.O_RDONLY, 0)
    if err != nil {
        return nil, err
    }
    defer unix.Close(fd)
    
    // Prepare SMART command structure
    var args [4 + 512]byte
    args[0] = WIN_SMART
    args[1] = 0  // sector count
    args[2] = SMART_READ_VALUES
    args[3] = 1  // LBA low (SMART signature)
    
    // Execute ioctl
    _, _, errno := syscall.Syscall(
        syscall.SYS_IOCTL,
        uintptr(fd),
        HDIO_DRIVE_CMD,
        uintptr(unsafe.Pointer(&args[0])),
    )
    
    if errno != 0 {
        return nil, errno
    }
    
    // Parse response from args[4:516]
    return args[4:516], nil
}

// NVMe Admin Command structure
type NvmeAdminCmd struct {
    Opcode    uint8
    Flags     uint8
    CommandID uint16
    NSID      uint32
    // ... additional fields
}

func (c *SmartIoctlClient) readNvmeSmartLog() ([]byte, error) {
    fd, err := unix.Open(c.devicePath, unix.O_RDONLY, 0)
    if err != nil {
        return nil, err
    }
    defer unix.Close(fd)
    
    // NVMe SMART/Health Information log page
    const NVME_LOG_SMART = 0x02
    const NVME_ADMIN_GET_LOG_PAGE = 0x02
    
    buffer := make([]byte, 512)
    cmd := NvmeAdminCmd{
        Opcode: NVME_ADMIN_GET_LOG_PAGE,
        NSID:   0xffffffff,
        // Set up log page parameters
    }
    
    // Execute NVMe admin command via ioctl
    // Implementation details vary by kernel interface
    
    return buffer, nil
}
```

#### Advantages

1. **Performance**: Direct kernel communication, no process overhead
2. **No external dependencies**: No smartctl binary required
3. **Real-time access**: Can maintain persistent file descriptors
4. **Fine-grained control**: Direct access to all device commands
5. **Low-level error handling**: Access to detailed error codes
6. **Efficient polling**: Can implement efficient monitoring loops

#### Disadvantages

1. **Platform-specific**: Different implementations for Linux, FreeBSD, Windows
2. **Complexity**: Must understand ATA, SCSI, NVMe protocols
3. **Device quirks**: Need to handle device-specific variations
4. **Maintenance burden**: Must keep up with protocol changes
5. **Permissions**: Typically requires root/administrator access
6. **Error-prone**: Low-level programming with unsafe operations
7. **Limited portability**: Linux ioctl differs from Windows DeviceIoControl

#### Platform Support

| Platform | API | Difficulty |
|----------|-----|-----------|
| Linux | `ioctl()` with `HDIO_*`, `NVME_*` | Medium |
| FreeBSD | `ioctl()` with `ATA_*`, `NVME_*` | Medium |
| macOS | `IOKit` framework | High |
| Windows | `DeviceIoControl()` with `IOCTL_*` | High |

#### Use Cases

- High-performance monitoring applications
- Embedded systems where smartctl is unavailable
- Applications requiring sub-second response times
- Custom hardware with specific requirements

#### Code Example

```go
package main

import (
    "fmt"
    "log"
)

// Hypothetical ioctl-based client
type IoctlSmartClient struct {
    deviceFd int
}

func main() {
    client := &IoctlSmartClient{}
    
    // Open device with proper permissions (requires root)
    if err := client.OpenDevice("/dev/sda"); err != nil {
        log.Fatal(err)
    }
    defer client.Close()
    
    // Read SMART data directly
    smartData, err := client.ReadSmartAttributes()
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Raw SMART attributes: %v\n", smartData)
    
    // Continuous monitoring example
    for {
        temp, err := client.ReadTemperature()
        if err != nil {
            log.Printf("Error reading temperature: %v", err)
            continue
        }
        fmt.Printf("Current temperature: %d°C\n", temp)
        time.Sleep(1 * time.Second)
    }
}
```

---

### Option 3: Shared Library with FFI (No CGO)

Use a Foreign Function Interface (FFI) library to call into a shared library compiled from smartmontools source.

#### Architecture

```
┌─────────────────┐
│  Go Application │
└────────┬────────┘
         │ FFI (purego/libffi)
         ▼
┌─────────────────┐
│ libsmartctl.so  │
│ (C++ library)   │
└────────┬────────┘
         │ ioctl/ATA/NVMe
         ▼
┌─────────────────┐
│  Storage Device │
└─────────────────┘
```

#### Implementation Details

Using [purego](https://github.com/ebitengine/purego) for CGO-free FFI:

```go
// Example using purego (no CGO)
package main

import (
    "github.com/ebitengine/purego"
    "unsafe"
)

// Hypothetical libsmartctl shared library interface
type SmartctlLib struct {
    lib uintptr
    
    // Function pointers
    getSmartData      uintptr
    getDeviceInfo     uintptr
    checkHealth       uintptr
    initializeDevice  uintptr
    cleanupDevice     uintptr
}

func LoadSmartctlLib(libraryPath string) (*SmartctlLib, error) {
    lib, err := purego.Dlopen(libraryPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
    if err != nil {
        return nil, err
    }
    
    s := &SmartctlLib{lib: lib}
    
    // Load function symbols
    purego.RegisterLibFunc(&s.getSmartData, lib, "smartctl_get_smart_data")
    purego.RegisterLibFunc(&s.getDeviceInfo, lib, "smartctl_get_device_info")
    purego.RegisterLibFunc(&s.checkHealth, lib, "smartctl_check_health")
    purego.RegisterLibFunc(&s.initializeDevice, lib, "smartctl_initialize_device")
    purego.RegisterLibFunc(&s.cleanupDevice, lib, "smartctl_cleanup_device")
    
    return s, nil
}

// C-compatible structure for SMART data
type CSmartData struct {
    Temperature    int32
    PowerOnHours   int64
    PowerCycles    int64
    AttributeCount int32
    // ... more fields
}

func (s *SmartctlLib) GetSmartData(devicePath string) (*CSmartData, error) {
    // Convert Go string to C string
    cDevicePath := append([]byte(devicePath), 0)
    
    var smartData CSmartData
    
    // Call C function via FFI
    ret, _, _ := purego.SyscallN(
        s.getSmartData,
        uintptr(unsafe.Pointer(&cDevicePath[0])),
        uintptr(unsafe.Pointer(&smartData)),
    )
    
    if ret != 0 {
        return nil, fmt.Errorf("failed to get SMART data: %d", ret)
    }
    
    return &smartData, nil
}
```

C API wrapper for smartmontools (would need to be created):

```c
// libsmartctl_wrapper.h
#ifndef LIBSMARTCTL_WRAPPER_H
#define LIBSMARTCTL_WRAPPER_H

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
    int temperature;
    long long power_on_hours;
    long long power_cycles;
    int attribute_count;
    // ... more fields
} smart_data_t;

// Initialize device handle
int smartctl_initialize_device(const char* device_path, void** handle);

// Get SMART data
int smartctl_get_smart_data(void* handle, smart_data_t* data);

// Check device health
int smartctl_check_health(void* handle, int* is_healthy);

// Cleanup device handle
void smartctl_cleanup_device(void* handle);

#ifdef __cplusplus
}
#endif

#endif
```

#### Advantages

1. **No CGO**: Uses pure Go FFI libraries like purego
2. **Performance**: Better than exec, close to native
3. **Leverage smartmontools**: Reuse battle-tested code
4. **No external binary**: Library embedded or distributed
5. **Cross-compilation**: Easier than CGO
6. **Type safety**: Can provide strong typing in Go layer

#### Disadvantages

1. **Library creation**: Need to create/maintain C API wrapper
2. **Build complexity**: Must compile smartmontools as shared library
3. **Distribution**: Need to ship or build shared library for each platform
4. **ABI compatibility**: Must manage C++ ABI issues
5. **Memory management**: Manual memory management across FFI boundary
6. **Debugging difficulty**: Harder to debug across FFI boundary
7. **Platform variations**: Different shared library formats (.so, .dylib, .dll)

#### Build Process

```bash
# Example: Building libsmartctl.so from smartmontools
# 1. Clone and prepare smartmontools source
git clone https://github.com/smartmontools/smartmontools.git
cd smartmontools

# 2. Create C API wrapper
# (Add wrapper code to expose C-compatible interface)

# 3. Build as shared library
./configure --enable-shared
make
# Results in libsmartctl.so (Linux), libsmartctl.dylib (macOS), smartctl.dll (Windows)

# 4. Install library
sudo make install
# Or distribute with your application
```

#### Use Cases

- Applications that need better performance than exec
- Cross-platform deployment without CGO complications
- Situations where reusing smartmontools logic is valuable
- Applications distributed as single binary + library

#### Code Example

```go
package main

import (
    "fmt"
    "log"
)

func main() {
    // Load shared library
    lib, err := LoadSmartctlLib("libsmartctl.so")
    if err != nil {
        log.Fatal(err)
    }
    
    // Initialize device
    device, err := lib.InitializeDevice("/dev/sda")
    if err != nil {
        log.Fatal(err)
    }
    defer lib.CleanupDevice(device)
    
    // Get SMART data
    smartData, err := lib.GetSmartData(device)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Temperature: %d°C\n", smartData.Temperature)
    fmt.Printf("Power On Hours: %d\n", smartData.PowerOnHours)
    
    // Check health
    healthy, err := lib.CheckHealth(device)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Device healthy: %v\n", healthy)
}
```

---

### Option 4: Hybrid Approach (ioctl + Shared Library)

Combine direct ioctl access for performance-critical paths with shared library fallback for complex operations.

#### Architecture

```
┌─────────────────────────────────────┐
│         Go Application              │
│  ┌──────────────────────────────┐  │
│  │   Smart Client Interface     │  │
│  └──────────┬───────────────────┘  │
│             │                       │
│        ┌────┴────┐                  │
│        │ Router  │                  │
│        └────┬────┘                  │
│             │                       │
│     ┌───────┴────────┐              │
│     ▼                ▼              │
│  ┌──────┐      ┌──────────┐        │
│  │ioctl │      │ libsmartctl│       │
│  │Direct│      │   (FFI)    │       │
│  └──┬───┘      └──────┬─────┘       │
└─────┼──────────────────┼────────────┘
      │                  │
      ▼                  ▼
┌─────────────────────────────────────┐
│         Storage Device              │
└─────────────────────────────────────┘
```

#### Implementation Details

```go
package main

// Hybrid client that chooses best approach per operation
type HybridSmartClient struct {
    ioctlClient    *IoctlSmartClient
    libClient      *LibSmartClient
    preferIoctl    bool
    capabilities   map[string]bool
}

func NewHybridClient(devicePath string) (*HybridSmartClient, error) {
    client := &HybridSmartClient{
        capabilities: make(map[string]bool),
    }
    
    // Initialize ioctl client
    ioctlClient, err := NewIoctlClient(devicePath)
    if err == nil {
        client.ioctlClient = ioctlClient
        client.capabilities["ioctl"] = true
    }
    
    // Initialize library client
    libClient, err := NewLibSmartClient()
    if err == nil {
        client.libClient = libClient
        client.capabilities["library"] = true
    }
    
    // Determine default preference
    client.preferIoctl = client.capabilities["ioctl"]
    
    return client, nil
}

// Fast path: direct ioctl for simple operations
func (c *HybridSmartClient) GetTemperature() (int, error) {
    if c.ioctlClient != nil {
        // Use ioctl for fast temperature reading
        return c.ioctlClient.ReadTemperature()
    }
    
    // Fallback to library
    if c.libClient != nil {
        data, err := c.libClient.GetSmartData()
        if err != nil {
            return 0, err
        }
        return data.Temperature, nil
    }
    
    return 0, fmt.Errorf("no available backend")
}

// Complex path: library for full SMART data
func (c *HybridSmartClient) GetFullSmartData() (*SMARTInfo, error) {
    // Prefer library for complex parsing
    if c.libClient != nil {
        return c.libClient.GetSmartData()
    }
    
    // Fallback to ioctl with manual parsing
    if c.ioctlClient != nil {
        rawData, err := c.ioctlClient.ReadSmartData()
        if err != nil {
            return nil, err
        }
        return parseSmartData(rawData)
    }
    
    return nil, fmt.Errorf("no available backend")
}

// Device health check: use fastest available
func (c *HybridSmartClient) CheckHealth() (bool, error) {
    // Try ioctl first if available
    if c.ioctlClient != nil {
        return c.ioctlClient.CheckHealth()
    }
    
    // Fallback to library
    if c.libClient != nil {
        return c.libClient.CheckHealth()
    }
    
    return false, fmt.Errorf("no available backend")
}

// Capability detection
func (c *HybridSmartClient) GetCapabilities() map[string]bool {
    return c.capabilities
}

// Allow runtime switching
func (c *HybridSmartClient) SetPreferIoctl(prefer bool) {
    c.preferIoctl = prefer && c.capabilities["ioctl"]
}
```

#### Strategy Selection

```go
// Operation routing based on requirements
type OperationStrategy int

const (
    StrategyFast       OperationStrategy = iota // Prefer ioctl
    StrategyReliable                            // Prefer library
    StrategyAutomatic                           // Choose best
)

func (c *HybridSmartClient) GetSmartData(strategy OperationStrategy) (*SMARTInfo, error) {
    switch strategy {
    case StrategyFast:
        if c.ioctlClient != nil {
            return c.ioctlClient.GetSmartData()
        }
        return c.libClient.GetSmartData()
        
    case StrategyReliable:
        if c.libClient != nil {
            return c.libClient.GetSmartData()
        }
        return c.ioctlClient.GetSmartData()
        
    case StrategyAutomatic:
        // Use heuristics to choose
        if c.needsComplexParsing() {
            return c.libClient.GetSmartData()
        }
        return c.ioctlClient.GetSmartData()
    }
    
    return nil, fmt.Errorf("invalid strategy")
}
```

#### Advantages

1. **Flexibility**: Choose optimal approach per operation
2. **Performance**: Fast operations via ioctl, complex via library
3. **Reliability**: Fallback options increase robustness
4. **Gradual migration**: Can transition between approaches
5. **Feature completeness**: Best of both worlds
6. **Graceful degradation**: Works even if one backend fails

#### Disadvantages

1. **Complexity**: Most complex implementation to maintain
2. **Code duplication**: Some logic duplicated across backends
3. **Testing burden**: Must test both paths
4. **Binary size**: Largest binary/deployment size
5. **Decision overhead**: Runtime routing decisions
6. **Consistency**: Ensuring consistent behavior across backends

#### Use Cases

- Production monitoring systems requiring maximum reliability
- Applications that need both high performance and feature completeness
- Gradual migration from one approach to another
- Systems that need to work across varied environments

#### Code Example

```go
package main

import (
    "fmt"
    "log"
    "time"
)

func main() {
    // Create hybrid client
    client, err := NewHybridClient("/dev/sda")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()
    
    // Check available backends
    caps := client.GetCapabilities()
    fmt.Printf("Available backends: %v\n", caps)
    
    // Fast monitoring loop using ioctl
    go func() {
        for {
            temp, err := client.GetTemperature()
            if err != nil {
                log.Printf("Error: %v", err)
                continue
            }
            if temp > 60 {
                log.Printf("WARNING: High temperature: %d°C", temp)
            }
            time.Sleep(1 * time.Second)
        }
    }()
    
    // Periodic full scan using library (more reliable)
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
        smartData, err := client.GetFullSmartData(StrategyReliable)
        if err != nil {
            log.Printf("Full scan error: %v", err)
            continue
        }
        
        // Process full SMART data
        fmt.Printf("Model: %s\n", smartData.ModelName)
        fmt.Printf("Health: %v\n", smartData.SmartStatus.Passed)
        
        // Check for failing attributes
        if smartData.AtaSmartData != nil {
            for _, attr := range smartData.AtaSmartData.Table {
                if attr.WhenFailed != "" {
                    log.Printf("ALERT: Attribute %d (%s) failed", 
                        attr.ID, attr.Name)
                }
            }
        }
    }
}
```

---

## Comparison Matrix

| Criteria | Option 1: smartctl Wrapper | Option 2: ioctl Direct | Option 3: Shared Library (FFI) | Option 4: Hybrid |
|----------|---------------------------|------------------------|-------------------------------|------------------|
| **Performance** | ⭐⭐ (process overhead) | ⭐⭐⭐⭐⭐ (direct access) | ⭐⭐⭐⭐ (near-native) | ⭐⭐⭐⭐⭐ (best of both) |
| **Portability** | ⭐⭐⭐⭐⭐ (cross-platform) | ⭐⭐ (platform-specific) | ⭐⭐⭐ (requires builds) | ⭐⭐⭐ (moderate) |
| **Implementation Complexity** | ⭐⭐⭐⭐⭐ (simple) | ⭐⭐ (complex) | ⭐⭐⭐ (moderate) | ⭐ (most complex) |
| **Maintenance Burden** | ⭐⭐⭐⭐⭐ (minimal) | ⭐⭐ (high) | ⭐⭐⭐ (moderate) | ⭐⭐ (high) |
| **External Dependencies** | ⭐⭐ (requires smartctl) | ⭐⭐⭐⭐⭐ (none) | ⭐⭐⭐ (shared library) | ⭐⭐⭐ (library optional) |
| **Feature Completeness** | ⭐⭐⭐⭐⭐ (full smartctl) | ⭐⭐⭐ (requires impl) | ⭐⭐⭐⭐⭐ (full smartctl) | ⭐⭐⭐⭐⭐ (comprehensive) |
| **Error Handling** | ⭐⭐⭐ (limited detail) | ⭐⭐⭐⭐⭐ (full control) | ⭐⭐⭐⭐ (good control) | ⭐⭐⭐⭐⭐ (best) |
| **Debugging Ease** | ⭐⭐⭐⭐⭐ (easy) | ⭐⭐⭐ (moderate) | ⭐⭐ (difficult) | ⭐⭐ (complex) |
| **Security** | ⭐⭐⭐⭐ (sandboxed) | ⭐⭐⭐ (raw access) | ⭐⭐⭐⭐ (library isolation) | ⭐⭐⭐ (varied) |
| **Binary Size** | ⭐⭐⭐⭐⭐ (small) | ⭐⭐⭐⭐⭐ (small) | ⭐⭐⭐ (medium+lib) | ⭐⭐ (large) |
| **Real-time Capability** | ⭐⭐ (poor) | ⭐⭐⭐⭐⭐ (excellent) | ⭐⭐⭐⭐ (good) | ⭐⭐⭐⭐⭐ (excellent) |
| **Cross-compilation** | ⭐⭐⭐⭐⭐ (easy) | ⭐⭐⭐⭐ (good) | ⭐⭐⭐⭐ (no CGO) | ⭐⭐⭐ (moderate) |

## Performance Comparison

### Latency Benchmarks (Estimated)

| Operation | smartctl Wrapper | ioctl Direct | Shared Library | Hybrid |
|-----------|-----------------|--------------|----------------|--------|
| Single temperature read | 50-100ms | 1-2ms | 2-5ms | 1-5ms |
| Full SMART data read | 100-200ms | 10-20ms | 15-30ms | 10-30ms |
| Device scan (10 devices) | 500-1000ms | 20-50ms | 30-80ms | 20-80ms |
| Health check only | 50-100ms | 1-2ms | 2-5ms | 1-5ms |

### Throughput (operations/second)

| Operation | smartctl Wrapper | ioctl Direct | Shared Library | Hybrid |
|-----------|-----------------|--------------|----------------|--------|
| Temperature reads | 10-20/s | 500-1000/s | 200-500/s | 200-1000/s |
| Full SMART reads | 5-10/s | 50-100/s | 30-60/s | 30-100/s |

## Platform Support Matrix

| Platform | Option 1 | Option 2 | Option 3 | Option 4 |
|----------|----------|----------|----------|----------|
| **Linux (x86_64)** | ✅ Full | ✅ Full | ✅ Full | ✅ Full |
| **Linux (ARM)** | ✅ Full | ✅ Full | ⚠️ Needs build | ⚠️ Partial |
| **macOS (Intel)** | ✅ Full | ⚠️ Limited (IOKit) | ⚠️ Needs build | ⚠️ Partial |
| **macOS (Apple Silicon)** | ✅ Full | ⚠️ Limited (IOKit) | ⚠️ Needs build | ⚠️ Partial |
| **Windows** | ✅ Full | ⚠️ Complex (DeviceIoControl) | ⚠️ Needs build | ⚠️ Partial |
| **FreeBSD** | ✅ Full | ⚠️ Different API | ⚠️ Needs build | ⚠️ Partial |

## Implementation Roadmap

### Phase 1: Current State (Option 1) ✅ Complete
- ✅ Command wrapper implemented
- ✅ JSON parsing
- ✅ Cross-platform support
- ✅ Basic test coverage
- ✅ Multi-path smartctl binary resolution (NAS platforms)
- ✅ SAT fallback for USB bridges
- ✅ Exit code bit decomposition
- ✅ `--scan-open` → `--scan` fallback
- ✅ Drive discovery with protocol probe
- ✅ Wear level normalization

### Phase 2: ioctl Exploration (Option 2) ⏭️ Deferred
- Create Linux ioctl proof-of-concept
- Benchmark against current implementation
- Document platform-specific requirements
- Evaluate feasibility for production

### Phase 3: Library Integration (Option 3) ✅ Complete
- ✅ purego FFI integration (no CGO)
- ✅ Build system for shared library
- ✅ Benchmark performance
- ✅ LibBackend implemented with full Backend interface

### Phase 4: Hybrid Implementation (Option 4) ⏭️ Future
- Design routing architecture
- Implement backend abstraction
- Create fallback mechanisms
- Comprehensive testing

## Recommendations

### For Current Development: **Option 1 (Default) + Option 3 (LibBackend)**

**Rationale:**
- Option 1: Proven, stable approach, excellent cross-platform support
- Option 3: Implemented via purego FFI for environments where process spawn overhead is undesirable
- Both backends implement the same `Backend` interface (ADR-002)
- Users can choose via `WithBackend()` option

### For Future v1.0+: **Option 2 (IoctlBackend) or Option 4 (Hybrid)**

**Rationale:**
- Maximum flexibility for users
- Performance where needed
- Reliability through fallbacks
- Gradual migration path

**Implementation Strategy:**
1. Keep Option 1 as stable default
2. Add Option 2 (ioctl) for Linux as optional high-performance backend
3. Add Option 3 (FFI) as optional backend for embedded scenarios
4. Provide consistent API across all backends
5. Runtime capability detection and selection

### For Specialized Use Cases:

| Use Case | Recommended Option |
|----------|-------------------|
| Server monitoring (periodic checks) | Option 1 |
| High-frequency monitoring (sub-second) | Option 2 or 4 |
| Embedded systems (no smartctl) | Option 2 or 3 |
| Cross-platform desktop app | Option 1 |
| Cloud-native microservice | Option 1 |
| Real-time dashboard | Option 4 |
| Single-binary distribution | Option 2 or 3 |

## Technical Considerations

### Security

**Option 1 (smartctl Wrapper):**
- Relies on system smartctl security
- Process isolation provides some sandboxing
- Requires appropriate sudo/permissions configuration

**Option 2 (ioctl):**
- Requires root/CAP_SYS_RAWIO capability
- Direct hardware access increases attack surface
- Must validate all inputs carefully

**Option 3 (Shared Library):**
- Library security depends on smartmontools implementation
- FFI boundary needs careful memory handling
- Reduced attack surface vs raw ioctl

**Option 4 (Hybrid):**
- Combined security considerations
- Need to secure all code paths

### Memory Management

**Option 1:** Simple - Go manages everything

**Option 2:** Moderate - unsafe operations but controlled scope

**Option 3:** Complex - manual management across FFI boundary
```go
// Example memory management in FFI
func (c *LibClient) GetData() (*Data, error) {
    cData := C.get_data()
    defer C.free_data(cData) // Must free C-allocated memory
    
    // Copy to Go-managed memory
    goData := convertCDataToGo(cData)
    return goData, nil
}
```

**Option 4:** Most complex - multiple memory domains

### Error Handling

**Best Practices Across Options:**

```go
// Consistent error wrapping
type SmartError struct {
    Operation string
    Device    string
    Err       error
    Code      int
}

func (e *SmartError) Error() string {
    return fmt.Sprintf("SMART operation %s failed on %s: %v (code: %d)", 
        e.Operation, e.Device, e.Err, e.Code)
}

// Usage example
func (c *Client) GetSMARTInfo(device string) (*SMARTInfo, error) {
    // ... implementation
    if err != nil {
        return nil, &SmartError{
            Operation: "GetSMARTInfo",
            Device:    device,
            Err:       err,
            Code:      exitCode,
        }
    }
    return info, nil
}
```

## References

### Technical Documentation

1. **ATA/ATAPI Command Set**
   - ATA/ATAPI-8 Specification
   - SMART Feature Set documentation
   - https://www.t13.org/

2. **NVMe Specification**
   - NVMe 1.4+ Specification
   - SMART/Health Information Log Page
   - https://nvmexpress.org/

3. **smartmontools**
   - Official documentation: https://www.smartmontools.org/
   - Source code: https://github.com/smartmontools/smartmontools
   - Wiki: https://www.smartmontools.org/wiki

4. **Linux Kernel APIs**
   - HDIO ioctl documentation
   - NVMe ioctl documentation
   - https://www.kernel.org/doc/html/latest/

5. **FFI Libraries**
   - purego: https://github.com/ebitengine/purego
   - CGO documentation: https://golang.org/cmd/cgo/

### Related Projects

1. **go-smart** - Alternative Go SMART library
2. **node-smart** - Node.js SMART bindings
3. **pySMART** - Python SMART library
4. **crystal-smart** - Crystal language SMART library

### Platform-Specific Documentation

**Linux:**
- `/usr/include/linux/hdreg.h` - ATA ioctl definitions
- `/usr/include/linux/nvme_ioctl.h` - NVMe ioctl definitions

**Windows:**
- IOCTL_STORAGE_* documentation
- IOCTL_SCSI_PASS_THROUGH documentation

**macOS:**
- IOKit documentation
- Storage device access guides

## Glossary

- **ATA**: Advanced Technology Attachment - interface standard for storage devices
- **FFI**: Foreign Function Interface - mechanism for calling code written in one language from another
- **ioctl**: Input/Output Control - system call for device-specific operations
- **NVMe**: Non-Volatile Memory Express - interface specification for SSDs
- **SCSI**: Small Computer System Interface - standard for connecting storage devices
- **SMART**: Self-Monitoring, Analysis, and Reporting Technology - monitoring system for drives
- **smartctl**: Command-line utility from smartmontools for accessing SMART data
- **CGO**: Go's mechanism for calling C code (avoided in Option 3)

## Conclusion

This document provides a comprehensive analysis of four approaches for accessing SMART data in the smartmontools-go library. Each option presents distinct trade-offs between performance, complexity, portability, and maintenance.

The current implementation (Option 1) provides an excellent foundation for the library, offering broad compatibility and ease of maintenance. As the project matures, implementing a hybrid approach (Option 4) could provide the best long-term solution, allowing users to choose their preferred balance of performance and reliability based on their specific requirements.

The decision should be driven by:
1. Target use cases and performance requirements
2. Development resources available for maintenance
3. Platform support requirements
4. Community feedback and contributions

This ADR will be revisited as the project evolves and new requirements emerge.

---

**Document Information:**
- **Created:** 2025-11-13
- **Status:** Proposed
- **Authors:** smartmontools-go development team
- **Related Issues:** Issue regarding SMART access options
