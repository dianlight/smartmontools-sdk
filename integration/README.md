# integration

End-to-end contracts spanning more than one tier of the monorepo — the tests
neither a standalone native-core repository nor a standalone bindings
repository could run on its own.

`core-go/` builds `native/` and the `native/capi/` wrapper, verifies the
12-symbol C ABI surface and `smartmon_abi_version()` compatibility, then runs
the Go bindings' `LibBackend` tests against the freshly built wrapper. See
[../docs/development/local-development.md](../docs/development/local-development.md#running-the-end-to-end-contract-test).
