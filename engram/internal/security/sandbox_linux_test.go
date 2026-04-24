//go:build linux
// +build linux

package security

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateAppArmorProfile(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		permissions Permissions
		wantContain []string
	}{
		{
			name: "Basic profile with filesystem permissions",
			cmd:  "/usr/bin/git",
			permissions: Permissions{
				Filesystem: []string{"/tmp", "~/.engram/"},
				Network:    []string{},
				Commands:   []string{},
			},
			wantContain: []string{
				"profile engram_",
				"/usr/bin/git rix,",
				"/tmp rw,",
				"deny network,",
			},
		},
		{
			name: "Profile with network permissions",
			cmd:  "/usr/bin/curl",
			permissions: Permissions{
				Filesystem: []string{"/tmp"},
				Network:    []string{"api.example.com"},
				Commands:   []string{},
			},
			wantContain: []string{
				"network inet stream,",
				"network inet6 stream,",
			},
		},
		{
			name: "Profile with command permissions",
			cmd:  "/usr/bin/script.sh",
			permissions: Permissions{
				Filesystem: []string{"/tmp"},
				Network:    []string{},
				Commands:   []string{"git", "jq"},
			},
			wantContain: []string{
				"git ix,",
				"jq ix,",
			},
		},
		{
			name: "Profile denies sensitive paths",
			cmd:  "/usr/bin/test",
			permissions: Permissions{
				Filesystem: []string{"/tmp"},
			},
			wantContain: []string{
				"deny /etc/** w,",
				"deny /root/** rw,",
				"deny @{HOME}/.ssh/** rw,",
				"deny @{HOME}/.aws/** rw,",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := generateAppArmorProfile(tt.cmd, tt.permissions)
			if err != nil {
				t.Fatalf("generateAppArmorProfile() error = %v", err)
			}

			for _, want := range tt.wantContain {
				if !strings.Contains(profile, want) {
					t.Errorf("Profile missing expected content: %q\nProfile:\n%s", want, profile)
				}
			}

			// Verify profile has proper structure
			if !strings.Contains(profile, "#include <tunables/global>") {
				t.Error("Profile missing tunables include")
			}
			if !strings.Contains(profile, "#include <abstractions/base>") {
				t.Error("Profile missing base abstractions")
			}
		})
	}
}

func TestHashCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"Simple command", "/usr/bin/git"},
		{"Command with path", "/usr/local/bin/custom"},
		{"Same command twice", "/usr/bin/test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := hashCommand(tt.cmd)

			// Hash should be 16 characters (8 bytes hex)
			if len(hash) != 16 {
				t.Errorf("hashCommand() length = %d, want 16", len(hash))
			}

			// Hash should be deterministic
			hash2 := hashCommand(tt.cmd)
			if hash != hash2 {
				t.Errorf("hashCommand() not deterministic: %s != %s", hash, hash2)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	homeDir, _ := os.UserHomeDir()

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "Tilde expansion",
			path: "~/.engram/",
			want: homeDir + "/.engram", // filepath.Join removes trailing slash
		},
		{
			name: "Absolute path unchanged",
			path: "/tmp/test",
			want: "/tmp/test",
		},
		{
			name: "Already using @{HOME}",
			path: "@{HOME}/.config",
			want: "@{HOME}/.config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.path)
			if got != tt.want {
				t.Errorf("expandPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsAppArmorAvailable(t *testing.T) {
	// This test depends on the system configuration
	// Just verify it doesn't panic
	available := isAppArmorAvailable()
	t.Logf("AppArmor available: %v", available)

	// If AppArmor is available, verify we can find aa-exec
	if available {
		// aa-exec should be in PATH
		_, err := os.Stat("/sys/module/apparmor/parameters/enabled")
		if err != nil {
			t.Logf("AppArmor sysfs not found (may be running in container): %v", err)
		}
	}
}

func TestApplyLinux_WithoutAppArmor(t *testing.T) {
	sandbox := NewSandbox()
	cmd := "/usr/bin/test"
	args := []string{"-f", "/tmp/file"}
	permissions := Permissions{
		Filesystem: []string{"/tmp"},
	}

	result, err := sandbox.applyLinux(cmd, args, permissions)
	if err != nil {
		t.Fatalf("applyLinux() error = %v", err)
	}

	// applyLinux returns unmodified command when:
	// 1. AppArmor is unavailable (!isAppArmorAvailable())
	// 2. AppArmor is available but profile loading fails (lack of root)
	// It only returns aa-exec wrapper when AppArmor is available AND we have permissions to load profiles

	// For user-level tests, we expect unmodified command in most cases
	// since loading AppArmor profiles requires root or CAP_MAC_ADMIN
	if result[0] == "aa-exec" {
		// Profile was successfully loaded (running with root or special permissions)
		t.Logf("AppArmor profile loaded successfully (running with elevated permissions)")
		if len(result) < 4 {
			t.Errorf("aa-exec wrapper should have at least 4 elements, got %d", len(result))
		}
	} else {
		// Most common case: profile loading failed, graceful fallback
		if result[0] != cmd {
			t.Errorf("applyLinux() should return unmodified command on fallback, got %v", result)
		}
	}
}

func TestWriteAppArmorProfile(t *testing.T) {
	profile := `#include <tunables/global>

profile test {
  #include <abstractions/base>
}
`
	cmd := "/usr/bin/test"

	profilePath, err := writeAppArmorProfile(profile, cmd)
	if err != nil {
		t.Fatalf("writeAppArmorProfile() error = %v", err)
	}
	defer os.Remove(profilePath)

	// Verify file was created
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		t.Error("Profile file was not created")
	}

	// Verify file contents
	content, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("Failed to read profile file: %v", err)
	}

	if string(content) != profile {
		t.Errorf("Profile content mismatch:\ngot:\n%s\nwant:\n%s", content, profile)
	}
}
