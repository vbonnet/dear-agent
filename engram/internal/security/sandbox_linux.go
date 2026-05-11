//go:build linux
// +build linux

package security

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// applyLinux applies AppArmor sandboxing on Linux
// Falls back to validation-only mode if AppArmor is unavailable
func (s *Sandbox) applyLinux(cmd string, args []string, permissions Permissions) ([]string, error) {
	// Check if AppArmor is available
	if !isAppArmorAvailable() {
		// Graceful degradation: return unmodified command
		// Validation is still performed by the Validator
		return append([]string{cmd}, args...), nil
	}

	// Generate AppArmor profile
	profile, err := generateAppArmorProfile(cmd, permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to generate AppArmor profile: %w", err)
	}

	// Get profile name
	profileName := fmt.Sprintf("engram_%s", hashCommand(cmd))

	// Try to load the profile into the kernel
	// This requires appropriate permissions (typically root or CAP_MAC_ADMIN)
	// If it fails, we fall back to validation-only mode
	profilePath, err := writeAppArmorProfile(profile, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to write AppArmor profile: %w", err)
	}

	// Attempt to load profile with apparmor_parser
	loadCmd := exec.Command("apparmor_parser", "-r", profilePath)
	if err := loadCmd.Run(); err != nil {
		// Profile loading failed (likely permission denied — apparmor_parser
		// needs CAP_MAC_ADMIN). This is an intentional graceful-degradation
		// path: fall back to validation-only mode rather than blocking the
		// caller. Returning nil err is correct here.
		// Clean up the profile file
		os.Remove(profilePath)
		return append([]string{cmd}, args...), nil //nolint:nilerr // intentional fallback to validation-only mode
	}

	// Profile loaded successfully, use aa-exec
	// Note: profile will persist until system reboot or manual removal
	sandboxArgs := []string{
		"aa-exec",
		"-p", profileName,
		"--",
		cmd,
	}
	sandboxArgs = append(sandboxArgs, args...)

	return sandboxArgs, nil
}

// isAppArmorAvailable checks if AppArmor is available on the system
func isAppArmorAvailable() bool {
	// Check if aa-exec is available
	_, err := exec.LookPath("aa-exec")
	if err != nil {
		return false
	}

	// Check if AppArmor is enabled
	// /sys/module/apparmor/parameters/enabled should contain "Y"
	data, err := os.ReadFile("/sys/module/apparmor/parameters/enabled")
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(data)) == "Y"
}

// generateAppArmorProfile generates an AppArmor profile from permissions.
// The error result is reserved for future failure modes (e.g., template
// rendering with constraints) and is always nil today.
func generateAppArmorProfile(cmd string, permissions Permissions) (string, error) { //nolint:unparam // future error path reserved
	var profile strings.Builder

	// Profile header
	profile.WriteString("#include <tunables/global>\n\n")

	// Profile name (based on command hash for uniqueness)
	profileName := fmt.Sprintf("engram_%s", hashCommand(cmd))
	fmt.Fprintf(&profile, "profile %s {\n", profileName)

	// Base abstractions (essential system libraries)
	profile.WriteString("  #include <abstractions/base>\n\n")

	// Command execution permission
	fmt.Fprintf(&profile, "  %s rix,\n\n", cmd)

	// Filesystem permissions
	if len(permissions.Filesystem) > 0 {
		profile.WriteString("  # Filesystem permissions\n")
		for _, path := range permissions.Filesystem {
			// Expand home directory
			expandedPath := expandPath(path)

			// Grant read/write access
			fmt.Fprintf(&profile, "  %s rw,\n", expandedPath)

			// If directory, allow subdirectory access
			if strings.HasSuffix(expandedPath, "/") {
				fmt.Fprintf(&profile, "  %s** rw,\n", expandedPath)
			}
		}
		profile.WriteString("\n")
	}

	// Network permissions
	if len(permissions.Network) > 0 {
		profile.WriteString("  # Network permissions\n")
		profile.WriteString("  network inet stream,\n")
		profile.WriteString("  network inet6 stream,\n")
		profile.WriteString("\n")
	} else {
		// Deny network by default
		profile.WriteString("  # Network denied\n")
		profile.WriteString("  deny network,\n\n")
	}

	// Deny sensitive paths
	profile.WriteString("  # Deny sensitive system paths\n")
	profile.WriteString("  deny /etc/** w,\n")
	profile.WriteString("  deny /root/** rw,\n")
	profile.WriteString("  deny @{HOME}/.ssh/** rw,\n")
	profile.WriteString("  deny @{HOME}/.aws/** rw,\n")
	profile.WriteString("  deny @{HOME}/.gnupg/** rw,\n")
	profile.WriteString("  deny /sys/** w,\n")
	profile.WriteString("  deny /proc/** w,\n")
	profile.WriteString("\n")

	// Allow execution of whitelisted commands
	if len(permissions.Commands) > 0 {
		profile.WriteString("  # Allowed command execution\n")
		for _, allowedCmd := range permissions.Commands {
			// Find full path
			cmdPath, err := exec.LookPath(allowedCmd)
			if err != nil {
				// If not found, use the command as-is
				cmdPath = allowedCmd
			}
			fmt.Fprintf(&profile, "  %s ix,\n", cmdPath)
		}
		profile.WriteString("\n")
	}

	profile.WriteString("}\n")

	return profile.String(), nil
}

// writeAppArmorProfile writes the profile to a temporary file
func writeAppArmorProfile(profile, cmd string) (string, error) {
	// Create profile in /tmp (readable by aa-exec)
	profileName := fmt.Sprintf("engram_%s", hashCommand(cmd))
	profilePath := filepath.Join("/tmp", fmt.Sprintf("%s.profile", profileName))

	// 0600: the profile must be readable by aa-exec running as the same
	// user; other users have no business reading it.
	err := os.WriteFile(profilePath, []byte(profile), 0600)
	if err != nil {
		return "", err
	}

	return profilePath, nil
}

// hashCommand creates a short hash of the command for profile naming
func hashCommand(cmd string) string {
	hash := sha256.Sum256([]byte(cmd))
	return fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path // Return as-is if we can't get home dir
		}
		return filepath.Join(homeDir, path[2:])
	}

	// For paths with @{HOME}, AppArmor will expand them
	if strings.Contains(path, "~") && !strings.Contains(path, "@{HOME}") {
		path = strings.ReplaceAll(path, "~", "@{HOME}")
	}

	return path
}
