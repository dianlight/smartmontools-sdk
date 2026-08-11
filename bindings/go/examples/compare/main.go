// SPDX-License-Identifier: GPL-2.0-or-later

// Package main demonstrates the CompareBackend — a virtual backend that runs
// two or more backends in parallel and logs any discrepancies between their
// results. The first backend is the master: its output is always returned to
// the caller. Secondary backends are shadow-tested; mismatches produce a
// warning log and errors produce an error log.
//
// # Intended use
//
// CompareBackend is designed to compare *different implementations* of the
// Backend interface against the same device, for example:
//
//   - ExecBackend (master) vs LibBackend — validates exec/SDK parity
//   - ExecBackend v7 vs ExecBackend v8   — validates smartctl upgrade safety
//
// Do NOT pair two identical ExecBackend instances. Both would shell out to
// the same smartctl binary on the same physical device at the same time.
// Most OS device drivers serialize ATA/SCSI command queues, so one call
// wins and the other may receive EBUSY or a partial response, causing
// spurious error and mismatch logs that have no diagnostic value.
//
// Run:
//
//	go run .
//
// This example uses ExecBackend as master and a snapshot-based secondary so
// it runs on any machine without needing two distinct backend implementations.
// The secondary pre-fetches all device data from the master once (sequentially,
// before the compare backend is created) and serves it entirely from memory —
// no subprocess is ever spawned by the secondary, so there is zero contention.
//
// Replace snapshotBackend with a real LibBackend in a production validation
// setup.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	smartmontools "github.com/dianlight/smartmontools-sdk/bindings/go/v8"
	comparebackend "github.com/dianlight/smartmontools-sdk/bindings/go/v8/backends/compare"
	execbackend "github.com/dianlight/smartmontools-sdk/bindings/go/v8/backends/exec"
	"github.com/dianlight/tlog"
	"github.com/fatih/color"
)

func main() {
	blue := color.New(color.FgBlue).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	fmt.Println(blue("Smartmontools CompareBackend Example"))
	fmt.Println(blue("====================================="))
	fmt.Println()
	fmt.Println("NOTE: CompareBackend is for comparing *different* backend implementations")
	fmt.Println("      (e.g. ExecBackend vs LibBackend). Using two identical ExecBackend")
	fmt.Println("      instances causes device contention: both processes hit the same")
	fmt.Println("      physical device simultaneously, which the OS serializes — one may")
	fmt.Println("      get EBUSY or a partial response, generating spurious log noise.")
	fmt.Println()

	// Use a structured logger so compare warnings/errors appear in the output.
	logger := tlog.NewLoggerWithLevel(tlog.LevelInfo)

	ctx := context.Background()

	// Master backend — the source of truth for all returned results.
	master, err := execbackend.New(
		execbackend.WithTLogHandler(logger),
	)
	if err != nil {
		fmt.Println(red(fmt.Sprintf("✗ Failed to create master backend: %v", err)))
		os.Exit(1)
	}

	// Secondary backend — pre-fetches all device data from the master once
	// (sequentially, before the compare backend is created) and then serves
	// those cached results entirely from memory. Because the secondary never
	// calls any subprocess, there is zero device contention when the compare
	// backend later runs master and secondary in parallel.
	//
	// In production, replace snapshotBackend with a real alternative such as
	// LibBackend:
	//
	//   lib, _ := libbackend.New(libbackend.WithTLogHandler(logger))
	//   secondary = lib
	fmt.Println(blue("Pre-fetching device data for the snapshot secondary..."))
	secondary, err := newSnapshotBackend(ctx, master)
	if err != nil {
		fmt.Println(red(fmt.Sprintf("✗ Failed to build snapshot: %v", err)))
		os.Exit(1)
	}
	fmt.Println(green("✓ Snapshot ready"))
	fmt.Println()

	// Wrap both backends in the compare backend.
	// Additional backends can be appended to the slice for broader coverage.
	compare, err := comparebackend.NewCompareBackend(
		[]smartmontools.Backend{master, secondary},
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

	fmt.Printf("Active backends: %s (master) + %s (secondary snapshot)\n\n",
		blue(master.Name()), blue(secondary.Name()))
	fmt.Println(green("✓ CompareBackend ready — no mismatches expected with the snapshot"))
	fmt.Println()

	// Wire the compare backend into the standard client API.
	client, err := smartmontools.NewClient(smartmontools.WithBackend(compare))
	if err != nil {
		tlog.Fatal("Failed to create client", "error", err)
	}

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

	// ── SMART information ─────────────────────────────────────────────────
	fmt.Println(blue("Getting SMART information..."))
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

	fmt.Println(green("✓ CompareBackend example completed successfully"))
	fmt.Println()
	fmt.Println("Tip: replace snapshotBackend with a real LibBackend to compare exec vs SDK parity.")
	fmt.Println("     'compare: result mismatch' warnings = backends returned different data.")
	fmt.Println("     'compare: secondary backend error'  = secondary backend failed.")
}

// snapshotBackend is a secondary backend that pre-fetches all device data from
// a source backend once, sequentially, and then serves it entirely from memory.
// Because it never spawns any subprocess, running it as a compare secondary
// causes zero device contention regardless of what the master backend does in
// its parallel goroutine.
//
// In production, replace this with a real alternative implementation such as
// LibBackend that queries the same device through a different code path.
type snapshotBackend struct {
	devices     []smartmontools.Device
	smartInfos  map[string]*smartmontools.SMARTInfo
	healths     map[string]bool
	deviceInfos map[string]map[string]any
	selfTests   map[string]*smartmontools.SelfTestInfo
}

// newSnapshotBackend fetches all device data from src once (sequentially) and
// returns a snapshotBackend that replays those results from memory. Errors for
// individual devices are silently ignored; if a device had an error during the
// pre-fetch, the snapshot simply returns nil for that device.
func newSnapshotBackend(ctx context.Context, src smartmontools.Backend) (*snapshotBackend, error) {
	devices, err := src.ScanDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot: ScanDevices: %w", err)
	}

	s := &snapshotBackend{
		devices:     devices,
		smartInfos:  make(map[string]*smartmontools.SMARTInfo, len(devices)),
		healths:     make(map[string]bool, len(devices)),
		deviceInfos: make(map[string]map[string]any, len(devices)),
		selfTests:   make(map[string]*smartmontools.SelfTestInfo, len(devices)),
	}

	for _, dev := range devices {
		if info, err := src.GetSMARTInfo(ctx, dev.Name); err == nil {
			s.smartInfos[dev.Name] = info
		}
		if healthy, err := src.CheckHealth(ctx, dev.Name); err == nil {
			s.healths[dev.Name] = healthy
		}
		if info, err := src.GetDeviceInfo(ctx, dev.Name); err == nil {
			s.deviceInfos[dev.Name] = info
		}
		if tests, err := src.GetAvailableSelfTests(ctx, dev.Name); err == nil {
			s.selfTests[dev.Name] = tests
		}
	}

	return s, nil
}

func (s *snapshotBackend) Name() string { return "snapshot" }
func (s *snapshotBackend) Close() error { return nil }

func (s *snapshotBackend) ScanDevices(_ context.Context) ([]smartmontools.Device, error) {
	return s.devices, nil
}

func (s *snapshotBackend) GetSMARTInfo(_ context.Context, path string) (*smartmontools.SMARTInfo, error) {
	if info, ok := s.smartInfos[path]; ok {
		return info, nil
	}
	return nil, errors.New("snapshot: no data for " + path)
}

func (s *snapshotBackend) CheckHealth(_ context.Context, path string) (bool, error) {
	if healthy, ok := s.healths[path]; ok {
		return healthy, nil
	}
	return false, errors.New("snapshot: no data for " + path)
}

func (s *snapshotBackend) GetDeviceInfo(_ context.Context, path string) (map[string]any, error) {
	if info, ok := s.deviceInfos[path]; ok {
		return info, nil
	}
	return nil, errors.New("snapshot: no data for " + path)
}

func (s *snapshotBackend) GetAvailableSelfTests(_ context.Context, path string) (*smartmontools.SelfTestInfo, error) {
	if tests, ok := s.selfTests[path]; ok {
		return tests, nil
	}
	return nil, errors.New("snapshot: no data for " + path)
}

// RunSelfTest, EnableSMART, DisableSMART, AbortSelfTest are write operations
// and are not cached — the snapshot backend does not execute them.
func (s *snapshotBackend) RunSelfTest(_ context.Context, _, _ string) error { return nil }
func (s *snapshotBackend) EnableSMART(_ context.Context, _ string) error    { return nil }
func (s *snapshotBackend) DisableSMART(_ context.Context, _ string) error   { return nil }
func (s *snapshotBackend) AbortSelfTest(_ context.Context, _ string) error  { return nil }
