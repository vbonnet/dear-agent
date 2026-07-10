package security

import (
	"os"
	"strings"
	"testing"
)

// TestNewAPIKeyManager verifies API key manager initialization
func TestNewAPIKeyManager(t *testing.T) {
	manager := NewAPIKeyManager()
	if manager == nil {
		t.Fatal("NewAPIKeyManager() returned nil")
	}
}

// TestGetAnthropicKey_Success verifies key retrieval from environment
func TestGetAnthropicKey_Success(t *testing.T) {
	// Save and restore env
	oldVal := os.Getenv("ANTHROPIC_API_KEY")
	defer func() {
		if oldVal != "" {
			t.Setenv("ANTHROPIC_API_KEY", oldVal)
		} else {
			os.Unsetenv("ANTHROPIC_API_KEY")
		}
	}()

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key-123")

	manager := NewAPIKeyManager()
	key, err := manager.GetAnthropicKey()
	if err != nil {
		t.Fatalf("GetAnthropicKey() failed: %v", err)
	}

	if key != "sk-ant-test-key-123" {
		t.Errorf("GetAnthropicKey() = %q, want %q", key, "sk-ant-test-key-123")
	}
}

// TestGetAnthropicKey_NotSet verifies error when key is not set
func TestGetAnthropicKey_NotSet(t *testing.T) {
	oldVal := os.Getenv("ANTHROPIC_API_KEY")
	defer func() {
		if oldVal != "" {
			t.Setenv("ANTHROPIC_API_KEY", oldVal)
		}
	}()

	os.Unsetenv("ANTHROPIC_API_KEY")

	manager := NewAPIKeyManager()
	_, err := manager.GetAnthropicKey()
	if err == nil {
		t.Fatal("GetAnthropicKey() succeeded with unset key, want error")
	}
}

func TestGetProviderKey_ModelFamilyProviders(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-proj-openai-test")
	t.Setenv("GEMINI_API_KEY", "AIzaGeminiTestKey123456789012345")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-openrouter-test")

	tests := []struct {
		provider string
		want     string
	}{
		{provider: "openai", want: "sk-proj-openai-test"},
		{provider: "gemini", want: "AIzaGeminiTestKey123456789012345"},
		{provider: "google", want: "AIzaGeminiTestKey123456789012345"},
		{provider: "openrouter", want: "sk-or-openrouter-test"},
	}

	manager := NewAPIKeyManager()
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got, err := manager.GetProviderKey(tt.provider)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("GetProviderKey(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestGetProviderKey_GoogleFallbackAndUnsupportedProvider(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "AIzaGoogleFallback123456789012345")

	manager := NewAPIKeyManager()
	if got, err := manager.GetProviderKey("gemini"); err != nil || got != "AIzaGoogleFallback123456789012345" {
		t.Fatalf("GetProviderKey(gemini) = %q, %v", got, err)
	}
	if _, err := manager.GetProviderKey("unknown"); err == nil {
		t.Fatal("GetProviderKey(unknown) error = nil")
	}
}

// TestValidateConfigFile_NoKeys verifies valid config passes
func TestValidateConfigFile_NoKeys(t *testing.T) {
	manager := NewAPIKeyManager()

	validConfigs := []string{
		"platform:\n  agent: claude-code\n  token_budget: 50000",
		"plugins:\n  paths:\n    - ./plugins",
		"# This is a comment about api keys but not actual keys",
	}

	for _, config := range validConfigs {
		err := manager.ValidateConfigFile(config)
		if err != nil {
			t.Errorf("ValidateConfigFile() failed for valid config: %v\nConfig: %s", err, config)
		}
	}
}

// TestValidateConfigFile_ContainsKeys verifies configs with keys are rejected
func TestValidateConfigFile_ContainsKeys(t *testing.T) {
	manager := NewAPIKeyManager()

	invalidConfigs := []string{
		"api_key: sk-ant-123456",
		"apiKey: my-secret-key",
		"ANTHROPIC_API_KEY: sk-ant-123",
		"key: sk-ant-api-key-here",
		"OPENAI_API_KEY: sk-proj-123",
		"GEMINI_API_KEY: AIza123",
		"OPENROUTER_API_KEY: sk-or-123",
	}

	for _, config := range invalidConfigs {
		err := manager.ValidateConfigFile(config)
		if err == nil {
			t.Errorf("ValidateConfigFile() succeeded with key in config, want error\nConfig: %s", config)
		}
	}
}

// TestSanitizeForLogs_AnthropicKey verifies API keys are redacted
func TestSanitizeForLogs_AnthropicKey(t *testing.T) {
	manager := NewAPIKeyManager()

	tests := []struct {
		input string
		want  string
	}{
		{
			input: "sk-ant-api-key-123456",
			want:  "sk-ant-***REDACTED***",
		},
		{
			input: "Using key: sk-ant-secret-key",
			want:  "Using key: sk-ant-***REDACTED***",
		},
		{
			input: "No sensitive data here",
			want:  "No sensitive data here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := manager.SanitizeForLogs(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeForLogs(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeForLogs_ModelFamilyProviderKeys(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-proj-openai-secret")
	t.Setenv("GEMINI_API_KEY", "AIzaGeminiSecret123456789012345")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-openrouter-secret")

	input := "openai=sk-proj-openai-secret gemini=AIzaGeminiSecret123456789012345 openrouter=sk-or-openrouter-secret"
	got := NewAPIKeyManager().SanitizeForLogs(input)
	for _, secret := range []string{"sk-proj-openai-secret", "AIzaGeminiSecret123456789012345", "sk-or-openrouter-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("SanitizeForLogs retained %q in %q", secret, got)
		}
	}
}

func TestSanitizeForLogs_UnsetNamedEnvironmentDoesNotCorruptMessage(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	input := "ANTHROPIC_API_KEY is not configured"
	if got := NewAPIKeyManager().SanitizeForLogs(input); got != input {
		t.Fatalf("SanitizeForLogs(%q) = %q", input, got)
	}
}

// TestSanitizeForLogs_EnvironmentVariable verifies env var values are redacted
func TestSanitizeForLogs_EnvironmentVariable(t *testing.T) {
	oldVal := os.Getenv("ANTHROPIC_API_KEY")
	defer func() {
		if oldVal != "" {
			t.Setenv("ANTHROPIC_API_KEY", oldVal)
		} else {
			os.Unsetenv("ANTHROPIC_API_KEY")
		}
	}()

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-env-key-789")

	manager := NewAPIKeyManager()

	input := "Error: ANTHROPIC_API_KEY = sk-ant-env-key-789"
	got := manager.SanitizeForLogs(input)

	if strings.Contains(got, "sk-ant-env-key-789") {
		t.Errorf("SanitizeForLogs() did not redact environment variable value\nGot: %s", got)
	}

	if !strings.Contains(got, "***REDACTED***") {
		t.Errorf("SanitizeForLogs() did not include redaction marker\nGot: %s", got)
	}
}

// TestSanitizeForLogs_NoKey verifies non-sensitive strings pass through
func TestSanitizeForLogs_NoKey(t *testing.T) {
	manager := NewAPIKeyManager()

	input := "Normal log message with no sensitive data"
	got := manager.SanitizeForLogs(input)

	if got != input {
		t.Errorf("SanitizeForLogs() modified non-sensitive string\nInput: %s\nGot: %s", input, got)
	}
}

// TestRotateKey verifies key rotation instructions
func TestRotateKey(t *testing.T) {
	manager := NewAPIKeyManager()

	instructions := manager.RotateKey()

	if instructions == "" {
		t.Fatal("RotateKey() returned empty string")
	}

	// Verify key content points (case-insensitive for "restart")
	expectedContents := map[string]bool{
		"console.anthropic.com": false,
		"ANTHROPIC_API_KEY":     false,
		"export":                false,
	}

	instructionsLower := strings.ToLower(instructions)
	for expected := range expectedContents {
		if strings.Contains(instructions, expected) || strings.Contains(instructionsLower, strings.ToLower(expected)) {
			expectedContents[expected] = true
		}
	}

	// Verify restart is mentioned (case insensitive)
	if !strings.Contains(instructionsLower, "restart") {
		t.Errorf("RotateKey() instructions missing 'restart'\nGot: %s", instructions)
	}

	for expected, found := range expectedContents {
		if !found {
			t.Errorf("RotateKey() instructions missing %q\nGot: %s", expected, instructions)
		}
	}
}

// TestValidateConfigFile_EdgeCases verifies edge case handling
func TestValidateConfigFile_EdgeCases(t *testing.T) {
	manager := NewAPIKeyManager()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "empty config",
			content: "",
			wantErr: false,
		},
		{
			name:    "key in comment",
			content: "# Don't put api_key: here",
			wantErr: true, // Still triggers (conservative)
		},
		{
			name:    "case sensitive",
			content: "API_KEY: value", // Uppercase, not in patterns
			wantErr: false,
		},
		{
			name:    "partial match",
			content: "This config has apiKey embedded in text",
			wantErr: false, // "apiKey:" pattern requires colon, this doesn't have one
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateConfigFile(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfigFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
