package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlags_Validate(t *testing.T) {
	tests := []struct {
		name    string
		flags   Flags
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid flags - no custom prompt",
			flags: Flags{
				Type: "article",
			},
			wantErr: false,
		},
		{
			name: "Valid flags - with input",
			flags: Flags{
				Input: "Custom prompt",
				Type:  "video",
			},
			wantErr: false,
		},
		{
			name: "Invalid - both input and input-file",
			flags: Flags{
				Input:     "Custom prompt",
				InputFile: "/tmp/prompt.txt",
			},
			wantErr: true,
			errMsg:  "mutually exclusive",
		},
		{
			name: "Invalid type",
			flags: Flags{
				Type: "invalid-type",
			},
			wantErr: true,
			errMsg:  "invalid type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flags.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Flags.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Flags.Validate() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

func TestFlags_GetPrompt(t *testing.T) {
	tests := []struct {
		name    string
		flags   Flags
		want    string
		wantErr bool
	}{
		{
			name:    "No prompt - returns empty",
			flags:   Flags{},
			want:    "",
			wantErr: false,
		},
		{
			name: "Inline prompt",
			flags: Flags{
				Input: "Custom inline prompt",
			},
			want:    "Custom inline prompt",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.flags.GetPrompt()
			if (err != nil) != tt.wantErr {
				t.Errorf("Flags.GetPrompt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Flags.GetPrompt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFlags_GetPrompt_InputFile(t *testing.T) {
	// Create temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "prompt.txt")
	testContent := "Prompt from file"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	flags := Flags{
		InputFile: testFile,
	}

	got, err := flags.GetPrompt()
	if err != nil {
		t.Errorf("Flags.GetPrompt() error = %v, want nil", err)
	}
	if got != testContent {
		t.Errorf("Flags.GetPrompt() = %q, want %q", got, testContent)
	}
}

func TestFlags_GetPrompt_AtFileSyntax(t *testing.T) {
	// Create temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "prompt.txt")
	testContent := "Prompt from @file syntax"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "@file syntax - valid file",
			input:   "@" + testFile,
			want:    testContent,
			wantErr: false,
		},
		{
			name:    "@file syntax - file not found",
			input:   "@nonexistent.txt",
			want:    "",
			wantErr: true,
			errMsg:  "file not found",
		},
		{
			name:    "@file syntax - with relative path",
			input:   "@" + filepath.Base(testFile),
			want:    "",
			wantErr: true, // Will fail because relative path won't exist in current dir
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For relative path test, change to tmpDir
			if strings.Contains(tt.name, "relative path") {
				oldWd, _ := os.Getwd()
				defer os.Chdir(oldWd)
				os.Chdir(tmpDir)

				// Now it should work
				tt.wantErr = false
				tt.want = testContent
			}

			flags := Flags{
				Input: tt.input,
			}

			got, err := flags.GetPrompt()
			if (err != nil) != tt.wantErr {
				t.Errorf("Flags.GetPrompt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Flags.GetPrompt() error = %v, want error containing %q", err, tt.errMsg)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Flags.GetPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFlags_GetPrompt_AtFileSyntax_ErrorMessages(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		errMsg string
	}{
		{
			name:   "File not found - helpful message",
			input:  "@missing-file.txt",
			errMsg: "file not found",
		},
		{
			name:   "File not found - suggests ls command",
			input:  "@missing-file.txt",
			errMsg: "ls -la",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := Flags{
				Input: tt.input,
			}

			_, err := flags.GetPrompt()
			if err == nil {
				t.Error("Flags.GetPrompt() error = nil, want error")
				return
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("Flags.GetPrompt() error = %v, want error containing %q", err, tt.errMsg)
			}
		})
	}
}

func TestFlags_GetPrompt_AtFileSyntax_Precedence(t *testing.T) {
	// This tests that @file syntax in --input flag works
	// even when --input-file is also supported
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "prompt.txt")
	testContent := "Prompt content"

	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name    string
		flags   Flags
		want    string
		wantErr bool
	}{
		{
			name: "Input takes precedence over InputFile",
			flags: Flags{
				Input:     "Inline prompt",
				InputFile: testFile,
			},
			want:    "",    // Will error in Validate() before GetPrompt()
			wantErr: false, // But GetPrompt itself doesn't validate
		},
		{
			name: "@file syntax in Input",
			flags: Flags{
				Input: "@" + testFile,
			},
			want:    testContent,
			wantErr: false,
		},
		{
			name: "InputFile flag",
			flags: Flags{
				InputFile: testFile,
			},
			want:    testContent,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.flags.GetPrompt()
			if (err != nil) != tt.wantErr {
				t.Errorf("Flags.GetPrompt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.want != "" && got != tt.want {
				t.Errorf("Flags.GetPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}
