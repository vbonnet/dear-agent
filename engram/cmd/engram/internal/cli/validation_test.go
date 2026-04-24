package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateEnum(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    string
		allowed  []string
		wantErr  bool
		errMatch string
	}{
		{
			name:    "valid value",
			field:   "format",
			value:   "json",
			allowed: []string{"json", "text", "table"},
			wantErr: false,
		},
		{
			name:    "empty value is allowed",
			field:   "format",
			value:   "",
			allowed: []string{"json", "text"},
			wantErr: false,
		},
		{
			name:     "invalid value",
			field:    "format",
			value:    "xml",
			allowed:  []string{"json", "text"},
			wantErr:  true,
			errMatch: "Invalid format: xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEnum(tt.field, tt.value, tt.allowed)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEnum() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidateEnum() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidateEnumRequired(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    string
		allowed  []string
		wantErr  bool
		errMatch string
	}{
		{
			name:    "valid value",
			field:   "shell",
			value:   "bash",
			allowed: []string{"bash", "zsh"},
			wantErr: false,
		},
		{
			name:     "empty value fails",
			field:    "shell",
			value:    "",
			allowed:  []string{"bash", "zsh"},
			wantErr:  true,
			errMatch: "shell is required",
		},
		{
			name:     "invalid value",
			field:    "shell",
			value:    "cmd",
			allowed:  []string{"bash", "zsh"},
			wantErr:  true,
			errMatch: "Invalid shell: cmd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEnumRequired(tt.field, tt.value, tt.allowed)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEnumRequired() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidateEnumRequired() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidateRange(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    float64
		min      float64
		max      float64
		wantErr  bool
		errMatch string
	}{
		{
			name:    "value in range",
			field:   "importance",
			value:   0.5,
			min:     0,
			max:     1,
			wantErr: false,
		},
		{
			name:    "value at min",
			field:   "importance",
			value:   0,
			min:     0,
			max:     1,
			wantErr: false,
		},
		{
			name:    "value at max",
			field:   "importance",
			value:   1,
			min:     0,
			max:     1,
			wantErr: false,
		},
		{
			name:     "value below min",
			field:    "importance",
			value:    -0.1,
			min:      0,
			max:      1,
			wantErr:  true,
			errMatch: "Invalid importance: -0.10",
		},
		{
			name:     "value above max",
			field:    "importance",
			value:    1.5,
			min:      0,
			max:      1,
			wantErr:  true,
			errMatch: "Invalid importance: 1.50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRange(tt.field, tt.value, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRange() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidateRange() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidateRangeInt(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    int
		min      int
		max      int
		wantErr  bool
		errMatch string
	}{
		{
			name:    "value in range",
			field:   "limit",
			value:   50,
			min:     1,
			max:     100,
			wantErr: false,
		},
		{
			name:     "value below min",
			field:    "limit",
			value:    0,
			min:      1,
			max:      100,
			wantErr:  true,
			errMatch: "Invalid limit: 0",
		},
		{
			name:     "value above max",
			field:    "limit",
			value:    150,
			min:      1,
			max:      100,
			wantErr:  true,
			errMatch: "Invalid limit: 150",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRangeInt(tt.field, tt.value, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRangeInt() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidateRangeInt() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidatePositive(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    int
		wantErr  bool
		errMatch string
	}{
		{
			name:    "positive value",
			field:   "limit",
			value:   10,
			wantErr: false,
		},
		{
			name:    "zero is valid",
			field:   "limit",
			value:   0,
			wantErr: false,
		},
		{
			name:     "negative value",
			field:    "limit",
			value:    -5,
			wantErr:  true,
			errMatch: "Invalid limit: -5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePositive(tt.field, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePositive() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidatePositive() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidateNonEmpty(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		value    string
		wantErr  bool
		errMatch string
	}{
		{
			name:    "non-empty value",
			field:   "namespace",
			value:   "user,alice",
			wantErr: false,
		},
		{
			name:     "empty value",
			field:    "namespace",
			value:    "",
			wantErr:  true,
			errMatch: "namespace cannot be empty",
		},
		{
			name:     "whitespace only",
			field:    "namespace",
			value:    "   ",
			wantErr:  true,
			errMatch: "namespace cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNonEmpty(tt.field, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNonEmpty() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidateNonEmpty() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidateNamespace(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		wantErr  bool
		errMatch string
	}{
		{
			name:    "valid namespace",
			value:   "user,alice",
			wantErr: false,
		},
		{
			name:    "valid namespace with spaces",
			value:   "user, alice, project",
			wantErr: false,
		},
		{
			name:    "single part",
			value:   "user",
			wantErr: false,
		},
		{
			name:     "empty namespace",
			value:    "",
			wantErr:  true,
			errMatch: "namespace cannot be empty",
		},
		{
			name:     "contains empty parts",
			value:    "user,,alice",
			wantErr:  true,
			errMatch: "contains empty parts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNamespace(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNamespace() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidateNamespace() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidateAtLeastOne(t *testing.T) {
	tests := []struct {
		name        string
		fields      map[string]string
		description string
		wantErr     bool
		errMatch    string
	}{
		{
			name: "one field provided",
			fields: map[string]string{
				"set-content": "new content",
				"set-type":    "",
			},
			description: "update field",
			wantErr:     false,
		},
		{
			name: "multiple fields provided",
			fields: map[string]string{
				"set-content": "content",
				"set-type":    "episodic",
			},
			description: "update field",
			wantErr:     false,
		},
		{
			name: "no fields provided",
			fields: map[string]string{
				"set-content": "",
				"set-type":    "",
			},
			description: "update field",
			wantErr:     true,
			errMatch:    "At least one update field is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAtLeastOne(tt.fields, tt.description)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAtLeastOne() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidateAtLeastOne() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidatePathExists(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name      string
		field     string
		path      string
		expectDir bool
		wantErr   bool
		errMatch  string
	}{
		{
			name:      "empty path is allowed",
			field:     "path",
			path:      "",
			expectDir: false,
			wantErr:   false,
		},
		{
			name:      "valid directory",
			field:     "path",
			path:      tmpDir,
			expectDir: true,
			wantErr:   false,
		},
		{
			name:      "valid file",
			field:     "path",
			path:      tmpFile,
			expectDir: false,
			wantErr:   false,
		},
		{
			name:      "directory expected but file provided",
			field:     "path",
			path:      tmpFile,
			expectDir: true,
			wantErr:   true,
			errMatch:  "Expected directory, found file",
		},
		{
			name:      "file expected but directory provided",
			field:     "path",
			path:      tmpDir,
			expectDir: false,
			wantErr:   true,
			errMatch:  "Expected file, found directory",
		},
		{
			name:      "path does not exist",
			field:     "path",
			path:      "/nonexistent/path",
			expectDir: false,
			wantErr:   true,
			errMatch:  "Path does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePathExists(tt.field, tt.path, tt.expectDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePathExists() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidatePathExists() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidateOutputFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		allowed  []OutputFormat
		wantErr  bool
		errMatch string
	}{
		{
			name:    "valid format - json",
			format:  "json",
			allowed: []OutputFormat{FormatJSON, FormatText},
			wantErr: false,
		},
		{
			name:    "valid format - table",
			format:  "table",
			allowed: []OutputFormat{FormatJSON, FormatText, FormatTable},
			wantErr: false,
		},
		{
			name:    "empty format is allowed",
			format:  "",
			allowed: []OutputFormat{FormatJSON},
			wantErr: false,
		},
		{
			name:     "invalid format",
			format:   "xml",
			allowed:  []OutputFormat{FormatJSON, FormatText},
			wantErr:  true,
			errMatch: "Invalid format: xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOutputFormat(tt.format, tt.allowed...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOutputFormat() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidateOutputFormat() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidateTier(t *testing.T) {
	tests := []struct {
		name     string
		tier     string
		wantErr  bool
		errMatch string
	}{
		{
			name:    "valid tier - user",
			tier:    "user",
			wantErr: false,
		},
		{
			name:    "valid tier - all",
			tier:    "all",
			wantErr: false,
		},
		{
			name:    "empty tier is allowed",
			tier:    "",
			wantErr: false,
		},
		{
			name:     "invalid tier",
			tier:     "invalid",
			wantErr:  true,
			errMatch: "Invalid tier: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTier(tt.tier)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTier() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidateTier() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

func TestValidateShellType(t *testing.T) {
	tests := []struct {
		name     string
		shell    string
		wantErr  bool
		errMatch string
	}{
		{
			name:    "valid shell - bash",
			shell:   "bash",
			wantErr: false,
		},
		{
			name:    "valid shell - zsh",
			shell:   "zsh",
			wantErr: false,
		},
		{
			name:     "empty shell",
			shell:    "",
			wantErr:  true,
			errMatch: "shell is required",
		},
		{
			name:     "invalid shell",
			shell:    "cmd",
			wantErr:  true,
			errMatch: "Invalid shell: cmd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateShellType(tt.shell)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateShellType() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMatch != "" {
				if !contains(err.Error(), tt.errMatch) {
					t.Errorf("ValidateShellType() error = %v, expected to contain %q", err, tt.errMatch)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
