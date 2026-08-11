// SPDX-License-Identifier: GPL-2.0-or-later

package exec

import (
	"bytes"
	"context"
	"log/slog"
	osexec "os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMinimalBackend(t *testing.T) *ExecBackend {
	t.Helper()
	backend, err := New(
		WithSmartctlPath("/usr/sbin/smartctl"),
		WithCommander(&mockCommander{cmds: map[string]*mockCmd{}}),
	)
	require.NoError(t, err)
	return backend
}

func newMinimalTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

func TestBuildArgs_ColdCache(t *testing.T) {
	b := newMinimalBackend(t)
	got := b.buildArgs("/dev/sda", "-a", "-j")
	assert.Equal(t, []string{"-a", "-j", "--nocheck=standby", "/dev/sda"}, got)
}

func TestBuildArgs_CachedATA(t *testing.T) {
	b := newMinimalBackend(t)
	b.setCachedDeviceType("/dev/sda", "ata")
	got := b.buildArgs("/dev/sda", "-a", "-j")
	assert.Equal(t, []string{"-a", "-j", "--nocheck=standby", "-d", "ata", "/dev/sda"}, got)
}

func TestBuildArgs_CachedSAT(t *testing.T) {
	b := newMinimalBackend(t)
	b.setCachedDeviceType("/dev/sda", "sat")
	got := b.buildArgs("/dev/sda", "-a", "-j")
	assert.Equal(t, []string{"-a", "-j", "--nocheck=standby", "-d", "sat", "/dev/sda"}, got)
}

func TestBuildArgs_CachedNVMe(t *testing.T) {
	b := newMinimalBackend(t)
	b.setCachedDeviceType("/dev/nvme0", "nvme")
	got := b.buildArgs("/dev/nvme0", "-a", "-j")
	assert.Equal(t, []string{"-a", "-j", "-d", "nvme", "/dev/nvme0"}, got)
}

func TestBuildArgs_MultipleFlags(t *testing.T) {
	b := newMinimalBackend(t)
	got := b.buildArgs("/dev/sda", "-c", "-j")
	assert.Equal(t, []string{"-c", "-j", "--nocheck=standby", "/dev/sda"}, got)
}

func TestLogSmartctlMessages_NilSmartctl(t *testing.T) {
	b := newMinimalBackend(t)
	assert.NotPanics(t, func() {
		b.logSmartctlMessages(context.Background(), &SMARTInfo{})
	})
}

func TestLogSmartctlMessages_SeverityRouting(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	b := &ExecBackend{logHandler: logger}

	prefix := t.Name()
	info := &SMARTInfo{
		Smartctl: &SmartctlInfo{
			Messages: []Message{
				{String: prefix + "_error", Severity: "error"},
				{String: prefix + "_warning", Severity: "warning"},
				{String: prefix + "_info", Severity: "information"},
				{String: prefix + "_default", Severity: ""},
			},
		},
	}

	b.logSmartctlMessages(context.Background(), info)

	logged := buf.String()
	assert.Contains(t, logged, "ERROR")
	assert.Contains(t, logged, prefix+"_error")
	assert.Contains(t, logged, "WARN")
	assert.Contains(t, logged, prefix+"_warning")
	assert.Contains(t, logged, prefix+"_info")
	assert.Contains(t, logged, prefix+"_default")
}

func TestLogSmartctlMessages_Deduplication(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	b := &ExecBackend{logHandler: logger}

	msg := t.Name() + "_dedup_msg"
	info := &SMARTInfo{Smartctl: &SmartctlInfo{Messages: []Message{{String: msg, Severity: "information"}}}}

	b.logSmartctlMessages(context.Background(), info)
	firstLen := buf.Len()
	require.Positive(t, firstLen)

	b.logSmartctlMessages(context.Background(), info)
	assert.Equal(t, firstLen, buf.Len())
}

func TestWithCommander_SetsDefaultCommanderFalse(t *testing.T) {
	mock := &mockCommander{cmds: map[string]*mockCmd{}}
	backend, err := New(
		WithSmartctlPath("/usr/sbin/smartctl"),
		WithCommander(mock),
	)
	require.NoError(t, err)
	assert.False(t, backend.defaultCommander)
	assert.Equal(t, mock, backend.commander)
}

func TestNew_DefaultCommanderTrue(t *testing.T) {
	backend, err := New()
	if err != nil {
		t.Skipf("smartctl not available: %v", err)
	}
	assert.True(t, backend.defaultCommander)
}

func TestParseSmartctlVersion(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		major   int
		minor   int
		wantErr bool
	}{
		{name: "linux typical", input: "smartctl 7.3 2022-02-28 r5338 [x86_64-linux] (local build)\n...", major: 7, minor: 3},
		{name: "mac typical", input: "smartctl 7.4 2023-12-30 r5678 (db:7.4/5678)\n...", major: 7, minor: 4},
		{name: "no match", input: "some random output", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			major, minor, err := parseSmartctlVersion(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.major, major)
			assert.Equal(t, tc.minor, minor)
		})
	}
}

const satFallbackDevice = "/dev/sata1"

// execFailureExitError returns a real *exec.ExitError whose ExitCode() is 1
// (bit 0 set, i.e. an "execution failure" per smartctl's exit-code bitmask),
// unlike the zero-value &osexec.ExitError{} whose ExitCode() is -1 (no
// ProcessState) and therefore satisfies every bitmask, masking bugs in
// code that branches on a specific bit.
func execFailureExitError(t *testing.T) error {
	t.Helper()
	cmd := osexec.Command("sh", "-c", "exit 1")
	err := cmd.Run()
	require.Error(t, err)
	return err
}

const satFallbackJSON = `{
"json_format_version": [1, 0],
"smartctl": {"version": [7, 5], "exit_status": 0},
"device": {"name": "/dev/sata1", "type": "sat"},
"model_name": "SAT Test Drive",
"smart_status": {"passed": true}
}`

func TestRetrySATFallback_DirectCall_Success(t *testing.T) {
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -a -j --nocheck=standby -d sat " + satFallbackDevice: {output: []byte(satFallbackJSON)},
	}}
	b := newMinimalBackend(t)
	b.commander = commander

	info, ok := b.retrySATFallback(context.Background(), satFallbackDevice)
	require.True(t, ok)
	assert.Equal(t, satFallbackDevice, info.Device.Name)
	assert.Equal(t, "SAT Test Drive", info.ModelName)

	cachedType, hasCached := b.getCachedDeviceType(satFallbackDevice)
	assert.True(t, hasCached)
	assert.Equal(t, "sat", cachedType)
}

func TestRetrySATFallback_DirectCall_FallsThrough(t *testing.T) {
	b := newMinimalBackend(t)
	info, ok := b.retrySATFallback(context.Background(), satFallbackDevice)
	assert.False(t, ok)
	assert.Nil(t, info)
}

func TestIsATADevice(t *testing.T) {
	tests := []struct {
		name       string
		deviceType string
		expected   bool
	}{
		{"ATA device", "ata", true},
		{"SAT device", "sat", true},
		{"SATA device", "sata", true},
		{"SCSI device", "scsi", true},
		{"Uppercase ATA", "ATA", true},
		{"Uppercase SAT", "SAT", true},
		{"NVMe device", "nvme", false},
		{"Empty string", "", false},
		{"Unknown type", "usb", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isATADevice(tt.deviceType))
		})
	}
}

func TestCheckSmartStatus_ATARunning(t *testing.T) {
	smartInfo := &SMARTInfo{SmartStatus: &SmartStatus{Passed: true}, AtaSmartData: &AtaSmartData{SelfTest: &SelfTest{Status: &StatusField{Value: 245, String: "Self-test routine in progress"}}}}
	status := checkSmartStatus(smartInfo)
	assert.True(t, status.Running)
	assert.True(t, status.Passed)
}

func TestCheckSmartStatus_ATANotRunning(t *testing.T) {
	smartInfo := &SMARTInfo{SmartStatus: &SmartStatus{Passed: true}, AtaSmartData: &AtaSmartData{SelfTest: &SelfTest{Status: &StatusField{Value: 0, String: "completed without error"}}}}
	status := checkSmartStatus(smartInfo)
	assert.False(t, status.Running)
	assert.True(t, status.Passed)
}

func TestCheckSmartStatus_ATABoundaryValues(t *testing.T) {
	tests := []struct {
		name            string
		value           int
		expectedRunning bool
	}{
		{"value 239 - not running", 239, false},
		{"value 240 - running", 240, true},
		{"value 245 - running", 245, true},
		{"value 253 - running", 253, true},
		{"value 254 - not running", 254, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			smartInfo := &SMARTInfo{SmartStatus: &SmartStatus{Passed: true}, AtaSmartData: &AtaSmartData{SelfTest: &SelfTest{Status: &StatusField{Value: tt.value, String: "test status"}}}}
			status := checkSmartStatus(smartInfo)
			assert.Equal(t, tt.expectedRunning, status.Running)
			assert.True(t, status.Passed)
		})
	}
}

func TestCheckSmartStatus_NVMeRunning(t *testing.T) {
	currentOp := 1
	smartInfo := &SMARTInfo{SmartStatus: &SmartStatus{Passed: true}, NvmeSmartTestLog: &NvmeSmartTestLog{CurrentOpeation: &currentOp}}
	status := checkSmartStatus(smartInfo)
	assert.True(t, status.Running)
	assert.True(t, status.Passed)
}

func TestCheckSmartStatus_NVMeNotRunning(t *testing.T) {
	currentOp := 0
	smartInfo := &SMARTInfo{SmartStatus: &SmartStatus{Passed: true}, NvmeSmartTestLog: &NvmeSmartTestLog{CurrentOpeation: &currentOp}}
	status := checkSmartStatus(smartInfo)
	assert.False(t, status.Running)
	assert.True(t, status.Passed)
}

func TestCheckSmartStatus_NVMeNilCurrentOperation(t *testing.T) {
	smartInfo := &SMARTInfo{SmartStatus: &SmartStatus{Passed: true}, NvmeSmartTestLog: &NvmeSmartTestLog{CurrentOpeation: nil}}
	status := checkSmartStatus(smartInfo)
	assert.False(t, status.Running)
	assert.True(t, status.Passed)
}

func TestCheckSmartStatus_NoTestData(t *testing.T) {
	smartInfo := &SMARTInfo{SmartStatus: &SmartStatus{Passed: false}}
	status := checkSmartStatus(smartInfo)
	assert.False(t, status.Running)
	assert.False(t, status.Passed)
}

func TestCheckSmartStatus_PreferATA(t *testing.T) {
	currentOp := 1
	smartInfo := &SMARTInfo{
		SmartStatus:      &SmartStatus{Passed: true},
		AtaSmartData:     &AtaSmartData{SelfTest: &SelfTest{Status: &StatusField{Value: 0, String: "completed"}}},
		NvmeSmartTestLog: &NvmeSmartTestLog{CurrentOpeation: &currentOp},
	}
	status := checkSmartStatus(smartInfo)
	assert.False(t, status.Running)
	assert.True(t, status.Passed)
}

func TestCheckSmartStatus_ATASelfTestNilStatus(t *testing.T) {
	smartInfo := &SMARTInfo{SmartStatus: &SmartStatus{Passed: true}, AtaSmartData: &AtaSmartData{SelfTest: &SelfTest{PollingMinutes: &PollingMinutes{Short: 2}}}}
	status := checkSmartStatus(smartInfo)
	assert.False(t, status.Running)
	assert.True(t, status.Passed)
}

func TestCheckSmartStatus_ExitCodeInfo_Zero(t *testing.T) {
	smartInfo := &SMARTInfo{SmartStatus: &SmartStatus{Passed: true}, Smartctl: &SmartctlInfo{ExitStatus: 0}}
	checkSmartStatus(smartInfo)
	assert.Nil(t, smartInfo.ExitCodeInfo)
}

func TestCheckSmartStatus_ExitCodeInfo_HealthBits(t *testing.T) {
	tests := []struct {
		name             string
		exitStatus       int
		expectedExecBits int
		expectedHealth   int
	}{
		{"only exec bits", 0x02, 0x02, 0x00},
		{"only health bits", 0x40, 0x00, 0x40},
		{"prefail attributes", 0x10, 0x00, 0x10},
		{"multiple health bits", 0x48, 0x00, 0x48},
		{"exec and health bits combined", 0x42, 0x02, 0x40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			smartInfo := &SMARTInfo{SmartStatus: &SmartStatus{}, Smartctl: &SmartctlInfo{ExitStatus: tt.exitStatus}}
			checkSmartStatus(smartInfo)
			require.NotNil(t, smartInfo.ExitCodeInfo)
			assert.Equal(t, tt.expectedExecBits, smartInfo.ExitCodeInfo.ExecBits)
			assert.Equal(t, tt.expectedHealth, smartInfo.ExitCodeInfo.HealthBits)
		})
	}
}

func TestLogHealthBits_DeduplicationByCache(t *testing.T) {
	b := &ExecBackend{healthBitsCache: make(map[string]int), logHandler: newMinimalTestLogger()}
	info := &SMARTInfo{ExitCodeInfo: &ExitCodeInfo{HealthBits: 0x40}}
	ctx := context.Background()
	const dev = "/dev/sda"

	b.logHealthBits(ctx, dev, info)
	b.healthBitsCacheMux.RLock()
	val, ok := b.healthBitsCache[dev]
	b.healthBitsCacheMux.RUnlock()
	require.True(t, ok)
	assert.Equal(t, 0x40, val)

	b.logHealthBits(ctx, dev, info)
	b.healthBitsCacheMux.RLock()
	val2 := b.healthBitsCache[dev]
	b.healthBitsCacheMux.RUnlock()
	assert.Equal(t, 0x40, val2)

	info2 := &SMARTInfo{ExitCodeInfo: &ExitCodeInfo{HealthBits: 0x10}}
	b.logHealthBits(ctx, dev, info2)
	b.healthBitsCacheMux.RLock()
	val3 := b.healthBitsCache[dev]
	b.healthBitsCacheMux.RUnlock()
	assert.Equal(t, 0x10, val3)
}

func TestIsUnknownUSBBridge(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		expected bool
	}{
		{"Unknown USB bridge message", []Message{{String: "/dev/sda: Unknown USB bridge [0x048d:0x1234 (0x200)]", Severity: "error"}}, true},
		{"No Unknown USB bridge message", []Message{{String: "Some other error", Severity: "error"}}, false},
		{"No messages", []Message{}, false},
		{"Multiple messages with Unknown USB bridge", []Message{{String: "Info message", Severity: "info"}, {String: "Unknown USB bridge detected", Severity: "error"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			smartInfo := &SMARTInfo{Smartctl: &SmartctlInfo{Messages: tt.messages}}
			assert.Equal(t, tt.expected, isUnknownUSBBridge(smartInfo))
		})
	}
	assert.False(t, isUnknownUSBBridge(nil))
	assert.False(t, isUnknownUSBBridge(&SMARTInfo{}))
}

func TestExtractUSBBridgeID(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		expected string
	}{
		{"Standard USB bridge message", []Message{{String: "/dev/sda: Unknown USB bridge [0x152d:0x578e (0x200)]", Severity: "error"}}, "usb:0x152d:0x578e"},
		{"USB bridge with uppercase hex", []Message{{String: "/dev/sda: Unknown USB bridge [0x152D:0xA580 (0x209)]", Severity: "error"}}, "usb:0x152d:0xa580"},
		{"No USB bridge message", []Message{{String: "Some other error", Severity: "error"}}, ""},
		{"Empty messages", []Message{}, ""},
		{"USB bridge in second message", []Message{{String: "Info message", Severity: "info"}, {String: "Unknown USB bridge [0x0bda:0x9201 (0xf200)]", Severity: "error"}}, "usb:0x0bda:0x9201"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			smartInfo := &SMARTInfo{Smartctl: &SmartctlInfo{Messages: tt.messages}}
			assert.Equal(t, tt.expected, extractUSBBridgeID(smartInfo))
		})
	}
	assert.Empty(t, extractUSBBridgeID(nil))
	assert.Empty(t, extractUSBBridgeID(&SMARTInfo{}))
}

func TestLoadDrivedbAddendum(t *testing.T) {
	cache := loadDrivedbAddendum()
	expectedEntries := map[string]string{
		"usb:0x152d:0x0578": "sat",
		"usb:0x152d:0x0562": "sat",
		"usb:0x0bda:0x9201": "sat",
		"usb:0x059f:0x1029": "sat",
	}
	for key, expectedValue := range expectedEntries {
		value, ok := cache[key]
		assert.True(t, ok, "Expected key %q to be in cache", key)
		assert.Equal(t, expectedValue, value)
	}
	assert.GreaterOrEqual(t, len(cache), 100)
}

func TestGetSMARTInfo_WithMockExitErrorFallback(t *testing.T) {
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -a -j --nocheck=standby " + satFallbackDevice:        {err: &osexec.ExitError{}},
		"/usr/sbin/smartctl -a -j --nocheck=standby -d sat " + satFallbackDevice: {output: []byte(satFallbackJSON)},
	}}
	b := newMinimalBackend(t)
	b.commander = commander
	info, _, err := b.getSMARTInfoInternal(context.Background(), satFallbackDevice)
	require.NoError(t, err)
	assert.Equal(t, satFallbackDevice, info.Device.Name)
}

func TestCheckHealth_WithMockExitErrorFallback(t *testing.T) {
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -H --nocheck=standby " + satFallbackDevice:           {err: execFailureExitError(t)},
		"/usr/sbin/smartctl -H --nocheck=standby -d sat " + satFallbackDevice:    {output: []byte("PASSED")},
		"/usr/sbin/smartctl -a -j --nocheck=standby -d sat " + satFallbackDevice: {output: []byte(satFallbackJSON)},
	}}
	b := newMinimalBackend(t)
	b.commander = commander
	healthy, err := b.CheckHealth(context.Background(), satFallbackDevice)
	require.NoError(t, err)
	assert.True(t, healthy)
	cachedType, hasCached := b.getCachedDeviceType(satFallbackDevice)
	assert.True(t, hasCached)
	assert.Equal(t, "sat", cachedType)
}

func TestGetDeviceInfo_WithMockExitErrorFallback(t *testing.T) {
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -i -j --nocheck=standby " + satFallbackDevice:        {err: execFailureExitError(t)},
		"/usr/sbin/smartctl -i -j --nocheck=standby -d sat " + satFallbackDevice: {output: []byte(satFallbackJSON)},
		"/usr/sbin/smartctl -a -j --nocheck=standby -d sat " + satFallbackDevice: {output: []byte(satFallbackJSON)},
	}}
	b := newMinimalBackend(t)
	b.commander = commander
	info, err := b.GetDeviceInfo(context.Background(), satFallbackDevice)
	require.NoError(t, err)
	assert.Equal(t, "SAT Test Drive", info["model_name"])
}

func TestGetAvailableSelfTests_WithMockExitErrorFallback(t *testing.T) {
	const capsJSON = `{"ata_smart_data": {"capabilities": {"self_tests_supported": true}}}`
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -c -j --nocheck=standby " + satFallbackDevice:        {err: execFailureExitError(t)},
		"/usr/sbin/smartctl -c -j --nocheck=standby -d sat " + satFallbackDevice: {output: []byte(capsJSON)},
		"/usr/sbin/smartctl -a -j --nocheck=standby -d sat " + satFallbackDevice: {output: []byte(satFallbackJSON)},
	}}
	b := newMinimalBackend(t)
	b.commander = commander
	info, err := b.GetAvailableSelfTests(context.Background(), satFallbackDevice)
	require.NoError(t, err)
	require.NotNil(t, info)
}
