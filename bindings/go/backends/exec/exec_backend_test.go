// SPDX-License-Identifier: GPL-2.0-or-later

package exec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ Backend          = (*ExecBackend)(nil)
	_ DiscoveryBackend = (*ExecBackend)(nil)
)

func TestExecBackend_Name(t *testing.T) {
	backend := &ExecBackend{}
	assert.Equal(t, "exec", backend.Name())
}

func TestExecBackend_Close(t *testing.T) {
	backend := &ExecBackend{}
	assert.NoError(t, backend.Close())
}

func TestNewExecBackend_WithCommander(t *testing.T) {
	backend, err := NewExecBackend(
		WithSmartctlPath("/usr/sbin/smartctl"),
		WithCommander(&mockCommander{cmds: map[string]*mockCmd{}}),
	)
	require.NoError(t, err)
	assert.Equal(t, "/usr/sbin/smartctl", backend.smartctlPath)
}

func TestExecBackend_DiscoverDevices(t *testing.T) {
	scanJSON := `{
"devices": [
{"name": "/dev/sda", "type": "ata"}
]
}`
	smartJSON := `{
"device": {"name": "/dev/sda", "type": "ata"},
"model_name": "Test Drive",
"serial_number": "SER123",
"smart_status": {"passed": true}
}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json":                      {output: []byte(scanJSON)},
			"/usr/sbin/smartctl -a -j --nocheck=standby -d ata /dev/sda": {output: []byte(smartJSON)},
		},
	}
	backend, err := NewExecBackend(
		WithSmartctlPath("/usr/sbin/smartctl"),
		WithCommander(commander),
	)
	require.NoError(t, err)

	results, err := backend.DiscoverDevices(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, DiscoveryResult{
		DevicePath:       "/dev/sda",
		DetectedProtocol: "ata",
		SMARTReadable:    true,
		Model:            "Test Drive",
		Serial:           "SER123",
	}, results[0])
}
