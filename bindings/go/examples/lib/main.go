// SPDX-License-Identifier: GPL-2.0-or-later

//go:build linux || darwin

// Package main demonstrates using the LibBackend (SDK) that loads the smartmon
// wrapper library at runtime via purego — no CGO required.
//
// Build the native core and the wrapper first (from the monorepo root):
//
//	cd native && ./autogen.sh --force && mkdir -p build && cd build && \
//	  ../configure --with-devel=yes --with-pic && make -C include && make -C lib libsmartmon.la
//	scripts/build-wrapper.sh
//
// Run the example with automatic library resolution (from repository root):
//
//	SMARTMON_LIB_PATH=packaging/artifacts/lib/libsmartmon_go.dylib go run examples/lib  # macOS
//	SMARTMON_LIB_PATH=packaging/artifacts/lib/libsmartmon_go.so    go run examples/lib  # Linux
//
// When SMARTMON_LIB_PATH is not set New() searches the standard system library
// paths (LD_LIBRARY_PATH / DYLD_LIBRARY_PATH, /usr/local/lib, etc.).
// If SMARTMON_LIB_PATH points to a missing file a warning is logged and the
// system search is used as a fallback.
//
// Pass an explicit path to bypass all automatic resolution:
//
//	go run . -lib /path/to/libsmartmon_go.so
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/dianlight/tlog"
	"github.com/fatih/color"

	smartmontools "github.com/dianlight/smartmontools-sdk/bindings/go"
	libbackend "github.com/dianlight/smartmontools-sdk/bindings/go/backends/lib"
)

func main() {
	libPath := flag.String("lib", "", "explicit path to libsmartmon_go.{so,dylib} (overrides SMARTMON_LIB_PATH and system search)")
	flag.Parse()

	blue := color.New(color.FgBlue).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	fmt.Println(blue("Smartmontools LibBackend (SDK) Example"))
	fmt.Println(blue("======================================"))
	fmt.Println()

	// Build options.  WithTLogHandler wires up structured logging so that any
	// warnings emitted by New() (e.g. SMARTMON_LIB_PATH missing, duplicate
	// system library detected) are visible on the terminal.
	libOpts := []libbackend.Option{
		libbackend.WithTLogHandler(tlog.NewLoggerWithLevel(tlog.LevelInfo)),
	}

	switch {
	case *libPath != "":
		// Explicit -lib flag: bypass all automatic resolution.
		libOpts = append(libOpts, libbackend.WithLibraryPath(*libPath))
		fmt.Printf("Using explicit library path: %s\n\n", blue(*libPath))
	case os.Getenv("SMARTMON_LIB_PATH") != "":
		// SMARTMON_LIB_PATH is set; New() will handle it automatically,
		// including a fallback if the file is missing.
		fmt.Printf("SMARTMON_LIB_PATH=%s (handled by New)\n\n",
			blue(os.Getenv("SMARTMON_LIB_PATH")))
	default:
		fmt.Println(yellow("SMARTMON_LIB_PATH not set – searching standard system library paths"))
		fmt.Println()
	}

	// Create the LibBackend.  This dlopen()s the shared library and initialises
	// the smartmontools singleton; no child process is spawned.
	// New() resolves the library path in the order:
	//   WithLibraryPath > SMARTMON_LIB_PATH (with fallback) > system search.
	lib, err := libbackend.New(libOpts...)
	if err != nil {
		fmt.Println(red(fmt.Sprintf("✗ Failed to load smartmon wrapper: %v", err)))
		fmt.Println()
		fmt.Println("Build the native core and the wrapper with (from the monorepo root):")
		fmt.Println("  scripts/build-wrapper.sh")
		fmt.Println("Then set SMARTMON_LIB_PATH or copy to a standard library directory.")
		os.Exit(1)
	}
	defer func() {
		if err := lib.Close(); err != nil {
			tlog.Warn("Failed to close lib backend", "error", err)
		}
	}()

	fmt.Println(green("✓ Wrapper library loaded successfully"))
	fmt.Println()

	// Wire the LibBackend into the high-level smartmontools client.
	client, err := smartmontools.NewClient(smartmontools.WithBackend(lib))
	if err != nil {
		tlog.Fatal("Failed to create client", "error", err)
	}

	ctx := context.Background()

	// ── Device discovery ──────────────────────────────────────────────────
	fmt.Println(blue("Scanning for devices..."))
	devices, err := client.ScanDevices(ctx)
	if err != nil {
		fmt.Println(yellow(fmt.Sprintf("Warning: %v", err)))
		fallbackDevice := "/dev/sda"
		switch runtime.GOOS {
		case "darwin":
			fallbackDevice = "/dev/disk0"
		case "linux":
			fallbackDevice = "/dev/sda"
		default:
			fallbackDevice = "/dev/sda"
		}
		devices = []smartmontools.Device{{Name: fallbackDevice, Type: "auto"}}
		fmt.Printf("Falling back to %s\n", fallbackDevice)
	}
	if len(devices) == 0 {
		fmt.Println(red("No devices found. Ensure you have sufficient permissions (e.g. sudo)."))
		os.Exit(1)
	}

	fmt.Printf("Found %s device(s):\n", green(fmt.Sprintf("%d", len(devices))))
	for i, d := range devices {
		fmt.Printf("  %d. %s (type: %s)\n", i+1, d.Name, d.Type)
	}
	fmt.Println()

	// Use the first device for the rest of the demo.
	devicePath := devices[0].Name
	fmt.Printf("Using device: %s\n\n", blue(devicePath))

	// ── Health check ──────────────────────────────────────────────────────
	fmt.Println(blue("Checking device health..."))
	healthy, err := client.CheckHealth(ctx, devicePath)
	if err != nil {
		fmt.Println(yellow(fmt.Sprintf("Warning: %v", err)))
	} else if healthy {
		fmt.Println(green("✓ Health: PASSED"))
	} else {
		fmt.Println(red("✗ Health: FAILED"))
	}
	fmt.Println()

	// ── SMART support ─────────────────────────────────────────────────────
	fmt.Println(blue("Checking SMART support..."))
	support, err := client.IsSMARTSupported(ctx, devicePath)
	if err != nil {
		fmt.Println(yellow(fmt.Sprintf("Warning: %v", err)))
	} else {
		if support.Available {
			fmt.Println(green("✓ SMART available"))
		} else {
			fmt.Println(red("✗ SMART not available"))
		}
		if support.Enabled {
			fmt.Println(green("✓ SMART enabled"))
		} else {
			fmt.Println(red("✗ SMART disabled"))
		}
	}
	fmt.Println()

	// ── Device information ────────────────────────────────────────────────
	fmt.Println(blue("Getting device information..."))
	devInfo, err := client.GetDeviceInfo(ctx, devicePath)
	if err != nil {
		fmt.Println(yellow(fmt.Sprintf("Warning: %v", err)))
	} else {
		if v, ok := devInfo["model_name"].(string); ok {
			fmt.Printf("  Model:    %s\n", v)
		}
		if v, ok := devInfo["serial_number"].(string); ok {
			fmt.Printf("  Serial:   %s\n", v)
		}
		if v, ok := devInfo["firmware_version"].(string); ok {
			fmt.Printf("  Firmware: %s\n", v)
		}
	}
	fmt.Println()

	// ── Full SMART data ───────────────────────────────────────────────────
	fmt.Println(blue("Getting SMART information..."))
	smartInfo, err := client.GetSMARTInfo(ctx, devicePath)
	if err != nil {
		fmt.Println(yellow(fmt.Sprintf("Warning: %v", err)))
	} else {
		fmt.Printf("  Model:      %s\n", smartInfo.ModelName)
		fmt.Printf("  Serial:     %s\n", smartInfo.SerialNumber)
		fmt.Printf("  Firmware:   %s\n", smartInfo.Firmware)
		if smartInfo.DiskType != "" {
			fmt.Printf("  Disk Type:  %s\n", smartInfo.DiskType)
		}
		if smartInfo.RotationRate != nil {
			if *smartInfo.RotationRate > 0 {
				fmt.Printf("  RPM:        %d\n", *smartInfo.RotationRate)
			} else {
				fmt.Println("  RPM:        0 (non-rotating)")
			}
		}
		if smartInfo.Temperature != nil {
			fmt.Printf("  Temp:       %d°C\n", smartInfo.Temperature.Current)
		}
		if smartInfo.PowerOnTime != nil {
			fmt.Printf("  Power-on:   %d hours\n", smartInfo.PowerOnTime.Hours)
		}
		if smartInfo.AtaSmartData != nil && len(smartInfo.AtaSmartData.Table) > 0 {
			fmt.Println("\n  Key SMART Attributes (ID 5, 9, 194, 197, 198):")
			for _, attr := range smartInfo.AtaSmartData.Table {
				if attr.ID == 5 || attr.ID == 9 || attr.ID == 194 || attr.ID == 197 || attr.ID == 198 {
					fmt.Printf("    %3d %-30s value=%d worst=%d thresh=%d\n",
						attr.ID, attr.Name, attr.Value, attr.Worst, attr.Thresh)
				}
			}
		}
	}
	fmt.Println()

	// ── Available self-tests ──────────────────────────────────────────────
	fmt.Println(blue("Available self-tests:"))
	tests, err := client.GetAvailableSelfTests(ctx, devicePath)
	if err != nil {
		fmt.Println(yellow(fmt.Sprintf("Warning: %v", err)))
	} else if len(tests.Available) == 0 {
		fmt.Println("  None reported by device")
	} else {
		for _, name := range tests.Available {
			if dur := tests.Durations[name]; dur > 0 {
				fmt.Printf("  - %-12s (~%d min)\n", name, dur)
			} else {
				fmt.Printf("  - %s\n", name)
			}
		}
	}
	fmt.Println()

	fmt.Println(green("✓ LibBackend example completed successfully"))
}
