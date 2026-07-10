// Package security implements sandboxing, permission validation, and API key management
// for secure plugin execution.
//
// Security is a critical concern when executing untrusted plugin code. This package
// provides multiple layers of protection:
//
//   - Sandboxing: OS-level isolation using AppArmor (Linux) or sandbox-exec (macOS)
//   - Permission validation: Explicit allow-lists for filesystem, network, commands
//   - API key security: Safe retrieval and validation of credentials
//
// Sandboxing strategy:
//  1. Validator checks plugin manifest permissions against request
//  2. Sandbox applies OS-specific restrictions before execution
//  3. Plugin runs with minimal privileges
//
// Platform-specific sandboxing:
//   - Linux: AppArmor profiles (when available)
//   - macOS: sandbox-exec with custom profiles
//   - Others: Graceful degradation with validation only
//
// Example usage:
//
//	validator := security.NewValidator()
//	if err := validator.ValidatePermissions(manifest.Permissions, requested); err != nil {
//	    return fmt.Errorf("permission denied: %w", err)
//	}
//
//	sandbox := security.NewSandbox()
//	args, err := sandbox.Apply(cmd, args, manifest.Permissions)
//	if err != nil {
//	    return fmt.Errorf("sandboxing failed: %w", err)
//	}
//
// See ADR-009 for security architecture and threat model.
package security

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var providerKeyEnvironments = map[string][]string{
	"anthropic":  {"ANTHROPIC_API_KEY"},
	"gemini":     {"GEMINI_API_KEY", "GOOGLE_API_KEY"},
	"google":     {"GEMINI_API_KEY", "GOOGLE_API_KEY"},
	"openai":     {"OPENAI_API_KEY"},
	"openrouter": {"OPENROUTER_API_KEY"},
}

var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]+`),
	regexp.MustCompile(`sk-or-[A-Za-z0-9_-]+`),
	regexp.MustCompile(`sk-proj-[A-Za-z0-9_-]+`),
	regexp.MustCompile(`AIza[A-Za-z0-9_-]{20,}`),
}

const minimumCredentialRedactionLength = 8

// APIKeyManager handles API key validation and security
type APIKeyManager struct{}

// NewAPIKeyManager creates a new API key manager
func NewAPIKeyManager() *APIKeyManager {
	return &APIKeyManager{}
}

// GetAnthropicKey retrieves Anthropic API key from environment
func (m *APIKeyManager) GetAnthropicKey() (string, error) {
	return m.GetProviderKey("anthropic")

}

// GetProviderKey retrieves the first configured API key for a supported provider.
func (m *APIKeyManager) GetProviderKey(provider string) (string, error) {
	environments, ok := providerKeyEnvironments[strings.ToLower(provider)]
	if !ok {
		return "", fmt.Errorf("unsupported API key provider %q", provider)
	}
	for _, environment := range environments {
		if key := os.Getenv(environment); key != "" {
			return key, nil
		}
	}
	return "", fmt.Errorf("%s not set", strings.Join(environments, " or "))
}

// ValidateConfigFile validates that a config file doesn't contain API keys
func (m *APIKeyManager) ValidateConfigFile(content string) error {
	// Check for common API key patterns
	patterns := []string{
		"api_key:",
		"apiKey:",
		"ANTHROPIC_API_KEY",
		"GEMINI_API_KEY",
		"GOOGLE_API_KEY",
		"OPENAI_API_KEY",
		"OPENROUTER_API_KEY",
		"sk-ant-",
		"sk-or-",
		"sk-proj-",
		"AIza",
	}

	for _, pattern := range patterns {
		if strings.Contains(content, pattern) {
			return fmt.Errorf("config file contains API key (keys must be in environment variables)")
		}
	}

	return nil
}

// SanitizeForLogs removes sensitive data from strings before logging
func (m *APIKeyManager) SanitizeForLogs(s string) string {
	for _, environments := range providerKeyEnvironments {
		for _, environment := range environments {
			if key := os.Getenv(environment); len(key) >= minimumCredentialRedactionLength {
				s = strings.ReplaceAll(s, key, "***REDACTED***")
			}
		}
	}
	for _, pattern := range credentialPatterns {
		s = pattern.ReplaceAllStringFunc(s, func(value string) string {
			if strings.HasPrefix(value, "AIza") {
				return "AIza***REDACTED***"
			}
			prefixEnd := strings.Index(value, "-") + 1
			if strings.HasPrefix(value, "sk-") {
				prefixEnd = strings.Index(value[prefixEnd:], "-") + prefixEnd + 1
			}
			return value[:prefixEnd] + "***REDACTED***"
		})
	}
	return s
}

// RotateKey provides instructions for key rotation
func (m *APIKeyManager) RotateKey() string {
	return `To rotate your Anthropic API key:

1. Generate new key at: https://console.anthropic.com/settings/keys
2. Update environment variable:
   export ANTHROPIC_API_KEY="sk-ant-new-key"
3. Restart engram

Keys are NEVER stored in config files for security.
`
}
