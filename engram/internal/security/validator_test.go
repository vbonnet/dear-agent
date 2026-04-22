package security

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// TestNewValidator verifies validator initialization
func TestNewValidator(t *testing.T) {
	validator := NewValidator()
	if validator == nil {
		t.Fatal("NewValidator() returned nil")
	}
}

// TestValidatePermissions_Valid verifies valid permissions pass
func TestValidatePermissions_Valid(t *testing.T) {
	validator := NewValidator()

	permissions := Permissions{
		Filesystem: []string{
			"/tmp/test/.engram",
			"/tmp",
			"./project",
		},
		Network: []string{
			"api.anthropic.com",
			"github.com",
		},
		Commands: []string{
			"git",
			"npm",
			"/usr/bin/python3",
		},
	}

	err := validator.ValidatePermissions(permissions)
	if err != nil {
		t.Errorf("ValidatePermissions() failed with valid permissions: %v", err)
	}
}

// TestValidatePermissions_RootFilesystem verifies root filesystem access is denied
func TestValidatePermissions_RootFilesystem(t *testing.T) {
	validator := NewValidator()

	permissions := Permissions{
		Filesystem: []string{"/"},
	}

	err := validator.ValidatePermissions(permissions)
	if err == nil {
		t.Fatal("ValidatePermissions() succeeded with root filesystem access, want error")
	}
}

// TestValidatePermissions_HomeDirectory verifies home directory access is allowed
func TestValidatePermissions_HomeDirectory(t *testing.T) {
	validator := NewValidator()

	permissions := Permissions{
		Filesystem: []string{
			"$HOME/.config",
			"~/.engram",
		},
	}

	err := validator.ValidatePermissions(permissions)
	if err != nil {
		t.Errorf("ValidatePermissions() failed with home directory access: %v", err)
	}
}

// TestValidatePermissions_NetworkWildcard verifies wildcard network access is allowed
func TestValidatePermissions_NetworkWildcard(t *testing.T) {
	validator := NewValidator()

	permissions := Permissions{
		Network: []string{"*"},
	}

	err := validator.ValidatePermissions(permissions)
	if err != nil {
		t.Errorf("ValidatePermissions() failed with wildcard network: %v", err)
	}
}

// TestValidatePermissions_InvalidCommand verifies non-absolute, non-well-known commands fail
func TestValidatePermissions_InvalidCommand(t *testing.T) {
	validator := NewValidator()

	permissions := Permissions{
		Commands: []string{"random-binary"},
	}

	err := validator.ValidatePermissions(permissions)
	if err == nil {
		t.Fatal("ValidatePermissions() succeeded with invalid command, want error")
	}
}

// TestValidatePermissions_WellKnownCommands verifies well-known commands pass
func TestValidatePermissions_WellKnownCommands(t *testing.T) {
	validator := NewValidator()

	wellKnown := []string{
		"git", "gh", "npm", "node", "python", "python3",
		"make", "docker", "kubectl",
	}

	for _, cmd := range wellKnown {
		permissions := Permissions{
			Commands: []string{cmd},
		}

		err := validator.ValidatePermissions(permissions)
		if err != nil {
			t.Errorf("ValidatePermissions() failed for well-known command %q: %v", cmd, err)
		}
	}
}

// TestValidatePermissions_AbsolutePathCommand verifies absolute path commands pass
func TestValidatePermissions_AbsolutePathCommand(t *testing.T) {
	validator := NewValidator()

	permissions := Permissions{
		Commands: []string{
			"/usr/bin/python3",
			"/bin/bash",
			"/usr/local/bin/custom-tool",
		},
	}

	err := validator.ValidatePermissions(permissions)
	if err != nil {
		t.Errorf("ValidatePermissions() failed with absolute path commands: %v", err)
	}
}

// TestValidateFilesystemPath_PathTraversal verifies path cleaning
func TestValidateFilesystemPath_PathTraversal(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		path    string
		wantErr bool
	}{
		{"/tmp/test/../../../", true}, // Cleans to /
		{"/tmp/../../../etc", false},  // Cleans to /etc (not root)
		{"./project/../..", false},    // Relative paths allowed
		{"/", true},                   // Root explicitly
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			err := validator.validateFilesystemPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFilesystemPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

// TestIsWellKnownCommand verifies well-known command detection
func TestIsWellKnownCommand(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		cmd  string
		want bool
	}{
		{"git", true},
		{"npm", true},
		{"python3", true},
		{"kubectl", true},
		{"random-binary", false},
		{"/usr/bin/git", false}, // Absolute paths are not "well-known"
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := validator.isWellKnownCommand(tt.cmd)
			if got != tt.want {
				t.Errorf("isWellKnownCommand(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// TestValidatePermissions_EmptyPermissions verifies empty permissions are valid
func TestValidatePermissions_EmptyPermissions(t *testing.T) {
	validator := NewValidator()

	permissions := Permissions{}

	err := validator.ValidatePermissions(permissions)
	if err != nil {
		t.Errorf("ValidatePermissions() failed with empty permissions: %v", err)
	}
}

// TestValidatePermissions_MultipleErrors verifies only first error is returned
func TestValidatePermissions_MultipleErrors(t *testing.T) {
	validator := NewValidator()

	permissions := Permissions{
		Filesystem: []string{"/"},       // Invalid
		Commands:   []string{"bad-cmd"}, // Also invalid
	}

	err := validator.ValidatePermissions(permissions)
	if err == nil {
		t.Fatal("ValidatePermissions() succeeded with multiple errors, want error")
	}

	// Should return filesystem error first (checked before commands)
	if err.Error() == "" {
		t.Error("Error message is empty")
	}
}

// TestSuspiciousPermissionLogging verifies suspicious permissions are logged
func TestSuspiciousPermissionLogging(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantLogged  bool
		wantReason  string
		wantMessage string
	}{
		{
			name:        "home directory with $HOME",
			path:        "$HOME/.config",
			wantLogged:  true,
			wantReason:  "home_directory_access",
			wantMessage: "Plugin requests home directory access",
		},
		{
			name:        "home directory with tilde",
			path:        "~/.engram",
			wantLogged:  true,
			wantReason:  "home_directory_access",
			wantMessage: "Plugin requests home directory access",
		},
		{
			name:        "sensitive system directory /etc",
			path:        "/etc/passwd",
			wantLogged:  true,
			wantReason:  "system_configuration",
			wantMessage: "Plugin requests sensitive system directory access",
		},
		{
			name:        "sensitive system directory /root",
			path:        "/root/.ssh",
			wantLogged:  true,
			wantReason:  "root_home_directory",
			wantMessage: "Plugin requests sensitive system directory access",
		},
		{
			name:        "sensitive user directory .ssh",
			path:        "/tmp/test/.ssh/id_rsa",
			wantLogged:  true,
			wantReason:  "ssh_keys",
			wantMessage: "Plugin requests sensitive user directory access",
		},
		{
			name:        "sensitive user directory .aws",
			path:        "/tmp/test/.aws/credentials",
			wantLogged:  true,
			wantReason:  "aws_credentials",
			wantMessage: "Plugin requests sensitive user directory access",
		},
		{
			name:        "sensitive file .env",
			path:        "/tmp/test/project/.env",
			wantLogged:  true,
			wantReason:  "sensitive_file_.env",
			wantMessage: "Plugin requests sensitive user directory access",
		},
		{
			name:        "wildcard pattern",
			path:        "/tmp/*.txt",
			wantLogged:  true,
			wantReason:  "wildcard_pattern",
			wantMessage: "Plugin requests broad filesystem permissions",
		},
		{
			name:       "normal safe path",
			path:       "/tmp/safe-dir",
			wantLogged: false,
		},
		{
			name:       "plugin directory",
			path:       "/tmp/test/.engram/plugins/my-plugin",
			wantLogged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create buffer to capture log output
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelWarn,
			}))

			// Create validator with custom logger
			validator := NewValidatorWithLogger(logger)

			// Validate permissions (will trigger logging)
			permissions := Permissions{
				Filesystem: []string{tt.path},
			}
			_ = validator.ValidatePermissions(permissions)

			// Check if logged
			logOutput := buf.String()
			if tt.wantLogged {
				if logOutput == "" {
					t.Errorf("Expected log output for path %q, got none", tt.path)
					return
				}

				// Parse JSON log
				var logEntry map[string]interface{}
				if err := json.Unmarshal([]byte(strings.TrimSpace(logOutput)), &logEntry); err != nil {
					t.Fatalf("Failed to parse log output as JSON: %v\nOutput: %s", err, logOutput)
				}

				// Verify log message
				if msg, ok := logEntry["msg"].(string); !ok || msg != tt.wantMessage {
					t.Errorf("Log message = %q, want %q", msg, tt.wantMessage)
				}

				// Verify reason field
				if reason, ok := logEntry["reason"].(string); !ok || reason != tt.wantReason {
					t.Errorf("Log reason = %q, want %q", reason, tt.wantReason)
				}

				// Verify path is logged
				if path, ok := logEntry["path"].(string); !ok || path != tt.path {
					t.Errorf("Log path = %q, want %q", path, tt.path)
				}

				// Verify risk is logged
				if _, ok := logEntry["risk"].(string); !ok {
					t.Error("Log missing 'risk' field")
				}
			} else {
				if logOutput != "" {
					t.Errorf("Expected no log output for path %q, got: %s", tt.path, logOutput)
				}
			}
		})
	}
}

// TestHomeDirectoryPathDetection verifies home directory detection
func TestHomeDirectoryPathDetection(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		original string
		clean    string
		want     bool
	}{
		{"$HOME/.config", "$HOME/.config", true},
		{"~/.engram", "~/.engram", true},
		{"~/project", "~/project", true},
		{"/tmp/test/.config", "/tmp/test/.config", false},
		{"/tmp", "/tmp", false},
	}

	for _, tt := range tests {
		t.Run(tt.original, func(t *testing.T) {
			got := validator.isHomeDirectoryPath(tt.original, tt.clean)
			if got != tt.want {
				t.Errorf("isHomeDirectoryPath(%q, %q) = %v, want %v", tt.original, tt.clean, got, tt.want)
			}
		})
	}
}

// TestSensitiveSystemPathDetection verifies system directory detection
func TestSensitiveSystemPathDetection(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		path string
		want string
	}{
		{"/etc/passwd", "system_configuration"},
		{"/etc", "system_configuration"},
		{"/root/.ssh", "root_home_directory"},
		{"/var/log", "system_variable_data"},
		{"/sys/class", "system_information"},
		{"/proc/cpuinfo", "process_information"},
		{"/boot/grub", "boot_configuration"},
		{"/tmp/safe", ""},
		{"/home/user", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := validator.isSensitiveSystemPath(tt.path)
			if got != tt.want {
				t.Errorf("isSensitiveSystemPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// TestSensitiveUserPathDetection verifies user directory/file detection
func TestSensitiveUserPathDetection(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		original string
		clean    string
		want     string
	}{
		{"/tmp/test/.ssh/id_rsa", "/tmp/test/.ssh/id_rsa", "ssh_keys"},
		{"/tmp/test/.aws/credentials", "/tmp/test/.aws/credentials", "aws_credentials"},
		{"/tmp/test/.gnupg/private", "/tmp/test/.gnupg/private", "gpg_keys"},
		{"/tmp/test/.config/app", "/tmp/test/.config/app", "user_configuration"},
		{"/tmp/test/project/.env", "/tmp/test/project/.env", "sensitive_file_.env"},
		{"~/project/.env.local", "~/project/.env.local", "sensitive_file_.env.local"},
		{"/tmp/test/credentials.json", "/tmp/test/credentials.json", "sensitive_file_credentials.json"},
		{"/tmp/safe.txt", "/tmp/safe.txt", ""},
		{"/tmp/test/project/main.go", "/tmp/test/project/main.go", ""},
	}

	for _, tt := range tests {
		t.Run(tt.original, func(t *testing.T) {
			got := validator.isSensitiveUserPath(tt.original, tt.clean)
			if got != tt.want {
				t.Errorf("isSensitiveUserPath(%q, %q) = %q, want %q", tt.original, tt.clean, got, tt.want)
			}
		})
	}
}

// TestBroadPermissionDetection verifies wildcard detection
func TestBroadPermissionDetection(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		path string
		want bool
	}{
		{"/tmp/*.txt", true},
		{"/tmp/test/**/*.go", true},
		{"/var/log/app-?.log", true},
		{"/tmp/specific-file.txt", false},
		{"/tmp/test/project", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := validator.isBroadPermission(tt.path)
			if got != tt.want {
				t.Errorf("isBroadPermission(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestNewValidatorWithLogger verifies custom logger initialization
func TestNewValidatorWithLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	validator := NewValidatorWithLogger(logger)
	if validator == nil {
		t.Fatal("NewValidatorWithLogger() returned nil")
	}
	if validator.logger != logger {
		t.Error("Validator logger not set correctly")
	}
}
