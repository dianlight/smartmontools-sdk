/*
 * libsmartctl.cpp — C API wrapper over smartmontools internals.
 *
 * Built only when BUILDING_LIBSMARTCTL is defined (set by the shared-library
 * configure target).  This file must be compiled as C++ because it includes
 * smartmontools C++ headers.
 */

#include "libsmartctl.h"

#include <cstdlib>
#include <cstring>
#include <string>
#include <sstream>

/* smartmontools internal headers */
#include "smartctl.h"
#include "ataprint.h"
#include "nvmeprint.h"
#include "scsiprint.h"
#include "dev_interface.h"
#include "json.h"

/* ------------------------------------------------------------------ */
/* Internal context structure                                           */
/* ------------------------------------------------------------------ */

struct smartctl_ctx {
    std::string device_type; /* overrides auto-detected protocol */
    std::string last_error;  /* populated on every failed call    */

    /* Options set via smartctl_set_option(). */
    int timeout_seconds;

    smartctl_ctx() : device_type("auto"), timeout_seconds(30) {}
};

/* ------------------------------------------------------------------ */
/* Internal helpers                                                     */
/* ------------------------------------------------------------------ */

/* capture_json_output — runs fn() with JSON output redirected to a string.
 * Returns the captured JSON.  Any exception from fn() is caught and
 * stored in ctx->last_error; the returned string will be empty on error. */
static std::string capture_json_output(smartctl_ctx *ctx,
                                       std::function<void()> fn)
{
    json jglob;
    jglob.set_enabled(true);

    /* Redirect the library's global JSON object for this call. */
    json *prev = set_json_globals(&jglob);

    std::string result;
    try {
        fn();
        std::ostringstream oss;
        jglob.print(oss, /*pretty=*/false);
        result = oss.str();
    } catch (const std::exception &e) {
        ctx->last_error = std::string("exception: ") + e.what();
    } catch (...) {
        ctx->last_error = "unknown exception in smartmontools";
    }

    set_json_globals(prev);
    return result;
}

/* set_error — stores a formatted error message in ctx. */
static int set_error(smartctl_ctx *ctx, const std::string &msg)
{
    if (ctx) ctx->last_error = msg;
    return -1;
}

/* ------------------------------------------------------------------ */
/* C API implementation                                                 */
/* ------------------------------------------------------------------ */

extern "C" {

int smartctl_abi_version(void)
{
    return LIBSMARTCTL_ABI_VERSION;
}

int smartctl_init(smartctl_ctx **ctx)
{
    if (!ctx) return -1;
    try {
        *ctx = new smartctl_ctx();
        return 0;
    } catch (...) {
        *ctx = nullptr;
        return -1;
    }
}

void smartctl_destroy(smartctl_ctx *ctx)
{
    delete ctx;
}

int smartctl_set_option(smartctl_ctx *ctx, const char *key, const char *value)
{
    if (!ctx || !key || !value) return -1;
    try {
        std::string k(key);
        std::string v(value);
        if (k == "device_type") {
            ctx->device_type = v;
            return 0;
        }
        if (k == "timeout_seconds") {
            ctx->timeout_seconds = std::stoi(v);
            return 0;
        }
        return set_error(ctx, "unknown option: " + k);
    } catch (...) {
        return set_error(ctx, "exception in smartctl_set_option");
    }
}

int smartctl_scan_devices(smartctl_ctx *ctx, char **out_json)
{
    if (!ctx || !out_json) return -1;
    try {
        std::string json_str = capture_json_output(ctx, [&]() {
            /* smartctl --scan is implemented via the device factory */
            smart_device_list devlist;
            get_all_devices_of_type(devlist, ctx->device_type.c_str(), true);

            json &j = *get_current_json();
            json::ref jdevs = j["devices"];
            jdevs.set_array();
            for (unsigned i = 0; i < devlist.size(); i++) {
                const smart_device *d = devlist.at(i);
                json::ref jd = jdevs[i];
                jd["name"].set_cstr(d->get_dev_name());
                jd["type"].set_cstr(d->get_dev_type());
            }
        });
        if (json_str.empty())
            return -1;
        *out_json = strdup(json_str.c_str());
        return *out_json ? 0 : -1;
    } catch (...) {
        return set_error(ctx, "exception in smartctl_scan_devices");
    }
}

int smartctl_get_smart_data(smartctl_ctx *ctx, const char *device,
                            char **out_json)
{
    if (!ctx || !device || !out_json) return -1;
    try {
        std::string json_str = capture_json_output(ctx, [&]() {
            /* Mirrors what smartctl -a -j does internally */
            smart_device::ptr dev =
                smi()->get_smart_device(device, ctx->device_type.c_str());
            if (!dev)
                throw std::runtime_error(std::string("cannot open ") + device);

            dev->open();
            ataPrintSmartValues(dev.get());
            nvmePrintAll(dev.get());
        });
        if (json_str.empty())
            return -1;
        *out_json = strdup(json_str.c_str());
        return *out_json ? 0 : -1;
    } catch (const std::exception &e) {
        return set_error(ctx, e.what());
    } catch (...) {
        return set_error(ctx, "exception in smartctl_get_smart_data");
    }
}

int smartctl_check_health(smartctl_ctx *ctx, const char *device,
                          int *out_healthy)
{
    if (!ctx || !device || !out_healthy) return -1;
    try {
        smart_device::ptr dev =
            smi()->get_smart_device(device, ctx->device_type.c_str());
        if (!dev)
            return set_error(ctx, std::string("cannot open ") + device);

        dev->open();
        int status = ataSmartStatus2(dev.get());
        if (status < 0) {
            ctx->last_error = "health check failed";
            return -1;
        }
        /* status == 0 means PASSED */
        *out_healthy = (status == 0) ? 1 : 0;
        return 0;
    } catch (const std::exception &e) {
        return set_error(ctx, e.what());
    } catch (...) {
        return set_error(ctx, "exception in smartctl_check_health");
    }
}

int smartctl_run_selftest(smartctl_ctx *ctx, const char *device,
                          const char *test_type)
{
    if (!ctx || !device || !test_type) return -1;
    try {
        smart_device::ptr dev =
            smi()->get_smart_device(device, ctx->device_type.c_str());
        if (!dev)
            return set_error(ctx, std::string("cannot open ") + device);

        dev->open();
        int rc = ataSmartSelfTest(dev.get(), test_type);
        if (rc != 0)
            return set_error(ctx, "self-test initiation failed");
        return 0;
    } catch (const std::exception &e) {
        return set_error(ctx, e.what());
    } catch (...) {
        return set_error(ctx, "exception in smartctl_run_selftest");
    }
}

int smartctl_enable_smart(smartctl_ctx *ctx, const char *device)
{
    if (!ctx || !device) return -1;
    try {
        smart_device::ptr dev =
            smi()->get_smart_device(device, ctx->device_type.c_str());
        if (!dev)
            return set_error(ctx, std::string("cannot open ") + device);
        dev->open();
        if (!ataEnableSmart(dev.get()))
            return set_error(ctx, "failed to enable SMART");
        return 0;
    } catch (...) {
        return set_error(ctx, "exception in smartctl_enable_smart");
    }
}

int smartctl_disable_smart(smartctl_ctx *ctx, const char *device)
{
    if (!ctx || !device) return -1;
    try {
        smart_device::ptr dev =
            smi()->get_smart_device(device, ctx->device_type.c_str());
        if (!dev)
            return set_error(ctx, std::string("cannot open ") + device);
        dev->open();
        if (!ataDisableSmart(dev.get()))
            return set_error(ctx, "failed to disable SMART");
        return 0;
    } catch (...) {
        return set_error(ctx, "exception in smartctl_disable_smart");
    }
}

int smartctl_abort_selftest(smartctl_ctx *ctx, const char *device)
{
    if (!ctx || !device) return -1;
    try {
        smart_device::ptr dev =
            smi()->get_smart_device(device, ctx->device_type.c_str());
        if (!dev)
            return set_error(ctx, std::string("cannot open ") + device);
        dev->open();
        if (!ataAbortSelfTest(dev.get()))
            return set_error(ctx, "failed to abort self-test");
        return 0;
    } catch (...) {
        return set_error(ctx, "exception in smartctl_abort_selftest");
    }
}

const char *smartctl_last_error(smartctl_ctx *ctx)
{
