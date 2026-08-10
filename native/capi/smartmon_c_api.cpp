// smartmon_c_api.cpp – CGO bridge implementation using libsmartmon.
//
// Uses the C++ API from dianlight/smartmontools-sdk to perform all SMART
// operations. The smart_interface global singleton is initialised once per
// process (std::call_once). All public functions are serialised through a
// global std::mutex because the smartmontools library is not thread-safe.
// The last-error string is a mutex-protected global, not thread_local:
// Go's purego backend calls run on goroutines that are not pinned to OS
// threads, so a thread_local set by one call could be read back from a
// different OS thread on the next call.

#include "smartmon_c_api.h"

#include <smartmon/dev_interface.h>
#include <smartmon/ata.h>
#include <smartmon/atacmds.h>
#include <smartmon/nvme.h>
#include <smartmon/nvmecmds.h>
#include <smartmon/byteorder.h>

#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <memory>
#include <mutex>
#include <string>

using namespace smartmon;

// ---------------------------------------------------------------------------
// Global / thread-local state
// ---------------------------------------------------------------------------

// Guards g_last_error only. Kept separate from g_mutex (which serialises SDK
// calls) so that error reporting never nests locks with the SDK call path.
static std::mutex g_error_mutex;
static std::string g_last_error;

// set_last_error/get_last_error replace a thread_local error string: Go's
// purego backend is not pinned to OS threads, so a goroutine can set an
// error on one OS thread and read it back on another. A thread_local would
// silently read back empty/stale state in that case; a mutex-protected
// global is visible to whichever OS thread reads it next.
static void set_last_error(const std::string & msg) {
    std::lock_guard<std::mutex> lk(g_error_mutex);
    g_last_error = msg;
}

static std::once_flag g_init_flag;
static bool           g_init_ok    = false;
static std::string    g_init_error;

// Serialises all calls into the smartmontools library.
static std::mutex g_mutex;

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// json_escape returns s with JSON special characters properly escaped.
static std::string json_escape(const std::string & s) {
    std::string r;
    r.reserve(s.size());
    for (unsigned char c : s) {
        switch (c) {
            case '"':  r += "\\\""; break;
            case '\\': r += "\\\\"; break;
            case '\n': r += "\\n";  break;
            case '\r': r += "\\r";  break;
            case '\t': r += "\\t";  break;
            default:
                if (c < 0x20) {
                    char buf[8];
                    snprintf(buf, sizeof(buf), "\\u%04x", (unsigned)c);
                    r += buf;
                } else {
                    r += static_cast<char>(c);
                }
        }
    }
    return r;
}

// fmt_ata_str converts a fixed-length ATA identity byte array (byte-swapped,
// space-padded) into a trimmed std::string using the SDK's formatter.
static std::string fmt_ata_str(const uint8_t * data, int len) {
    char buf[64] = {};
    ata_format_id_string(buf, data, len);
    std::string s(buf);
    while (!s.empty() && (s.back() == ' ' || s.back() == '\0'))
        s.pop_back();
    return s;
}

// fmt_nvme_str converts a fixed-length NVMe identity string (space-padded,
// NOT null-terminated) into a trimmed std::string.
static std::string fmt_nvme_str(const char * data, int len) {
    int end = len;
    while (end > 0 && (data[end - 1] == ' ' || data[end - 1] == '\0'))
        --end;
    return std::string(data, end);
}

// safe_type returns "" when t is NULL or empty, otherwise t.
static const char * safe_type(const char * t) {
    return (t && *t) ? t : "";
}

// open_dev opens a device with optional type hint; writes the last-error
// string on failure. Guards device and smi() explicitly: smi() is null if
// smartmon_init() was never called (or failed), and a caller across the C
// ABI boundary could pass a NULL device pointer.
static std::unique_ptr<smart_device> open_dev(const char * device, const char * dev_type) {
    if (!device) {
        set_last_error("NULL device pointer");
        return nullptr;
    }
    smart_interface * iface = smi();
    if (!iface) {
        set_last_error("smart_interface not initialised; call smartmon_init() first");
        return nullptr;
    }
    smart_device * raw = iface->get_smart_device(device, safe_type(dev_type));
    if (!raw) {
        set_last_error(iface->get_errmsg());
        return nullptr;
    }
    std::unique_ptr<smart_device> dev(raw);
    if (!smart_device::autodetect_open(dev)) {
        set_last_error(dev ? dev->get_errmsg() : iface->get_errmsg());
        return nullptr;
    }
    return dev;
}

// ---------------------------------------------------------------------------
// ATA JSON builder
// ---------------------------------------------------------------------------

static std::string build_ata_json(ata_device * ata, const char * devname, const char * devtype) {
    ata_identify_device      id     = {};
    ata_smart_values         sv     = {};
    ata_smart_thresholds_pvt thresh = {};
    ata_vendor_attr_defs     defs;

    // fix_swapped_id=false: ata_format_id_string() (used below by
    // fmt_ata_str) already performs the byte-pair swap on read. Passing
    // true here as well double-swaps model/serial/firmware back to their
    // original on-wire order — see native/lib/examples/lsdisk.cpp, the
    // SDK's own reference program, which pairs fix_swapped_id=false with
    // ata_format_id_string() the same way.
    if (ata_read_identity(ata, &id, /*fix_swapped_id=*/false) < 0) {
        set_last_error(ata->get_errmsg());
        return "";
    }
    if (ataReadSmartValues(ata, &sv) < 0) {
        set_last_error(std::string("failed to read SMART values for device ") + devname + ": " + ata->get_errmsg());
        return "";
    }
    // Some SAT/USB bridges and similar devices don't support the READ SMART
    // THRESHOLDS command at all; smartctl itself tolerates this and falls
    // back to zero-initialized thresholds (ata_get_attr_state() treats a
    // zero threshold entry as "no threshold data"), so a failure here must
    // not abort the whole read the way a genuine SMART VALUES failure does.
    if (ataReadSmartThresholds(ata, &thresh) < 0)
        thresh = ata_smart_thresholds_pvt();

    int  smart_status = ataSmartStatus2(ata);
    if (smart_status < 0) {
        set_last_error(std::string("failed to read SMART status for device ") + devname + ": " + ata->get_errmsg());
        return "";
    }
    int  rotation     = ata_get_rotation_rate(&id);
    bool avail        = (id.command_set_1 & 0x0001) != 0;
    bool enabled      = (id.cfs_enable_1  & 0x0001) != 0;
    int  temp_val     = static_cast<int>(ata_return_temperature_value(&sv, defs));

    // Extract power-on hours (attr 9) and power-cycle count (attr 12).
    int64_t poh = 0, pcc = 0;
    for (int i = 0; i < NUMBER_ATA_SMART_ATTRIBUTES; i++) {
        const auto & a = sv.vendor_attributes[i];
        if (a.id == 9)  poh = static_cast<int64_t>(ata_get_attr_raw_value(a, defs));
        if (a.id == 12) pcc = static_cast<int64_t>(ata_get_attr_raw_value(a, defs));
    }

    std::string j;
    j.reserve(8192);

    j += "{\"device\":{\"name\":\"" + json_escape(devname) + "\",\"type\":\"" +
         json_escape(devtype) + "\"}";
    j += ",\"model_name\":\""       + json_escape(fmt_ata_str(id.model,     40)) + "\"";
    j += ",\"serial_number\":\""    + json_escape(fmt_ata_str(id.serial_no, 20)) + "\"";
    j += ",\"firmware_version\":\"" + json_escape(fmt_ata_str(id.fw_rev,     8)) + "\"";

    if (rotation >= 0)
        j += ",\"rotation_rate\":" + std::to_string(rotation);

    j += ",\"smart_status\":{\"passed\":" +
         std::string(smart_status == 1 ? "true" : "false") + "}";
    j += ",\"smart_support\":{\"available\":" + std::string(avail   ? "true" : "false") +
         ",\"enabled\":"                      + std::string(enabled ? "true" : "false") + "}";

    if (temp_val > 0)
        j += ",\"temperature\":{\"current\":" + std::to_string(temp_val) + "}";
    if (poh > 0)
        j += ",\"power_on_time\":{\"hours\":" + std::to_string(poh) + "}";
    if (pcc > 0)
        j += ",\"power_cycle_count\":" + std::to_string(pcc);

    // ata_smart_data ----------------------------------------------------------
    j += ",\"ata_smart_data\":{";

    j += "\"capabilities\":{"
         "\"self_tests_supported\":"          +
         std::string(isSupportSelfTest(&sv)          ? "true" : "false") +
         ",\"conveyance_self_test_supported\":" +
         std::string(isSupportConveyanceSelfTest(&sv) ? "true" : "false") + "}";

    j += ",\"self_test\":{\"polling_minutes\":{"
         "\"short\":"      + std::to_string(TestTime(&sv, SHORT_SELF_TEST))      +
         ",\"extended\":"  + std::to_string(TestTime(&sv, EXTEND_SELF_TEST))     +
         ",\"conveyance\":" + std::to_string(TestTime(&sv, CONVEYANCE_SELF_TEST)) +
         "}}";  // closes polling_minutes and self_test

    j += ",\"table\":[";
    bool first_attr = true;
    for (int i = 0; i < NUMBER_ATA_SMART_ATTRIBUTES; i++) {
        const auto & attr = sv.vendor_attributes[i];
        if (attr.id == 0)
            continue;

        unsigned char threshval = 0;
        auto state = ata_get_attr_state(attr, i, thresh.thres_entries, defs, &threshval);

        uint16_t flags_val = uile16_to_uint(attr.flags);
        uint64_t raw_val   = ata_get_attr_raw_value(attr, defs);
        std::string attr_name = ata_get_smart_attr_name(attr.id, defs, rotation > 0 ? rotation : 0);
        std::string raw_str   = ata_format_attr_raw_value(attr, defs);

        const char * when_failed = "";
        if (state == ATTRSTATE_FAILED_NOW)  when_failed = "now";
        else if (state == ATTRSTATE_FAILED_PAST) when_failed = "past";

        if (!first_attr) j += ",";
        first_attr = false;

        j += "{\"id\":"     + std::to_string(static_cast<int>(attr.id));
        j += ",\"name\":\"" + json_escape(attr_name) + "\"";
        j += ",\"value\":"  + std::to_string(static_cast<int>(attr.current));
        j += ",\"worst\":"  + std::to_string(static_cast<int>(attr.worst));
        j += ",\"thresh\":" + std::to_string(static_cast<int>(threshval));
        if (*when_failed)
            j += ",\"when_failed\":\"" + std::string(when_failed) + "\"";

        j += ",\"flags\":{"
             "\"value\":"        + std::to_string(static_cast<int>(flags_val)) +
             ",\"string\":\"\""
             ",\"prefailure\":"    + std::string(ATTRIBUTE_FLAGS_PREFAILURE(flags_val)    ? "true" : "false") +
             ",\"updated_online\":" + std::string(ATTRIBUTE_FLAGS_ONLINE(flags_val)       ? "true" : "false") +
             ",\"performance\":"  + std::string(ATTRIBUTE_FLAGS_PERFORMANCE(flags_val)   ? "true" : "false") +
             ",\"error_rate\":"   + std::string(ATTRIBUTE_FLAGS_ERRORRATE(flags_val)     ? "true" : "false") +
             ",\"event_count\":"  + std::string(ATTRIBUTE_FLAGS_EVENTCOUNT(flags_val)    ? "true" : "false") +
             ",\"auto_keep\":"    + std::string(ATTRIBUTE_FLAGS_SELFPRESERVING(flags_val) ? "true" : "false") +
             "}";

        j += ",\"raw\":{\"value\":" + std::to_string(static_cast<int64_t>(raw_val)) +
             ",\"string\":\"" + json_escape(raw_str) + "\"}}";
    }
    j += "]";  // closes table
    j += "}";  // closes ata_smart_data
    j += "}";  // closes root object
    return j;
}

// ---------------------------------------------------------------------------
// NVMe JSON builder
// ---------------------------------------------------------------------------

static std::string build_nvme_json(nvme_device * nvme, const char * devname) {
    nvme_id_ctrl   id_ctrl   = {};
    nvme_smart_log smart_log = {};

    if (!nvme_read_id_ctrl(nvme, id_ctrl)) {
        set_last_error(nvme->get_errmsg());
        return "";
    }
    if (!nvme_read_smart_log(nvme, 0xFFFFFFFF, smart_log)) {
        set_last_error(nvme->get_errmsg());
        return "";
    }

    bool passed  = (smart_log.critical_warning == 0);
    uint16_t temp_k = uile16_to_uint(smart_log.temperature);
    int temp_c = (temp_k > 273) ? static_cast<int>(temp_k - 273) : 0;

    uint64_t poh              = uile128_clamp_to_uint64(smart_log.power_on_hours);
    uint64_t pcc              = uile128_clamp_to_uint64(smart_log.power_cycles);
    uint64_t data_read        = uile128_clamp_to_uint64(smart_log.data_units_read);
    uint64_t data_written     = uile128_clamp_to_uint64(smart_log.data_units_written);
    uint64_t host_reads       = uile128_clamp_to_uint64(smart_log.host_reads);
    uint64_t host_writes      = uile128_clamp_to_uint64(smart_log.host_writes);
    uint64_t ctrl_busy        = uile128_clamp_to_uint64(smart_log.ctrl_busy_time);
    uint64_t unsafe_shutdowns = uile128_clamp_to_uint64(smart_log.unsafe_shutdowns);
    uint64_t media_errors     = uile128_clamp_to_uint64(smart_log.media_errors);
    uint64_t num_err_log      = uile128_clamp_to_uint64(smart_log.num_err_log_entries);

    std::string j;
    j.reserve(2048);

    j += "{\"device\":{\"name\":\"" + json_escape(devname) + "\",\"type\":\"nvme\"}";
    j += ",\"model_name\":\""       + json_escape(fmt_nvme_str(id_ctrl.mn, 40)) + "\"";
    j += ",\"serial_number\":\""    + json_escape(fmt_nvme_str(id_ctrl.sn, 20)) + "\"";
    j += ",\"firmware_version\":\"" + json_escape(fmt_nvme_str(id_ctrl.fr,  8)) + "\"";

    j += ",\"smart_status\":{\"passed\":" + std::string(passed ? "true" : "false") + "}";
    j += ",\"smart_support\":{\"available\":true,\"enabled\":true}";

    if (temp_c > 0)
        j += ",\"temperature\":{\"current\":" + std::to_string(temp_c) + "}";
    if (poh > 0)
        j += ",\"power_on_time\":{\"hours\":" + std::to_string(poh) + "}";
    if (pcc > 0)
        j += ",\"power_cycle_count\":" + std::to_string(pcc);

    j += ",\"nvme_smart_health_information_log\":{"
         "\"critical_warning\":"         + std::to_string(static_cast<int>(smart_log.critical_warning)) +
         ",\"temperature\":"             + std::to_string(temp_c) +
         ",\"available_spare\":"         + std::to_string(static_cast<int>(smart_log.avail_spare)) +
         ",\"available_spare_threshold\":" + std::to_string(static_cast<int>(smart_log.spare_thresh)) +
         ",\"percentage_used\":"         + std::to_string(static_cast<int>(smart_log.percent_used)) +
         ",\"data_units_read\":"         + std::to_string(data_read) +
         ",\"data_units_written\":"      + std::to_string(data_written) +
         ",\"host_read_commands\":"      + std::to_string(host_reads) +
         ",\"host_write_commands\":"     + std::to_string(host_writes) +
         ",\"controller_busy_time\":"    + std::to_string(ctrl_busy) +
         ",\"power_cycles\":"            + std::to_string(pcc) +
         ",\"power_on_hours\":"          + std::to_string(poh) +
         ",\"unsafe_shutdowns\":"        + std::to_string(unsafe_shutdowns) +
         ",\"media_errors\":"            + std::to_string(media_errors) +
         ",\"num_err_log_entries\":"     + std::to_string(num_err_log) +
         ",\"warning_temp_time\":"       + std::to_string(smart_log.warning_temp_time) +
         ",\"critical_comp_time\":"      + std::to_string(smart_log.critical_comp_time);

    // Temperature sensors: include only non-zero entries.
    bool first_sensor = true;
    for (int i = 0; i < 8; i++) {
        uint16_t sk = smart_log.temp_sensor[i];
        if (sk == 0) continue;
        if (first_sensor) { j += ",\"temperature_sensors\":["; first_sensor = false; }
        else j += ",";
        j += std::to_string(sk > 273 ? static_cast<int>(sk - 273) : 0);
    }
    if (!first_sensor) j += "]";

    j += "}";  // closes nvme_smart_health_information_log
    j += "}";  // closes root object
    return j;
}

// ---------------------------------------------------------------------------
// Public C API
// ---------------------------------------------------------------------------

unsigned smartmon_abi_version(void) {
    return (SMARTMON_ABI_VERSION_MAJOR << 16) |
           (SMARTMON_ABI_VERSION_MINOR << 8)  |
            SMARTMON_ABI_VERSION_PATCH;
}

int smartmon_init(void) {
    std::call_once(g_init_flag, []() {
        try {
            smart_interface::init();
            g_init_ok = true;
        } catch (const std::exception & e) {
            g_init_error = e.what();
        } catch (...) {
            g_init_error = "unknown error initialising smartmon interface";
        }
    });
    if (!g_init_ok)
        set_last_error(g_init_error);
    return g_init_ok ? 0 : -1;
}

void smartmon_cleanup(void) {
    // No-op: the smart_interface singleton lives for the process lifetime.
}

const char * smartmon_last_error(void) {
    // The returned pointer must stay valid until the next call on this
    // thread, so the global error is copied into a thread-local buffer
    // rather than returned directly — g_last_error itself must NOT be
    // thread_local, since the call that set it may run on a different OS
    // thread than the one reading it back (Go goroutines migrate threads).
    static thread_local std::string tl_error_copy;
    {
        std::lock_guard<std::mutex> lk(g_error_mutex);
        tl_error_copy = g_last_error;
    }
    return tl_error_copy.c_str();
}

void smartmon_free_string(char * s) {
    free(s);
}

int smartmon_scan_devices(char ** out_json) {
    if (!out_json) {
        set_last_error("NULL output pointer");
        return -1;
    }
    std::lock_guard<std::mutex> lk(g_mutex);
    try {
        smart_interface * iface = smi();
        if (!iface) {
            set_last_error("smart_interface not initialised; call smartmon_init() first");
            return -1;
        }
        smart_device_list devlist;
        // Use the public overload that takes a types vector; empty means "all".
        smart_devtype_list types;
        if (!iface->scan_smart_devices(devlist, types)) {
            set_last_error(iface->get_errmsg());
            return -1;
        }
        std::string j;
        j.reserve(512);
        j += "{\"devices\":[";
        for (unsigned i = 0; i < devlist.size(); i++) {
            if (i > 0) j += ",";
            const smart_device * dev = devlist.at(i);
            j += "{\"name\":\"" + json_escape(dev->get_dev_name()) + "\"" +
                 ",\"type\":\"" + json_escape(dev->get_dev_type()) + "\"}";
        }
        j += "]}";
        *out_json = strdup(j.c_str());
        return *out_json ? 0 : -1;
    } catch (const std::exception & e) {
        set_last_error(e.what());
        return -1;
    }
}

int smartmon_get_smart_data(const char * device, const char * dev_type, char ** out_json) {
    if (!out_json) {
        set_last_error("NULL output pointer");
        return -1;
    }
    std::lock_guard<std::mutex> lk(g_mutex);
    try {
        auto dev = open_dev(device, dev_type);
        if (!dev) return -1;

        std::string json;
        if (dev->is_ata()) {
            json = build_ata_json(dev->to_ata(), device, dev->get_dev_type());
        } else if (dev->is_nvme()) {
            json = build_nvme_json(dev->to_nvme(), device);
        } else {
            set_last_error(std::string("unsupported device type: ") + dev->get_dev_type());
            return -1;
        }

        if (json.empty()) return -1;  // g_last_error already set by builder

        *out_json = strdup(json.c_str());
        return *out_json ? 0 : -1;
    } catch (const std::exception & e) {
        set_last_error(e.what());
        return -1;
    }
}

int smartmon_check_health(const char * device, const char * dev_type, int * out_healthy) {
    if (!out_healthy) {
        set_last_error("NULL output pointer");
        return -1;
    }
    std::lock_guard<std::mutex> lk(g_mutex);
    try {
        auto dev = open_dev(device, dev_type);
        if (!dev) return -1;

        if (dev->is_ata()) {
            int rc = ataSmartStatus2(dev->to_ata());
            if (rc < 0) {
                set_last_error(dev->get_errmsg());
                return -1;
            }
            *out_healthy = (rc == 1) ? 1 : 0;
        } else if (dev->is_nvme()) {
            nvme_smart_log sl = {};
            if (!nvme_read_smart_log(dev->to_nvme(), 0xFFFFFFFF, sl)) {
                set_last_error(dev->get_errmsg());
                return -1;
            }
            *out_healthy = (sl.critical_warning == 0) ? 1 : 0;
        } else {
            set_last_error(std::string("unsupported device type: ") + dev->get_dev_type());
            return -1;
        }
        return 0;
    } catch (const std::exception & e) {
        set_last_error(e.what());
        return -1;
    }
}

int smartmon_enable_smart(const char * device, const char * dev_type) {
    std::lock_guard<std::mutex> lk(g_mutex);
    try {
        auto dev = open_dev(device, dev_type);
        if (!dev) return -1;
        if (!dev->is_ata()) {
            set_last_error("enable SMART is only supported on ATA devices");
            return -1;
        }
        if (ataEnableSmart(dev->to_ata()) < 0) {
            set_last_error(dev->get_errmsg());
            return -1;
        }
        return 0;
    } catch (const std::exception & e) {
        set_last_error(e.what());
        return -1;
    }
}

int smartmon_disable_smart(const char * device, const char * dev_type) {
    std::lock_guard<std::mutex> lk(g_mutex);
    try {
        auto dev = open_dev(device, dev_type);
        if (!dev) return -1;
        if (!dev->is_ata()) {
            set_last_error("disable SMART is only supported on ATA devices");
            return -1;
        }
        if (ataDisableSmart(dev->to_ata()) < 0) {
            set_last_error(dev->get_errmsg());
            return -1;
        }
        return 0;
    } catch (const std::exception & e) {
        set_last_error(e.what());
        return -1;
    }
}

int smartmon_run_selftest(const char * device, const char * dev_type, const char * test_type) {
    std::lock_guard<std::mutex> lk(g_mutex);
    try {
        if (!test_type) {
            set_last_error("NULL test type");
            return -1;
        }

        auto dev = open_dev(device, dev_type);
        if (!dev) return -1;

        if (dev->is_ata()) {
            ata_smart_values sv = {};
            if (ataReadSmartValues(dev->to_ata(), &sv) < 0) {
                set_last_error(std::string("failed to read SMART values for device ") + device + ": " + dev->get_errmsg());
                return -1;
            }

            int testtype;
            if (strcmp(test_type, "short") == 0)
                testtype = SHORT_SELF_TEST;
            else if (strcmp(test_type, "long") == 0)
                testtype = EXTEND_SELF_TEST;
            else if (strcmp(test_type, "conveyance") == 0)
                testtype = CONVEYANCE_SELF_TEST;
            else {
                set_last_error(std::string("unknown test type: ") + test_type);
                return -1;
            }
            if (ataSmartTest(dev->to_ata(), testtype, /*force=*/false,
                             ata_selective_selftest_args{}, &sv, /*num_sectors=*/0) < 0) {
                set_last_error(dev->get_errmsg());
                return -1;
            }
        } else if (dev->is_nvme()) {
            uint8_t stc;
            if (strcmp(test_type, "short") == 0)
                stc = 1;
            else if (strcmp(test_type, "long") == 0)
                stc = 2;
            else {
                set_last_error(std::string("NVMe does not support test type: ") + test_type);
                return -1;
            }
            if (!nvme_self_test(dev->to_nvme(), stc, 0)) {
                set_last_error(dev->get_errmsg());
                return -1;
            }
        } else {
            set_last_error(std::string("unsupported device type: ") + dev->get_dev_type());
            return -1;
        }
        return 0;
    } catch (const std::exception & e) {
        set_last_error(e.what());
        return -1;
    }
}

int smartmon_abort_selftest(const char * device, const char * dev_type) {
    std::lock_guard<std::mutex> lk(g_mutex);
    try {
        auto dev = open_dev(device, dev_type);
        if (!dev) return -1;

        if (dev->is_ata()) {
            ata_smart_values sv = {};
            if (ataReadSmartValues(dev->to_ata(), &sv) < 0) {
                set_last_error(std::string("failed to read SMART values for device ") + device + ": " + dev->get_errmsg());
                return -1;
            }
            if (ataSmartTest(dev->to_ata(), ABORT_SELF_TEST, /*force=*/false,
                             ata_selective_selftest_args{}, &sv, /*num_sectors=*/0) < 0) {
                set_last_error(dev->get_errmsg());
                return -1;
            }
        } else if (dev->is_nvme()) {
            if (!nvme_self_test(dev->to_nvme(), 0x0f, 0)) {  // 0x0f = abort
                set_last_error(dev->get_errmsg());
                return -1;
            }
        } else {
            set_last_error(std::string("unsupported device type: ") + dev->get_dev_type());
            return -1;
        }
        return 0;
    } catch (const std::exception & e) {
        set_last_error(e.what());
        return -1;
    }
}
