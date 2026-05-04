package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"os/user"
	"strings"
)

// LingerStatus represents the linger state for a user
type LingerStatus struct {
	Enabled        bool
	Username       string
	UID            string
	LoginctlExists bool
	ErrorMessage   string
}

// CheckLingering verifies if the current user has lingering enabled
// Lingering prevents systemd from killing user processes (like tmux) on logout
func CheckLingering() (*LingerStatus, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	status := &LingerStatus{
		Username: currentUser.Username,
		UID:      currentUser.Uid,
	}

	// Check if loginctl exists
	if _, err := exec.LookPath("loginctl"); err != nil {
		status.LoginctlExists = false
		status.ErrorMessage = "loginctl not found (systemd not available)"
		return status, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}
	status.LoginctlExists = true

	// Run: loginctl show-user $USER --property=Linger
	cmd := exec.Command("loginctl", "show-user", currentUser.Username, "--property=Linger")
	output, err := cmd.Output()
	if err != nil {
		// User might not have a session yet
		status.Enabled = false
		status.ErrorMessage = fmt.Sprintf("loginctl show-user failed: %v", err)
		return status, nil
	}

	// Parse output: "Linger=yes" or "Linger=no"
	outputStr := strings.TrimSpace(string(output))
	status.Enabled = outputStr == "Linger=yes"

	return status, nil
}

// EnableLingering enables user lingering for the current user
// This requires either root privileges or polkit authorization
func EnableLingering() error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	// Check if loginctl exists
	if _, err := exec.LookPath("loginctl"); err != nil {
		return fmt.Errorf("loginctl not found - systemd is not available on this system")
	}

	// Run: loginctl enable-linger $USER
	cmd := exec.Command("loginctl", "enable-linger", currentUser.Username)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable linger: %w\nStderr: %s", err, stderr.String())
	}

	return nil
}

// DisableLingering disables user lingering for the current user
// This is mainly for testing or cleanup purposes
func DisableLingering() error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	// Check if loginctl exists
	if _, err := exec.LookPath("loginctl"); err != nil {
		return fmt.Errorf("loginctl not found - systemd is not available on this system")
	}

	// Run: loginctl disable-linger $USER
	cmd := exec.Command("loginctl", "disable-linger", currentUser.Username)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to disable linger: %w\nStderr: %s", err, stderr.String())
	}

	return nil
}

// GetLingerPath returns the path to the linger file for the current user
// This file's existence indicates lingering is enabled
func GetLingerPath() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %w", err)
	}

	// Linger files are stored at: /var/lib/systemd/linger/$UID
	return fmt.Sprintf("/var/lib/systemd/linger/%s", currentUser.Uid), nil
}

// IsLingerSupported checks if the system supports lingering (has systemd)
func IsLingerSupported() bool {
	_, err := exec.LookPath("loginctl")
	return err == nil
}

// LingerInfo describes the system's loginctl-linger status.
type LingerInfo struct {
	Supported      bool
	Enabled        bool
	Username       string
	UID            string
	LingerFilePath string
	SystemdVersion string
	ErrorMessage   string
}

// GetLingerInfo retrieves comprehensive lingering information
func GetLingerInfo() (*LingerInfo, error) {
	info := &LingerInfo{
		Supported: IsLingerSupported(),
	}

	if !info.Supported {
		info.ErrorMessage = "systemd not available on this system"
		return info, nil
	}

	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	info.Username = currentUser.Username
	info.UID = currentUser.Uid

	// Get linger file path
	lingerPath, err := GetLingerPath()
	if err != nil {
		info.ErrorMessage = fmt.Sprintf("failed to get linger path: %v", err)
	} else {
		info.LingerFilePath = lingerPath
	}

	// Check linger status
	status, err := CheckLingering()
	if err != nil {
		info.ErrorMessage = fmt.Sprintf("failed to check linger status: %v", err)
		return info, nil
	}

	info.Enabled = status.Enabled

	// Get systemd version
	cmd := exec.Command("systemctl", "--version")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) > 0 {
			info.SystemdVersion = strings.TrimSpace(lines[0])
		}
	}

	return info, nil
}

// FormatLingerStatus returns a human-readable description of linger status
func FormatLingerStatus(status *LingerStatus) string {
	if !status.LoginctlExists {
		return "❓ Linger status unknown (systemd not available)"
	}

	if status.ErrorMessage != "" && !status.Enabled {
		return fmt.Sprintf("⚠ Linger check failed: %s", status.ErrorMessage)
	}

	if status.Enabled {
		return fmt.Sprintf("✓ Linger ENABLED for user %s (sessions will persist)", status.Username)
	}

	return fmt.Sprintf("✗ Linger DISABLED for user %s (sessions may die on logout)", status.Username)
}

// GetRecommendation returns a recommendation message based on linger status
func GetRecommendation(status *LingerStatus) string {
	if !status.LoginctlExists {
		return "Lingering is not available on this system. Tmux sessions may be killed on logout."
	}

	if status.Enabled {
		return "No action needed - sessions will persist after logout."
	}

	return fmt.Sprintf("Run 'loginctl enable-linger %s' to prevent sessions from being killed on logout.", status.Username)
}
