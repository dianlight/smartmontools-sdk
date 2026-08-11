# Import path migration

The Go module path changed when `smartmontools-go` moved into this monorepo
as `bindings/go/`:

| Old | New |
|---|---|
| `github.com/dianlight/smartmontools-go` | `github.com/dianlight/smartmontools-sdk/bindings/go/v8` |
| `github.com/dianlight/smartmontools-go/types` | `github.com/dianlight/smartmontools-sdk/bindings/go/v8/types` |
| `github.com/dianlight/smartmontools-go/backends/exec` | `github.com/dianlight/smartmontools-sdk/bindings/go/v8/backends/exec` |
| `github.com/dianlight/smartmontools-go/backends/lib` | `github.com/dianlight/smartmontools-sdk/bindings/go/v8/backends/lib` |
| `github.com/dianlight/smartmontools-go/backends/shadow` | `github.com/dianlight/smartmontools-sdk/bindings/go/v8/backends/shadow` |
| `github.com/dianlight/smartmontools-go/backends/compare` | `github.com/dianlight/smartmontools-sdk/bindings/go/v8/backends/compare` |

Every subpackage moved as a unit — only the module prefix changed, nothing
was renamed or restructured within the tree.

## `go.mod`

```
- require github.com/dianlight/smartmontools-go v0.4.1
+ require github.com/dianlight/smartmontools-sdk/bindings/go/v8 v8.0.0
```

There is no intermediate `v0.5.0` step to migrate through: that version was
never actually tagged (`git tag -l 'bindings/go/*'` is empty before this
scheme), so the first real release of this module is `bindings/go/v8.0.0`,
matching the native core's version under the release cascade described in
[../development/release-process.md](../development/release-process.md).

Tag prefix also changed: the old repository tagged bare `vX.Y.Z`; the new
module is tagged `bindings/go/vX.Y.Z` (required by Go's module-in-subdirectory
resolution rules — see [../bindings/go.md](../bindings/go.md#install)).

## Rewriting imports

A plain `sed` across your Go sources handles the common case:

```bash
grep -rl 'github.com/dianlight/smartmontools-go' --include='*.go' . | \
  xargs sed -i 's#github.com/dianlight/smartmontools-go#github.com/dianlight/smartmontools-sdk/bindings/go/v8#g'
```

On macOS, `sed -i` needs an explicit (empty) backup extension:

```bash
xargs sed -i '' 's#github.com/dianlight/smartmontools-go#github.com/dianlight/smartmontools-sdk/bindings/go/v8#g'
```

Then update `go.mod`/`go.sum` and re-run `gofmt` to fix import grouping,
since the new path sorts differently:

```bash
go mod tidy
gofmt -w -l $(grep -rl 'smartmontools-sdk/bindings/go' --include='*.go' .)
```

## What does *not* need a code change

The public API — `Client`, the `Backend` interface, `ExecBackend`,
`LibBackend`, `CompareBackend`, and every type under `types/` — is
byte-for-byte identical across the move. If your code compiles after the
import rewrite above, it is migrated. The one new runtime behaviour is the
`smartmon_abi_version()` check inside `LibBackend.New` (see
[../architecture/abi-contract.md](../architecture/abi-contract.md)); it only
matters if you also build and ship your own copy of the wrapper library.

See [compatibility-matrix.md](compatibility-matrix.md) for the exact version
correspondence.
