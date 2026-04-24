package security

import (
	"runtime"
	"strings"
	"testing"
)

// TestNewSandbox verifies sandbox initialization
func TestNewSandbox(t *testing.T) {
	sandbox := NewSandbox()
	if sandbox == nil {
		t.Fatal("NewSandbox() returned nil")
	}

	if sandbox.platform != runtime.GOOS {
		t.Errorf("Sandbox.platform = %q, want %q", sandbox.platform, runtime.GOOS)
	}
}

// TestApply_Darwin verifies sandbox-exec wrapping on macOS
func TestApply_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test on non-Darwin platform")
	}

	sandbox := NewSandbox()

	permissions := Permissions{
		Filesystem: []string{"/tmp", "/tmp/test/.engram"},
		Network:    []string{"api.anthropic.com"},
	}

	cmd := "python3"
	args := []string{"script.py", "--arg"}

	result, err := sandbox.Apply(cmd, args, permissions)
	if err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	// Should be wrapped in sandbox-exec
	if result[0] != "sandbox-exec" {
		t.Errorf("Apply() result[0] = %q, want %q", result[0], "sandbox-exec")
	}

	// Should have -p flag with profile
	if result[1] != "-p" {
		t.Errorf("Apply() result[1] = %q, want %q", result[1], "-p")
	}

	// Profile should be in result[2]
	profile := result[2]
	if !strings.Contains(profile, "(version 1)") {
		t.Errorf("Profile missing version declaration: %s", profile)
	}
	if !strings.Contains(profile, "(deny default)") {
		t.Errorf("Profile missing deny-all: %s", profile)
	}

	// Original command should follow profile
	if result[3] != cmd {
		t.Errorf("Apply() result[3] = %q, want %q", result[3], cmd)
	}

	// Original args should be preserved
	if result[4] != "script.py" || result[5] != "--arg" {
		t.Errorf("Apply() args not preserved: %v", result[4:])
	}
}

// TestApply_UnsupportedPlatform verifies graceful degradation
func TestApply_UnsupportedPlatform(t *testing.T) {
	sandbox := &Sandbox{platform: "unsupported"}

	permissions := Permissions{}
	cmd := "test-cmd"
	args := []string{"arg1", "arg2"}

	result, err := sandbox.Apply(cmd, args, permissions)
	if err != nil {
		t.Fatalf("Apply() failed on unsupported platform: %v", err)
	}

	// Should return unmodified command
	if result[0] != cmd {
		t.Errorf("Apply() result[0] = %q, want %q", result[0], cmd)
	}
	if len(result) != 3 {
		t.Errorf("Apply() result length = %d, want 3", len(result))
	}
}

// TestBuildDarwinProfile verifies sandbox-exec profile generation
func TestBuildDarwinProfile(t *testing.T) {
	sandbox := NewSandbox()

	permissions := Permissions{
		Filesystem: []string{"/tmp", "/tmp/test/project"},
		Network:    []string{"github.com"},
	}

	profile := sandbox.buildDarwinProfile("/bin/echo", permissions)

	// Verify basic structure
	if !strings.Contains(profile, "(version 1)") {
		t.Error("Profile missing version declaration")
	}
	if !strings.Contains(profile, "(deny default)") {
		t.Error("Profile missing deny-all")
	}

	// Verify filesystem permissions
	if !strings.Contains(profile, "/tmp") {
		t.Error("Profile missing /tmp filesystem permission")
	}
	if !strings.Contains(profile, "/tmp/test/project") {
		t.Error("Profile missing /tmp/test/project filesystem permission")
	}

	// Verify network permission
	if !strings.Contains(profile, "(allow network*)") {
		t.Error("Profile missing network permission")
	}

	// Verify basic operations allowed
	if !strings.Contains(profile, "(allow process*)") {
		t.Error("Profile missing process permission")
	}
	if !strings.Contains(profile, "(allow sysctl-read)") {
		t.Error("Profile missing sysctl-read permission")
	}
}

// TestBuildDarwinProfile_NoNetwork verifies network is not allowed when not requested
func TestBuildDarwinProfile_NoNetwork(t *testing.T) {
	sandbox := NewSandbox()

	permissions := Permissions{
		Filesystem: []string{"/tmp"},
		Network:    []string{}, // Empty network
	}

	profile := sandbox.buildDarwinProfile("/bin/echo", permissions)

	if strings.Contains(profile, "(allow network*)") {
		t.Error("Profile contains network permission when not requested")
	}
}

// TestBuildDarwinProfile_EmptyPermissions verifies minimal profile with no permissions
func TestBuildDarwinProfile_EmptyPermissions(t *testing.T) {
	sandbox := NewSandbox()

	permissions := Permissions{}

	profile := sandbox.buildDarwinProfile("/bin/echo", permissions)

	// Should still have basic structure
	if !strings.Contains(profile, "(version 1)") {
		t.Error("Profile missing version declaration")
	}
	if !strings.Contains(profile, "(deny default)") {
		t.Error("Profile missing deny-all")
	}

	// Should have basic operations
	if !strings.Contains(profile, "(allow process*)") {
		t.Error("Profile missing process permission")
	}

	// Should always allow file-read* for system libraries, but not file-write*
	if !strings.Contains(profile, "(allow file-read*)") {
		t.Error("Profile missing file-read permission (required for system libraries)")
	}
	if strings.Contains(profile, "file-write*") {
		t.Error("Profile contains file-write permissions when none requested")
	}
	if strings.Contains(profile, "(allow network*)") {
		t.Error("Profile contains network permissions when none requested")
	}
}

// TestPermissions_Structure verifies Permissions type
func TestPermissions_Structure(t *testing.T) {
	permissions := Permissions{
		Filesystem: []string{"/path1", "/path2"},
		Network:    []string{"domain1.com", "domain2.com"},
		Commands:   []string{"git", "npm"},
	}

	if len(permissions.Filesystem) != 2 {
		t.Errorf("Filesystem length = %d, want 2", len(permissions.Filesystem))
	}
	if len(permissions.Network) != 2 {
		t.Errorf("Network length = %d, want 2", len(permissions.Network))
	}
	if len(permissions.Commands) != 2 {
		t.Errorf("Commands length = %d, want 2", len(permissions.Commands))
	}
}

// TestApply_MultipleFilesystemPaths verifies multiple paths in profile
func TestApply_MultipleFilesystemPaths(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Skipping macOS-specific test")
	}

	sandbox := NewSandbox()

	permissions := Permissions{
		Filesystem: []string{
			"/tmp",
			"/tmp/test/.engram",
			"/var/log",
		},
	}

	result, err := sandbox.Apply("test", []string{}, permissions)
	if err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	profile := result[2]

	for _, path := range permissions.Filesystem {
		if !strings.Contains(profile, path) {
			t.Errorf("Profile missing filesystem path %q", path)
		}
	}
}
