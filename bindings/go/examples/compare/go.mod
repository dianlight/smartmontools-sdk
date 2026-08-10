module example_compare

go 1.26.0

replace github.com/dianlight/smartmontools-sdk/bindings/go => ../..

require (
	github.com/dianlight/smartmontools-sdk/bindings/go v0.0.0-00010101000000-000000000000
	github.com/dianlight/tlog v0.2.2
	github.com/fatih/color v1.18.0
)

require (
	github.com/k0kubun/pp/v3 v3.5.0 // indirect
	github.com/lmittmann/tint v1.1.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/samber/lo v1.52.0 // indirect
	github.com/samber/slog-common v0.19.0 // indirect
	github.com/samber/slog-formatter v1.2.2 // indirect
	github.com/samber/slog-multi v1.7.0 // indirect
	gitlab.com/tozd/go/errors v0.10.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
)
