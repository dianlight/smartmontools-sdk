// SPDX-License-Identifier: GPL-2.0-or-later

//go:build linux || darwin

// Package main demonstrates CompareBackend wired with two genuinely different
// backend implementations: ExecBackend (master) and LibBackend (secondary).
//
// ExecBackend shells out to the smartctl binary for every request.
// LibBackend loads libsmartmon_go.{so,dylib} via purego and calls the
// smartmontools C++ SDK directly — no child process is ever spawned.
//
// CompareBackend runs both in parallel for every request, uses the master's
// result as the response, and logs a warning whenever the two disagree.
// This is the primary production use-case: validating that a new backend
// implementation produces the same output as the battle-tested exec backend.
//
// # Prerequisites
//
// Build the native core and the wrapper once, from the monorepo root:
//
//	cd native && ./autogen.sh --force && mkdir -p build && cd build && \
//	  ../configure --with-devel=yes --with-pic && make -C include && make -C lib libsmartmon.la
//	scripts/build-wrapper.sh
//
// # Running
//
//	# macOS
//	SMARTMON_LIB_PATH=../../../../packaging/artifacts/lib/libsmartmon_go.dylib go run .
//
//	# Linux
//	SMARTMON_LIB_PATH=../../../../packaging/artifacts/lib/libsmartmon_go.so go run .
//
//	# Explicit flag (overrides env var and system search)
//	go run . -lib /path/to/libsmartmon_go.so
//
// # Interpreting output
//
// When both backends agree you will see no compare log lines.
// A "compare: result mismatch" warning means the two backends returned
// different data for the same request — investigate which one is wrong.
// A "compare: secondary backend error" error means LibBackend failed while
// ExecBackend succeeded — the master result is still returned to the caller.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	smartmontools "github.com/dianlight/smartmontools-sdk/bindings/go"
	comparebackend "github.com/dianlight/smartmontools-sdk/bindings/go/backends/compare"
	execbackend "github.com/dianlight/smartmontools-sdk/bindings/go/backends/exec"
	libbackend "github.com/dianlight/smartmontools-sdk/bindings/go/backends/lib"
	"github.com/dianlight/tlog"
	"github.com/fatih/color"
)

func main() {
	libPath := flag.String("lib", "", "explicit path to libsmartmon_go.{so,dylib} (overrides SMARTMON_LIB_PATH and system search)")
	flag.Parse()

	blue := color.New(color.FgBlue).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	fmt.Println(blue("Smartmontools CompareBackend: ExecBackend vs LibBackend"))
	fmt.Println(blue("========================================================="))
	fmt.Println()
	fmt.Println("Both backends query the same physical device in parallel.")
	fmt.Println("ExecBackend (master) shells out to smartctl.")
	fmt.Println("LibBackend (secondary) calls the smartmontools C++ SDK via purego.")
	fmt.Println("Any discrepancy between the two is logged as a warning.")
	fmt.Println()

	// Shared structured logger so both backends and the compare layer write
	// to the same output stream.
	logger := tlog.NewLoggerWithLevel(tlog.LevelInfo)

	// ── ExecBackend (master) ──────────────────────────────────────────────
	exec, err := execbackend.New(
		execbackend.WithTLogHandler(logger),
	)
	if err != nil {
		fmt.Println(red(fmt.Sprintf("✗ Failed to create exec backend: %v", err)))
		os.Exit(1)
	}
	fmt.Println(green(fmt.Sprintf("✓ ExecBackend ready (%s)", exec.Name())))

	// ── LibBackend (secondary) ────────────────────────────────────────────
	libOpts := []libbackend.Option{
		libbackend.WithTLogHandler(logger),
	}
	switch {
	case *libPath != "":
		libOpts = append(libOpts, libbackend.WithLibraryPath(*libPath))
		fmt.Printf("  Using explicit library path: %s\n", blue(*libPath))
	case os.Getenv("SMARTMON_LIB_PATH") != "":
		fmt.Printf("  SMARTMON_LIB_PATH=%s\n", blue(os.Getenv("SMARTMON_LIB_PATH")))
	default:
		fmt.Println(yellow("  SMARTMON_LIB_PATH not set — searching standard system library paths"))
	}

	lib, err := libbackend.New(libOpts...)
	if err != nil {
		fmt.Println(red(fmt.Sprintf("✗ Failed to load smartmon wrapper: %v", err)))
		fmt.Println()
		fmt.Println("Build the native core and the wrapper with (from the monorepo root):")
		fmt.Println("  scripts/build-wrapper.sh")
		fmt.Println("Then set SMARTMON_LIB_PATH or pass -lib /path/to/libsmartmon_go.so")
		os.Exit(1)
	}
	defer func() {
		if err := lib.Close(); err != nil {
			tlog.Warn("Failed to close lib backend", "error", err)
		}
	}()
	fmt.Println(green(fmt.Sprintf("✓ LibBackend ready (%s)", lib.Name())))
	fmt.Println()

	// ── CompareBackend ────────────────────────────────────────────────────
	// ExecBackend is the master: its results are always returned to the caller.
	// LibBackend is the secondary: its results are compared silently in the
	// background; mismatches and errors are written to the logger.
	compare, err := comparebackend.NewCompareBackend(
		[]smartmontools.Backend{exec, lib},
		comparebackend.WithTLogHandler(logger),
	)
	if err != nil {
		fmt.Println(red(fmt.Sprintf("✗ Failed to create compare backend: %v", err)))
		os.Exit(1)
	}
	defer func() {
		if err := compare.Close(); err != nil {
			tlog.Warn("Failed to close compare backend", "error", err)
		}
	}()

	fmt.Printf("Active: %s (master) vs %s (secondary)\n\n",
		blue(exec.Name()), blue(lib.Name()))
	fmt.Println(green("✓ CompareBackend ready"))
	fmt.Println()

	// Wire the compare backend into the standard client API.
	client, err := smartmontools.NewClient(smartmontools.WithBackend(compare))
	if err != nil {
		tlog.Fatal("Failed to create client", "error", err)
	}

	ctx := context.Background()

	// ── Device discovery ──────────────────────────────────────────────────
	fmt.Println(blue("Scanning for devices (both backends run in parallel)..."))
	devices, err := client.ScanDevices(ctx)
	if err != nil {
		fmt.Println(yellow(fmt.Sprintf("Warning: %v", err)))
		devices = []smartmontools.Device{{Name: "/dev/sda", Type: "auto"}}
		fmt.Println("Falling back to /dev/sda")
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

	devicePath := devices[0].Name
	fmt.Printf("Using device: %s\n\n", blue(devicePath))

	// ── Health check ──────────────────────────────────────────────────────
	fmt.Println(blue("Checking device health (exec master, lib shadow)..."))
	healthy, err := client.CheckHealth(ctx, devicePath)
	if err != nil {
		fmt.Println(yellow(fmt.Sprintf("Warning: %v", err)))
	} else if healthy {
		fmt.Println(green("✓ Health: PASSED"))
	} else {
		fmt.Println(red("✗ Health: FAILED"))
	}
	fmt.Println()

	// ── SMART information ─────────────────────────────────────────────────
	fmt.Println(blue("Getting SMART information (exec master, lib shadow)..."))
	smartInfo, err := client.GetSMARTInfo(ctx, devicePath)
	if err != nil {
		fmt.Println(yellow(fmt.Sprintf("Warning: %v", err)))
	} else {
		fmt.Printf("  Model:     %s\n", smartInfo.ModelName)
		fmt.Printf("  Serial:    %s\n", smartInfo.SerialNumber)
		fmt.Printf("  Firmware:  %s\n", smartInfo.Firmware)
		if smartInfo.DiskType != "" {
			fmt.Printf("  Disk Type: %s\n", smartInfo.DiskType)
		}
		if smartInfo.Temperature != nil {
			fmt.Printf("  Temp:      %d°C\n", smartInfo.Temperature.Current)
		}
		if smartInfo.PowerOnTime != nil {
			fmt.Printf("  Power-on:  %d hours\n", smartInfo.PowerOnTime.Hours)
		}
		if smartInfo.AtaSmartData != nil && len(smartInfo.AtaSmartData.Table) > 0 {
			fmt.Println("\n  Key SMART Attributes (ID 5, 9, 194, 197, 198):")
			for _, attr := range smartInfo.AtaSmartData.Table {
				switch attr.ID {
				case 5, 9, 194, 197, 198:
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

	fmt.Println(green("✓ Compare example completed"))
	fmt.Println()
	fmt.Println("  No 'compare:' log lines above → exec and lib backends agreed on all results.")
	fmt.Println("  'compare: result mismatch' → the two backends returned different data.")
	fmt.Println("  'compare: secondary backend error' → lib backend failed; exec result was used.")
}
