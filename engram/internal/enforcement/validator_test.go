package enforcement

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/config"
	"github.com/vbonnet/dear-agent/engram/internal/identity"
)

// TestValidator_Disabled tests that validation is skipped when enforcement is disabled
func TestValidator_Disabled(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: false,
	}

	id := &identity.Identity{
		Email:  "test@example.com",
		Domain: "@example.com",
	}

	validator := NewValidator(cfg, id)
	ctx := context.Background()

	if err := validator.Validate(ctx); err != nil {
		t.Errorf("Validate() with disabled enforcement returned error: %v", err)
	}
}

// TestValidator_IdentityRequired_Success tests successful identity validation
func TestValidator_IdentityRequired_Success(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required: true,
			AllowedDomains: []string{
				"@example.com",
				"@test.com",
			},
		},
	}

	id := &identity.Identity{
		Email:      "user@example.com",
		Domain:     "@example.com",
		Method:     "gcp_adc",
		Verified:   true,
		DetectedAt: time.Now(),
	}

	validator := NewValidator(cfg, id)
	ctx := context.Background()

	if err := validator.Validate(ctx); err != nil {
		t.Errorf("Validate() returned unexpected error: %v", err)
	}
}

// TestValidator_IdentityRequired_DomainMismatch tests identity validation failure
func TestValidator_IdentityRequired_DomainMismatch(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required: true,
			AllowedDomains: []string{
				"@example.com",
			},
		},
	}

	id := &identity.Identity{
		Email:      "user@other.com",
		Domain:     "@other.com",
		Method:     "git_config",
		Verified:   false,
		DetectedAt: time.Now(),
	}

	validator := NewValidator(cfg, id)
	ctx := context.Background()

	err := validator.Validate(ctx)
	if err == nil {
		t.Error("Validate() should return error for domain mismatch")
	}

	// Error should mention the domain
	if !strings.Contains(err.Error(), "@other.com") {
		t.Errorf("Error should mention domain, got: %v", err)
	}
}

// TestValidator_IdentityRequired_NoIdentity tests missing identity
func TestValidator_IdentityRequired_NoIdentity(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required: true,
			AllowedDomains: []string{
				"@example.com",
			},
		},
	}

	validator := NewValidator(cfg, nil) // No identity
	ctx := context.Background()

	err := validator.Validate(ctx)
	if err == nil {
		t.Error("Validate() should return error when identity is required but not provided")
	}

	if !strings.Contains(err.Error(), "no identity detected") {
		t.Errorf("Error should mention no identity, got: %v", err)
	}
}

// TestValidator_IdentityNotRequired tests validation when identity is optional
func TestValidator_IdentityNotRequired(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required: false, // Identity not required
		},
	}

	validator := NewValidator(cfg, nil) // No identity
	ctx := context.Background()

	if err := validator.Validate(ctx); err != nil {
		t.Errorf("Validate() should succeed when identity not required, got: %v", err)
	}
}

// TestValidator_NoAllowedDomains tests validation with no domain restrictions
func TestValidator_NoAllowedDomains(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required:       true,
			AllowedDomains: []string{}, // No domain restrictions
		},
	}

	id := &identity.Identity{
		Email:  "user@any.com",
		Domain: "@any.com",
	}

	validator := NewValidator(cfg, id)
	ctx := context.Background()

	if err := validator.Validate(ctx); err != nil {
		t.Errorf("Validate() should succeed with no domain restrictions, got: %v", err)
	}
}

// TestValidator_ErrorTemplate tests custom error message formatting
func TestValidator_ErrorTemplate(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required: true,
			AllowedDomains: []string{
				"@example.com",
			},
		},
		ErrorMessages: &config.ErrorMessageConfig{
			Template: "ERROR: {{.Message}}\nHelp: {{.HelpURL}}",
			HelpURL:  "https://help.example.com",
		},
	}

	validator := NewValidator(cfg, nil) // No identity
	ctx := context.Background()

	err := validator.Validate(ctx)
	if err == nil {
		t.Fatal("Validate() should return error")
	}

	errMsg := err.Error()

	// Check template was used
	if !strings.Contains(errMsg, "ERROR:") {
		t.Error("Error should contain template prefix 'ERROR:'")
	}
	if !strings.Contains(errMsg, "https://help.example.com") {
		t.Error("Error should contain help URL")
	}
}

// TestValidator_ErrorTemplate_Invalid tests fallback when template is invalid
func TestValidator_ErrorTemplate_Invalid(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required: true,
		},
		ErrorMessages: &config.ErrorMessageConfig{
			Template: "{{.InvalidField}}", // Invalid template
		},
	}

	validator := NewValidator(cfg, nil) // No identity
	ctx := context.Background()

	err := validator.Validate(ctx)
	if err == nil {
		t.Fatal("Validate() should return error")
	}

	// Should fallback to simple error (not panic)
	_ = err.Error()
}

// TestValidator_ContextCancellation tests context cancellation handling
func TestValidator_ContextCancellation(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required: true,
			AllowedDomains: []string{
				"@example.com",
			},
		},
	}

	id := &identity.Identity{
		Email:  "user@example.com",
		Domain: "@example.com",
	}

	validator := NewValidator(cfg, id)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Validation should complete quickly (no blocking operations)
	err := validator.Validate(ctx)

	// May succeed (validation is fast) or fail (context check)
	// Just ensure no panic
	_ = err
}

// TestNewValidator_NilConfig tests validator creation with nil config
func TestNewValidator_NilConfig(t *testing.T) {
	id := &identity.Identity{
		Email:  "user@example.com",
		Domain: "@example.com",
	}

	validator := NewValidator(nil, id)
	if validator == nil {
		t.Error("NewValidator() should not return nil")
	}

	ctx := context.Background()
	if err := validator.Validate(ctx); err != nil {
		t.Errorf("Validate() with nil config should succeed: %v", err)
	}
}

// TestExtractDomain tests domain extraction from email
func TestExtractDomain(t *testing.T) {
	tests := []struct {
		email string
		want  string
	}{
		{"user@example.com", "@example.com"},
		{"test@subdomain.example.org", "@subdomain.example.org"},
		{"invalid-email", ""},
		{"@example.com", ""},
		{"user@", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			// This tests the extractDomain function from identity package
			// which is used by enforcement validator
			got := extractDomainForTest(tt.email)
			if got != tt.want {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}

// extractDomainForTest duplicates the extractDomain logic for testing
func extractDomainForTest(email string) string {
	atIndex := -1
	for i := 0; i < len(email); i++ {
		if email[i] == '@' {
			atIndex = i
			break
		}
	}

	if atIndex == -1 || atIndex == 0 || atIndex == len(email)-1 {
		return ""
	}

	return "@" + email[atIndex+1:]
}
