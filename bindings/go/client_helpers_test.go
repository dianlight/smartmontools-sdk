// SPDX-License-Identifier: GPL-2.0-or-later

package smartmontools

import (
	"context"
	osexec "os/exec"
	"testing"

	smtypes "github.com/dianlight/smartmontools-sdk/bindings/go/v8/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMinimalClient(t *testing.T) *Client {
	t.Helper()
	client, err := NewClient(
		WithSmartctlPath("/usr/sbin/smartctl"),
		WithCommander(&mockCommander{cmds: map[string]*mockCmd{}}),
	)
	require.NoError(t, err)
	return client.(*Client)
}

func TestResolveCtx_NilReturnsDefault(t *testing.T) {
	c := newMinimalClient(t)
	type ctxKey struct{}
	sentinel := context.WithValue(context.Background(), ctxKey{}, "sentinel")
	c.defaultCtx = sentinel

	var nilCtx context.Context
	got := c.resolveCtx(nilCtx)
	assert.Equal(t, sentinel, got)
}

func TestResolveCtx_NonNilPassthrough(t *testing.T) {
	c := newMinimalClient(t)
	type ctxKey struct{}
	explicit := context.WithValue(context.Background(), ctxKey{}, "explicit")
	assert.Equal(t, explicit, c.resolveCtx(explicit))
}

func TestPopulateSelfTestInfo_ATAFull(t *testing.T) {
	info := &SelfTestInfo{Available: []string{}, Durations: make(map[string]int)}
	smtypes.PopulateSelfTestInfo(info, &AtaSmartData{
		Capabilities: &Capabilities{SelfTestsSupported: true, ConveyanceSelfTestSupported: true, ExecOfflineImmediate: true},
		SelfTest:     &SelfTest{PollingMinutes: &PollingMinutes{Short: 2, Extended: 48, Conveyance: 5}},
	}, nil, nil)
	assert.Equal(t, []string{"short", "long", "conveyance", "offline"}, info.Available)
	assert.Equal(t, map[string]int{"short": 2, "long": 48, "conveyance": 5}, info.Durations)
}

func TestPopulateSelfTestInfo_ATANoSelfTestBlock(t *testing.T) {
	info := &SelfTestInfo{Available: []string{}, Durations: make(map[string]int)}
	smtypes.PopulateSelfTestInfo(info, &AtaSmartData{Capabilities: &Capabilities{SelfTestsSupported: true}}, nil, nil)
	assert.Equal(t, []string{"short", "long"}, info.Available)
	assert.Empty(t, info.Durations)
}

func TestPopulateSelfTestInfo_ATANilCapabilities(t *testing.T) {
	info := &SelfTestInfo{Available: []string{}, Durations: make(map[string]int)}
	smtypes.PopulateSelfTestInfo(info, &AtaSmartData{Capabilities: nil}, nil, nil)
	assert.Empty(t, info.Available)
	assert.Empty(t, info.Durations)
}

func TestPopulateSelfTestInfo_NVMeViaCaps(t *testing.T) {
	info := &SelfTestInfo{Available: []string{}, Durations: make(map[string]int)}
	smtypes.PopulateSelfTestInfo(info, nil, &NvmeControllerCapabilities{SelfTest: true}, nil)
	assert.Equal(t, []string{"short"}, info.Available)
	assert.Empty(t, info.Durations)
}

func TestPopulateSelfTestInfo_NVMeViaOptional(t *testing.T) {
	info := &SelfTestInfo{Available: []string{}, Durations: make(map[string]int)}
	smtypes.PopulateSelfTestInfo(info, nil, nil, &NvmeOptionalAdminCommands{SelfTest: true})
	assert.Equal(t, []string{"short"}, info.Available)
	assert.Empty(t, info.Durations)
}

func TestPopulateSelfTestInfo_NVMeBothFieldsOnceShort(t *testing.T) {
	info := &SelfTestInfo{Available: []string{}, Durations: make(map[string]int)}
	smtypes.PopulateSelfTestInfo(info, nil, &NvmeControllerCapabilities{SelfTest: true}, &NvmeOptionalAdminCommands{SelfTest: true})
	assert.Equal(t, []string{"short"}, info.Available)
	assert.Empty(t, info.Durations)
}

func TestPopulateSelfTestInfo_AllNil(t *testing.T) {
	info := &SelfTestInfo{Available: []string{}, Durations: make(map[string]int)}
	smtypes.PopulateSelfTestInfo(info, nil, nil, nil)
	assert.Empty(t, info.Available)
	assert.Empty(t, info.Durations)
}

const satFallbackDevice = "/dev/sata1"

const satFallbackJSON = `{
"json_format_version": [1, 0],
"smartctl": {"version": [7, 5], "exit_status": 0},
"device": {"name": "/dev/sata1", "type": "sat"},
"model_name": "SAT Test Drive",
"smart_status": {"passed": true}
}`

func TestGetSMARTInfo_SATFallback_Success(t *testing.T) {
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -a -j --nocheck=standby " + satFallbackDevice:        {err: &osexec.ExitError{}},
		"/usr/sbin/smartctl -a -j --nocheck=standby -d sat " + satFallbackDevice: {output: []byte(satFallbackJSON)},
	}}
	client, err := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	require.NoError(t, err)

	info, err := client.GetSMARTInfo(context.Background(), satFallbackDevice)
	require.NoError(t, err)
	assert.Equal(t, satFallbackDevice, info.Device.Name)
	assert.Equal(t, "SAT Test Drive", info.ModelName)

	backend := client.(*Client).backend.(*ExecBackend)
	cachedType, hasCached := backend.DeviceTypeHint(satFallbackDevice)
	assert.True(t, hasCached)
	assert.Equal(t, "sat", cachedType)
}

func TestGetSMARTInfo_SATFallback_SkippedWhenCached(t *testing.T) {
	mockJSON := `{
"device": {"name": "/dev/sata1", "type": "sat"},
"model_name": "Cached Drive",
"smart_status": {"passed": true}
}`
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -a -j --nocheck=standby -d sat " + satFallbackDevice: {output: []byte(mockJSON)},
	}}
	client, err := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	require.NoError(t, err)

	client.(*Client).backend.(*ExecBackend).SetDeviceTypeHint(satFallbackDevice, "sat")
	info, err := client.GetSMARTInfo(context.Background(), satFallbackDevice)
	require.NoError(t, err)
	assert.Equal(t, "Cached Drive", info.ModelName)
}
