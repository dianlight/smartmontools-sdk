// SPDX-License-Identifier: GPL-2.0-or-later

package compare

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logRecorder captures log calls for assertion in tests.
type logRecorder struct {
	mu     sync.Mutex
	warns  []string
	errs   []string
	debugs []string
}

func (l *logRecorder) Debug(msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debugs = append(l.debugs, msg)
}

func (l *logRecorder) DebugContext(_ context.Context, msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debugs = append(l.debugs, msg)
}

func (l *logRecorder) InfoContext(_ context.Context, msg string, _ ...any) {}

func (l *logRecorder) WarnContext(_ context.Context, msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}

func (l *logRecorder) ErrorContext(_ context.Context, msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, msg)
}

// mockBackend is a configurable Backend implementation for testing.
type mockBackend struct {
	name                string
	scanDevicesFn       func(ctx context.Context) ([]Device, error)
	getSMARTInfoFn      func(ctx context.Context, path string) (*SMARTInfo, error)
	checkHealthFn       func(ctx context.Context, path string) (bool, error)
	getDeviceInfoFn     func(ctx context.Context, path string) (map[string]any, error)
	runSelfTestFn       func(ctx context.Context, path, testType string) error
	getAvailableTestsFn func(ctx context.Context, path string) (*SelfTestInfo, error)
	enableSMARTFn       func(ctx context.Context, path string) error
	disableSMARTFn      func(ctx context.Context, path string) error
	abortSelfTestFn     func(ctx context.Context, path string) error
	closeFn             func() error
}

func (m *mockBackend) Name() string { return m.name }

func (m *mockBackend) ScanDevices(ctx context.Context) ([]Device, error) {
	if m.scanDevicesFn != nil {
		return m.scanDevicesFn(ctx)
	}
	return nil, nil
}

func (m *mockBackend) GetSMARTInfo(ctx context.Context, path string) (*SMARTInfo, error) {
	if m.getSMARTInfoFn != nil {
		return m.getSMARTInfoFn(ctx, path)
	}
	return nil, nil
}

func (m *mockBackend) CheckHealth(ctx context.Context, path string) (bool, error) {
	if m.checkHealthFn != nil {
		return m.checkHealthFn(ctx, path)
	}
	return true, nil
}

func (m *mockBackend) GetDeviceInfo(ctx context.Context, path string) (map[string]any, error) {
	if m.getDeviceInfoFn != nil {
		return m.getDeviceInfoFn(ctx, path)
	}
	return nil, nil
}

func (m *mockBackend) RunSelfTest(ctx context.Context, path, testType string) error {
	if m.runSelfTestFn != nil {
		return m.runSelfTestFn(ctx, path, testType)
	}
	return nil
}

func (m *mockBackend) GetAvailableSelfTests(ctx context.Context, path string) (*SelfTestInfo, error) {
	if m.getAvailableTestsFn != nil {
		return m.getAvailableTestsFn(ctx, path)
	}
	return &SelfTestInfo{Available: []string{}, Durations: map[string]int{}}, nil
}

func (m *mockBackend) EnableSMART(ctx context.Context, path string) error {
	if m.enableSMARTFn != nil {
		return m.enableSMARTFn(ctx, path)
	}
	return nil
}

func (m *mockBackend) DisableSMART(ctx context.Context, path string) error {
	if m.disableSMARTFn != nil {
		return m.disableSMARTFn(ctx, path)
	}
	return nil
}

func (m *mockBackend) AbortSelfTest(ctx context.Context, path string) error {
	if m.abortSelfTestFn != nil {
		return m.abortSelfTestFn(ctx, path)
	}
	return nil
}

func (m *mockBackend) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

// mockDiscoveryBackend extends mockBackend to implement DiscoveryBackend.
type mockDiscoveryBackend struct {
	mockBackend
	discoverDevicesFn func(ctx context.Context) ([]DiscoveryResult, error)
}

func (m *mockDiscoveryBackend) DiscoverDevices(ctx context.Context) ([]DiscoveryResult, error) {
	if m.discoverDevicesFn != nil {
		return m.discoverDevicesFn(ctx)
	}
	return nil, nil
}

// newTestBackend builds a minimal, compile-time checked instance.
func newTestBackend(name string) *mockBackend {
	return &mockBackend{name: name}
}

func TestNewCompareBackend_RequiresAtLeastTwoBackends(t *testing.T) {
	_, err := NewCompareBackend([]Backend{newTestBackend("only")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2 backends")
}

func TestNewCompareBackend_RejectsNilBackend(t *testing.T) {
	_, err := NewCompareBackend([]Backend{newTestBackend("a"), nil})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil backend")
}

func TestNewCompareBackend_DefensiveCopy(t *testing.T) {
	a := newTestBackend("a")
	b := newTestBackend("b")
	backends := []Backend{a, b}
	cb, err := NewCompareBackend(backends)
	require.NoError(t, err)
	// Mutating the original slice must not affect the backend.
	backends[0] = newTestBackend("replaced")
	assert.Equal(t, "a", cb.backends[0].Name())
}

func TestCompareBackend_Name(t *testing.T) {
	cb, err := NewCompareBackend([]Backend{newTestBackend("a"), newTestBackend("b")})
	require.NoError(t, err)
	assert.Equal(t, "compare", cb.Name())
}

func TestCompareBackend_Close_AllBackendsClosed(t *testing.T) {
	closed := make([]bool, 2)
	backends := []Backend{
		&mockBackend{name: "a", closeFn: func() error { closed[0] = true; return nil }},
		&mockBackend{name: "b", closeFn: func() error { closed[1] = true; return nil }},
	}
	cb, err := NewCompareBackend(backends)
	require.NoError(t, err)
	require.NoError(t, cb.Close())
	assert.True(t, closed[0])
	assert.True(t, closed[1])
}

func TestCompareBackend_Close_CombinesErrors(t *testing.T) {
	backends := []Backend{
		&mockBackend{name: "a", closeFn: func() error { return errors.New("close a") }},
		&mockBackend{name: "b", closeFn: func() error { return errors.New("close b") }},
	}
	cb, err := NewCompareBackend(backends)
	require.NoError(t, err)
	err = cb.Close()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "close a")
	assert.Contains(t, err.Error(), "close b")
}

func TestCompareBackend_ScanDevices_MasterResultReturned(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sda", Type: "ata"}}, nil
	}}
	secondary := &mockBackend{name: "secondary", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sda", Type: "ata"}}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	devices, err := cb.ScanDevices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Device{{Name: "/dev/sda", Type: "ata"}}, devices)
	assert.Empty(t, log.warns)
	assert.Empty(t, log.errs)
}

func TestCompareBackend_ScanDevices_MismatchLogsWarning(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sda"}}, nil
	}}
	secondary := &mockBackend{name: "secondary", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sdb"}}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	devices, err := cb.ScanDevices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Device{{Name: "/dev/sda"}}, devices)
	assert.Len(t, log.warns, 1)
	assert.Empty(t, log.errs)
}

func TestCompareBackend_ScanDevices_SecondaryErrorLogged(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sda"}}, nil
	}}
	secondary := &mockBackend{name: "secondary", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return nil, errors.New("secondary failure")
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	devices, err := cb.ScanDevices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Device{{Name: "/dev/sda"}}, devices)
	assert.Empty(t, log.warns)
	assert.Len(t, log.errs, 1)
}

func TestCompareBackend_ScanDevices_MasterErrorReturned(t *testing.T) {
	masterErr := errors.New("master failed")
	master := &mockBackend{name: "master", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return nil, masterErr
	}}
	secondary := &mockBackend{name: "secondary", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sda"}}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary})
	require.NoError(t, err)

	_, err = cb.ScanDevices(context.Background())
	assert.ErrorIs(t, err, masterErr)
}

func TestCompareBackend_ScanDevices_OrderIndependent(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sda"}, {Name: "/dev/sdb"}}, nil
	}}
	secondary := &mockBackend{name: "secondary", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sdb"}, {Name: "/dev/sda"}}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	_, err = cb.ScanDevices(context.Background())
	require.NoError(t, err)
	assert.Empty(t, log.warns, "different order should not trigger a mismatch warning")
}

func TestCompareBackend_GetSMARTInfo_MasterResultReturned(t *testing.T) {
	log := &logRecorder{}
	info := &SMARTInfo{ModelName: "TestDrive", SerialNumber: "SN123"}
	master := &mockBackend{name: "master", getSMARTInfoFn: func(_ context.Context, _ string) (*SMARTInfo, error) {
		return info, nil
	}}
	secondary := &mockBackend{name: "secondary", getSMARTInfoFn: func(_ context.Context, _ string) (*SMARTInfo, error) {
		return &SMARTInfo{ModelName: "TestDrive", SerialNumber: "SN123"}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	result, err := cb.GetSMARTInfo(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.Equal(t, info, result)
	assert.Empty(t, log.warns)
}

func TestCompareBackend_GetSMARTInfo_MismatchIgnoresComputedFields(t *testing.T) {
	log := &logRecorder{}
	// DiskType is json:"-" so it should not cause a mismatch warning.
	master := &mockBackend{name: "master", getSMARTInfoFn: func(_ context.Context, _ string) (*SMARTInfo, error) {
		return &SMARTInfo{ModelName: "Drive", DiskType: "SSD"}, nil
	}}
	secondary := &mockBackend{name: "secondary", getSMARTInfoFn: func(_ context.Context, _ string) (*SMARTInfo, error) {
		return &SMARTInfo{ModelName: "Drive", DiskType: "HDD"}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	_, err = cb.GetSMARTInfo(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.Empty(t, log.warns, "computed fields tagged json:\"-\" should not trigger a mismatch")
}

func TestCompareBackend_GetSMARTInfo_MismatchIgnoresSmartctlMetadata(t *testing.T) {
	log := &logRecorder{}
	// Smartctl is exec-backend metadata; only the master (exec) would have it populated.
	// The secondary (e.g. lib backend) returns nil. This must not cause a mismatch.
	master := &mockBackend{name: "master", getSMARTInfoFn: func(_ context.Context, _ string) (*SMARTInfo, error) {
		return &SMARTInfo{
			ModelName: "Drive",
			Smartctl:  &SmartctlInfo{ExitStatus: 0},
		}, nil
	}}
	secondary := &mockBackend{name: "secondary", getSMARTInfoFn: func(_ context.Context, _ string) (*SMARTInfo, error) {
		return &SMARTInfo{ModelName: "Drive"}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	_, err = cb.GetSMARTInfo(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.Empty(t, log.warns, "exec-only 'smartctl' metadata should not trigger a mismatch")
}

func TestCompareBackend_GetSMARTInfo_MismatchLogsWarning(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master", getSMARTInfoFn: func(_ context.Context, _ string) (*SMARTInfo, error) {
		return &SMARTInfo{ModelName: "Drive-A"}, nil
	}}
	secondary := &mockBackend{name: "secondary", getSMARTInfoFn: func(_ context.Context, _ string) (*SMARTInfo, error) {
		return &SMARTInfo{ModelName: "Drive-B"}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	_, err = cb.GetSMARTInfo(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.Len(t, log.warns, 1)
}

func TestCompareBackend_CheckHealth_MismatchLogsWarning(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master", checkHealthFn: func(_ context.Context, _ string) (bool, error) {
		return true, nil
	}}
	secondary := &mockBackend{name: "secondary", checkHealthFn: func(_ context.Context, _ string) (bool, error) {
		return false, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	healthy, err := cb.CheckHealth(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.True(t, healthy)
	assert.Len(t, log.warns, 1)
}

func TestCompareBackend_GetDeviceInfo_MismatchLogsWarning(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master", getDeviceInfoFn: func(_ context.Context, _ string) (map[string]any, error) {
		return map[string]any{"model": "A"}, nil
	}}
	secondary := &mockBackend{name: "secondary", getDeviceInfoFn: func(_ context.Context, _ string) (map[string]any, error) {
		return map[string]any{"model": "B"}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	result, err := cb.GetDeviceInfo(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"model": "A"}, result)
	assert.Len(t, log.warns, 1)
}

func TestCompareBackend_RunSelfTest_SecondaryErrorLogged(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master"}
	secondary := &mockBackend{name: "secondary", runSelfTestFn: func(_ context.Context, _, _ string) error {
		return errors.New("secondary failure")
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	err = cb.RunSelfTest(context.Background(), "/dev/sda", "short")
	require.NoError(t, err)
	assert.Len(t, log.errs, 1)
}

func TestCompareBackend_RunSelfTest_MasterFailSecondarySuccessLogsWarning(t *testing.T) {
	log := &logRecorder{}
	masterErr := errors.New("master failure")
	master := &mockBackend{name: "master", runSelfTestFn: func(_ context.Context, _, _ string) error {
		return masterErr
	}}
	secondary := &mockBackend{name: "secondary"}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	err = cb.RunSelfTest(context.Background(), "/dev/sda", "short")
	assert.ErrorIs(t, err, masterErr)
	assert.Len(t, log.warns, 1)
}

func TestCompareBackend_GetAvailableSelfTests_OrderIndependent(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master", getAvailableTestsFn: func(_ context.Context, _ string) (*SelfTestInfo, error) {
		return &SelfTestInfo{Available: []string{"short", "long"}, Durations: map[string]int{"short": 2, "long": 120}}, nil
	}}
	secondary := &mockBackend{name: "secondary", getAvailableTestsFn: func(_ context.Context, _ string) (*SelfTestInfo, error) {
		return &SelfTestInfo{Available: []string{"long", "short"}, Durations: map[string]int{"short": 2, "long": 120}}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	_, err = cb.GetAvailableSelfTests(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.Empty(t, log.warns, "different Available order should not trigger a mismatch")
}

func TestCompareBackend_EnableSMART_SecondaryErrorLogged(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master"}
	secondary := &mockBackend{name: "secondary", enableSMARTFn: func(_ context.Context, _ string) error {
		return errors.New("enable failed")
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	require.NoError(t, cb.EnableSMART(context.Background(), "/dev/sda"))
	assert.Len(t, log.errs, 1)
}

func TestCompareBackend_DisableSMART_SecondaryErrorLogged(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master"}
	secondary := &mockBackend{name: "secondary", disableSMARTFn: func(_ context.Context, _ string) error {
		return errors.New("disable failed")
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	require.NoError(t, cb.DisableSMART(context.Background(), "/dev/sda"))
	assert.Len(t, log.errs, 1)
}

func TestCompareBackend_AbortSelfTest_SecondaryErrorLogged(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master"}
	secondary := &mockBackend{name: "secondary", abortSelfTestFn: func(_ context.Context, _ string) error {
		return errors.New("abort failed")
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	require.NoError(t, cb.AbortSelfTest(context.Background(), "/dev/sda"))
	assert.Len(t, log.errs, 1)
}

func TestCompareBackend_DiscoverDevices_BothDiscovery(t *testing.T) {
	log := &logRecorder{}
	master := &mockDiscoveryBackend{
		mockBackend: mockBackend{name: "master"},
		discoverDevicesFn: func(_ context.Context) ([]DiscoveryResult, error) {
			return []DiscoveryResult{{DevicePath: "/dev/sda", SMARTReadable: true}}, nil
		},
	}
	secondary := &mockDiscoveryBackend{
		mockBackend: mockBackend{name: "secondary"},
		discoverDevicesFn: func(_ context.Context) ([]DiscoveryResult, error) {
			return []DiscoveryResult{{DevicePath: "/dev/sda", SMARTReadable: true}}, nil
		},
	}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	results, err := cb.DiscoverDevices(context.Background())
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Empty(t, log.warns)
	assert.Empty(t, log.errs)
}

func TestCompareBackend_DiscoverDevices_FallbackForNonDiscovery(t *testing.T) {
	log := &logRecorder{}
	master := &mockDiscoveryBackend{
		mockBackend: mockBackend{name: "master"},
		discoverDevicesFn: func(_ context.Context) ([]DiscoveryResult, error) {
			return []DiscoveryResult{{DevicePath: "/dev/sda", DetectedProtocol: "ata", SMARTReadable: true, Model: "TestDrive", Serial: "SN1"}}, nil
		},
	}
	// secondary does not implement DiscoveryBackend; fallback uses ScanDevices + GetSMARTInfo.
	secondary := &mockBackend{
		name: "secondary",
		scanDevicesFn: func(_ context.Context) ([]Device, error) {
			return []Device{{Name: "/dev/sda", Type: "ata"}}, nil
		},
		getSMARTInfoFn: func(_ context.Context, _ string) (*SMARTInfo, error) {
			return &SMARTInfo{ModelName: "TestDrive", SerialNumber: "SN1", Device: Device{Type: "ata"}}, nil
		},
	}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	results, err := cb.DiscoverDevices(context.Background())
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Empty(t, log.warns, "matching fallback result should not trigger a warning")
}

func TestCompareBackend_DiscoverDevices_MismatchLogsWarning(t *testing.T) {
	log := &logRecorder{}
	master := &mockDiscoveryBackend{
		mockBackend: mockBackend{name: "master"},
		discoverDevicesFn: func(_ context.Context) ([]DiscoveryResult, error) {
			return []DiscoveryResult{{DevicePath: "/dev/sda"}}, nil
		},
	}
	secondary := &mockDiscoveryBackend{
		mockBackend: mockBackend{name: "secondary"},
		discoverDevicesFn: func(_ context.Context) ([]DiscoveryResult, error) {
			return []DiscoveryResult{{DevicePath: "/dev/sdb"}}, nil
		},
	}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	_, err = cb.DiscoverDevices(context.Background())
	require.NoError(t, err)
	assert.Len(t, log.warns, 1)
}

func TestCompareBackend_CancelledContext_SuppressesLogs(t *testing.T) {
	log := &logRecorder{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	master := &mockBackend{name: "master", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sda"}}, nil
	}}
	secondary := &mockBackend{name: "secondary", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sdb"}}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, secondary}, WithLogHandler(log))
	require.NoError(t, err)

	cb.ScanDevices(ctx) //nolint:errcheck
	assert.Empty(t, log.warns, "cancelled context should suppress mismatch logs")
}

func TestCompareBackend_ThreeBackends(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "a", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sda"}}, nil
	}}
	second := &mockBackend{name: "b", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sda"}}, nil
	}}
	third := &mockBackend{name: "c", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sdb"}}, nil
	}}
	cb, err := NewCompareBackend([]Backend{master, second, third}, WithLogHandler(log))
	require.NoError(t, err)

	devices, err := cb.ScanDevices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Device{{Name: "/dev/sda"}}, devices)
	assert.Len(t, log.warns, 1, "only the third backend differs; expect one warning")
}

func TestCompareBackend_WithSlogHandler(t *testing.T) {
	// Verify the option is accepted without panicking.
	_, err := NewCompareBackend(
		[]Backend{newTestBackend("a"), newTestBackend("b")},
		WithSlogHandler(nil),
	)
	require.NoError(t, err)
}

func TestCompareBackend_InterfaceCompliance(t *testing.T) {
	var _ Backend = (*CompareBackend)(nil)
	var _ DiscoveryBackend = (*CompareBackend)(nil)
}

func TestCompareBackend_ScanDevices_MultipleSecondaries_AllErrorsLogged(t *testing.T) {
	log := &logRecorder{}
	master := &mockBackend{name: "master", scanDevicesFn: func(_ context.Context) ([]Device, error) {
		return []Device{{Name: "/dev/sda"}}, nil
	}}
	backends := []Backend{master}
	for i := 0; i < 3; i++ {
		n := fmt.Sprintf("secondary-%d", i)
		backends = append(backends, &mockBackend{
			name: n,
			scanDevicesFn: func(_ context.Context) ([]Device, error) {
				return nil, errors.New("fail")
			},
		})
	}
	cb, err := NewCompareBackend(backends, WithLogHandler(log))
	require.NoError(t, err)

	_, err = cb.ScanDevices(context.Background())
	require.NoError(t, err)
	assert.Len(t, log.errs, 3)
}
