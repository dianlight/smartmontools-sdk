// SPDX-License-Identifier: GPL-2.0-or-later

package smartmontools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSMARTEnabledStatusForATADevices verifies that the Enabled field
// is correctly populated from smartctl JSON output for ATA devices
func TestSMARTEnabledStatusForATADevices(t *testing.T) {
	tests := []struct {
		name            string
		mockJSON        string
		expectAvailable bool
		expectEnabled   bool
	}{
		{
			name: "ATA device with SMART enabled",
			mockJSON: `{
				"device": {"name": "/dev/sda", "type": "sat"},
				"model_name": "Test HDD",
				"serial_number": "12345",
				"smart_support": {
					"available": true,
					"enabled": true
				},
				"ata_smart_data": {
					"offline_data_collection": {
						"status": {"value": 0, "string": "completed"}
					}
				}
			}`,
			expectAvailable: true,
			expectEnabled:   true,
		},
		{
			name: "ATA device with SMART disabled",
			mockJSON: `{
				"device": {"name": "/dev/sda", "type": "sat"},
				"model_name": "Test HDD",
				"serial_number": "12345",
				"smart_support": {
					"available": true,
					"enabled": false
				}
			}`,
			expectAvailable: true,
			expectEnabled:   false,
		},
		{
			name: "ATA device without smart_support field (fallback to ata_smart_data)",
			mockJSON: `{
				"device": {"name": "/dev/sda", "type": "sat"},
				"model_name": "Test HDD",
				"serial_number": "12345",
				"ata_smart_data": {
					"offline_data_collection": {
						"status": {"value": 0, "string": "completed"}
					}
				}
			}`,
			expectAvailable: true,
			expectEnabled:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commander := &mockCommander{
				cmds: map[string]*mockCmd{
					// Initial call (device type not yet cached)
					"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(tt.mockJSON)},
					// Second call after GetSMARTInfo caches the type from the JSON response
					"/usr/sbin/smartctl -a -j --nocheck=standby -d sat /dev/sda": {output: []byte(tt.mockJSON)},
				},
			}
			client, err := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
			require.NoError(t, err)

			// Test via GetSMARTInfo
			smartInfo, err := client.GetSMARTInfo(context.Background(), "/dev/sda")
			require.NoError(t, err)
			require.NotNil(t, smartInfo)

			// Test via IsSMARTSupported
			supportInfo, err := client.IsSMARTSupported(context.Background(), "/dev/sda")
			require.NoError(t, err)
			require.NotNil(t, supportInfo)

			assert.Equal(t, tt.expectAvailable, supportInfo.Available, "Available status mismatch")
			assert.Equal(t, tt.expectEnabled, supportInfo.Enabled, "Enabled status mismatch")
		})
	}
}

// TestSMARTDisabledDoesNotWakeDisk verifies that when SMART is disabled,
// the library correctly reports it without requiring additional disk access
func TestSMARTDisabledDoesNotWakeDisk(t *testing.T) {
	mockJSON := `{
		"device": {"name": "/dev/sda", "type": "sat"},
		"model_name": "Test HDD",
		"serial_number": "12345",
		"smart_support": {
			"available": true,
			"enabled": false
		}
	}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
		},
	}
	client, err := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	require.NoError(t, err)

	// First call to get SMART info (this would be cached by the application)
	smartInfo, err := client.GetSMARTInfo(context.Background(), "/dev/sda")
	require.NoError(t, err)
	require.NotNil(t, smartInfo)
	require.NotNil(t, smartInfo.SmartSupport)

	// The Enabled field should be accessible from the SMARTInfo directly
	// without needing another call to IsSMARTSupported
	assert.True(t, smartInfo.SmartSupport.Available)
	assert.False(t, smartInfo.SmartSupport.Enabled)

	// Verify that GetSMARTSupportFromInfo works correctly with this SMARTInfo
	support := client.(*Client).GetSMARTSupportFromInfo(smartInfo)
	assert.True(t, support.Available)
	assert.False(t, support.Enabled, "SMART should be disabled as per the smartctl output")
}

// TestGetSMARTSupportFromInfo verifies the new helper method that extracts
// SMART support status from a cached SMARTInfo without disk I/O
func TestGetSMARTSupportFromInfo(t *testing.T) {
	tests := []struct {
		name            string
		smartInfo       *SMARTInfo
		expectAvailable bool
		expectEnabled   bool
	}{
		{
			name: "With smart_support field - enabled",
			smartInfo: &SMARTInfo{
				SmartSupport: &SmartSupport{
					Available: true,
					Enabled:   true,
				},
			},
			expectAvailable: true,
			expectEnabled:   true,
		},
		{
			name: "With smart_support field - disabled",
			smartInfo: &SMARTInfo{
				SmartSupport: &SmartSupport{
					Available: true,
					Enabled:   false,
				},
			},
			expectAvailable: true,
			expectEnabled:   false,
		},
		{
			name: "Fallback to AtaSmartData",
			smartInfo: &SMARTInfo{
				AtaSmartData: &AtaSmartData{
					Table: []SmartAttribute{},
				},
			},
			expectAvailable: true,
			expectEnabled:   true,
		},
		{
			name: "Fallback to NvmeSmartHealth",
			smartInfo: &SMARTInfo{
				NvmeSmartHealth: &NvmeSmartHealth{
					Temperature: 45,
				},
			},
			expectAvailable: true,
			expectEnabled:   true,
		},
		{
			name:            "No SMART support",
			smartInfo:       &SMARTInfo{},
			expectAvailable: false,
			expectEnabled:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commander := &mockCommander{
				cmds: map[string]*mockCmd{},
			}
			client, err := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
			require.NoError(t, err)

			support := client.GetSMARTSupportFromInfo(tt.smartInfo)
			assert.Equal(t, tt.expectAvailable, support.Available, "Available status mismatch")
			assert.Equal(t, tt.expectEnabled, support.Enabled, "Enabled status mismatch")
		})
	}
}
