#ifndef LIBSMARTCTL_H
#define LIBSMARTCTL_H

#ifdef __cplusplus
extern "C" {
#endif

/* ABI version — increment on every incompatible API change. */
#define LIBSMARTCTL_ABI_VERSION 1

/**
 * Opaque smartctl execution context.
 * Allocate with smartctl_init(); release with smartctl_destroy().
 */
typedef struct smartctl_ctx smartctl_ctx;

/**
 * smartctl_abi_version — returns LIBSMARTCTL_ABI_VERSION.
 * Callers should verify this equals the version they were built against.
 */
int smartctl_abi_version(void);

/**
 * smartctl_init — allocates and initialises a new context.
 * Returns 0 on success; non-zero on allocation failure.
 * On success *ctx is set to the new context pointer.
 */
int smartctl_init(smartctl_ctx **ctx);

/**
 * smartctl_destroy — destroys a context and frees all associated memory.
 * Passing a NULL ctx is a no-op.
 */
void smartctl_destroy(smartctl_ctx *ctx);

/**
 * smartctl_set_option — sets a named option on the context.
 * Returns 0 on success; non-zero if the option is unknown or value invalid.
 * Recognised keys: "device_type" (default "auto"), "timeout_seconds".
 */
int smartctl_set_option(smartctl_ctx *ctx, const char *key, const char *value);

/**
 * smartctl_scan_devices — enumerates storage devices.
 * On success sets *out_json to a JSON string (same format as smartctl --scan -j)
 * and returns 0.  The caller must release *out_json with smartctl_free_string().
 * Returns non-zero on failure; call smartctl_last_error() for details.
 */
int smartctl_scan_devices(smartctl_ctx *ctx, char **out_json);

/**
 * smartctl_get_smart_data — retrieves full SMART data for a device.
 * On success sets *out_json to a JSON string (same format as smartctl -a -j)
 * and returns 0.  The caller must release *out_json with smartctl_free_string().
 * Returns non-zero on failure; call smartctl_last_error() for details.
 */
int smartctl_get_smart_data(smartctl_ctx *ctx, const char *device, char **out_json);

/**
 * smartctl_check_health — checks SMART overall-health assessment.
 * On success sets *out_healthy to 1 (passed) or 0 (failed/unknown) and returns 0.
 * Returns non-zero on error; call smartctl_last_error() for details.
 */
int smartctl_check_health(smartctl_ctx *ctx, const char *device, int *out_healthy);

/**
 * smartctl_run_selftest — starts a SMART self-test.
 * test_type must be one of: "short", "long", "conveyance", "offline".
 * Returns 0 on success; non-zero on failure.
 */
int smartctl_run_selftest(smartctl_ctx *ctx, const char *device, const char *test_type);

/** smartctl_enable_smart — enables SMART on a device. Returns 0 on success. */
int smartctl_enable_smart(smartctl_ctx *ctx, const char *device);

/** smartctl_disable_smart — disables SMART on a device. Returns 0 on success. */
int smartctl_disable_smart(smartctl_ctx *ctx, const char *device);

/** smartctl_abort_selftest — aborts a running self-test. Returns 0 on success. */
int smartctl_abort_selftest(smartctl_ctx *ctx, const char *device);

/**
 * smartctl_last_error — returns the last error message for the context.
 * The returned pointer is valid until the next API call on ctx or until
 * smartctl_destroy() is called.  Do NOT call smartctl_free_string() on it.
 */
const char *smartctl_last_error(smartctl_ctx *ctx);
