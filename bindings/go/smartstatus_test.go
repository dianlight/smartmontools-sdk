// SPDX-License-Identifier: GPL-2.0-or-later

package smartmontools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSMARTInfo_PopulatesSmartStatus(t *testing.T) {
	mockJSON := `{
"device": {"name": "/dev/sda", "type": "ata"},
"model_name": "Test Drive",
"smart_status": {"passed": true},
"ata_smart_data": {"self_test": {"status": {"value": 245, "string": "Self-test routine in progress"}}}
}`
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
	}}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetSMARTInfo(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.True(t, info.SmartStatus.Running)
	assert.True(t, info.SmartStatus.Passed)
}

func TestGetSMARTInfo_StandbyMode_PopulatesSmartStatus(t *testing.T) {
	mockJSON := `{
"device": {"name": "/dev/sda", "type": "ata"},
"model_name": "Test Drive",
"smart_status": {"passed": true},
"ata_smart_data": {"self_test": {"status": {"value": 240, "string": "Self-test running"}}}
}`
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sdx": {output: []byte(mockJSON)},
	}}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetSMARTInfo(context.Background(), "/dev/sdx")
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.True(t, info.SmartStatus.Running)
	assert.True(t, info.SmartStatus.Passed)
}

func TestRunSelfTestWithProgress_UsesRemainingPercent(t *testing.T) {
	capsJSON := `{
"ata_smart_data": {
"capabilities": {"self_tests_supported": true},
"self_test": {"polling_minutes": {"short": 2}}
}
}`
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -c -j --nocheck=standby /dev/sda": {output: []byte(capsJSON)},
		"/usr/sbin/smartctl -t short /dev/sda":                {output: []byte("")},
	}}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	assert.NoError(t, client.RunSelfTestWithProgress(context.Background(), "/dev/sda", "short", nil))
}

func TestStatusFieldUnmarshal_WithRemainingPercent(t *testing.T) {
	jsonData := `{"value":245,"string":"Self-test routine in progress","remaining_percent":60}`
	var status StatusField
	err := status.UnmarshalJSON([]byte(jsonData))
	assert.NoError(t, err)
	assert.Equal(t, 245, status.Value)
	assert.Equal(t, "Self-test routine in progress", status.String)
	require.NotNil(t, status.RemainingPercent)
	assert.Equal(t, 60, *status.RemainingPercent)
}

func TestNvmeSmartTestLog(t *testing.T) {
	mockJSON := `{
"device": {"name": "/dev/nvme0n1", "type": "nvme"},
"smart_status": {"passed": true},
"nvme_smart_test_log": {"current_operation": 1, "current_completion": 45}
}`
	commander := &mockCommander{cmds: map[string]*mockCmd{
		"/usr/sbin/smartctl -a -j -d nvme /dev/nvme0n1": {output: []byte(mockJSON)},
	}}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	client.(*Client).backend.(*ExecBackend).SetDeviceTypeHint("/dev/nvme0n1", "nvme")

	info, err := client.GetSMARTInfo(context.Background(), "/dev/nvme0n1")
	assert.NoError(t, err)
	require.NotNil(t, info.NvmeSmartTestLog)
	require.NotNil(t, info.NvmeSmartTestLog.CurrentOpeation)
	assert.Equal(t, 1, *info.NvmeSmartTestLog.CurrentOpeation)
	require.NotNil(t, info.NvmeSmartTestLog.CurrentCompletion)
	assert.Equal(t, 45, *info.NvmeSmartTestLog.CurrentCompletion)
	assert.True(t, info.SmartStatus.Running)
}

func TestWearLevelPercent_NVMe(t *testing.T) {
	info := &SMARTInfo{DiskType: "NVMe", NvmeSmartHealth: &NvmeSmartHealth{PercentageUsed: 23}}
	got := info.WearLevelPercent()
	require.NotNil(t, got)
	assert.Equal(t, 23, *got)
}

func TestWearLevelPercent_NVMe_NilHealth(t *testing.T) {
	assert.Nil(t, (&SMARTInfo{DiskType: "NVMe"}).WearLevelPercent())
}

func TestWearLevelPercent_SSD_Attr231(t *testing.T) {
	info := &SMARTInfo{DiskType: "SSD", AtaSmartData: &AtaSmartData{Table: []SmartAttribute{{ID: SmartAttrSSDLifeLeft, Value: 75}, {ID: SmartAttrWearLevelingCount, Value: 60}}}}
	got := info.WearLevelPercent()
	require.NotNil(t, got)
	assert.Equal(t, 25, *got)
}

func TestWearLevelPercent_SSD_Attr177(t *testing.T) {
	info := &SMARTInfo{DiskType: "SSD", AtaSmartData: &AtaSmartData{Table: []SmartAttribute{{ID: SmartAttrWearLevelingCount, Value: 80}}}}
	got := info.WearLevelPercent()
	require.NotNil(t, got)
	assert.Equal(t, 20, *got)
}

func TestWearLevelPercent_SSD_Attr173(t *testing.T) {
	info := &SMARTInfo{DiskType: "SSD", AtaSmartData: &AtaSmartData{Table: []SmartAttribute{{ID: SmartAttrSSDLifeUsed, Raw: Raw{Value: 42}}}}}
	got := info.WearLevelPercent()
	require.NotNil(t, got)
	assert.Equal(t, 42, *got)
}

func TestWearLevelPercent_HDD(t *testing.T) {
	assert.Nil(t, (&SMARTInfo{DiskType: "HDD"}).WearLevelPercent())
}

func TestWearLevelPercent_Unknown(t *testing.T) {
	assert.Nil(t, (&SMARTInfo{DiskType: "Unknown"}).WearLevelPercent())
}

func TestWearLevelPercent_SSD_NoRelevantAttrs(t *testing.T) {
	info := &SMARTInfo{DiskType: "SSD", AtaSmartData: &AtaSmartData{Table: []SmartAttribute{{ID: 9, Value: 99}, {ID: 12, Value: 99}}}}
	assert.Nil(t, info.WearLevelPercent())
}

func TestWearLevelPercent_Clamping(t *testing.T) {
	tests := []struct {
		name string
		info *SMARTInfo
		want int
	}{
		{"NVMe percentage_used > 100 clamped to 100", &SMARTInfo{DiskType: "NVMe", NvmeSmartHealth: &NvmeSmartHealth{PercentageUsed: 120}}, 100},
		{"SSD attr231 value=0 gives 100", &SMARTInfo{DiskType: "SSD", AtaSmartData: &AtaSmartData{Table: []SmartAttribute{{ID: SmartAttrSSDLifeLeft, Value: 0}}}}, 100},
		{"SSD attr231 value=100 gives 0", &SMARTInfo{DiskType: "SSD", AtaSmartData: &AtaSmartData{Table: []SmartAttribute{{ID: SmartAttrSSDLifeLeft, Value: 100}}}}, 0},
		{"SSD attr173 raw > 100 clamped to 100", &SMARTInfo{DiskType: "SSD", AtaSmartData: &AtaSmartData{Table: []SmartAttribute{{ID: SmartAttrSSDLifeUsed, Raw: Raw{Value: 200}}}}}, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.WearLevelPercent()
			require.NotNil(t, got)
			assert.Equal(t, tt.want, *got)
		})
	}
}
