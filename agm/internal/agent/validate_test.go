package agent

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestIsVertexAIConfigured(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name: "CLOUD_ML_REGION set",
			envVars: map[string]string{
				"CLOUD_ML_REGION": "us-east5",
			},
			expected: true,
		},
		{
			name: "GOOGLE_CLOUD_PROJECT set",
			envVars: map[string]string{
				"GOOGLE_CLOUD_PROJECT": "my-project",
			},
			expected: true,
		},
		{
			name: "GCP_PROJECT set",
			envVars: map[string]string{
				"GCP_PROJECT": "my-project",
			},
			expected: true,
		},
		{
			name: "GOOGLE_APPLICATION_CREDENTIALS set",
			envVars: map[string]string{
				"GOOGLE_APPLICATION_CREDENTIALS": "/path/to/creds.json",
			},
			expected: true,
		},
		{
			name: "ANTHROPIC_VERTEX_PROJECT_ID set",
			envVars: map[string]string{
				"ANTHROPIC_VERTEX_PROJECT_ID": "my-vertex-project",
			},
			expected: true,
		},
		{
			name: "CLAUDE_CODE_USE_VERTEX set",
			envVars: map[string]string{
				"CLAUDE_CODE_USE_VERTEX": "1",
			},
			expected: true,
		},
		{
			name: "Multiple vars set",
			envVars: map[string]string{
				"CLOUD_ML_REGION":      "us-east5",
				"GOOGLE_CLOUD_PROJECT": "my-project",
			},
			expected: true,
		},
		{
			name:     "No vars set",
			envVars:  map[string]string{},
			expected: false,
		},
		{
			name: "Empty string values",
			envVars: map[string]string{
				"CLOUD_ML_REGION":      "",
				"GOOGLE_CLOUD_PROJECT": "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all Vertex AI related env vars first
			clearVertexEnvVars := []string{
				"CLOUD_ML_REGION",
				"GOOGLE_CLOUD_PROJECT",
				"GCP_PROJECT",
				"GOOGLE_APPLICATION_CREDENTIALS",
				"ANTHROPIC_VERTEX_PROJECT_ID",
				"CLAUDE_CODE_USE_VERTEX",
			}

			originalValues := make(map[string]string)
			for _, key := range clearVertexEnvVars {
				originalValues[key] = os.Getenv(key)
				os.Unsetenv(key)
			}

			// Restore original values after test
			defer func() {
				for key, val := range originalValues {
					if val != "" {
						t.Setenv(key, val)
					} else {
						os.Unsetenv(key)
					}
				}
			}()

			// Set test env vars
			for key, val := range tt.envVars {
				t.Setenv(key, val)
			}

			result := isVertexAIConfigured()
			if result != tt.expected {
				t.Errorf("isVertexAIConfigured() = %v, expected %v (env vars: %v)",
					result, tt.expected, tt.envVars)
			}
		})
	}
}

// mockLookPath replaces lookPath for testing and restores it on cleanup
func mockLookPath(t *testing.T, found map[string]bool) {
	t.Helper()
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(file string) (string, error) {
		if found[file] {
			return "/usr/bin/" + file, nil
		}
		return "", fmt.Errorf("not found: %s", file)
	}
}

// clearClaudeEnv unsets all Claude auth env vars and restores them on cleanup
func clearClaudeEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"ANTHROPIC_API_KEY", "CLOUD_ML_REGION", "GOOGLE_CLOUD_PROJECT",
		"GCP_PROJECT", "GOOGLE_APPLICATION_CREDENTIALS", "CLAUDECODE",
		"ANTHROPIC_VERTEX_PROJECT_ID", "CLAUDE_CODE_USE_VERTEX",
	}
	saved := make(map[string]string, len(keys))
	for _, k := range keys {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			if v != "" {
				t.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	})
}

func TestValidateHarnessAvailability_VertexAI(t *testing.T) {
	clearClaudeEnv(t)
	mockLookPath(t, map[string]bool{}) // no binaries on PATH

	t.Run("Claude with Vertex AI configured", func(t *testing.T) {
		clearClaudeEnv(t)
		mockLookPath(t, map[string]bool{})
		t.Setenv("CLOUD_ML_REGION", "us-east5")

		err := ValidateHarnessAvailability("claude-code")
		if err != nil {
			t.Errorf("Expected no error with Vertex AI configured, got: %v", err)
		}
	})

	t.Run("Claude with ANTHROPIC_VERTEX_PROJECT_ID", func(t *testing.T) {
		clearClaudeEnv(t)
		mockLookPath(t, map[string]bool{})
		t.Setenv("ANTHROPIC_VERTEX_PROJECT_ID", "my-project")

		err := ValidateHarnessAvailability("claude-code")
		if err != nil {
			t.Errorf("Expected no error with ANTHROPIC_VERTEX_PROJECT_ID, got: %v", err)
		}
	})

	t.Run("Claude with CLAUDE_CODE_USE_VERTEX", func(t *testing.T) {
		clearClaudeEnv(t)
		mockLookPath(t, map[string]bool{})
		t.Setenv("CLAUDE_CODE_USE_VERTEX", "1")

		err := ValidateHarnessAvailability("claude-code")
		if err != nil {
			t.Errorf("Expected no error with CLAUDE_CODE_USE_VERTEX, got: %v", err)
		}
	})

	t.Run("Claude with ANTHROPIC_API_KEY", func(t *testing.T) {
		clearClaudeEnv(t)
		mockLookPath(t, map[string]bool{})
		t.Setenv("ANTHROPIC_API_KEY", "test-key")

		err := ValidateHarnessAvailability("claude-code")
		if err != nil {
			t.Errorf("Expected no error with API key, got: %v", err)
		}
	})

	t.Run("Claude with Claude Code CLI", func(t *testing.T) {
		clearClaudeEnv(t)
		mockLookPath(t, map[string]bool{})
		t.Setenv("CLAUDECODE", "1")

		err := ValidateHarnessAvailability("claude-code")
		if err != nil {
			t.Errorf("Expected no error with Claude Code CLI, got: %v", err)
		}
	})

	t.Run("Claude without any auth or binary", func(t *testing.T) {
		clearClaudeEnv(t)
		mockLookPath(t, map[string]bool{})

		err := ValidateHarnessAvailability("claude-code")
		if err == nil {
			t.Error("Expected error without API key, Vertex AI, or binary, got nil")
		}
		var hErr *HarnessUnavailableError
		if !errors.As(err, &hErr) {
			t.Errorf("Expected HarnessUnavailableError, got: %T", err)
		}
	})
}

func TestValidateHarnessAvailability_BinaryOnPath(t *testing.T) {
	t.Run("Claude available when binary on PATH", func(t *testing.T) {
		clearClaudeEnv(t)
		mockLookPath(t, map[string]bool{"claude": true})

		err := ValidateHarnessAvailability("claude-code")
		if err != nil {
			t.Errorf("Expected no error with claude on PATH, got: %v", err)
		}
	})

	t.Run("Gemini available when binary on PATH", func(t *testing.T) {
		clearClaudeEnv(t)
		mockLookPath(t, map[string]bool{"gemini": true})
		os.Unsetenv("GEMINI_API_KEY")

		err := ValidateHarnessAvailability("gemini-cli")
		if err != nil {
			t.Errorf("Expected no error with gemini on PATH, got: %v", err)
		}
	})

	t.Run("Codex available when binary on PATH", func(t *testing.T) {
		clearClaudeEnv(t)
		mockLookPath(t, map[string]bool{"codex": true})
		t.Setenv("OPENAI_API_KEY", "") // restored on test cleanup
		os.Unsetenv("OPENAI_API_KEY")

		err := ValidateHarnessAvailability("codex-cli")
		if err != nil {
			t.Errorf("Expected no error with codex on PATH, got: %v", err)
		}
	})

	t.Run("Gemini unavailable without binary or key", func(t *testing.T) {
		mockLookPath(t, map[string]bool{})
		origKey := os.Getenv("GEMINI_API_KEY")
		os.Unsetenv("GEMINI_API_KEY")
		t.Cleanup(func() {
			if origKey != "" {
				t.Setenv("GEMINI_API_KEY", origKey)
			}
		})

		err := ValidateHarnessAvailability("gemini-cli")
		if err == nil {
			t.Error("Expected error for gemini-cli without binary or key")
		}
	})
}

func TestIsHarnessBinaryOnPath(t *testing.T) {
	t.Run("returns true when binary found", func(t *testing.T) {
		mockLookPath(t, map[string]bool{"claude": true})
		if !isHarnessBinaryOnPath("claude-code") {
			t.Error("Expected true when claude binary is on PATH")
		}
	})

	t.Run("returns false when binary not found", func(t *testing.T) {
		mockLookPath(t, map[string]bool{})
		if isHarnessBinaryOnPath("claude-code") {
			t.Error("Expected false when claude binary is not on PATH")
		}
	})

	t.Run("returns false for unknown harness", func(t *testing.T) {
		mockLookPath(t, map[string]bool{"anything": true})
		if isHarnessBinaryOnPath("unknown-harness") {
			t.Error("Expected false for unknown harness")
		}
	})
}
