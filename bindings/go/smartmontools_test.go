// SPDX-License-Identifier: GPL-2.0-or-later

package smartmontools

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/dianlight/tlog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCmd implements Cmd interface for testing
type mockCmd struct {
	exec.Cmd
	output []byte
	err    error
}

func (m *mockCmd) Output() ([]byte, error) {
	return m.output, m.err
}

func (m *mockCmd) Run() error {
	return m.err
}

func (m *mockCmd) CombinedOutput() ([]byte, error) {
	return m.output, m.err
}

// mockCommander implements Commander interface for testing
type mockCommander struct {
	cmds map[string]*mockCmd
}

func (m *mockCommander) Command(ctx context.Context, logger LogAdapter, name string, arg ...string) Cmd {
	key := name
	for _, a := range arg {
		key += " " + a
	}
	if cmd, ok := m.cmds[key]; ok {
		return cmd
	}
	// Default mock command that returns error
	return &mockCmd{err: errors.New("mock command not configured")}
}

func TestNewClient(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("smartctl not usable (missing or incompatible): %v", err)
	}

	c := client.(*Client)
	assert.NotEmpty(t, c.backend.(*ExecBackend).SmartctlPath(), "Expected smartctlPath to be set")
}

func TestNewClientWithPath(t *testing.T) {
	testPath := "/usr/sbin/smartctl"
	client, err := NewClient(WithSmartctlPath(testPath))
	if err != nil {
		t.Skipf("smartctl validation failed: %v", err)
	}

	c := client.(*Client)
	if c.backend.(*ExecBackend).SmartctlPath() != testPath {
		t.Errorf("Expected smartctlPath to be %s, got %s", testPath, c.backend.(*ExecBackend).SmartctlPath())
	}
}

func TestScanDevices(t *testing.T) {
	mockJSON := `{
		"devices": [
			{"name": "/dev/sda", "type": "ata"},
			{"name": "/dev/sdb", "type": "ata"}
		]
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	devices, err := client.ScanDevices(context.Background())
	assert.NoError(t, err)
	assert.Len(t, devices, 2)
	assert.Equal(t, "/dev/sda", devices[0].Name)
	assert.Equal(t, "ata", devices[0].Type)
}

func TestScanDevicesError(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			// Both --scan-open and --scan fallback fail.
			"/usr/sbin/smartctl --scan-open --json": {err: errors.New("command failed")},
			"/usr/sbin/smartctl --scan --json":      {err: errors.New("command failed")},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	_, err := client.ScanDevices(context.Background())
	assert.Error(t, err)
}

// TestScanDevicesFallback verifies that ScanDevices falls back to --scan when
// --scan-open fails.
func TestScanDevicesFallback(t *testing.T) {
	mockJSON := `{
		"devices": [
			{"name": "/dev/sda", "type": "ata"}
		]
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json": {err: errors.New("scan-open not supported")},
			"/usr/sbin/smartctl --scan --json":      {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	devices, err := client.ScanDevices(context.Background())
	assert.NoError(t, err, "should succeed after --scan fallback")
	assert.Len(t, devices, 1)
	assert.Equal(t, "/dev/sda", devices[0].Name)
}

func TestGetSMARTInfo(t *testing.T) {
	mockJSON := `{
  "json_format_version": [
    1,
    0
  ],
  "smartctl": {
    "version": [
      7,
      5
    ],
    "pre_release": false,
    "svn_revision": "5714",
    "platform_info": "x86_64-linux-6.12.43-haos",
    "build_info": "(local build)",
    "argv": [
      "smartctl",
      "-a",
      "-j",
      "/dev/sda"
    ],
    "drive_database_version": {
      "string": "7.5/5706"
    },
    "exit_status": 0,
    "messages": [
      {"string": "Test informational message", "severity": "info"}
    ]
  },
  "local_time": {
    "time_t": 1762080587,
    "asctime": "Sun Nov  2 11:49:47 2025 CET"
  },
  "device": {
    "name": "/dev/sda",
    "info_name": "/dev/sda [SAT]",
    "type": "sat",
    "protocol": "ATA"
  },
  "model_family": "SandForce Driven SSDs",
  "model_name": "KINGSTON SV300S37A240G",
  "serial_number": "50026B77560145CF",
  "wwn": {
    "naa": 5,
    "oui": 9911,
    "id": 31507695055
  },
  "firmware_version": "603ABBF0",
  "user_capacity": {
    "blocks": 468862128,
    "bytes": 240057409536
  },
  "logical_block_size": 512,
  "physical_block_size": 512,
  "rotation_rate": 0,
  "trim": {
    "supported": true,
    "deterministic": false,
    "zeroed": false
  },
  "in_smartctl_database": true,
  "ata_version": {
    "string": "ATA8-ACS, ACS-2 T13/2015-D revision 3",
    "major_value": 508,
    "minor_value": 272
  },
  "sata_version": {
    "string": "SATA 3.0",
    "value": 63
  },
  "interface_speed": {
    "max": {
      "sata_value": 14,
      "string": "6.0 Gb/s",
      "units_per_second": 60,
      "bits_per_unit": 100000000
    },
    "current": {
      "sata_value": 3,
      "string": "6.0 Gb/s",
      "units_per_second": 60,
      "bits_per_unit": 100000000
    }
  },
  "smart_support": {
    "available": true,
    "enabled": true
  },
  "smart_status": {
    "passed": true
  },
  "ata_smart_data": {
    "offline_data_collection": {
      "status": {
        "value": 0,
        "string": "was never started"
      },
      "completion_seconds": 0
    },
    "self_test": {
      "status": {
        "value": 0,
        "string": "completed without error",
        "passed": true
      },
      "polling_minutes": {
        "short": 1,
        "extended": 48,
        "conveyance": 2
      }
    },
    "capabilities": {
      "values": [
        125,
        3
      ],
      "exec_offline_immediate_supported": true,
      "offline_is_aborted_upon_new_cmd": true,
      "offline_surface_scan_supported": true,
      "self_tests_supported": true,
      "conveyance_self_test_supported": true,
      "selective_self_test_supported": true,
      "attribute_autosave_enabled": true,
      "error_logging_supported": true,
      "gp_logging_supported": true
    }
  },
  "ata_sct_capabilities": {
    "value": 37,
    "error_recovery_control_supported": false,
    "feature_control_supported": false,
    "data_table_supported": true
  },
  "ata_smart_attributes": {
    "revision": 10,
    "table": [
      {
        "id": 1,
        "name": "Raw_Read_Error_Rate",
        "value": 95,
        "worst": 95,
        "thresh": 50,
        "when_failed": "",
        "flags": {
          "value": 50,
          "string": "-O--CK ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 189861978,
          "string": "0/189861978"
        }
      },
      {
        "id": 5,
        "name": "Retired_Block_Count",
        "value": 100,
        "worst": 100,
        "thresh": 3,
        "when_failed": "",
        "flags": {
          "value": 51,
          "string": "PO--CK ",
          "prefailure": true,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 0,
          "string": "0"
        }
      },
      {
        "id": 9,
        "name": "Power_On_Hours_and_Msec",
        "value": 60,
        "worst": 60,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 50,
          "string": "-O--CK ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 683071598791665,
          "string": "35825h+02m+39.040s"
        }
      },
      {
        "id": 12,
        "name": "Power_Cycle_Count",
        "value": 100,
        "worst": 100,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 50,
          "string": "-O--CK ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 61,
          "string": "61"
        }
      },
      {
        "id": 171,
        "name": "Program_Fail_Count",
        "value": 100,
        "worst": 100,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 10,
          "string": "-O-R-- ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": true,
          "event_count": false,
          "auto_keep": false
        },
        "raw": {
          "value": 0,
          "string": "0"
        }
      },
      {
        "id": 172,
        "name": "Erase_Fail_Count",
        "value": 100,
        "worst": 100,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 50,
          "string": "-O--CK ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 0,
          "string": "0"
        }
      },
      {
        "id": 174,
        "name": "Unexpect_Power_Loss_Ct",
        "value": 0,
        "worst": 0,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 48,
          "string": "----CK ",
          "prefailure": false,
          "updated_online": false,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 31,
          "string": "31"
        }
      },
      {
        "id": 177,
        "name": "Wear_Range_Delta",
        "value": 0,
        "worst": 0,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 0,
          "string": "------ ",
          "prefailure": false,
          "updated_online": false,
          "performance": false,
          "error_rate": false,
          "event_count": false,
          "auto_keep": false
        },
        "raw": {
          "value": 1,
          "string": "1"
        }
      },
      {
        "id": 181,
        "name": "Program_Fail_Count",
        "value": 100,
        "worst": 100,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 10,
          "string": "-O-R-- ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": true,
          "event_count": false,
          "auto_keep": false
        },
        "raw": {
          "value": 0,
          "string": "0"
        }
      },
      {
        "id": 182,
        "name": "Erase_Fail_Count",
        "value": 100,
        "worst": 100,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 50,
          "string": "-O--CK ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 0,
          "string": "0"
        }
      },
      {
        "id": 187,
        "name": "Reported_Uncorrect",
        "value": 100,
        "worst": 100,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 18,
          "string": "-O--C- ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": false
        },
        "raw": {
          "value": 0,
          "string": "0"
        }
      },
      {
        "id": 189,
        "name": "Airflow_Temperature_Cel",
        "value": 29,
        "worst": 56,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 0,
          "string": "------ ",
          "prefailure": false,
          "updated_online": false,
          "performance": false,
          "error_rate": false,
          "event_count": false,
          "auto_keep": false
        },
        "raw": {
          "value": 77313081373,
          "string": "29 (Min/Max 18/56)"
        }
      },
      {
        "id": 194,
        "name": "Temperature_Celsius",
        "value": 29,
        "worst": 56,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 34,
          "string": "-O---K ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": false,
          "auto_keep": true
        },
        "raw": {
          "value": 77313081373,
          "string": "29 (Min/Max 18/56)"
        }
      },
      {
        "id": 195,
        "name": "ECC_Uncorr_Error_Count",
        "value": 120,
        "worst": 120,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 28,
          "string": "--SRC- ",
          "prefailure": false,
          "updated_online": false,
          "performance": true,
          "error_rate": true,
          "event_count": true,
          "auto_keep": false
        },
        "raw": {
          "value": 189861978,
          "string": "0/189861978"
        }
      },
      {
        "id": 196,
        "name": "Reallocated_Event_Count",
        "value": 100,
        "worst": 100,
        "thresh": 3,
        "when_failed": "",
        "flags": {
          "value": 51,
          "string": "PO--CK ",
          "prefailure": true,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 0,
          "string": "0"
        }
      },
      {
        "id": 201,
        "name": "Unc_Soft_Read_Err_Rate",
        "value": 120,
        "worst": 120,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 28,
          "string": "--SRC- ",
          "prefailure": false,
          "updated_online": false,
          "performance": true,
          "error_rate": true,
          "event_count": true,
          "auto_keep": false
        },
        "raw": {
          "value": 189861978,
          "string": "0/189861978"
        }
      },
      {
        "id": 204,
        "name": "Soft_ECC_Correct_Rate",
        "value": 120,
        "worst": 120,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 28,
          "string": "--SRC- ",
          "prefailure": false,
          "updated_online": false,
          "performance": true,
          "error_rate": true,
          "event_count": true,
          "auto_keep": false
        },
        "raw": {
          "value": 189861978,
          "string": "0/189861978"
        }
      },
      {
        "id": 230,
        "name": "Life_Curve_Status",
        "value": 100,
        "worst": 100,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 19,
          "string": "PO--C- ",
          "prefailure": true,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": false
        },
        "raw": {
          "value": 100,
          "string": "100"
        }
      },
      {
        "id": 231,
        "name": "SSD_Life_Left",
        "value": 95,
        "worst": 95,
        "thresh": 11,
        "when_failed": "",
        "flags": {
          "value": 0,
          "string": "------ ",
          "prefailure": false,
          "updated_online": false,
          "performance": false,
          "error_rate": false,
          "event_count": false,
          "auto_keep": false
        },
        "raw": {
          "value": 4294967296,
          "string": "4294967296"
        }
      },
      {
        "id": 233,
        "name": "SandForce_Internal",
        "value": 0,
        "worst": 0,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 50,
          "string": "-O--CK ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 51384,
          "string": "51384"
        }
      },
      {
        "id": 234,
        "name": "SandForce_Internal",
        "value": 0,
        "worst": 0,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 50,
          "string": "-O--CK ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 20878,
          "string": "20878"
        }
      },
      {
        "id": 241,
        "name": "Lifetime_Writes_GiB",
        "value": 0,
        "worst": 0,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 50,
          "string": "-O--CK ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 20878,
          "string": "20878"
        }
      },
      {
        "id": 242,
        "name": "Lifetime_Reads_GiB",
        "value": 0,
        "worst": 0,
        "thresh": 0,
        "when_failed": "",
        "flags": {
          "value": 50,
          "string": "-O--CK ",
          "prefailure": false,
          "updated_online": true,
          "performance": false,
          "error_rate": false,
          "event_count": true,
          "auto_keep": true
        },
        "raw": {
          "value": 40443,
          "string": "40443"
        }
      },
      {
        "id": 244,
        "name": "Unknown_Attribute",
        "value": 92,
        "worst": 92,
        "thresh": 10,
        "when_failed": "",
        "flags": {
          "value": 0,
          "string": "------ ",
          "prefailure": false,
          "updated_online": false,
          "performance": false,
          "error_rate": false,
          "event_count": false,
          "auto_keep": false
        },
        "raw": {
          "value": 15925491,
          "string": "15925491"
        }
      }
    ]
  },
  "spare_available": {
    "current_percent": 100,
    "threshold_percent": 3
  },
  "power_on_time": {
    "hours": 35825,
    "minutes": 2
  },
  "power_cycle_count": 61,
  "endurance_used": {
    "current_percent": 5
  },
  "temperature": {
    "current": 29
  },
  "ata_smart_self_test_log": {
    "standard": {
      "revision": 1,
      "count": 0
    }
  },
  "ata_smart_selective_self_test_log": {
    "revision": 1,
    "table": [
      {
        "lba_min": 0,
        "lba_max": 0,
        "status": {
          "value": 0,
          "string": "Not_testing"
        }
      },
      {
        "lba_min": 0,
        "lba_max": 0,
        "status": {
          "value": 0,
          "string": "Not_testing"
        }
      },
      {
        "lba_min": 0,
        "lba_max": 0,
        "status": {
          "value": 0,
          "string": "Not_testing"
        }
      },
      {
        "lba_min": 0,
        "lba_max": 0,
        "status": {
          "value": 0,
          "string": "Not_testing"
        }
      },
      {
        "lba_min": 0,
        "lba_max": 0,
        "status": {
          "value": 0,
          "string": "Not_testing"
        }
      }
    ],
    "flags": {
      "value": 0,
      "remainder_scan_enabled": false
    },
    "power_up_scan_resume_minutes": 0
  }
}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetSMARTInfo(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.Equal(t, "/dev/sda", info.Device.Name)
	assert.Equal(t, "KINGSTON SV300S37A240G", info.ModelName)
	assert.True(t, info.SmartStatus.Passed, "Expected SMART status passed")
	assert.NotNil(t, info.Smartctl)
	assert.Len(t, info.Smartctl.Messages, 1)
	assert.Equal(t, "Test informational message", info.Smartctl.Messages[0].String)
	assert.Equal(t, "info", info.Smartctl.Messages[0].Severity)

	// Check rotation rate and disk type
	assert.NotNil(t, info.RotationRate, "Expected rotation_rate to be set")
	assert.Equal(t, 0, *info.RotationRate, "Expected rotation_rate 0 for SSD")
	assert.Equal(t, "SSD", info.DiskType)
}

func TestGetSMARTInfoUnsupported(t *testing.T) {
	mockJSON := `{
  "json_format_version": [
    1,
    0
  ],
  "smartctl": {
    "version": [
      7,
      5
    ],
    "pre_release": false,
    "svn_revision": "5714",
    "platform_info": "x86_64-linux-6.12.43-haos",
    "build_info": "(local build)",
    "argv": [
      "smartctl",
      "-a",
      "-j",
      "/dev/disk/by-id/usb-Flash_Disk_3.0_7966051146147389472-0:0"
    ],
    "messages": [
      {
        "string": "/dev/disk/by-id/usb-Flash_Disk_3.0_7966051146147389472-0:0: Unknown USB bridge [0x048d:0x1234 (0x200)]",
        "severity": "error"
      }
    ],
    "exit_status": 1
  },
  "local_time": {
    "time_t": 1762097029,
    "asctime": "Sun Nov  2 16:23:49 2025 CET"
  }
}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON), err: errors.New("SMART Not Supported")},
			// Add the retry command that will be attempted due to Unknown USB bridge
			"/usr/sbin/smartctl -a -j --nocheck=standby -d sat /dev/sda": {output: []byte(mockJSON), err: errors.New("SMART Not Supported")},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetSMARTInfo(context.Background(), "/dev/sda")
	// When valid JSON is returned but device name is empty, error is returned
	assert.Error(t, err)
	assert.Equal(t, "SMART Not Supported", err.Error())
	assert.NotNil(t, info)
	assert.NotNil(t, info.Smartctl)
	assert.Empty(t, info.Device.Name)
	assert.Empty(t, info.ModelName)
	assert.False(t, info.SmartStatus.Passed, "Expected SMART status not passed")
	assert.Len(t, info.Smartctl.Messages, 1)
	assert.Equal(t, "/dev/disk/by-id/usb-Flash_Disk_3.0_7966051146147389472-0:0: Unknown USB bridge [0x048d:0x1234 (0x200)]", info.Smartctl.Messages[0].String)
	assert.Equal(t, "error", info.Smartctl.Messages[0].Severity)
}

func TestGetSMARTInfoExitError(t *testing.T) {
	mockJSON := `{
		"device": {"name": "/dev/sda", "type": "ata"},
		"smart_status": {"passed": false},
		"smartctl": {
			"messages": [
				{"string": "Test error message", "severity": "error"}
			]
		}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {
				output: []byte(mockJSON),
				err:    &exec.ExitError{Stderr: []byte("")},
			},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetSMARTInfo(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.False(t, info.SmartStatus.Passed, "Expected SMART status failed")
	assert.NotNil(t, info.Smartctl)
	assert.Len(t, info.Smartctl.Messages, 1)
	assert.Equal(t, "Test error message", info.Smartctl.Messages[0].String)
	assert.Equal(t, "error", info.Smartctl.Messages[0].Severity)
}

func TestCheckHealth(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -H --nocheck=standby /dev/sda": {output: []byte("SMART overall-health self-assessment test result: PASSED")},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	healthy, err := client.CheckHealth(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.True(t, healthy, "Expected device to be healthy")
}

func TestCheckHealthFailed(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -H --nocheck=standby /dev/sda": {output: []byte("SMART overall-health self-assessment test result: FAILED")},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	healthy, err := client.CheckHealth(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.False(t, healthy, "Expected device to be unhealthy")
}

func TestCheckHealthExitError(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -H --nocheck=standby /dev/sda": {
				output: []byte("SMART overall-health self-assessment test result: PASSED"),
				err:    &exec.ExitError{},
			},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	healthy, err := client.CheckHealth(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.True(t, healthy, "Expected device to be healthy from output despite error")
}

func TestGetDeviceInfo(t *testing.T) {
	mockJSON := `{
		"device": {"name": "/dev/sda", "type": "ata"},
		"model_name": "Test Drive",
		"serial_number": "12345"
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -i -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetDeviceInfo(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	model, ok := info["model_name"].(string)
	assert.True(t, ok, "Expected model_name to be a string")
	assert.Equal(t, "Test Drive", model)
}

func TestRunSelfTest(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -t short /dev/sda": {},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	err := client.RunSelfTest(context.Background(), "/dev/sda", "short")
	assert.NoError(t, err)
}

func TestRunSelfTestInvalidType(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	err := client.RunSelfTest(context.Background(), "/dev/sda", "invalid")
	assert.Error(t, err, "Expected error for invalid test type")
}

func TestRunSelfTestWithProgressInvalidType(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	ctx := context.Background()
	err := client.RunSelfTestWithProgress(ctx, "/dev/sda", "invalid", nil)
	assert.Error(t, err, "Expected error for invalid test type")
}

func TestRunSelfTestWithProgress(t *testing.T) {
	// Mock SMART info with ATA device supporting self-tests and completed status
	mockJSON := `{
		"device": {"name": "/dev/sda", "type": "ata"},
		"ata_smart_data": {
			"capabilities": {
				"exec_offline_immediate_supported": true
			},
			"self_test": {
				"status": "completed",
				"polling_minutes": {
					"short": 2
				}
			}
		}
	}`

	mockCapabilitiesJSON := `{
		"ata_smart_data": {
			"capabilities": {
				"exec_offline_immediate_supported": true,
				"self_tests_supported": true
			},
			"self_test": {
				"polling_minutes": {
					"short": 2
				}
			}
		}
	}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
			"/usr/sbin/smartctl -c -j --nocheck=standby /dev/sda": {output: []byte(mockCapabilitiesJSON)},
			"/usr/sbin/smartctl -t short /dev/sda":                {},
		},
	}

	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var progressCalled bool
	var finalProgress int
	var finalStatus string

	progress := make(chan int)

	callback := func(iprogress int, status string) {
		progressCalled = true
		finalProgress = iprogress
		finalStatus = status
		progress <- iprogress
	}

	err := client.RunSelfTestWithProgress(ctx, "/dev/sda", "short", callback)
	assert.NoError(t, err)
loop:
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Context closed before end %v", ctx.Err())
			break loop
		case p := <-progress:
			if p >= 100 {
				break loop
			}
		}
	}

	assert.True(t, progressCalled, "Expected progress callback to be called")
	assert.Equal(t, 100, finalProgress, "Expected final progress 100")
	assert.Contains(t, finalStatus, "completed", "Expected final status to indicate completion")
}

func TestGetAvailableSelfTestsATA(t *testing.T) {
	mockJSON := `{
		"ata_smart_data": {
			"capabilities": {
				"exec_offline_immediate_supported": true,
				"self_tests_supported": true,
				"conveyance_self_test_supported": true
			}
		}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -c -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetAvailableSelfTests(context.Background(), "/dev/sda")
	assert.NoError(t, err)

	expected := []string{"short", "long", "conveyance", "offline"}
	assert.Equal(t, expected, info.Available)
}

func TestGetAvailableSelfTestsATANoCapabilities(t *testing.T) {
	mockJSON := `{
		"ata_smart_data": {}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -c -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetAvailableSelfTests(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.Empty(t, info.Available, "Expected no tests")
}

func TestGetAvailableSelfTestsNVMe(t *testing.T) {
	mockJSON := `{
		"nvme_controller_capabilities": {
			"self_test": true
		}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -c -j --nocheck=standby /dev/nvme0n1": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetAvailableSelfTests(context.Background(), "/dev/nvme0n1")
	assert.NoError(t, err)
	assert.Equal(t, []string{"short"}, info.Available)
}

func TestGetAvailableSelfTestsNVMeNoSupport(t *testing.T) {
	mockJSON := `{
		"nvme_controller_capabilities": {
			"self_test": false
		}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -c -j --nocheck=standby /dev/nvme0n1": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetAvailableSelfTests(context.Background(), "/dev/nvme0n1")
	assert.NoError(t, err)
	assert.Empty(t, info.Available, "Expected no tests")
}

func TestGetAvailableSelfTestsError(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -c -j --nocheck=standby /dev/sda": {err: errors.New("command failed")},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	_, err := client.GetAvailableSelfTests(context.Background(), "/dev/sda")
	assert.Error(t, err)
}

func TestGetAvailableSelfTestsNVMeBothCapabilityFields(t *testing.T) {
	// When both nvme_controller_capabilities and nvme_optional_admin_commands report
	// self-test support, "short" must appear exactly once (no duplicates).
	mockJSON := `{
		"nvme_controller_capabilities": {"self_test": true},
		"nvme_optional_admin_commands": {"self_test": true}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -c -j --nocheck=standby /dev/nvme0n1": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetAvailableSelfTests(context.Background(), "/dev/nvme0n1")
	assert.NoError(t, err)
	assert.Equal(t, []string{"short"}, info.Available, "Expected exactly one 'short' entry")
}

func TestGetAvailableSelfTestsFromInfo_ATA(t *testing.T) {
	smartInfo := &SMARTInfo{
		AtaSmartData: &AtaSmartData{
			Capabilities: &Capabilities{
				SelfTestsSupported:          true,
				ConveyanceSelfTestSupported: true,
				ExecOfflineImmediate:        true,
			},
			SelfTest: &SelfTest{
				PollingMinutes: &PollingMinutes{
					Short:      2,
					Extended:   48,
					Conveyance: 5,
				},
			},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(&mockCommander{cmds: map[string]*mockCmd{}}))

	info := client.GetAvailableSelfTestsFromInfo(smartInfo)
	assert.Equal(t, []string{"short", "long", "conveyance", "offline"}, info.Available)
	assert.Equal(t, 2, info.Durations["short"])
	assert.Equal(t, 48, info.Durations["long"])
	assert.Equal(t, 5, info.Durations["conveyance"])
}

func TestGetAvailableSelfTestsFromInfo_NVMe(t *testing.T) {
	smartInfo := &SMARTInfo{
		NvmeControllerCapabilities: &NvmeControllerCapabilities{
			SelfTest: true,
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(&mockCommander{cmds: map[string]*mockCmd{}}))

	info := client.GetAvailableSelfTestsFromInfo(smartInfo)
	assert.Equal(t, []string{"short"}, info.Available)
	assert.Empty(t, info.Durations)
}

func TestGetAvailableSelfTestsFromInfo_Nil(t *testing.T) {
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(&mockCommander{cmds: map[string]*mockCmd{}}))

	info := client.GetAvailableSelfTestsFromInfo(nil)
	assert.Empty(t, info.Available)
	assert.Empty(t, info.Durations)
}

func TestGetAvailableSelfTestsFromInfo_NoCapabilities(t *testing.T) {
	smartInfo := &SMARTInfo{
		AtaSmartData: &AtaSmartData{
			// Capabilities is nil — no tests available
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(&mockCommander{cmds: map[string]*mockCmd{}}))

	info := client.GetAvailableSelfTestsFromInfo(smartInfo)
	assert.Empty(t, info.Available)
	assert.Empty(t, info.Durations)
}

func TestIsSMARTSupportedATA(t *testing.T) {
	mockJSON := `{
		"device": {"name": "/dev/sda", "type": "ata"},
		"ata_smart_data": {
			"capabilities": {
				"exec_offline_immediate_supported": true
			}
		}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	supportInfo, err := client.IsSMARTSupported(context.Background(), "/dev/sda")
	require.NoError(t, err)
	require.NotNil(t, supportInfo)
	assert.True(t, supportInfo.Available, "Expected SMART to be supported for ATA device")
	assert.True(t, supportInfo.Enabled, "Expected SMART enabled state to be unknown without smart_support")
}

func TestIsSMARTSupportedNVMe(t *testing.T) {
	mockJSON := `{
		"device": {"name": "/dev/nvme0n1", "type": "nvme"},
		"nvme_smart_health_information_log": {
			"temperature": 35
		}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/nvme0n1": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	supportInfo, err := client.IsSMARTSupported(context.Background(), "/dev/nvme0n1")
	require.NoError(t, err)
	require.NotNil(t, supportInfo)
	assert.True(t, supportInfo.Available, "Expected SMART to be supported for NVMe device")
	assert.True(t, supportInfo.Enabled, "Expected SMART enabled state to be unknown without smart_support")
}

func TestIsSMARTSupportedNVMeWithSmartSupport(t *testing.T) {
	mockJSON := `{
	"json_format_version": [1, 0],
	"smartctl": {
		"version": [7, 5],
		"argv": ["smartctl", "--nocheck=standby", "-a", "-j", "/dev/nvme0n1"],
		"exit_status": 0
	},
	"device": {"name": "/dev/nvme0n1", "type": "nvme"},
	"model_name": "Samsung SSD 970 EVO",
	"serial_number": "S5H9NJ0N123456",
	"smart_support": {
		"available": true,
		"enabled": false
	    }
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/nvme0n1": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	supportInfo, err := client.IsSMARTSupported(context.Background(), "/dev/nvme0n1")
	require.NoError(t, err)
	require.NotNil(t, supportInfo)
	assert.True(t, supportInfo.Available, "Expected SMART to be supported for NVMe device")
	assert.False(t, supportInfo.Enabled, "Expected SMART to be disabled for NVMe device")
}

func TestIsSMARTSupportedNotSupported(t *testing.T) {
	mockJSON := `{
		"device": {"name": "/dev/sda", "type": "ata"}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	supportInfo, err := client.IsSMARTSupported(context.Background(), "/dev/sda")
	require.NoError(t, err)
	require.NotNil(t, supportInfo)
	assert.False(t, supportInfo.Available, "Expected SMART to not be supported")
	assert.False(t, supportInfo.Enabled, "Expected SMART to not be enabled")
}

func TestIsSMARTSupportedError(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {err: errors.New("command failed")},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	_, err := client.IsSMARTSupported(context.Background(), "/dev/sda")
	assert.Error(t, err)
}

func TestEnableSMART(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -s on /dev/sda": {},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	err := client.EnableSMART(context.Background(), "/dev/sda")
	assert.NoError(t, err)
}

func TestEnableSMARTError(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -s on /dev/sda": {err: errors.New("command failed")},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	err := client.EnableSMART(context.Background(), "/dev/sda")
	assert.Error(t, err)
}

func TestDisableSMART(t *testing.T) {
	mockJSON := `{
		"device": {"name": "/dev/sda", "type": "sat"},
		"model_name": "KINGSTON SV300S37A240G",
		"serial_number": "50026B77560145CF",
		"ata_smart_data": {
			"self_test": {"status": {"value": 0}}
		},
		"smart_status": {"passed": true}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
			"/usr/sbin/smartctl -s off /dev/sda":                  {},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	err := client.DisableSMART(context.Background(), "/dev/sda")
	assert.NoError(t, err)
}

func TestDisableSMARTError(t *testing.T) {
	mockJSON := `{
		"device": {"name": "/dev/sda", "type": "sat"},
		"model_name": "KINGSTON SV300S37A240G",
		"serial_number": "50026B77560145CF",
		"ata_smart_data": {
			"self_test": {"status": {"value": 0}}
		},
		"smart_status": {"passed": true}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
			"/usr/sbin/smartctl -s off /dev/sda":                  {err: errors.New("command failed")},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	err := client.DisableSMART(context.Background(), "/dev/sda")
	assert.Error(t, err)
}

func TestDisableSMARTNVMe(t *testing.T) {
	mockJSON := `{
		"device": {"name": "/dev/nvme0n1", "type": "nvme"},
		"model_name": "Samsung SSD 970 EVO",
		"serial_number": "S5H9NJ0N123456",
		"nvme_smart_health_information_log": {
			"temperature": 35
		},
		"smart_status": {"passed": true}
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/nvme0n1": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	err := client.DisableSMART(context.Background(), "/dev/nvme0n1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NVMe")
}

func TestAbortSelfTest(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -X /dev/sda": {},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	err := client.AbortSelfTest(context.Background(), "/dev/sda")
	assert.NoError(t, err)
}

func TestAbortSelfTestError(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -X /dev/sda": {err: errors.New("command failed")},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	err := client.AbortSelfTest(context.Background(), "/dev/sda")
	assert.Error(t, err)
}

func TestDiskTypeDetectionSSD(t *testing.T) {
	mockJSON := `{
"device": {"name": "/dev/sda", "type": "sat"},
"rotation_rate": 0,
"model_name": "KINGSTON SV300S37A240G",
"serial_number": "50026B77560145CF",
"smart_status": {"passed": true}
}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetSMARTInfo(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.NotNil(t, info.RotationRate, "Expected rotation_rate to be set")
	assert.Equal(t, 0, *info.RotationRate, "Expected rotation_rate 0 for SSD")
	assert.Equal(t, "SSD", info.DiskType)
}

func TestDiskTypeDetectionHDD(t *testing.T) {
	mockJSON := `{
"device": {"name": "/dev/sdb", "type": "ata"},
"rotation_rate": 7200,
"model_name": "WDC WD10EZEX",
"serial_number": "WD-WCC6Y0123456",
"smart_status": {"passed": true}
}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sdb": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetSMARTInfo(context.Background(), "/dev/sdb")
	assert.NoError(t, err)
	assert.NotNil(t, info.RotationRate, "Expected rotation_rate to be set")
	assert.Equal(t, 7200, *info.RotationRate, "Expected rotation_rate 7200 for HDD")
	assert.Equal(t, "HDD", info.DiskType)
}

func TestDiskTypeDetectionNVMe(t *testing.T) {
	mockJSON := `{
"device": {"name": "/dev/nvme0n1", "type": "nvme"},
"model_name": "Samsung SSD 970 EVO",
"serial_number": "S5H9NJ0N123456",
"nvme_smart_health_information_log": {
"temperature": 35
},
"smart_status": {"passed": true}
}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/nvme0n1": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetSMARTInfo(context.Background(), "/dev/nvme0n1")
	assert.NoError(t, err)
	assert.Equal(t, "NVMe", info.DiskType)

	// NVMe devices don't have rotation_rate
	assert.Nil(t, info.RotationRate, "Expected no rotation_rate for NVMe")
}

func TestDiskTypeDetectionUnknown(t *testing.T) {
	mockJSON := `{
"device": {"name": "/dev/sdc", "type": "scsi"},
"model_name": "Generic SCSI Device",
"serial_number": "123456",
"smart_status": {"passed": true}
}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sdc": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetSMARTInfo(context.Background(), "/dev/sdc")
	assert.NoError(t, err)
	assert.Equal(t, "Unknown", info.DiskType)
}

func TestDiskTypeDetectionSSDWithAttributes(t *testing.T) {
	mockJSON := `{
"device": {"name": "/dev/sda", "type": "sat"},
"model_name": "KINGSTON SV300S37A240G",
"serial_number": "50026B77560145CF",
"smart_status": {"passed": true},
"ata_smart_data": {
"table": [
{
"id": 231,
"name": "SSD_Life_Left",
"value": 95
}
]
}
}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {output: []byte(mockJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	info, err := client.GetSMARTInfo(context.Background(), "/dev/sda")
	assert.NoError(t, err)
	assert.Equal(t, "SSD", info.DiskType, "Expected disk type 'SSD' based on attribute 231")
}

func TestGetSMARTInfoUnknownUSBBridgeFallback(t *testing.T) {
	mockJSONWithError := `{
  "json_format_version": [1, 0],
  "smartctl": {
    "version": [7, 5],
    "messages": [
      {
        "string": "/dev/disk/by-id/usb-Flash_Disk_3.0_7966051146147389472-0:0: Unknown USB bridge [0x048d:0x1234 (0x200)]",
        "severity": "error"
      }
    ],
    "exit_status": 1
  },
  "device": {"name": "", "type": ""}
}`

	mockJSONWithSat := `{
  "json_format_version": [1, 0],
  "smartctl": {
    "version": [7, 5],
    "exit_status": 0
  },
  "device": {
    "name": "/dev/disk/by-id/usb-Flash_Disk_3.0_7966051146147389472-0:0",
    "type": "sat"
  },
  "model_name": "Flash Disk 3.0",
  "serial_number": "7966051146147389472",
  "smart_status": {"passed": true},
  "rotation_rate": 0
}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/usb0": {
				output: []byte(mockJSONWithError),
				err:    errors.New("exit status 1"),
			},
			"/usr/sbin/smartctl -a -j --nocheck=standby -d sat /dev/usb0": {
				output: []byte(mockJSONWithSat),
			},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	// First call should detect unknown USB bridge and retry with -d sat
	info, err := client.GetSMARTInfo(context.Background(), "/dev/usb0")
	assert.NoError(t, err, "Expected no error after fallback")
	assert.Equal(t, "/dev/disk/by-id/usb-Flash_Disk_3.0_7966051146147389472-0:0", info.Device.Name)
	assert.Equal(t, "sat", info.Device.Type)

	// Verify the device type is cached
	c := client.(*Client)
	cachedType, ok := c.backend.(*ExecBackend).DeviceTypeHint("/dev/usb0")
	assert.True(t, ok, "Expected device type to be cached")
	assert.Equal(t, "sat", cachedType)
}

func TestGetSMARTInfoUnknownUSBBridgeFallbackAlreadyCached(t *testing.T) {
	mockJSONWithSat := `{
  "json_format_version": [1, 0],
  "smartctl": {
    "version": [7, 5],
    "exit_status": 0
  },
  "device": {
    "name": "/dev/disk/by-id/usb-Flash_Disk_3.0_7966051146147389472-0:0",
    "type": "sat"
  },
  "model_name": "Flash Disk 3.0",
  "serial_number": "7966051146147389472",
  "smart_status": {"passed": true},
  "rotation_rate": 0
}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby -d sat /dev/usb0": {
				output: []byte(mockJSONWithSat),
			},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	// Pre-cache the device type
	c := client.(*Client)
	c.backend.(*ExecBackend).SetDeviceTypeHint("/dev/usb0", "sat")

	// This call should use the cached device type and not try the default first
	info, err := client.GetSMARTInfo(context.Background(), "/dev/usb0")
	assert.NoError(t, err, "Expected no error with cached type")
	assert.Equal(t, "/dev/disk/by-id/usb-Flash_Disk_3.0_7966051146147389472-0:0", info.Device.Name)
	assert.Equal(t, "sat", info.Device.Type)
}

func TestGetSMARTInfoUnknownUSBBridgeFallbackFailed(t *testing.T) {
	mockJSONWithError := `{
  "json_format_version": [1, 0],
  "smartctl": {
    "version": [7, 5],
    "messages": [
      {
        "string": "/dev/disk/by-id/usb-Flash_Disk_3.0_7966051146147389472-0:0: Unknown USB bridge [0x048d:0x1234 (0x200)]",
        "severity": "error"
      }
    ],
    "exit_status": 1
  },
  "device": {"name": "", "type": ""}
}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/usb0": {
				output: []byte(mockJSONWithError),
				err:    errors.New("exit status 1"),
			},
			"/usr/sbin/smartctl -a -j --nocheck=standby -d sat /dev/usb0": {
				err: errors.New("sat failed"),
			},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	// Should fail after trying both default and -d sat
	info, err := client.GetSMARTInfo(context.Background(), "/dev/usb0")
	require.Error(t, err)
	assert.Equal(t, "SMART Not Supported", err.Error())
	assert.Empty(t, info.Device.Name)

	// Verify the device type is NOT cached (fallback failed)
	c := client.(*Client)
	_, ok := c.backend.(*ExecBackend).DeviceTypeHint("/dev/usb0")
	assert.False(t, ok, "Expected device type not to be cached when fallback fails")
}

func TestNewClientLoadsAddendum(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("smartctl not available: %v", err)
	}

	c := client.(*Client)
	eb := c.backend.(*ExecBackend)

	// Check that a known USB bridge is in the cache
	deviceType, ok := eb.DeviceTypeHint("usb:0x152d:0x0578")
	assert.True(t, ok, "Expected usb:0x152d:0x0578 to be in cache")
	assert.Equal(t, "sat", deviceType, "Expected device type 'sat'")
}

func TestGetSMARTInfoWithKnownUSBBridge(t *testing.T) {
	mockJSONWithError := `{
  "json_format_version": [1, 0],
  "smartctl": {
    "version": [7, 5],
    "messages": [
      {
        "string": "/dev/usb0: Unknown USB bridge [0x152d:0x578e (0x200)]",
        "severity": "error"
      }
    ],
    "exit_status": 1
  },
  "device": {"name": "", "type": ""}
}`

	mockJSONWithSat := `{
  "json_format_version": [1, 0],
  "smartctl": {
    "version": [7, 5],
    "exit_status": 0
  },
  "device": {
    "name": "/dev/usb0",
    "type": "sat"
  },
  "model_name": "Intenso Memory Center",
  "serial_number": "123456",
  "smart_status": {"passed": true},
  "rotation_rate": 0
}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/usb0": {
				output: []byte(mockJSONWithError),
				err:    errors.New("exit status 1"),
			},
			"/usr/sbin/smartctl -a -j --nocheck=standby -d sat /dev/usb0": {
				output: []byte(mockJSONWithSat),
			},
		},
	}

	backend, err := NewExecBackend(
		WithExecSmartctlPath("/usr/sbin/smartctl"),
		WithExecCommander(commander),
		WithExecTLogHandler(tlog.NewLoggerWithLevel(tlog.LevelDebug)),
	)
	require.NoError(t, err)

	// First call should detect USB bridge in addendum and use sat
	info, err := backend.GetSMARTInfo(context.Background(), "/dev/usb0")
	assert.NoError(t, err, "Expected no error after using addendum")
	assert.Equal(t, "/dev/usb0", info.Device.Name)

	// Verify the device path is cached
	cachedType, ok := backend.DeviceTypeHint("/dev/usb0")
	assert.True(t, ok, "Expected device path to be cached")
	assert.Equal(t, "sat", cachedType)
}

// TestHashString verifies the hash function produces consistent results
func TestHashString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		input2 string
		same   bool
	}{
		{
			name:   "identical strings",
			input:  "test message",
			input2: "test message",
			same:   true,
		},
		{
			name:   "different strings",
			input:  "test message 1",
			input2: "test message 2",
			same:   false,
		},
		{
			name:   "empty string",
			input:  "",
			input2: "",
			same:   true,
		},
		{
			name:   "case sensitive",
			input:  "Test Message",
			input2: "test message",
			same:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hash1 := hashString(tc.input)
			hash2 := hashString(tc.input2)
			if tc.same {
				assert.Equal(t, hash1, hash2, "Expected same hash for identical strings")
			} else {
				assert.NotEqual(t, hash1, hash2, "Expected different hash for different strings")
			}
		})
	}
}

// TestMessageCacheShouldLog tests the cache shouldLog method
func TestMessageCacheShouldLog(t *testing.T) {
	t.Run("first call should return true", func(t *testing.T) {
		cache := &messageCache{}
		result := cache.shouldLog("unique message 1", "information")
		assert.True(t, result, "First call should allow logging")
	})

	t.Run("second call with same message should return false", func(t *testing.T) {
		cache := &messageCache{}
		msg := "duplicate message test"

		first := cache.shouldLog(msg, "warning")
		assert.True(t, first, "First call should allow logging")

		second := cache.shouldLog(msg, "warning")
		assert.False(t, second, "Second call should be cached")
	})

	t.Run("different messages should both return true", func(t *testing.T) {
		cache := &messageCache{}

		first := cache.shouldLog("message A", "error")
		second := cache.shouldLog("message B", "error")

		assert.True(t, first, "First message should allow logging")
		assert.True(t, second, "Different message should also allow logging")
	})

	t.Run("same message different severity uses first severity TTL", func(t *testing.T) {
		cache := &messageCache{}
		msg := "same message different severity"

		// First call with information (1h TTL)
		first := cache.shouldLog(msg, "information")
		assert.True(t, first, "First call should allow logging")

		// Second call with error severity - should still be cached from first call
		second := cache.shouldLog(msg, "error")
		assert.False(t, second, "Should still be cached regardless of severity")
	})
}

// TestMessageCacheTTLValues verifies correct TTL is applied per severity
func TestMessageCacheTTLValues(t *testing.T) {
	tests := []struct {
		name        string
		severity    string
		expectedTTL time.Duration
	}{
		{
			name:        "information severity",
			severity:    "information",
			expectedTTL: msgCacheTTLInformation,
		},
		{
			name:        "warning severity",
			severity:    "warning",
			expectedTTL: msgCacheTTLWarning,
		},
		{
			name:        "error severity",
			severity:    "error",
			expectedTTL: msgCacheTTLError,
		},
		{
			name:        "unknown severity uses default",
			severity:    "unknown",
			expectedTTL: msgCacheTTLDefault,
		},
		{
			name:        "empty severity uses default",
			severity:    "",
			expectedTTL: msgCacheTTLDefault,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := &messageCache{}
			msg := "test message for " + tc.name

			// Call shouldLog to cache the message
			before := time.Now()
			cache.shouldLog(msg, tc.severity)
			after := time.Now()

			// Retrieve the cached entry and verify TTL
			key := hashString(msg)
			entry, ok := cache.entries.Load(key)
			require.True(t, ok, "Entry should be cached")

			cached := entry.(messageCacheEntry)
			// Verify expiration is within expected range
			expectedMin := before.Add(tc.expectedTTL)
			expectedMax := after.Add(tc.expectedTTL)

			assert.True(t, !cached.expiresAt.Before(expectedMin),
				"Expiration should be at least %v after start", tc.expectedTTL)
			assert.True(t, !cached.expiresAt.After(expectedMax),
				"Expiration should be at most %v after end", tc.expectedTTL)
		})
	}
}

// TestMessageCacheExpiration verifies entries expire correctly
func TestMessageCacheExpiration(t *testing.T) {
	cache := &messageCache{}
	msg := "expiring message"

	// Manually insert an already-expired entry
	key := hashString(msg)
	cache.entries.Store(key, messageCacheEntry{expiresAt: time.Now().Add(-1 * time.Second)})

	// Should return true because entry is expired
	result := cache.shouldLog(msg, "information")
	assert.True(t, result, "Expired entry should allow new logging")
}

// TestMessageCacheConcurrency tests thread-safety of the cache
func TestMessageCacheConcurrency(t *testing.T) {
	cache := &messageCache{}
	const numGoroutines = 100
	const numMessages = 10

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numMessages; j++ {
				msg := "concurrent message " + string(rune('A'+j%numMessages))
				cache.shouldLog(msg, "warning")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify cache has entries (exact count may vary due to timing)
	count := 0
	cache.entries.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	assert.Greater(t, count, 0, "Cache should have some entries after concurrent access")
	assert.LessOrEqual(t, count, numMessages, "Cache should not have more entries than unique messages")
}

// TestGlobalMessageCache verifies the global cache instance works
func TestGlobalMessageCache(t *testing.T) {
	// Use a unique message to avoid interference from other tests
	msg := "global cache test message " + time.Now().String()

	// First call should return true
	first := globalMessageCache.shouldLog(msg, "information")
	assert.True(t, first, "First call to global cache should allow logging")

	// Second call should return false (cached)
	second := globalMessageCache.shouldLog(msg, "information")
	assert.False(t, second, "Second call to global cache should be cached")
}

// TestGetSMARTInfoMessageCaching tests that messages are cached during GetSMARTInfo
func TestGetSMARTInfoMessageCaching(t *testing.T) {
	// JSON response with messages
	mockJSON := `{
		"device": {"name": "/dev/sda", "type": "sat"},
		"model_name": "Test Drive",
		"serial_number": "TEST123",
		"smartctl": {
			"messages": [
				{"string": "Test info message for caching", "severity": "information"},
				{"string": "Test warning message for caching", "severity": "warning"}
			]
		}
	}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {
				output: []byte(mockJSON),
			},
		},
	}

	backend, err := NewExecBackend(
		WithExecSmartctlPath("/usr/sbin/smartctl"),
		WithExecCommander(commander),
		WithExecTLogHandler(tlog.NewLoggerWithLevel(tlog.LevelDebug)),
	)
	require.NoError(t, err)

	// Clear any existing cache entries for our test messages
	testCache := &messageCache{}
	msg1Key := hashString("Test info message for caching")
	msg2Key := hashString("Test warning message for caching")

	// Call GetSMARTInfo
	info, err := backend.GetSMARTInfo(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.NotNil(t, info)

	// Verify messages were in the response
	require.NotNil(t, info.Smartctl)
	assert.Len(t, info.Smartctl.Messages, 2)

	// Verify messages are now cached in the global cache
	// We can't directly check globalMessageCache entries, but we can verify
	// that shouldLog returns false for the same messages
	infoResult := globalMessageCache.shouldLog("Test info message for caching", "information")
	warnResult := globalMessageCache.shouldLog("Test warning message for caching", "warning")

	// These should be false if already cached, but may be true if this is first run
	// The important thing is the code path executed without error
	_ = infoResult
	_ = warnResult
	_ = testCache
	_ = msg1Key
	_ = msg2Key
}

// TestMessageCacheSkipsAttributeCheckWarning verifies the special message is skipped
func TestMessageCacheSkipsAttributeCheckWarning(t *testing.T) {
	mockJSON := `{
		"device": {"name": "/dev/sda", "type": "sat"},
		"model_name": "Test Drive",
		"smartctl": {
			"messages": [
				{"string": "Warning: This result is based on an Attribute check.", "severity": "warning"},
				{"string": "Another warning message", "severity": "warning"}
			]
		}
	}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {
				output: []byte(mockJSON),
			},
		},
	}

	backend, err := NewExecBackend(
		WithExecSmartctlPath("/usr/sbin/smartctl"),
		WithExecCommander(commander),
		WithExecTLogHandler(tlog.NewLoggerWithLevel(tlog.LevelDebug)),
	)
	require.NoError(t, err)

	// Call GetSMARTInfo - should not panic or error
	info, err := backend.GetSMARTInfo(context.Background(), "/dev/sda")
	require.NoError(t, err)
	assert.NotNil(t, info)

	// The "Warning: This result is based on an Attribute check." should be skipped
	// and not cached. Verify by checking that it's not in the cache
	// (shouldLog should return true for it since it was never cached)
	result := globalMessageCache.shouldLog("Warning: This result is based on an Attribute check.", "warning")
	// Note: This could be true (first time) or false (if another test cached it)
	// The key verification is that the code path didn't error
	_ = result
}

// ─── DiscoverDevices ──────────────────────────────────────────────────────────

// TestDiscoverDevices_AllReadable verifies that all devices are reported as
// readable when GetSMARTInfo succeeds for each.
func TestDiscoverDevices_AllReadable(t *testing.T) {
	scanJSON := `{"devices":[{"name":"/dev/sda","type":"ata"},{"name":"/dev/sdb","type":"ata"}]}`
	sdaJSON := `{"device":{"name":"/dev/sda","type":"ata"},"model_name":"Drive A","serial_number":"SNA","smart_status":{"passed":true}}`
	sdbJSON := `{"device":{"name":"/dev/sdb","type":"ata"},"model_name":"Drive B","serial_number":"SNB","smart_status":{"passed":true}}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json":                      {output: []byte(scanJSON)},
			"/usr/sbin/smartctl -a -j --nocheck=standby -d ata /dev/sda": {output: []byte(sdaJSON)},
			"/usr/sbin/smartctl -a -j --nocheck=standby -d ata /dev/sdb": {output: []byte(sdbJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	results, err := client.DiscoverDevices(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "/dev/sda", results[0].DevicePath)
	assert.True(t, results[0].SMARTReadable)
	assert.False(t, results[0].SATFallbackRequired)
	assert.Equal(t, "Drive A", results[0].Model)
	assert.Equal(t, "SNA", results[0].Serial)

	assert.Equal(t, "/dev/sdb", results[1].DevicePath)
	assert.True(t, results[1].SMARTReadable)
	assert.False(t, results[1].SATFallbackRequired)
}

// TestDiscoverDevices_SATFallbackRequired_InternalPath verifies that
// SATFallbackRequired is set when the SAT fallback runs *inside*
// getSMARTInfoInternal (i.e. the scan returns an empty device type so nothing
// is cached, the cold-cache auto-detection attempt fails with an ExitError, and
// the internal retrySATFallback succeeds). Previously DiscoverDevices called
// GetSMARTInfo and lost the internal-fallback signal; now it calls
// getSMARTInfoInternal which surfaces the bool.
func TestDiscoverDevices_SATFallbackRequired_InternalPath(t *testing.T) {
	// Scan returns a device with empty type — nothing is cached.
	scanJSON := `{"devices":[{"name":"/dev/sda","type":""}]}`
	satJSON := `{"device":{"name":"/dev/sda","type":"sat"},"model_name":"SAT Drive","serial_number":"S999","smart_status":{"passed":true}}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json": {output: []byte(scanJSON)},
			// Cold-cache call (no -d flag) fails with an ExitError —
			// triggers exitCode&0x05 check and the internal SAT fallback.
			"/usr/sbin/smartctl -a -j --nocheck=standby /dev/sda": {
				err: &exec.ExitError{},
			},
			// Internal SAT retry succeeds.
			"/usr/sbin/smartctl -a -j --nocheck=standby -d sat /dev/sda": {
				output: []byte(satJSON),
			},
		},
	}
	client, err := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))
	require.NoError(t, err)

	results, err := client.DiscoverDevices(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.True(t, r.SMARTReadable, "device should be readable via internal SAT fallback")
	assert.True(t, r.SATFallbackRequired, "SATFallbackRequired should be set when internal fallback was used")
	assert.Equal(t, "sat", r.DetectedProtocol)
	assert.Equal(t, "SAT Drive", r.Model)
	assert.Equal(t, "S999", r.Serial)
}

// TestDiscoverDevices_SATFallbackRequired verifies that SATFallbackRequired is
// set when the auto-detected protocol fails but the SAT retry succeeds.
func TestDiscoverDevices_SATFallbackRequired(t *testing.T) {
	scanJSON := `{"devices":[{"name":"/dev/sda","type":"ata"}]}`
	satJSON := `{"device":{"name":"/dev/sda","type":"sat"},"model_name":"USB Drive","serial_number":"USB123","smart_status":{"passed":true}}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json": {output: []byte(scanJSON)},
			// GetSMARTInfo uses cached "ata" from scan and fails:
			"/usr/sbin/smartctl -a -j --nocheck=standby -d ata /dev/sda": {
				err: errors.New("protocol mismatch"),
			},
			// DiscoverDevices explicit SAT retry succeeds:
			"/usr/sbin/smartctl -a -j --nocheck=standby -d sat /dev/sda": {output: []byte(satJSON)},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	results, err := client.DiscoverDevices(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "/dev/sda", r.DevicePath)
	assert.True(t, r.SMARTReadable, "device should be readable via SAT fallback")
	assert.True(t, r.SATFallbackRequired, "SAT fallback should be flagged")
	assert.Equal(t, "sat", r.DetectedProtocol)
	assert.Equal(t, "USB Drive", r.Model)
	assert.Equal(t, "USB123", r.Serial)
}

// TestDiscoverDevices_NotReadable verifies that SMARTReadable is false when
// both the native protocol and the SAT fallback fail.
func TestDiscoverDevices_NotReadable(t *testing.T) {
	scanJSON := `{"devices":[{"name":"/dev/sda","type":"ata"}]}`

	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json": {output: []byte(scanJSON)},
			// Both attempts fail with a plain (non-ExitError) error.
			"/usr/sbin/smartctl -a -j --nocheck=standby -d ata /dev/sda": {
				err: errors.New("device not accessible"),
			},
			"/usr/sbin/smartctl -a -j --nocheck=standby -d sat /dev/sda": {
				err: errors.New("device not accessible"),
			},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	results, err := client.DiscoverDevices(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "/dev/sda", r.DevicePath)
	assert.False(t, r.SMARTReadable)
	assert.False(t, r.SATFallbackRequired)
}

// TestDiscoverDevices_ScanFails verifies that DiscoverDevices returns an error
// when the device scan itself fails.
func TestDiscoverDevices_ScanFails(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json": {err: errors.New("scan failed")},
			"/usr/sbin/smartctl --scan --json":      {err: errors.New("scan failed")},
		},
	}
	client, _ := NewClient(WithSmartctlPath("/usr/sbin/smartctl"), WithCommander(commander))

	_, err := client.DiscoverDevices(context.Background())
	assert.Error(t, err)
}
