package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

func TestFlagsValidate(t *testing.T) {
	tests := []struct {
		name      string
		flags     types.Flags
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid flags - no input",
			flags:     types.Flags{},
			wantError: false,
		},
		{
			name:      "Valid flags - input only",
			flags:     types.Flags{Input: "Focus on security"},
			wantError: false,
		},
		{
			name:      "Valid flags - valid type",
			flags:     types.Flags{Type: "video"},
			wantError: false,
		},
		{
			name:      "Invalid - both input and input-file",
			flags:     types.Flags{Input: "prompt", InputFile: "/tmp/file.txt"},
			wantError: true,
			errorMsg:  "cannot specify both --input and --input-file",
		},
		{
			name:      "Invalid - invalid content type",
			flags:     types.Flags{Type: "invalid"},
			wantError: true,
			errorMsg:  "invalid content type",
		},
		{
			name:      "Invalid - non-existent input file",
			flags:     types.Flags{InputFile: "/tmp/nonexistent-file-12345.txt"},
			wantError: true,
			errorMsg:  "input file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(&tt.flags)
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid HTTP URL",
			url:       "http://example.com",
			wantError: false,
		},
		{
			name:      "Valid HTTPS URL",
			url:       "https://example.com/path",
			wantError: false,
		},
		{
			name:      "Valid YouTube URL",
			url:       "https://www.youtube.com/watch?v=VIDEO_ID",
			wantError: false,
		},
		{
			name:      "Valid arxiv URL",
			url:       "https://arxiv.org/abs/2601.20802",
			wantError: false,
		},
		{
			name:      "Invalid - no scheme",
			url:       "example.com",
			wantError: true,
			errorMsg:  "invalid URL",
		},
		{
			name:      "Invalid - unsupported scheme",
			url:       "ftp://example.com",
			wantError: true,
			errorMsg:  "invalid URL scheme",
		},
		{
			name:      "Invalid - empty URL",
			url:       "",
			wantError: true,
			errorMsg:  "invalid URL",
		},
		{
			name:      "Invalid - malformed URL",
			url:       "http://",
			wantError: true,
			errorMsg:  "missing host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestGetPrompt(t *testing.T) {
	// Create temporary directory for test files
	tmpDir := t.TempDir()

	// Create test file with content
	testFile := filepath.Join(tmpDir, "prompt.txt")
	testContent := "This is a test prompt from a file"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name       string
		flags      types.Flags
		wantPrompt string
		wantError  bool
		errorMsg   string
	}{
		{
			name:       "No input - return empty",
			flags:      types.Flags{},
			wantPrompt: "",
			wantError:  false,
		},
		{
			name:       "Input flag provided",
			flags:      types.Flags{Input: "Focus on security"},
			wantPrompt: "Focus on security",
			wantError:  false,
		},
		{
			name:       "Input flag with whitespace - return empty",
			flags:      types.Flags{Input: "   "},
			wantPrompt: "",
			wantError:  false,
		},
		{
			name:       "Input file provided",
			flags:      types.Flags{InputFile: testFile},
			wantPrompt: testContent,
			wantError:  false,
		},
		{
			name:      "Input file not found",
			flags:     types.Flags{InputFile: "/tmp/nonexistent-12345.txt"},
			wantError: true,
			errorMsg:  "failed to read input file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt, err := GetPrompt(&tt.flags)
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
				if prompt != tt.wantPrompt {
					t.Errorf("Expected prompt %q, got %q", tt.wantPrompt, prompt)
				}
			}
		})
	}
}

func TestValidContentTypes(t *testing.T) {
	validTypes := []string{"video", "article", "arxiv", "huggingface"}

	for _, typ := range validTypes {
		t.Run("Valid type: "+typ, func(t *testing.T) {
			flags := types.Flags{Type: typ}
			err := Validate(&flags)
			if err != nil {
				t.Errorf("Expected valid type %q to pass validation, got error: %v", typ, err)
			}
		})
	}
}

func TestInputFileMutualExclusion(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "prompt.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	flags := types.Flags{
		Input:     "short prompt",
		InputFile: testFile,
	}

	err := Validate(&flags)
	if err == nil {
		t.Error("Expected error when both --input and --input-file are specified")
	}
	if !strings.Contains(err.Error(), "cannot specify both") {
		t.Errorf("Expected error about mutual exclusion, got: %v", err)
	}
}
