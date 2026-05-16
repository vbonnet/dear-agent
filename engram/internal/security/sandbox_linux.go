//go:build linux
// +build linux

package security

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// isPermissionDeniedError reports whether err comes from running
// apparmor_parser without CAP_MAC_ADMIN. We treat that case as the normal
// dev/CI path (validation-only fallback). Anything else — including
// syntactically invalid profiles produced by attacker-influenced
// permissions — fails closed.
func isPermissionDeniedError(err error) bool {
	if err == nil {
		return false
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if status, ok := ee.Sys().(syscall.WaitStatus); ok {
			if status.ExitStatus() == 13 {
				return true
			}
		}
	}
	return errors.Is(err, os.ErrPermission)
}

// apparmorProbeProfile is a constant, trivially-valid AppArmor profile used
// only to answer "can this process load *any* profile at all?". It is fixed
// source code and never incorporates attacker-influenced input, so its load
// result is an unambiguous environmental signal, not a property of the
// generated, permission-derived profile.
const apparmorProbeProfile = "#include <tunables/global>\nprofile engram_probe_noop flags=(complain) {\n  #include <abstractions/base>\n}\n"

// canLoadAppArmorProfiles reports whether this process is able to load an
// AppArmor profile into the kernel. It attempts to load apparmorProbeProfile;
// because that profile is constant (never derived from permissions), a
// failure here can only mean the environment forbids policy loading at all —
// missing CAP_MAC_ADMIN in dev/CI, a sandboxed CI runner, apparmor_parser
// absent, securityfs unavailable, etc. This is what lets applyLinux
// distinguish an environmental inability to sandbox (safe to fall back to
// validation-only, same intent as the historical exit-code-13 check) from a
// failure specific to the *generated*, permission-derived profile (a possible
// profile-text injection — must fail closed).
//
// It deliberately does not parse apparmor_parser's exit code or stderr
// wording, which vary across distro and runner image: classic setups exit 13
// when unprivileged, but GitHub's ubuntu runner exits 1. That brittle
// exit-code heuristic is exactly what broke CI.
func canLoadAppArmorProfiles() bool {
	f, err := os.CreateTemp("", "engram_apparmor_probe_*.profile")
	if err != nil {
		return false
	}
	probePath := f.Name()
	defer os.Remove(probePath)
	if _, err := f.WriteString(apparmorProbeProfile); err != nil {
		f.Close()
		return false
	}
	if err := f.Close(); err != nil {
		return false
	}
	if err := exec.Command("apparmor_parser", "-r", probePath).Run(); err != nil {
		return false
	}
	// Best effort: unload the probe so it does not linger until reboot.
	_ = exec.Command("apparmor_parser", "-R", probePath).Run()
	return true
}

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

	// Attempt to load profile with apparmor_parser.
	//
	// The previous implementation collapsed any apparmor_parser failure
	// (permission denied, syntactically invalid profile, kernel module
	// missing, …) into "run the command unsandboxed." Together with the
	// permission-validator's narrow filtering, that turned a profile-text
	// injection (e.g., a plugin manifest with a newline in its filesystem
	// permission entry) into a privilege escalation: craft a value that
	// breaks the profile syntax, and apparmor_parser's failure dropped
	// the sandbox entirely.
	//
	// Now we distinguish:
	//   - the environment cannot load *any* profile (no CAP_MAC_ADMIN in
	//     dev/CI, sandboxed runner, apparmor_parser absent, securityfs
	//     unavailable) → keep the validation-only fallback. The validators
	//     we run upstream already reject the dangerous AppArmor
	//     metacharacters, so the profile we would have loaded is the only
	//     thing missing. We detect this with a constant known-good probe
	//     rather than apparmor_parser's exit code, which is 13 on classic
	//     setups but 1 on (e.g.) GitHub's ubuntu runner — the brittle
	//     exit-code check is what broke CI.
	//   - the environment *can* load the probe but not this profile →
	//     fail closed. The only difference between the two is the
	//     permission-derived, attacker-influenceable profile text, so a
	//     failure there is a real signal worth surfacing, not papering over.
	loadCmd := exec.Command("apparmor_parser", "-r", profilePath)
	if err := loadCmd.Run(); err != nil {
		os.Remove(profilePath)
		if isPermissionDeniedError(err) || !canLoadAppArmorProfiles() {
			return append([]string{cmd}, args...), nil //nolint:nilerr // intentional fallback to validation-only mode
		}
		return nil, fmt.Errorf("apparmor_parser failed to load profile (failing closed; refusing unsandboxed exec): %w", err)
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
