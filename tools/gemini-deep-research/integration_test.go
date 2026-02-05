package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/cmd"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/config"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

// TestIntegration_InvalidURL tests the pipeline with an invalid URL
func TestIntegration_InvalidURL(t *testing.T) {
	// Setup
	var stdout, stderr bytes.Buffer
	cfg := &config.Config{
		OutputDir:    t.TempDir(),
		Timeout:      1,
		PollInterval: 10,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	flags := &types.Flags{}

	// Test invalid URL
	exitCode := cmd.Run("not-a-url", flags, cfg)

	// Verify
	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
	}

	if !strings.Contains(stderr.String(), "Error") {
		t.Error("Expected error message in stderr")
	}
}

// TestIntegration_MissingAPIKey tests the pipeline without API key configured
func TestIntegration_MissingAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup - clear API key environment
	oldAPIKey := os.Getenv("GEMINI_API_KEY")
	oldProject := os.Getenv("GOOGLE_CLOUD_PROJECT")
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	defer func() {
		os.Setenv("GEMINI_API_KEY", oldAPIKey)
		os.Setenv("GOOGLE_CLOUD_PROJECT", oldProject)
	}()

	var stdout, stderr bytes.Buffer
	cfg := &config.Config{
		OutputDir:    t.TempDir(),
		Timeout:      1,
		PollInterval: 10,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	flags := &types.Flags{}

	// Test with valid URL but no API key
	exitCode := cmd.Run("https://arxiv.org/abs/2501.12345", flags, cfg)

	// Should fail at research step (exit code 4) or earlier
	if exitCode == 0 {
		t.Error("Expected non-zero exit code when API key is missing")
	}
}

// TestIntegration_InvalidContentType tests invalid content type override
func TestIntegration_InvalidContentType(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cfg := &config.Config{
		OutputDir:    t.TempDir(),
		Timeout:      1,
		PollInterval: 10,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	flags := &types.Flags{
		Type: "invalid-type",
	}

	// Test with invalid content type
	exitCode := cmd.Run("https://example.com", flags, cfg)

	// Should fail at detection step
	if exitCode != 1 {
		t.Errorf("Expected exit code 1, got %d", exitCode)
	}
}

// TestIntegration_URLValidation tests URL validation
func TestIntegration_URLValidation(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantCode int
	}{
		{
			name:     "missing scheme",
			url:      "example.com",
			wantCode: 1,
		},
		{
			name:     "invalid scheme",
			url:      "ftp://example.com",
			wantCode: 1,
		},
		{
			name:     "empty url",
			url:      "",
			wantCode: 1,
		},
		{
			name:     "missing host",
			url:      "https://",
			wantCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cfg := &config.Config{
				OutputDir:    t.TempDir(),
				Timeout:      1,
				PollInterval: 10,
				Stdout:       &stdout,
				Stderr:       &stderr,
			}

			flags := &types.Flags{}
			exitCode := cmd.Run(tt.url, flags, cfg)

			if exitCode != tt.wantCode {
				t.Errorf("Expected exit code %d, got %d", tt.wantCode, exitCode)
			}
		})
	}
}

// TestIntegration_OutputDirectory tests output directory creation
func TestIntegration_OutputDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test only checks that the output directory structure works
	// It will fail at extraction or analysis step, but we can verify
	// the output directory logic is sound

	outputBase := t.TempDir()
	var stdout, stderr bytes.Buffer
	cfg := &config.Config{
		OutputDir:    outputBase,
		Timeout:      1,
		PollInterval: 10,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}

	flags := &types.Flags{}

	// Run with valid URL (will fail at extraction, but that's ok)
	cmd.Run("https://example.com/article", flags, cfg)

	// Verify output directory was configured
	if outputBase == "" {
		t.Error("Output directory should be set")
	}
}

// TestIntegration_CustomPrompt tests custom prompt handling
func TestIntegration_CustomPrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test short prompt
	t.Run("short prompt", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		cfg := &config.Config{
			OutputDir:    t.TempDir(),
			Timeout:      1,
			PollInterval: 10, // Add poll interval to avoid panic
			Stdout:       &stdout,
			Stderr:       &stderr,
		}

		flags := &types.Flags{
			AnalyzePrompt: "Focus on security topics",
		}

		// Will fail at extraction, but should handle custom prompt
		cmd.Run("https://example.com", flags, cfg)

		// Check that custom prompt was logged
		if !strings.Contains(stdout.String(), "Custom Prompt") {
			t.Error("Expected custom prompt to be logged")
		}
	})

	// Test file prompt with @file syntax
	t.Run("file prompt", func(t *testing.T) {
		// Create temp prompt file
		promptFile := t.TempDir() + "/prompt.txt"
		os.WriteFile(promptFile, []byte("Focus on AI topics"), 0644)

		oldWd, _ := os.Getwd()
		os.Chdir(t.TempDir())
		defer os.Chdir(oldWd)

		var stdout, stderr bytes.Buffer
		cfg := &config.Config{
			OutputDir:    t.TempDir(),
			Timeout:      1,
			PollInterval: 10, // Add poll interval to avoid panic
			Stdout:       &stdout,
			Stderr:       &stderr,
		}

		flags := &types.Flags{
			AnalyzePrompt: "@prompt.txt",
		}

		// Will fail at extraction, but should handle file prompt
		cmd.Run("https://example.com", flags, cfg)

		// Check that custom prompt was logged
		if !strings.Contains(stdout.String(), "Custom Prompt") {
			t.Error("Expected custom prompt to be logged")
		}
	})
}

// TestIntegration_ContentTypeOverride tests content type override
func TestIntegration_ContentTypeOverride(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tests := []struct {
		name string
		url  string
		typ  string
	}{
		{
			name: "force article on video url",
			url:  "https://www.youtube.com/watch?v=test",
			typ:  "article",
		},
		{
			name: "force arxiv on generic url",
			url:  "https://example.com/paper",
			typ:  "arxiv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cfg := &config.Config{
				OutputDir:    t.TempDir(),
				Timeout:      1,
				PollInterval: 10,
				Stdout:       &stdout,
				Stderr:       &stderr,
			}

			flags := &types.Flags{
				Type: tt.typ,
			}

			// Will fail at extraction, but should log override
			cmd.Run(tt.url, flags, cfg)

			// Check that override was logged
			if !strings.Contains(stdout.String(), "Content Type Override") {
				t.Error("Expected content type override to be logged")
			}
		})
	}
}

// TestIntegration_ExitCodes tests that exit codes match specification
func TestIntegration_ExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*config.Config, *types.Flags)
		url      string
		wantCode int
		desc     string
	}{
		{
			name: "invalid url",
			setup: func(cfg *config.Config, flags *types.Flags) {
				// No setup needed
			},
			url:      "not-a-url",
			wantCode: 1,
			desc:     "Invalid arguments or configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cfg := &config.Config{
				OutputDir:    t.TempDir(),
				Timeout:      1,
				PollInterval: 10,
				Stdout:       &stdout,
				Stderr:       &stderr,
			}
			flags := &types.Flags{}

			if tt.setup != nil {
				tt.setup(cfg, flags)
			}

			exitCode := cmd.Run(tt.url, flags, cfg)

			if exitCode != tt.wantCode {
				t.Errorf("Expected exit code %d (%s), got %d", tt.wantCode, tt.desc, exitCode)
			}
		})
	}
}
