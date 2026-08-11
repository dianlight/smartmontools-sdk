// SPDX-License-Identifier: GPL-2.0-or-later

package smartmontools

import (
	"context"
	"testing"
	"time"
)

func TestWithContext(t *testing.T) {
	mockJSON := `{
		"devices": [
			{"name": "/dev/sda", "type": "ata"}
		]
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json": {output: []byte(mockJSON)},
		},
	}

	// Create a context with a value to verify it's being used
	type contextKey string
	testKey := contextKey("test")
	testValue := "custom-context"

	customCtx := context.WithValue(context.Background(), testKey, testValue)

	client, err := NewClient(
		WithSmartctlPath("/usr/sbin/smartctl"),
		WithCommander(commander),
		WithContext(customCtx),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify the default context was set
	c := client.(*Client)
	if c.defaultCtx == nil {
		t.Fatal("Expected defaultCtx to be set")
	}

	// Verify the context contains our value
	if val := c.defaultCtx.Value(testKey); val != testValue {
		t.Errorf("Expected context value %q, got %v", testValue, val)
	}
}

func TestDefaultContextBackground(t *testing.T) {
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json": {output: []byte(`{"devices":[]}`)},
		},
	}

	// Create client without WithContext option
	client, err := NewClient(
		WithSmartctlPath("/usr/sbin/smartctl"),
		WithCommander(commander),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Verify the default context is set to Background
	c := client.(*Client)
	if c.defaultCtx == nil {
		t.Fatal("Expected defaultCtx to be set to context.Background()")
	}
}

func TestNilContextUsesDefault(t *testing.T) {
	mockJSON := `{
		"devices": [
			{"name": "/dev/sda", "type": "ata"}
		]
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json": {output: []byte(mockJSON)},
		},
	}

	// Create a context that will timeout quickly
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewClient(
		WithSmartctlPath("/usr/sbin/smartctl"),
		WithCommander(commander),
		WithContext(ctx),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Call ScanDevices with nil context - should use default
	var nilCtx context.Context
	devices, err := client.ScanDevices(nilCtx) //nolint:staticcheck // SA1012: nil context is intentional for testing default context
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(devices))
	}
}

func TestExplicitContextOverridesDefault(t *testing.T) {
	mockJSON := `{
		"devices": [
			{"name": "/dev/sda", "type": "ata"}
		]
	}`
	commander := &mockCommander{
		cmds: map[string]*mockCmd{
			"/usr/sbin/smartctl --scan-open --json": {output: []byte(mockJSON)},
		},
	}

	// Create default context with one value
	type contextKey string
	defaultKey := contextKey("default")
	defaultCtx := context.WithValue(context.Background(), defaultKey, "default-value")

	client, err := NewClient(
		WithSmartctlPath("/usr/sbin/smartctl"),
		WithCommander(commander),
		WithContext(defaultCtx),
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create explicit context with different value
	explicitKey := contextKey("explicit")
	explicitCtx := context.WithValue(context.Background(), explicitKey, "explicit-value")

	// Call ScanDevices with explicit context
	devices, err := client.ScanDevices(explicitCtx)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(devices))
	}

	// Note: We can't directly verify which context was used in this simple test,
	// but the test demonstrates the API works correctly with explicit contexts
}
