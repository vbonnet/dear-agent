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
	"strings"
)

// APIKeyManager handles API key validation and security
type APIKeyManager struct{}

// NewAPIKeyManager creates a new API key manager
func NewAPIKeyManager() *APIKeyManager {
	return &APIKeyManager{}
}

// GetAnthropicKey retrieves Anthropic API key from environment
func (m *APIKeyManager) GetAnthropicKey() (string, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	return key, nil
}

// ValidateConfigFile validates that a config file doesn't contain API keys
func (m *APIKeyManager) ValidateConfigFile(content string) error {
	// Check for common API key patterns
	patterns := []string{
		"api_key:",
		"apiKey:",
		"ANTHROPIC_API_KEY",
		"sk-ant-",
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
	// Redact API keys
	if strings.HasPrefix(s, "sk-ant-") {
		return "sk-ant-***REDACTED***"
	}

	// Redact environment variables that look like keys
	if strings.Contains(s, "ANTHROPIC_API_KEY") {
		return strings.Replace(s, os.Getenv("ANTHROPIC_API_KEY"), "***REDACTED***", -1)
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
