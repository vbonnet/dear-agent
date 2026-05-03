package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateSafePath_PathTraversal tests path traversal attack prevention
func TestValidateSafePath_PathTraversal(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	allowedPaths := []string{
		filepath.Join(home, ".engram"),
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		// Valid paths
		{
			name:    "valid path within allowed directory",
			path:    filepath.Join(home, ".engram", "test.md"),
			wantErr: false,
		},
		{
			name:    "valid path with subdirectory",
			path:    filepath.Join(home, ".engram", "subdir", "test.md"),
			wantErr: false,
		},
		{
			name:    "tilde expansion valid",
			path:    "~/.engram/test.md",
			wantErr: false,
		},
		{
			name:    "env var expansion valid",
			path:    "$HOME/.engram/test.md",
			wantErr: false,
		},

		// Path traversal attacks
		{
			name:    "simple traversal",
			path:    "../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "complex traversal from allowed dir",
			path:    filepath.Join(home, ".engram", "..", "..", "etc", "passwd"),
			wantErr: true,
		},
		{
			name:    "absolute path outside allowed",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "tilde traversal",
			path:    "~/../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "absolute path to root",
			path:    "/",
			wantErr: true,
		},
		{
			name:    "absolute path to tmp",
			path:    "/tmp/evil",
			wantErr: true,
		},

		// Null byte injection
		{
			name:    "null byte in path",
			path:    filepath.Join(home, ".engram", "test\x00.md"),
			wantErr: true,
		},

		// Edge cases
		{
			name:    "empty path",
			path:    "",
			wantErr: false, // Empty paths are allowed for optional fields
		},
		{
			name:    "single dot",
			path:    filepath.Join(home, ".engram", "."),
			wantErr: false, // . is cleaned away
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSafePath("path", tt.path, allowedPaths)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSafePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

// TestValidateMaxLength_DoS tests denial-of-service prevention via length limits
func TestValidateMaxLength_DoS(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		maxLen  int
		wantErr bool
	}{
		{
			name:    "normal length",
			value:   "test query",
			maxLen:  1000,
			wantErr: false,
		},
		{
			name:    "at maximum",
			value:   strings.Repeat("a", 1000),
			maxLen:  1000,
			wantErr: false,
		},
		{
			name:    "one over maximum",
			value:   strings.Repeat("a", 1001),
			maxLen:  1000,
			wantErr: true,
		},
		{
			name:    "extreme length",
			value:   strings.Repeat("a", 1000000),
			maxLen:  1000,
			wantErr: true,
		},
		{
			name:    "empty string",
			value:   "",
			maxLen:  1000,
			wantErr: false,
		},
		{
			name:    "unicode characters",
			value:   strings.Repeat("🔥", 500),
			maxLen:  1000,
			wantErr: true, // Each emoji is 4 bytes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMaxLength("query", tt.value, tt.maxLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMaxLength() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateSafeEnvExpansion_Injection tests environment variable injection prevention
func TestValidateSafeEnvExpansion_Injection(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	allowedPaths := []string{
		filepath.Join(home, ".engram"),
	}

	// Set up test environment variables
	t.Setenv("TEST_SAFE_PATH", filepath.Join(home, ".engram", "config.yaml"))
	t.Setenv("TEST_EVIL_PATH", "/etc/shadow")
	defer os.Unsetenv("TEST_SAFE_PATH")
	defer os.Unsetenv("TEST_EVIL_PATH")

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:    "safe env expansion",
			value:   "$HOME/.engram/config.yaml",
			wantErr: false,
		},
		{
			name:    "safe custom env var",
			value:   "$TEST_SAFE_PATH",
			wantErr: false,
		},
		{
			name:    "malicious env var",
			value:   "$TEST_EVIL_PATH",
			wantErr: true,
		},
		{
			name:    "env var with traversal",
			value:   "$HOME/../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "no expansion needed",
			value:   filepath.Join(home, ".engram", "test.md"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSafeEnvExpansion("config", tt.value, allowedPaths)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSafeEnvExpansion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateNoTraversal tests directory traversal pattern detection
func TestValidateNoTraversal(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid path",
			path:    "engrams/test.md",
			wantErr: false,
		},
		{
			name:    "parent directory",
			path:    "../test.md",
			wantErr: true,
		},
		{
			name:    "multiple parent directories",
			path:    "../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "hidden traversal",
			path:    "subdir/../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNoTraversal("path", tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNoTraversal(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

// TestValidateNoShellMetacharacters tests command injection prevention
func TestValidateNoShellMetacharacters(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:    "safe alphanumeric",
			value:   "test123",
			wantErr: false,
		},
		{
			name:    "safe with spaces",
			value:   "test query string",
			wantErr: false,
		},
		{
			name:    "semicolon injection",
			value:   "test; rm -rf /",
			wantErr: true,
		},
		{
			name:    "pipe injection",
			value:   "test | cat /etc/passwd",
			wantErr: true,
		},
		{
			name:    "command substitution backticks",
			value:   "test`whoami`",
			wantErr: true,
		},
		{
			name:    "command substitution dollar",
			value:   "test$(whoami)",
			wantErr: true,
		},
		{
			name:    "redirect output",
			value:   "test > /tmp/output",
			wantErr: true,
		},
		{
			name:    "redirect input",
			value:   "test < /etc/passwd",
			wantErr: true,
		},
		{
			name:    "background execution",
			value:   "test &",
			wantErr: true,
		},
		{
			name:    "logical and",
			value:   "test && evil",
			wantErr: true,
		},
		{
			name:    "logical or",
			value:   "test || evil",
			wantErr: true,
		},
		{
			name:    "newline injection",
			value:   "test\nrm -rf /",
			wantErr: true,
		},
		{
			name:    "null byte",
			value:   "test\x00evil",
			wantErr: true,
		},
		{
			name:    "empty string",
			value:   "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNoShellMetacharacters("value", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNoShellMetacharacters(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

// TestValidateAlphanumeric tests alphanumeric validation
func TestValidateAlphanumeric(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		allowHyphens bool
		wantErr      bool
	}{
		{
			name:         "valid alphanumeric",
			value:        "test123",
			allowHyphens: false,
			wantErr:      false,
		},
		{
			name:         "valid with hyphens",
			value:        "test-123",
			allowHyphens: true,
			wantErr:      false,
		},
		{
			name:         "valid with underscores",
			value:        "test_123",
			allowHyphens: true,
			wantErr:      false,
		},
		{
			name:         "hyphen not allowed",
			value:        "test-123",
			allowHyphens: false,
			wantErr:      true,
		},
		{
			name:         "special characters",
			value:        "test@123",
			allowHyphens: false,
			wantErr:      true,
		},
		{
			name:         "spaces not allowed",
			value:        "test 123",
			allowHyphens: false,
			wantErr:      true,
		},
		{
			name:         "empty string",
			value:        "",
			allowHyphens: false,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAlphanumeric("value", tt.value, tt.allowHyphens)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAlphanumeric(%q, %v) error = %v, wantErr %v", tt.value, tt.allowHyphens, err, tt.wantErr)
			}
		})
	}
}

// TestValidateNamespaceComponents tests namespace security limits
func TestValidateNamespaceComponents(t *testing.T) {
	tests := []struct {
		name          string
		namespace     string
		maxComponents int
		maxCompLen    int
		wantErr       bool
	}{
		{
			name:          "valid namespace",
			namespace:     "user,alice,project",
			maxComponents: 10,
			maxCompLen:    50,
			wantErr:       false,
		},
		{
			name:          "at max components",
			namespace:     strings.Join(make([]string, 10), ","),
			maxComponents: 10,
			maxCompLen:    50,
			wantErr:       true, // Empty components
		},
		{
			name:          "too many components",
			namespace:     "a,b,c,d,e,f,g,h,i,j,k",
			maxComponents: 10,
			maxCompLen:    50,
			wantErr:       true,
		},
		{
			name:          "component too long",
			namespace:     strings.Repeat("a", 51),
			maxComponents: 10,
			maxCompLen:    50,
			wantErr:       true,
		},
		{
			name:          "empty component",
			namespace:     "user,,project",
			maxComponents: 10,
			maxCompLen:    50,
			wantErr:       true,
		},
		{
			name:          "single component",
			namespace:     "user",
			maxComponents: 10,
			maxCompLen:    50,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNamespaceComponents(tt.namespace, tt.maxComponents, tt.maxCompLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNamespaceComponents() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateQuery tests query validation
func TestValidateQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		maxLen  int
		wantErr bool
	}{
		{
			name:    "valid query",
			query:   "test query",
			maxLen:  1000,
			wantErr: false,
		},
		{
			name:    "empty query",
			query:   "",
			maxLen:  1000,
			wantErr: true, // ValidateNonEmpty should fail
		},
		{
			name:    "query too long",
			query:   strings.Repeat("a", 1001),
			maxLen:  1000,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuery(tt.query, tt.maxLen)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateQuery() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGetSafeHomePath tests safe home path construction
func TestGetSafeHomePath(t *testing.T) {
	tests := []struct {
		name     string
		subpaths []string
		wantErr  bool
	}{
		{
			name:     "valid subpath",
			subpaths: []string{".engram", "test.md"},
			wantErr:  false,
		},
		{
			name:     "multiple subpaths",
			subpaths: []string{".engram", "subdir", "test.md"},
			wantErr:  false,
		},
		{
			name:     "no subpaths",
			subpaths: []string{},
			wantErr:  false,
		},
		{
			name:     "traversal attempt",
			subpaths: []string{"..", "etc", "passwd"},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := GetSafeHomePath(tt.subpaths...)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSafeHomePath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				home, _ := os.UserHomeDir()
				if !strings.HasPrefix(path, home) {
					t.Errorf("GetSafeHomePath() returned path outside home: %s", path)
				}
			}
		})
	}
}

// TestGetAllowedPaths tests allowed paths retrieval
func TestGetAllowedPaths(t *testing.T) {
	paths, err := GetAllowedPaths()
	if err != nil {
		t.Fatalf("GetAllowedPaths() error = %v", err)
	}

	if len(paths) < 2 {
		t.Errorf("GetAllowedPaths() returned %d paths, want at least 2", len(paths))
	}

	home, _ := os.UserHomeDir()
	for _, path := range paths {
		if !strings.HasPrefix(path, home) {
			t.Errorf("GetAllowedPaths() returned path outside home: %s", path)
		}
	}
}

// TestValidateFileExtension tests file extension validation
func TestValidateFileExtension(t *testing.T) {
	allowedExts := []string{".md", ".json", ".yaml"}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid markdown",
			path:    "test.md",
			wantErr: false,
		},
		{
			name:    "valid json",
			path:    "config.json",
			wantErr: false,
		},
		{
			name:    "valid yaml",
			path:    "config.yaml",
			wantErr: false,
		},
		{
			name:    "invalid extension",
			path:    "script.sh",
			wantErr: true,
		},
		{
			name:    "no extension",
			path:    "README",
			wantErr: true,
		},
		{
			name:    "case insensitive",
			path:    "TEST.MD",
			wantErr: false,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileExtension("path", tt.path, allowedExts)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFileExtension(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

// TestValidateRelativePathSafe tests relative path validation
func TestValidateRelativePathSafe(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	basePath := filepath.Join(home, ".engram")

	tests := []struct {
		name    string
		relPath string
		wantErr bool
	}{
		{
			name:    "valid relative path",
			relPath: "subdir/test.md",
			wantErr: false,
		},
		{
			name:    "traversal attempt",
			relPath: "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "absolute path",
			relPath: "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "empty path",
			relPath: "",
			wantErr: false,
		},
		{
			name:    "current directory",
			relPath: ".",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRelativePathSafe("path", tt.relPath, basePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRelativePathSafe(%q) error = %v, wantErr %v", tt.relPath, err, tt.wantErr)
			}
		})
	}
}
