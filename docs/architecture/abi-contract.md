# The C ABI contract

`native/capi/smartmon_c_api.h` is the only interface any binding is allowed
to depend on (see [dependency-rules.md](dependency-rules.md)). This document
is the contract that interface makes, and how it is enforced in practice.

## The symbol surface

Exactly 12 exported symbols:

```c
unsigned smartmon_abi_version(void);

int smartmon_init(void);
void smartmon_cleanup(void);

int smartmon_scan_devices(char **out_json);
int smartmon_get_smart_data(const char *device, const char *dev_type, char **out_json);
int smartmon_check_health(const char *device, const char *dev_type, int *out_healthy);
int smartmon_enable_smart(const char *device, const char *dev_type);
int smartmon_disable_smart(const char *device, const char *dev_type);
int smartmon_run_selftest(const char *device, const char *dev_type, const char *test_type);
int smartmon_abort_selftest(const char *device, const char *dev_type);

void smartmon_free_string(char *s);
const char *smartmon_last_error(void);
```

Functions that return JSON allocate a heap string into `*out_json`; the
caller must free it with `smartmon_free_string`. All fallible functions
return `0` on success and a non-zero code on failure, with the human-readable
detail retrievable via `smartmon_last_error()`.

## Versioning: three-tier semver

`smartmon_abi_version()` returns a `uint32` packed as
`(major << 16) | (minor << 8) | patch`. The three tiers have different
compatibility guarantees:

| Tier | Meaning | Binding obligation |
|---|---|---|
| **major** | A symbol was removed, renamed, or had its signature changed | Must match the binding's compiled-against major **exactly**. Any mismatch is incompatible. |
| **minor** | A symbol was added; nothing existing changed | The wrapper's minor must be **`>=`** what the binding requires. A newer wrapper is fine; an older one is missing something the binding needs. |
| **patch** | Internal behaviour changed, no symbol added/removed/resignatured | Unchecked. Cannot affect a correctly-written binding. |

This is deliberately **not** a simple "additive-only, never break" policy —
major version bumps are allowed and expected over the project's lifetime.
What the contract guarantees is that a break is *announced* via the major
number and *detectable at load time*, not discovered as a crash.

`bindings/go/backends/lib/lib.go` implements the check exactly this way:

```go
const (
    abiMajorRequired = 1
    abiMinorRequired = 0
)

func checkABIVersion(f *libFuncs) error {
    v := f.abiVersion()
    major := (v >> 16) & 0xff
    minor := (v >> 8) & 0xff
    patch := v & 0xff
    if major != abiMajorRequired || minor < abiMinorRequired {
        return fmt.Errorf("incompatible smartmon C ABI version %d.%d.%d ...")
    }
    return nil
}
```

called immediately after `registerFuncs` and before `smartmon_init`, so an
incompatible wrapper is rejected before any real work happens.

## Why this exists: `purego` does not validate signatures

`bindings/go`'s `LibBackend` loads the wrapper with `purego.Dlopen` and binds
each function with `purego.RegisterLibFunc(fptr, lib, name)`. That call
resolves a symbol **by name only** — it has no way to check that the C
function it finds actually matches the Go function pointer's signature.

This produces two very different failure modes:

- **A missing symbol** (e.g. a pre-monorepo wrapper build, which predates
  `smartmon_abi_version` entirely) fails loudly: `purego.RegisterLibFunc`
  panics, which `registerFunc` converts into a clear Go error naming the
  missing symbol.
- **A symbol that exists but has a different signature** — say, a future
  major version reorders parameters or changes a return type — binds
  **successfully**. The first call reads or writes memory according to the
  Go side's expectation of the layout, not the actual C function's. This is
  undefined behaviour: possibly a crash, possibly silent data corruption,
  never a clean error.

An earlier design draft (Go-repo PR #39) claimed that a mismatched ABI would
make the binding "silently fall back to the exec backend." That is
incorrect, and dangerously reassuring — there is no such fallback path, and
the actual failure mode is the undefined behaviour described above.
`smartmon_abi_version()` exists specifically because it is the **only**
mismatch `purego`'s binding mechanism can be made to catch: an explicit,
checkable value, tested before any other symbol is called.

## What changed at the monorepo boundary

Two artifacts previously lived across a repository boundary and are now
co-located, which is what makes the ABI version check meaningful rather than
just decorative:

- `native/capi/smartmon_c_api.{cpp,h}` (the wrapper's actual source) and
  `bindings/go/backends/lib/lib.go` (the code that calls it) are built from
  one commit, in one repository. There is no separate "SDK builds this from
  a pinned Go tag" step — that cross-repo build step *was* the circular
  dependency issue #17 was opened to resolve, and it no longer exists.
- The version pin that used to be a hand-maintained string
  (`SMARTMONTOOLS_GO_REF: v0.4.1`, duplicated in both repositories with no
  mechanism keeping them equal) is now a runtime-checked integer that the
  wrapper itself reports.
