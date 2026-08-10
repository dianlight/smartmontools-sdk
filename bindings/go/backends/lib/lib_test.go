// SPDX-License-Identifier: GPL-2.0-or-later

//go:build linux || darwin

package lib

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	smtypes "github.com/dianlight/smartmontools-sdk/bindings/go/types"
)

var _ Backend = (*LibBackend)(nil)

// TestLibBackend_Name verifies that Name returns "lib".
func TestLibBackend_Name(t *testing.T) {
	b := &LibBackend{}
	assert.Equal(t, "lib", b.Name())
}

// TestLibBackend_Close_Idempotent verifies that Close can be called multiple times safely.
func TestLibBackend_Close_Idempotent(t *testing.T) {
	b := &LibBackend{}
	assert.NoError(t, b.Close())
	assert.NoError(t, b.Close())
}

// TestNew_InvalidPath verifies that New returns an error for a non-existent library path.
func TestNew_InvalidPath(t *testing.T) {
	_, err := New(WithLibraryPath("/nonexistent/libsmartmon_go.so"))
	require.Error(t, err)
}

// TestNew_MissingLibrary verifies that New returns an error when no library is found.
func TestNew_MissingLibrary(t *testing.T) {
	if integrationEnabled() {
		t.Skip("SMARTMON_INTEGRATION=1: a real wrapper library is expected to be present")
	}
	_, err := New()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestWithLogHandler verifies that the log handler option is applied without panic.
func TestWithLogHandler(t *testing.T) {
	b := &LibBackend{}
	WithLogHandler(nil)(b)
	assert.Nil(t, b.logHandler)
}

// newTestBackend constructs a LibBackend with injectable fake C functions, enabling
// unit testing of the Go wrapper logic without a real shared library.
func newTestBackend(t *testing.T, fns *libFuncs) *LibBackend {
	t.Helper()
	return &LibBackend{funcs: fns}
}

// makeCStr allocates a null-terminated byte slice in Go heap memory and returns
// its address as unsafe.Pointer, simulating a C string for fake C functions.
func makeCStr(t *testing.T, s string) unsafe.Pointer {
	t.Helper()
	b := make([]byte, len(s)+1)
	copy(b, s)
	t.Cleanup(func() { _ = b })
	if len(b) == 0 {
		return nil
	}
	return unsafe.Pointer(&b[0])
}

// fakeLastError returns a lastError function that always returns the given message.
func fakeLastError(t *testing.T, msg string) func() unsafe.Pointer {
	ptr := makeCStr(t, msg)
	return func() unsafe.Pointer { return ptr }
}

// TestScanDevices_Success verifies that ScanDevices parses a JSON device list.
func TestScanDevices_Success(t *testing.T) {
	scanJSON := `{"devices":[{"name":"/dev/sda","type":"ata"},{"name":"/dev/sdb","type":"nvme"}]}`
	b := newTestBackend(t, &libFuncs{
		scanDevices: func(out *unsafe.Pointer) int32 {
			*out = makeCStr(t, scanJSON)
			return 0
		},
		freeString: func(unsafe.Pointer) {},
		lastError:  fakeLastError(t, ""),
	})

	devices, err := b.ScanDevices(context.Background())
	require.NoError(t, err)
	require.Len(t, devices, 2)
	assert.Equal(t, Device{Name: "/dev/sda", Type: "ata"}, devices[0])
	assert.Equal(t, Device{Name: "/dev/sdb", Type: "nvme"}, devices[1])
}

// TestScanDevices_Error verifies that a non-zero return code yields an error.
func TestScanDevices_Error(t *testing.T) {
	b := newTestBackend(t, &libFuncs{
		scanDevices: func(_ *unsafe.Pointer) int32 { return 1 },
		lastError:   fakeLastError(t, "scan failed"),
	})

	_, err := b.ScanDevices(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scan failed")
}

// TestScanDevices_ContextCancelled verifies early return when context is done.
func TestScanDevices_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := newTestBackend(t, &libFuncs{})
	_, err := b.ScanDevices(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

// TestGetSMARTInfo_Success verifies JSON parsing of full SMART data.
func TestGetSMARTInfo_Success(t *testing.T) {
	info := map[string]any{
		"device":        map[string]any{"name": "/dev/sda", "type": "ata"},
		"model_name":    "Test Drive",
		"serial_number": "SN123456",
	}
	raw, err := json.Marshal(info)
	require.NoError(t, err)

	b := newTestBackend(t, &libFuncs{
		getSmartData: func(_, _ string, out *unsafe.Pointer) int32 {
			*out = makeCStr(t, string(raw))
			return 0
		},
		freeString: func(unsafe.Pointer) {},
		lastError:  fakeLastError(t, ""),
	})

	got, err := b.GetSMARTInfo(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.Equal(t, "Test Drive", got.ModelName)
	assert.Equal(t, "SN123456", got.SerialNumber)
}

// TestGetSMARTInfo_ContextCancelled verifies early return when context is done.
func TestGetSMARTInfo_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := newTestBackend(t, &libFuncs{})
	_, err := b.GetSMARTInfo(ctx, "/dev/sda")
	require.ErrorIs(t, err, context.Canceled)
}

// TestGetSMARTInfo_Error verifies error propagation from C API.
func TestGetSMARTInfo_Error(t *testing.T) {
	b := newTestBackend(t, &libFuncs{
		getSmartData: func(_, _ string, _ *unsafe.Pointer) int32 { return 2 },
		lastError:    fakeLastError(t, "device not found"),
	})

	_, err := b.GetSMARTInfo(context.Background(), "/dev/sda")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "device not found")
}

// TestCheckHealth_Healthy verifies that healthy=1 maps to true.
func TestCheckHealth_Healthy(t *testing.T) {
	b := newTestBackend(t, &libFuncs{
		checkHealth: func(_, _ string, out *int32) int32 {
			*out = 1
			return 0
		},
		lastError: fakeLastError(t, ""),
	})

	ok, err := b.CheckHealth(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestCheckHealth_Unhealthy verifies that healthy=0 maps to false.
func TestCheckHealth_Unhealthy(t *testing.T) {
	b := newTestBackend(t, &libFuncs{
		checkHealth: func(_, _ string, out *int32) int32 {
			*out = 0
			return 0
		},
		lastError: fakeLastError(t, ""),
	})

	ok, err := b.CheckHealth(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.False(t, ok)
}

// TestCheckHealth_Error verifies error propagation.
func TestCheckHealth_Error(t *testing.T) {
	b := newTestBackend(t, &libFuncs{
		checkHealth: func(_, _ string, _ *int32) int32 { return 1 },
		lastError:   fakeLastError(t, "ioctl error"),
	})

	_, err := b.CheckHealth(context.Background(), "/dev/sda")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ioctl error")
}

// TestGetDeviceInfo_Success verifies JSON parsing of device info.
func TestGetDeviceInfo_Success(t *testing.T) {
	payload := `{"model_name":"Test","firmware_version":"1.0"}`
	b := newTestBackend(t, &libFuncs{
		getSmartData: func(_, _ string, out *unsafe.Pointer) int32 {
			*out = makeCStr(t, payload)
			return 0
		},
		freeString: func(unsafe.Pointer) {},
		lastError:  fakeLastError(t, ""),
	})

	result, err := b.GetDeviceInfo(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.Equal(t, "Test", result["model_name"])
	assert.Equal(t, "1.0", result["firmware_version"])
}

// TestRunSelfTest_Success verifies success path and argument forwarding.
func TestRunSelfTest_Success(t *testing.T) {
	var gotDevice, gotType string
	b := newTestBackend(t, &libFuncs{
		runSelftest: func(device, _, testType string) int32 {
			gotDevice = device
			gotType = testType
			return 0
		},
		lastError: fakeLastError(t, ""),
	})

	err := b.RunSelfTest(context.Background(), "/dev/sda", "short")
	require.NoError(t, err)
	assert.Equal(t, "/dev/sda", gotDevice)
	assert.Equal(t, "short", gotType)
}

// TestRunSelfTest_Error verifies error propagation.
func TestRunSelfTest_Error(t *testing.T) {
	b := newTestBackend(t, &libFuncs{
		runSelftest: func(_, _, _ string) int32 { return 1 },
		lastError:   fakeLastError(t, "self-test not supported"),
	})

	err := b.RunSelfTest(context.Background(), "/dev/sda", "short")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "self-test not supported")
}

// TestEnableSMART_Success verifies success path.
func TestEnableSMART_Success(t *testing.T) {
	b := newTestBackend(t, &libFuncs{
		enableSmart: func(_, _ string) int32 { return 0 },
		lastError:   fakeLastError(t, ""),
	})
	assert.NoError(t, b.EnableSMART(context.Background(), "/dev/sda"))
}

// TestDisableSMART_Error verifies error propagation.
func TestDisableSMART_Error(t *testing.T) {
	b := newTestBackend(t, &libFuncs{
		disableSmart: func(_, _ string) int32 { return 1 },
		lastError:    fakeLastError(t, "NVMe does not support disable"),
	})
	err := b.DisableSMART(context.Background(), "/dev/nvme0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NVMe does not support disable")
}

// TestAbortSelfTest_Success verifies success path.
func TestAbortSelfTest_Success(t *testing.T) {
	b := newTestBackend(t, &libFuncs{
		abortSelftest: func(_, _ string) int32 { return 0 },
		lastError:     fakeLastError(t, ""),
	})
	assert.NoError(t, b.AbortSelfTest(context.Background(), "/dev/sda"))
}

// TestGetAvailableSelfTests_ATA verifies ATA self-test capability parsing via
// PopulateSelfTestInfo (pure Go logic, no library call required for this layer).
func TestGetAvailableSelfTests_ATA(t *testing.T) {
	payload := `{
		"ata_smart_data": {
			"capabilities": {"self_tests_supported": true, "conveyance_self_test_supported": true},
			"self_test": {
				"polling_minutes": {"short": 2, "extended": 120, "conveyance": 5}
			}
		}
	}`
	b := newTestBackend(t, &libFuncs{
		getSmartData: func(_, _ string, out *unsafe.Pointer) int32 {
			*out = makeCStr(t, payload)
			return 0
		},
		freeString: func(unsafe.Pointer) {},
		lastError:  fakeLastError(t, ""),
	})

	info, err := b.GetAvailableSelfTests(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.Contains(t, info.Available, "short")
	assert.Contains(t, info.Available, "long")
	assert.Equal(t, 2, info.Durations["short"])
	assert.Equal(t, 120, info.Durations["long"])
}

// TestPopulateSelfTestInfo_ATA tests the pure Go self-test info parsing directly.
func TestPopulateSelfTestInfo_ATA(t *testing.T) {
	pm := smtypes.PollingMinutes{Short: 1, Extended: 60, Conveyance: 3}
	st := smtypes.SelfTest{PollingMinutes: &pm}
	caps := smtypes.Capabilities{SelfTestsSupported: true, ConveyanceSelfTestSupported: true}
	ata := &smtypes.AtaSmartData{
		Capabilities: &caps,
		SelfTest:     &st,
	}
	info := &smtypes.SelfTestInfo{
		Available: []string{},
		Durations: make(map[string]int),
	}
	smtypes.PopulateSelfTestInfo(info, ata, nil, nil)

	assert.Contains(t, info.Available, "short")
	assert.Contains(t, info.Available, "long")
	assert.Contains(t, info.Available, "conveyance")
	assert.Equal(t, 1, info.Durations["short"])
	assert.Equal(t, 60, info.Durations["long"])
	assert.Equal(t, 3, info.Durations["conveyance"])
}

// TestGoString verifies the null-terminated C string decoder.
func TestGoString(t *testing.T) {
	cases := []struct {
		name string
		s    string
	}{
		{"empty", ""},
		{"simple", "hello"},
		{"version", "smartmon-7.5"},
		{"with spaces", "/dev/sda type=ata"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := make([]byte, len(tc.s)+1)
			copy(buf, tc.s)
			var ptr unsafe.Pointer
			if len(tc.s) > 0 {
				ptr = unsafe.Pointer(&buf[0])
			}
			assert.Equal(t, tc.s, goString(ptr))
		})
	}
}

// TestGoString_Nil verifies that a nil pointer returns an empty string.
func TestGoString_Nil(t *testing.T) {
	assert.Equal(t, "", goString(nil))
}

// TestLastError_NoFuncs verifies that lastError on an uninitialised backend returns a sensible error.
func TestLastError_NoFuncs(t *testing.T) {
	b := &LibBackend{}
	err := b.lastError()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialised")
}

// TestCallWithStringOut_NilOutput verifies that a nil output pointer is reported as an error.
func TestCallWithStringOut_NilOutput(t *testing.T) {
	b := newTestBackend(t, &libFuncs{
		freeString: func(unsafe.Pointer) {},
		lastError:  fakeLastError(t, ""),
	})

	_, err := b.callWithStringOut(func(out *unsafe.Pointer) int32 {
		// Deliberately do not write to *out to simulate a library bug.
		return 0
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil output")
}

// withDefaultLibPaths temporarily replaces defaultLibPaths for the duration of
// the test and restores the original slice on cleanup.
func withDefaultLibPaths(t *testing.T, paths []string) {
	t.Helper()
	orig := defaultLibPaths
	defaultLibPaths = paths
	t.Cleanup(func() { defaultLibPaths = orig })
}

// TestSameDir verifies the directory comparison helper used for path warnings.
func TestSameDir(t *testing.T) {
	assert.True(t, sameDir("/a/b/libfoo.so", "/a/b/libbar.so"), "same directory")
	assert.False(t, sameDir("/a/b/libfoo.so", "/a/c/libfoo.so"), "sibling directories")
	assert.False(t, sameDir("/usr/local/lib/lib.so", "/usr/lib/lib.so"), "different prefix")
	// Cleaned paths must be treated as equal.
	assert.True(t, sameDir("/a/b/../b/libfoo.so", "/a/b/libbar.so"), "cleaned path equals")
}

// TestCheckABIVersion verifies the three-tier semver contract: major must
// match exactly, minor must be greater than or equal to what this binding
// requires, patch is informational only.
func TestCheckABIVersion(t *testing.T) {
	encode := func(major, minor, patch uint32) uint32 {
		return (major << 16) | (minor << 8) | patch
	}
	withVersion := func(v uint32) *libFuncs {
		return &libFuncs{abiVersion: func() uint32 { return v }}
	}

	t.Run("exact match", func(t *testing.T) {
		err := checkABIVersion(withVersion(encode(abiMajorRequired, abiMinorRequired, 0)))
		assert.NoError(t, err)
	})

	t.Run("newer compatible minor", func(t *testing.T) {
		err := checkABIVersion(withVersion(encode(abiMajorRequired, abiMinorRequired+1, 3)))
		assert.NoError(t, err)
	})

	t.Run("older minor rejected", func(t *testing.T) {
		var required uint32 = abiMinorRequired
		if required == 0 {
			t.Skip("abiMinorRequired is 0; no smaller minor version exists to test")
		}
		err := checkABIVersion(withVersion(encode(abiMajorRequired, required-1, 0)))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "incompatible")
	})

	t.Run("mismatched major rejected", func(t *testing.T) {
		err := checkABIVersion(withVersion(encode(abiMajorRequired+1, 0, 0)))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "incompatible")
	})
}

// TestFindSystemLibPath_NotFound verifies that findSystemLibPath returns (_, false)
// when none of the default paths exist.
func TestFindSystemLibPath_NotFound(t *testing.T) {
	withDefaultLibPaths(t, []string{"/nonexistent/libsmartmon_go.so"})
	_, ok := findSystemLibPath()
	assert.False(t, ok)
}

// TestFindSystemLibPath_Found verifies that findSystemLibPath returns the first
// existing path from defaultLibPaths.
func TestFindSystemLibPath_Found(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "libsmartmon_go.so")
	require.NoError(t, os.WriteFile(lib, []byte("stub"), 0o644))

	withDefaultLibPaths(t, []string{"/nonexistent/nope.so", lib})

	got, ok := findSystemLibPath()
	assert.True(t, ok)
	assert.Equal(t, lib, got)
}

// TestNew_EnvPath_Missing_FallsBack verifies that when SMARTMON_LIB_PATH points
// to a missing file, New emits a warning and falls back to the system library
// search.  The test expects a "not found" error because no system library is
// installed in CI.
func TestNew_EnvPath_Missing_FallsBack(t *testing.T) {
	if integrationEnabled() {
		t.Skip("SMARTMON_INTEGRATION=1: a real wrapper library is expected to be present")
	}
	t.Setenv("SMARTMON_LIB_PATH", "/nonexistent/libsmartmon_go.so")

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	_, err := New(WithSlogHandler(logger))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, buf.String(), "SMARTMON_LIB_PATH is set but the file was not found")
}

// TestNew_EnvPath_SystemLibWarning verifies that when SMARTMON_LIB_PATH points
// to a valid path but the library also exists in a different standard system
// path, New emits a warning before attempting to load.
func TestNew_EnvPath_SystemLibWarning(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	lib1 := filepath.Join(dir1, "libsmartmon_go.so")
	lib2 := filepath.Join(dir2, "libsmartmon_go.so")
	require.NoError(t, os.WriteFile(lib1, []byte("stub"), 0o644))
	require.NoError(t, os.WriteFile(lib2, []byte("stub"), 0o644))

	// Override defaultLibPaths so findSystemLibPath finds lib2.
	withDefaultLibPaths(t, []string{lib2})
	t.Setenv("SMARTMON_LIB_PATH", lib1)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// New will warn and then fail (the stubs are not real shared libraries).
	_, err := New(WithSlogHandler(logger))
	require.Error(t, err)
	assert.Contains(t, buf.String(), "SMARTMON_LIB_PATH is set but library also found in a different system path")
}

// TestIntegration_ScanDevices is an integration test that runs only when
// SMARTMON_LIB_PATH is set to a real smartmon wrapper library path.
func TestIntegration_ScanDevices(t *testing.T) {
	libPath := integrationLibPath(t)
	b, err := New(WithLibraryPath(libPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close() })

	devices, err := b.ScanDevices(context.Background())
	require.NoError(t, err)
	t.Logf("found %d device(s)", len(devices))
	for _, d := range devices {
		assert.NotEmpty(t, d.Name)
	}
}

// TestIntegration_GetSMARTInfo is an integration test that exercises the full
// SMART data path against a real device. Set SMARTMON_SDK_DEVICE to run.
func TestIntegration_GetSMARTInfo(t *testing.T) {
	libPath := integrationLibPath(t)
	devicePath, ok := os.LookupEnv("SMARTMON_SDK_DEVICE")
	if !ok {
		t.Skip("set SMARTMON_SDK_DEVICE to run SMART data integration tests")
	}

	b, err := New(WithLibraryPath(libPath))
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close() })

	info, err := b.GetSMARTInfo(context.Background(), devicePath)
	require.NoError(t, err)
	assert.NotEmpty(t, info.ModelName)
	assert.NotEmpty(t, info.SerialNumber)
	t.Logf("model=%s serial=%s", info.ModelName, info.SerialNumber)
}

// integrationEnabled reports whether SMARTMON_INTEGRATION=1 is set. Integration
// tests must be explicitly opted into so that a missing wrapper library fails
// the build instead of silently skipping.
func integrationEnabled() bool {
	return os.Getenv("SMARTMON_INTEGRATION") == "1"
}

// integrationLibPath returns the wrapper library path from SMARTMON_LIB_PATH.
// When SMARTMON_INTEGRATION is not set to "1" the test is skipped. When it is
// set, SMARTMON_LIB_PATH is required and its absence fails the test rather
// than skipping it.
func integrationLibPath(t *testing.T) string {
	t.Helper()
	if !integrationEnabled() {
		t.Skip("set SMARTMON_INTEGRATION=1 and SMARTMON_LIB_PATH to run integration tests")
	}
	path, ok := os.LookupEnv("SMARTMON_LIB_PATH")
	if !ok || path == "" {
		t.Fatal("SMARTMON_INTEGRATION=1 requires SMARTMON_LIB_PATH to be set")
	}
	return path
}
