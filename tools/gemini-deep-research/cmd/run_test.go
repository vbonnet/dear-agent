package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/config"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

func TestRun(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cmd.Run tests in short mode (requires external dependencies)")
	}

	tests := []struct {
		name         string
		url          string
		flags        *types.Flags
		cfg          *config.Config
		wantExitCode int
		checkOutput  func(t *testing.T, stdout, stderr string)
	}{
		{
			name:  "Valid URL - will fail at extraction",
			url:   "https://www.youtube.com/watch?v=VIDEO_ID",
			flags: &types.Flags{},
			cfg: &config.Config{
				OutputDir:    "./output",
				Timeout:      60,
				PollInterval: 10,
			},
			wantExitCode: 2, // Extraction will fail (no real video)
			checkOutput: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, "Configuration:") {
					t.Error("Expected configuration output")
				}
				if !strings.Contains(stdout, "Step 1: Detecting content type") {
					t.Error("Expected pipeline execution")
				}
			},
		},
		{
			name:  "Invalid URL - missing scheme",
			url:   "example.com",
			flags: &types.Flags{},
			cfg: &config.Config{
				OutputDir: "./output",
				Timeout:   60,
			},
			wantExitCode: 1,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stderr, "Error:") {
					t.Error("Expected error message in stderr")
				}
				if !strings.Contains(stderr, "Examples of valid URLs:") {
					t.Error("Expected URL examples in error output")
				}
			},
		},
		{
			name:  "Invalid URL - wrong scheme",
			url:   "ftp://example.com",
			flags: &types.Flags{},
			cfg: &config.Config{
				OutputDir: "./output",
				Timeout:   60,
			},
			wantExitCode: 1,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stderr, "invalid URL scheme") {
					t.Error("Expected invalid scheme error")
				}
			},
		},
		{
			name: "With custom prompt",
			url:  "https://example.com/article",
			flags: &types.Flags{
				Input: "Focus on security topics and vulnerabilities",
			},
			cfg: &config.Config{
				OutputDir:    "./output",
				Timeout:      60,
				PollInterval: 10,
			},
			wantExitCode: 2, // Will fail at extraction
			checkOutput: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, "Custom Prompt:") {
					t.Error("Expected custom prompt in output")
				}
			},
		},
		{
			name: "With type override",
			url:  "https://www.youtube.com/watch?v=VIDEO_ID",
			flags: &types.Flags{
				Type: "article",
			},
			cfg: &config.Config{
				OutputDir:    "./output",
				Timeout:      60,
				PollInterval: 10,
			},
			wantExitCode: 2, // Will fail at extraction
			checkOutput: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, "Content Type Override: article") {
					t.Error("Expected content type override in output")
				}
			},
		},
		{
			name:  "With custom output directory",
			url:   "https://arxiv.org/abs/2601.20802",
			flags: &types.Flags{},
			cfg: &config.Config{
				OutputDir:    "/tmp/research",
				Timeout:      120,
				PollInterval: 10,
			},
			wantExitCode: 3, // Will fail at analysis (no gemini CLI in test env probably)
			checkOutput: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, "Output Directory: /tmp/research") {
					t.Error("Expected custom output directory in output")
				}
				if !strings.Contains(stdout, "Timeout: 120 minutes") {
					t.Error("Expected custom timeout in output")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout and stderr
			var stdout, stderr bytes.Buffer
			tt.cfg.Stdout = &stdout
			tt.cfg.Stderr = &stderr

			// Validate config
			if err := tt.cfg.Validate(); err != nil {
				t.Fatalf("Config validation failed: %v", err)
			}

			// Run command
			exitCode := Run(tt.url, tt.flags, tt.cfg)

			// Check exit code
			if exitCode != tt.wantExitCode {
				t.Errorf("Expected exit code %d, got %d", tt.wantExitCode, exitCode)
			}

			// Check output
			if tt.checkOutput != nil {
				tt.checkOutput(t, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunURLValidation(t *testing.T) {
	cfg := &config.Config{
		OutputDir: "./output",
		Timeout:   60,
	}

	var stderr bytes.Buffer
	cfg.Stderr = &stderr
	cfg.Stdout = &bytes.Buffer{}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Config validation failed: %v", err)
	}

	// Test invalid URL
	exitCode := Run("not-a-url", &types.Flags{}, cfg)
	if exitCode != 1 {
		t.Errorf("Expected exit code 1 for invalid URL, got %d", exitCode)
	}

	// Check that error output includes examples
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "Examples of valid URLs:") {
		t.Error("Expected URL examples in error output")
	}
	if !strings.Contains(stderrStr, "youtube.com") {
		t.Error("Expected YouTube example in error output")
	}
	if !strings.Contains(stderrStr, "arxiv.org") {
		t.Error("Expected arxiv example in error output")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "Short string - no truncation",
			input:  "Hello",
			maxLen: 10,
			want:   "Hello",
		},
		{
			name:   "Exact length - no truncation",
			input:  "Hello",
			maxLen: 5,
			want:   "Hello",
		},
		{
			name:   "Long string - truncate with ellipsis",
			input:  "This is a very long string that needs truncation",
			maxLen: 20,
			want:   "This is a very lo...",
		},
		{
			name:   "Empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
			if len(got) > tt.maxLen {
				t.Errorf("truncate result length %d exceeds maxLen %d", len(got), tt.maxLen)
			}
		})
	}
}
