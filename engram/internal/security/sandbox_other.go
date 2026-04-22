//go:build !linux
// +build !linux

package security

// applyLinux is a fallback stub for non-Linux platforms
// On non-Linux systems, this returns the command unmodified
// since AppArmor/seccomp are Linux-specific
func (s *Sandbox) applyLinux(cmd string, args []string, permissions Permissions) ([]string, error) {
	// Not applicable on non-Linux platforms
	// Graceful degradation: return unmodified command
	return append([]string{cmd}, args...), nil
}
