// smartmon_c_api.h – extern "C" bridge between CGO and libsmartmon.
//
// All functions are thread-safe through an internal global mutex.
// The smart_interface singleton is initialised once per process; call
// smartmon_init() before any other function.
#pragma once

#ifdef __cplusplus
extern "C" {
#endif

// ABI version encoding: (major << 16) | (minor << 8) | patch.
// Major changes may break existing callers (removed/renamed/resignatured
// symbols); minor changes are additive only (new symbols, never removed or
// resignatured ones); patch changes affect neither symbols nor semantics.
#define SMARTMON_ABI_VERSION_MAJOR 1
#define SMARTMON_ABI_VERSION_MINOR 0
#define SMARTMON_ABI_VERSION_PATCH 0

// smartmon_abi_version returns this wrapper's C ABI version, encoded as
// (major<<16)|(minor<<8)|patch. Callers must reject a mismatched major
// version; a minor version greater than or equal to what the caller was
// built against is compatible. This symbol was introduced in ABI 1.0 — its
// absence identifies a pre-monorepo wrapper build.
unsigned smartmon_abi_version(void);

// smartmon_init initialises the smartmontools smart_interface singleton.
// Returns 0 on success; a negative value on failure. Subsequent calls are
// no-ops that return the same result as the first call.
int smartmon_init(void);

// smartmon_cleanup is reserved for future use and currently a no-op.
// The smart_interface singleton lives for the lifetime of the process.
void smartmon_cleanup(void);

// smartmon_scan_devices enumerates all storage devices visible to the OS.
// On success it writes a heap-allocated JSON string to *out_json and returns 0.
// The caller must release the string with smartmon_free_string().
// JSON format: {"devices":[{"name":"/dev/sda","type":"ata"}, ...]}
int smartmon_scan_devices(char **out_json);

// smartmon_get_smart_data reads full SMART/health data for device.
// dev_type may be NULL or "" to request auto-detection.
// On success it writes a heap-allocated JSON string to *out_json and returns 0.
// The JSON schema is compatible with the SMARTInfo struct in internal/types.
// The caller must release the string with smartmon_free_string().
int smartmon_get_smart_data(const char *device, const char *dev_type, char **out_json);

// smartmon_check_health performs an overall-health self-assessment check.
// dev_type may be NULL or "" for auto-detection.
// Sets *out_healthy to 1 if the device passes, 0 if it fails.
// Returns 0 on success, negative on error.
int smartmon_check_health(const char *device, const char *dev_type, int *out_healthy);

// smartmon_enable_smart enables SMART on the device. Returns 0 on success.
int smartmon_enable_smart(const char *device, const char *dev_type);

// smartmon_disable_smart disables SMART on the device. Returns 0 on success.
int smartmon_disable_smart(const char *device, const char *dev_type);

// smartmon_run_selftest starts a SMART self-test.
// test_type must be "short", "long", or "conveyance". Returns 0 on success.
int smartmon_run_selftest(const char *device, const char *dev_type, const char *test_type);

// smartmon_abort_selftest aborts a running SMART self-test. Returns 0 on success.
int smartmon_abort_selftest(const char *device, const char *dev_type);

// smartmon_free_string releases a JSON string returned by this API.
void smartmon_free_string(char *s);

// smartmon_last_error returns a description of the last error on the calling
// thread. The pointer is valid until the next call on this thread.
const char *smartmon_last_error(void);

#ifdef __cplusplus
}
#endif
