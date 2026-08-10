# packaging

Build output staged for release tarballs. `packaging/artifacts/` is
gitignored — it's populated by `scripts/build-wrapper.sh` and consumed by
`.github/workflows/release-core.yml`, which packages it per target into
`libsmartmon-${VERSION}-${target}.tar.gz`.

See [../docs/development/release-process.md](../docs/development/release-process.md).
