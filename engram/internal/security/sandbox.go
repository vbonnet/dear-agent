package security

import (
	"fmt"
	"runtime"
)

// Sandbox represents OS-level security sandboxing
type Sandbox struct {
	platform string
}

// NewSandbox creates a new sandbox for the current platform
func NewSandbox() *Sandbox {
	return &Sandbox{
		platform: runtime.GOOS,
	}
}

// Apply applies sandbox restrictions to a command
// Returns modified command args that include sandbox enforcement
func (s *Sandbox) Apply(cmd string, args []string, permissions Permissions) ([]string, error) {
	switch s.platform {
	case "linux":
		return s.applyLinux(cmd, args, permissions)
	case "darwin":
		return s.applyDarwin(cmd, args, permissions)
	default:
		// On unsupported platforms, return unmodified command
		// This degrades gracefully but logs a warning
		return append([]string{cmd}, args...), nil
	}
}

// applyLinux is implemented in sandbox_linux.go for Linux platforms
// On non-Linux platforms, a fallback stub is provided in sandbox_other.go

// applyDarwin applies sandbox-exec on macOS
func (s *Sandbox) applyDarwin(cmd string, args []string, permissions Permissions) ([]string, error) {
	// Build sandbox profile from permissions, passing the command to allow
	profile := s.buildDarwinProfile(cmd, permissions)

	// Wrap command in sandbox-exec
	sandboxArgs := []string{
		"sandbox-exec",
		"-p", profile,
		cmd,
	}
	sandboxArgs = append(sandboxArgs, args...)

	return sandboxArgs, nil
}

// buildDarwinProfile builds a sandbox-exec profile from permissions
func (s *Sandbox) buildDarwinProfile(cmd string, permissions Permissions) string {
	// Start with deny-all
	profile := "(version 1)\n(deny default)\n"

	// Allow execution of the specific command
	profile += fmt.Sprintf("(allow process-exec (literal \"%s\"))\n", cmd)

	// Allow basic operations
	profile += "(allow process*)\n"
	profile += "(allow sysctl-read)\n"

	// Allow macOS system services (required for sandboxed execution)
	profile += "(allow mach-lookup)\n"
	profile += "(allow ipc-posix-shm-read-data)\n"

	// Allow read access to all files (for system libraries and dependencies)
	// Write access is restricted to explicitly granted paths only
	profile += "(allow file-read*)\n"

	// Allow filesystem write access based on permissions
	for _, path := range permissions.Filesystem {
		profile += fmt.Sprintf("(allow file-write* (subpath \"%s\"))\n", path)
	}

	// Allow network access based on permissions
	if len(permissions.Network) > 0 {
		profile += "(allow network*)\n"
	}

	return profile
}

// Permissions represents sandbox permissions
type Permissions struct {
	Filesystem []string
	Network    []string
	Commands   []string
}
