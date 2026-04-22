package tmux

import (
	"os"
	"os/exec"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsLingerSupported(t *testing.T) {
	// This test simply checks if loginctl is in PATH
	supported := IsLingerSupported()

	// Check if loginctl exists
	_, err := exec.LookPath("loginctl")
	if err != nil {
		assert.False(t, supported, "should not be supported when loginctl missing")
	} else {
		assert.True(t, supported, "should be supported when loginctl exists")
	}
}

func TestCheckLingering(t *testing.T) {
	if !IsLingerSupported() {
		t.Skip("Skipping test - systemd not available")
	}

	status, err := CheckLingering()
	assert.NoError(t, err, "should not error when loginctl exists")
	assert.NotNil(t, status, "should return status")

	currentUser, err := user.Current()
	require.NoError(t, err)

	assert.Equal(t, currentUser.Username, status.Username, "should match current username")
	assert.Equal(t, currentUser.Uid, status.UID, "should match current UID")
	assert.True(t, status.LoginctlExists, "loginctl should exist")

	// We can't assert the exact value of Enabled since it depends on system configuration
	// But we can verify it's a boolean
	assert.IsType(t, false, status.Enabled, "enabled should be a boolean")
}

func TestCheckLingering_NoSystemd(t *testing.T) {
	// This test is hard to run without actually removing loginctl
	// We can only test it indirectly through IsLingerSupported
	if IsLingerSupported() {
		t.Skip("Skipping - systemd is available")
	}

	status, err := CheckLingering()
	assert.NoError(t, err, "should not error even without systemd")
	assert.NotNil(t, status, "should return status")
	assert.False(t, status.LoginctlExists, "loginctl should not exist")
	assert.Contains(t, status.ErrorMessage, "loginctl not found", "should explain why check failed")
}

func TestGetLingerPath(t *testing.T) {
	path, err := GetLingerPath()
	assert.NoError(t, err, "should not error")

	currentUser, err := user.Current()
	require.NoError(t, err)

	expectedPath := "/var/lib/systemd/linger/" + currentUser.Uid
	assert.Equal(t, expectedPath, path, "should return correct linger file path")
}

func TestGetLingerInfo(t *testing.T) {
	info, err := GetLingerInfo()
	assert.NoError(t, err, "should not error")
	assert.NotNil(t, info, "should return info")

	currentUser, err := user.Current()
	require.NoError(t, err)

	assert.Equal(t, IsLingerSupported(), info.Supported, "supported flag should match")

	if info.Supported {
		assert.Equal(t, currentUser.Username, info.Username, "should match current username")
		assert.Equal(t, currentUser.Uid, info.UID, "should match current UID")
		assert.NotEmpty(t, info.LingerFilePath, "should have linger file path")

		// SystemdVersion should be populated if systemctl exists
		if _, err := exec.LookPath("systemctl"); err == nil {
			assert.NotEmpty(t, info.SystemdVersion, "should have systemd version")
		}
	} else {
		assert.Contains(t, info.ErrorMessage, "not available", "should explain why not supported")
	}
}

func TestFormatLingerStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   *LingerStatus
		contains string
	}{
		{
			name: "linger enabled",
			status: &LingerStatus{
				Enabled:        true,
				Username:       "testuser",
				LoginctlExists: true,
			},
			contains: "ENABLED",
		},
		{
			name: "linger disabled",
			status: &LingerStatus{
				Enabled:        false,
				Username:       "testuser",
				LoginctlExists: true,
			},
			contains: "DISABLED",
		},
		{
			name: "systemd not available",
			status: &LingerStatus{
				LoginctlExists: false,
			},
			contains: "unknown",
		},
		{
			name: "error checking linger",
			status: &LingerStatus{
				Enabled:        false,
				LoginctlExists: true,
				ErrorMessage:   "some error",
			},
			contains: "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatLingerStatus(tt.status)
			assert.Contains(t, result, tt.contains, "should contain expected text")
		})
	}
}

func TestGetRecommendation(t *testing.T) {
	tests := []struct {
		name     string
		status   *LingerStatus
		contains string
	}{
		{
			name: "linger enabled - no action",
			status: &LingerStatus{
				Enabled:        true,
				LoginctlExists: true,
			},
			contains: "No action needed",
		},
		{
			name: "linger disabled - recommend enable",
			status: &LingerStatus{
				Enabled:        false,
				Username:       "testuser",
				LoginctlExists: true,
			},
			contains: "loginctl enable-linger",
		},
		{
			name: "systemd not available",
			status: &LingerStatus{
				LoginctlExists: false,
			},
			contains: "not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRecommendation(tt.status)
			assert.Contains(t, result, tt.contains, "should contain expected recommendation")
		})
	}
}

// Integration tests below - these require actual system access
// and should be skipped in CI or restricted environments

func TestEnableLingering_Integration(t *testing.T) {
	if os.Getenv("CI") != "" || os.Getuid() != 0 {
		t.Skip("Skipping integration test - requires root or may affect system state")
	}

	if !IsLingerSupported() {
		t.Skip("Skipping - systemd not available")
	}

	// This is a destructive test that modifies system state
	// Only run with explicit permission
	if os.Getenv("RUN_DESTRUCTIVE_TESTS") != "1" {
		t.Skip("Skipping destructive test - set RUN_DESTRUCTIVE_TESTS=1 to run")
	}

	// Save current state
	originalStatus, err := CheckLingering()
	require.NoError(t, err)

	// Enable lingering
	err = EnableLingering()
	if err != nil {
		// Might fail due to permissions or polkit
		t.Logf("Failed to enable lingering (expected in restricted env): %v", err)
		return
	}

	// Verify it was enabled
	newStatus, err := CheckLingering()
	require.NoError(t, err)
	assert.True(t, newStatus.Enabled, "lingering should be enabled")

	// Restore original state if it was disabled
	if !originalStatus.Enabled {
		DisableLingering()
	}
}

func TestDisableLingering_Integration(t *testing.T) {
	if os.Getenv("CI") != "" || os.Getuid() != 0 {
		t.Skip("Skipping integration test - requires root or may affect system state")
	}

	if !IsLingerSupported() {
		t.Skip("Skipping - systemd not available")
	}

	if os.Getenv("RUN_DESTRUCTIVE_TESTS") != "1" {
		t.Skip("Skipping destructive test - set RUN_DESTRUCTIVE_TESTS=1 to run")
	}

	// Save current state
	originalStatus, err := CheckLingering()
	require.NoError(t, err)

	// Disable lingering
	err = DisableLingering()
	if err != nil {
		t.Logf("Failed to disable lingering (expected in restricted env): %v", err)
		return
	}

	// Verify it was disabled
	newStatus, err := CheckLingering()
	require.NoError(t, err)
	assert.False(t, newStatus.Enabled, "lingering should be disabled")

	// Restore original state if it was enabled
	if originalStatus.Enabled {
		EnableLingering()
	}
}
