/* SPDX-License-Identifier: GPL-2.0-or-later
 *
 * abi_check.c — standalone ABI-version gate for integration/core-go/run.sh.
 *
 * dlopen()s a built wrapper library directly (independent of Go/purego) and
 * checks smartmon_abi_version() against the major/minor this build of
 * bindings/go/backends/lib requires, applying the same three-tier semver
 * rule as checkABIVersion() in lib.go: major must match exactly, minor must
 * be >= required.
 *
 * Usage: abi_check <path-to-wrapper> <required-major> <required-minor>
 */
#include <dlfcn.h>
#include <stdio.h>
#include <stdlib.h>

int main(int argc, char **argv) {
    if (argc != 4) {
        fprintf(stderr, "usage: %s <wrapper-path> <required-major> <required-minor>\n", argv[0]);
        return 2;
    }

    const char *path = argv[1];
    unsigned required_major = (unsigned)strtoul(argv[2], NULL, 10);
    unsigned required_minor = (unsigned)strtoul(argv[3], NULL, 10);

    void *handle = dlopen(path, RTLD_LAZY | RTLD_LOCAL);
    if (!handle) {
        fprintf(stderr, "abi_check: dlopen(%s) failed: %s\n", path, dlerror());
        return 1;
    }

    dlerror();
    unsigned (*abi_version)(void) = (unsigned (*)(void))dlsym(handle, "smartmon_abi_version");
    const char *dlsym_err = dlerror();
    if (dlsym_err != NULL) {
        fprintf(stderr, "abi_check: missing smartmon_abi_version symbol: %s\n", dlsym_err);
        fprintf(stderr, "abi_check: this identifies a pre-monorepo wrapper build\n");
        dlclose(handle);
        return 1;
    }

    unsigned version = abi_version();
    unsigned major = (version >> 16) & 0xff;
    unsigned minor = (version >> 8) & 0xff;
    unsigned patch = version & 0xff;

    printf("abi_check: wrapper reports ABI %u.%u.%u\n", major, minor, patch);

    dlclose(handle);

    if (major != required_major || minor < required_minor) {
        fprintf(stderr,
                "abi_check: FAIL: incompatible ABI %u.%u.%u (required major=%u, minor>=%u)\n",
                major, minor, patch, required_major, required_minor);
        return 1;
    }

    printf("abi_check: OK: ABI %u.%u.%u satisfies required major=%u, minor>=%u\n",
           major, minor, patch, required_major, required_minor);
    return 0;
}
