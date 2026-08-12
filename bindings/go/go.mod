module github.com/dianlight/smartmontools-sdk/bindings/go/v8

go 1.26

retract (
	// Published 2026-08-11 in error: upstream smartmontools 8.0 is not yet
	// released, so this tree was (and, until AC_INIT's release date is set,
	// still is) a pre-release, and must not carry a final version. See
	// docs/development/release-process.md for the -pre.<revs> scheme this
	// tree now uses instead, and docs/migration/compatibility-matrix.md for
	// the retraction/tombstone history.
	v8.0.0
	// Retraction tombstone: contains no code changes relative to v8.0.0,
	// exists only to carry the retraction above. Self-retracting so that,
	// once both v8.0.0 and this version are excluded, `go get`'s @latest
	// query falls through to the newest v8.0.2-pre.N prerelease instead of
	// resolving to either burned final version. Do not delete this tag or
	// treat it as safe to skip — it is the mechanism, not a mistake.
	v8.0.1
)

require (
	github.com/dianlight/tlog v0.2.2
	github.com/ebitengine/purego v0.10.2
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/k0kubun/pp/v3 v3.5.0 // indirect
	github.com/lmittmann/tint v1.1.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/samber/lo v1.52.0 // indirect
	github.com/samber/slog-common v0.19.0 // indirect
	github.com/samber/slog-formatter v1.2.2 // indirect
	github.com/samber/slog-multi v1.7.0 // indirect
	gitlab.com/tozd/go/errors v0.10.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
