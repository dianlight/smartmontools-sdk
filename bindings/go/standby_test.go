// SPDX-License-Identifier: GPL-2.0-or-later

package smartmontools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSMARTInfoWithNoCheckStandby(t *testing.T) {
	mockJSON := `{
"json_format_version": [1, 0],
"smartctl": {"version": [7, 5], "exit_status": 0},
"device": {"name": "/dev/sda", "type": "sat"},
"model_name": "Test Drive",
"smart_status": {"passed": true}
}`
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
	}}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	info, err := client.GetSMARTInfo(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.Equal(t, "/dev/sda", info.Device.Name)
	assert.Equal(t, "Test Drive", info.ModelName)
}

func TestCheckHealthWithNoCheckStandby(t *testing.T) {
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -H --nocheck=standby /dev/sda": {output: []byte("SMART overall-health self-assessment test result: PASSED")},
	}}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	healthy, err := client.CheckHealth(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.True(t, healthy)
}

func TestGetDeviceInfoWithNoCheckStandby(t *testing.T) {
	mockJSON := `{"device":{"name":"/dev/sda","type":"ata"},"model_name":"Test Drive","serial_number":"12345"}`
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -i -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
	}}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	info, err := client.GetDeviceInfo(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	model, ok := info["model_name"].(string)
	assert.True(t, ok)
	assert.Equal(t, "Test Drive", model)
}

func TestGetAvailableSelfTestsWithNoCheckStandby(t *testing.T) {
	mockJSON := `{"ata_smart_data":{"capabilities":{"self_tests_supported":true}}}`
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -c -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
	}}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	info, err := client.GetAvailableSelfTests(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.Len(t, info.Available, 2)
}

func TestGetSMARTInfoWithCachedATADeviceType(t *testing.T) {
	mockJSON := `{"device":{"name":"/dev/sda","type":"sat"},"model_name":"Test Drive","smart_status":{"passed":true}}`
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -a -j --nocheck=standby -d sat /dev/sda": {output: []byte(mockJSON)},
	}}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	client.(*Client).backend.(*ExecBackend).SetDeviceTypeHint("/dev/sda", "sat")
	info, err := client.GetSMARTInfo(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.Equal(t, "/dev/sda", info.Device.Name)
	assert.Equal(t, "Test Drive", info.ModelName)
}

func TestGetSMARTInfoWithCachedNVMeDeviceType(t *testing.T) {
	mockJSON := `{"device":{"name":"/dev/nvme0n1","type":"nvme"},"model_name":"NVMe Drive","nvme_smart_health_information_log":{"temperature":35}}`
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -a -j -d nvme /dev/nvme0n1": {output: []byte(mockJSON)},
	}}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	client.(*Client).backend.(*ExecBackend).SetDeviceTypeHint("/dev/nvme0n1", "nvme")
	info, err := client.GetSMARTInfo(context.Background(), "/dev/nvme0n1")
	assert.NoError(t, err)
	assert.Equal(t, "/dev/nvme0n1", info.Device.Name)
	assert.Equal(t, "NVMe", info.DiskType)
}

func TestInStandbyField(t *testing.T) {
	info := SMARTInfo{Device: Device{Name: "/dev/sda", Type: "sat"}, InStandby: true}
	assert.True(t, info.InStandby)
	assert.Equal(t, "/dev/sda", info.Device.Name)
}
