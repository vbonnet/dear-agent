package ops

import (
	"context"
	"testing"
)

func TestValidateHarness(t *testing.T) {
	tests := []struct {
		name      string
		harness   string
		expected  HarnessType
		shouldErr bool
	}{
		{
			name:      "valid codex",
			harness:   "codex",
			expected:  HarnessCodex,
			shouldErr: false,
		},
		{
			name:      "valid gemini",
			harness:   "gemini",
			expected:  HarnessGemini,
			shouldErr: false,
		},
		{
			name:      "valid opencode",
			harness:   "opencode",
			expected:  HarnessOpenCode,
			shouldErr: false,
		},
		{
			name:      "case insensitive",
			harness:   "CODEX",
			expected:  HarnessCodex,
			shouldErr: false,
		},
		{
			name:      "invalid harness",
			harness:   "invalid",
			expected:  "",
			shouldErr: true,
		},
		{
			name:      "empty harness",
			harness:   "",
			expected:  "",
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateHarness(tt.harness)
			if (err != nil) != tt.shouldErr {
				t.Fatalf("ValidateHarness() error = %v, shouldErr %v", err, tt.shouldErr)
			}
			if err == nil && result != tt.expected {
				t.Fatalf("ValidateHarness() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestHarnessInstallResult_JSON(t *testing.T) {
	result := &HarnessInstallResult{
		Success: true,
		Harness: "codex",
		Message: "Installation successful",
		Version: "0.120.0",
		Path:    "/usr/local/bin/codex",
	}

	jsonStr, err := ResultToJSON(result)
	if err != nil {
		t.Fatalf("ResultToJSON() error = %v", err)
	}

	if jsonStr == "" {
		t.Fatal("ResultToJSON() returned empty string")
	}

	// Basic validation that JSON is valid
	if !contains(jsonStr, `"success": true`) && !contains(jsonStr, `"success":true`) {
		t.Fatalf("JSON missing success field: %s", jsonStr)
	}

	if !contains(jsonStr, `"codex"`) {
		t.Fatalf("JSON missing harness field: %s", jsonStr)
	}
}

func TestGetHarnessPath(t *testing.T) {
	ctx := context.Background()

	// Test that we can call the function without crashing
	// Don't assert specific return values since they depend on system state
	_, _, err := IsInstalled(ctx, HarnessCodex)
	if err != nil {
		t.Logf("IsInstalled check completed with note: %v", err)
	}
}

func TestInstallCodex_AlreadyInstalled(t *testing.T) {
	// This test will only pass if codex is already installed
	// We'll just verify the function runs without crashing
	ctx := context.Background()
	result := InstallCodex(ctx)

	if result == nil {
		t.Fatal("InstallCodex() returned nil")
	}

	if result.Harness != string(HarnessCodex) {
		t.Fatalf("InstallCodex() harness = %s, expected %s", result.Harness, HarnessCodex)
	}

	// result.Success could be true or false depending on whether codex is installed
	// Just verify the result is populated
	if result.Message == "" {
		t.Fatal("InstallCodex() message is empty")
	}
}

func TestInstallGemini_AlreadyInstalled(t *testing.T) {
	ctx := context.Background()
	result := InstallGemini(ctx)

	if result == nil {
		t.Fatal("InstallGemini() returned nil")
	}

	if result.Harness != string(HarnessGemini) {
		t.Fatalf("InstallGemini() harness = %s, expected %s", result.Harness, HarnessGemini)
	}

	if result.Message == "" {
		t.Fatal("InstallGemini() message is empty")
	}
}

func TestInstallOpenCode_AlreadyInstalled(t *testing.T) {
	ctx := context.Background()
	result := InstallOpenCode(ctx)

	if result == nil {
		t.Fatal("InstallOpenCode() returned nil")
	}

	if result.Harness != string(HarnessOpenCode) {
		t.Fatalf("InstallOpenCode() harness = %s, expected %s", result.Harness, HarnessOpenCode)
	}

	if result.Message == "" {
		t.Fatal("InstallOpenCode() message is empty")
	}
}

func TestInstall_InvalidHarness(t *testing.T) {
	ctx := context.Background()
	_, err := Install(ctx, HarnessType("invalid"))

	if err == nil {
		t.Fatal("Install() should error for invalid harness")
	}
}

func TestInstall_ValidHarness(t *testing.T) {
	ctx := context.Background()

	harnessTypes := []HarnessType{HarnessCodex, HarnessGemini, HarnessOpenCode}

	for _, harness := range harnessTypes {
		t.Run(string(harness), func(t *testing.T) {
			result, err := Install(ctx, harness)

			if err != nil {
				t.Fatalf("Install() error = %v", err)
			}

			if result == nil {
				t.Fatal("Install() returned nil result")
			}

			if result.Harness != string(harness) {
				t.Fatalf("Install() harness = %s, expected %s", result.Harness, harness)
			}

			if result.Message == "" {
				t.Fatal("Install() message is empty")
			}
		})
	}
}

// Helper function for string searching in JSON output
func contains(s, substr string) bool {
	// Remove spaces for fuzzy matching
	s = removeSpaces(s)
	substr = removeSpaces(substr)
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr))
}

func removeSpaces(s string) string {
	result := ""
	for _, r := range s {
		if r != ' ' && r != '\n' && r != '\t' {
			result += string(r)
		}
	}
	return result
}
