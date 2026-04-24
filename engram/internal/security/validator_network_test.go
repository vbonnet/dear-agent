package security

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// --- Network Permission Validation Tests ---

// TestValidateNetwork_ValidDomains verifies valid domain formats are accepted
func TestValidateNetwork_ValidDomains(t *testing.T) {
	tests := []struct {
		name    string
		network string
	}{
		{"simple domain", "github.com"},
		{"subdomain", "api.github.com"},
		{"multi-subdomain", "a.b.c.example.com"},
		{"country TLD", "example.co.uk"},
		{"long TLD", "example.technology"},
		{"hyphenated", "my-site.com"},
		{"numbers in label", "cdn1.example.com"},
		{"mixed case", "Example.COM"},
	}

	validator := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateNetwork(tt.network)
			if err != nil {
				t.Errorf("validateNetwork(%q) failed: %v", tt.network, err)
			}
		})
	}
}

// TestValidateNetwork_ValidIPv4 verifies valid IPv4 addresses are accepted
func TestValidateNetwork_ValidIPv4(t *testing.T) {
	tests := []struct {
		name    string
		network string
	}{
		{"simple IPv4", "1.1.1.1"},
		{"max octets", "255.255.255.255"},
		{"zero address", "0.0.0.0"},
	}

	validator := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateNetwork(tt.network)
			if err != nil {
				t.Errorf("validateNetwork(%q) failed: %v", tt.network, err)
			}
		})
	}
}

// TestValidateNetwork_ValidIPv6 verifies valid IPv6 addresses are accepted
func TestValidateNetwork_ValidIPv6(t *testing.T) {
	tests := []struct {
		name    string
		network string
	}{
		{"compressed", "2001:db8::1"},
		{"full notation", "2001:0db8:0000:0000:0000:0000:0000:0001"},
		{"zero address", "::"},
	}

	validator := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateNetwork(tt.network)
			if err != nil {
				t.Errorf("validateNetwork(%q) failed: %v", tt.network, err)
			}
		})
	}
}

// TestValidateNetwork_ValidCIDR verifies valid CIDR ranges are accepted
func TestValidateNetwork_ValidCIDR(t *testing.T) {
	tests := []struct {
		name    string
		network string
	}{
		{"IPv4 CIDR", "192.168.1.0/24"},
		{"IPv6 CIDR", "2001:db8::/32"},
		{"single host /32", "1.1.1.1/32"},
	}

	validator := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateNetwork(tt.network)
			if err != nil {
				t.Errorf("validateNetwork(%q) failed: %v", tt.network, err)
			}
		})
	}
}

// TestValidateNetwork_InvalidFormats verifies invalid formats are rejected
func TestValidateNetwork_InvalidFormats(t *testing.T) {
	tests := []struct {
		name    string
		network string
	}{
		{"empty string", ""},
		{"single label", "github"},
		{"no TLD", "example."},
		{"leading dot", ".com"},
		{"leading hyphen", "-example.com"},
		{"trailing hyphen", "example-.com"},
		{"space in domain", "exam ple.com"},
		{"special chars", "example@.com"},
		{"invalid IPv4", "256.1.1.1"},
		{"incomplete IPv4", "192.168.1"},
		{"invalid IPv6", "gggg::1"},
		{"invalid CIDR", "192.168.1.0/33"},
		{"URL instead", "http://example.com"},
		{"port number", "example.com:8080"},
	}

	validator := NewValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateNetwork(tt.network)
			if err == nil {
				t.Errorf("validateNetwork(%q) succeeded, want error", tt.network)
			}
		})
	}
}

// TestValidateNetwork_Wildcard verifies wildcard is accepted
func TestValidateNetwork_Wildcard(t *testing.T) {
	validator := NewValidator()

	err := validator.validateNetwork("*")
	if err != nil {
		t.Errorf("validateNetwork(\"*\") failed: %v", err)
	}
}

// TestValidateNetwork_WildcardLogging verifies wildcard generates warning log
func TestValidateNetwork_WildcardLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	}))
	validator := NewValidatorWithLogger(logger)

	_ = validator.validateNetwork("*")

	logOutput := buf.String()
	if logOutput == "" {
		t.Fatal("Expected warning log for wildcard, got none")
	}

	// Parse JSON log
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(logOutput)), &logEntry); err != nil {
		t.Fatalf("Failed to parse log: %v", err)
	}

	// Verify fields
	if msg := logEntry["msg"]; msg != "Plugin requests unrestricted network access" {
		t.Errorf("Log message = %q, want 'Plugin requests unrestricted network access'", msg)
	}
	if reason := logEntry["reason"]; reason != "wildcard_network_access" {
		t.Errorf("Log reason = %q, want 'wildcard_network_access'", reason)
	}
}

// TestValidateNetwork_AuditTrailLogging verifies all network permissions are logged
func TestValidateNetwork_AuditTrailLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	validator := NewValidatorWithLogger(logger)

	_ = validator.validateNetwork("github.com")

	logOutput := buf.String()
	if logOutput == "" {
		t.Fatal("Expected info log for valid domain, got none")
	}

	// Should have at least 2 log entries (validation start + success)
	lines := strings.Split(strings.TrimSpace(logOutput), "\n")
	if len(lines) < 2 {
		t.Errorf("Expected at least 2 log entries, got %d", len(lines))
	}
}

// TestValidateNetwork_SuspiciousPatternLogging verifies suspicious patterns generate warnings
func TestValidateNetwork_SuspiciousPatternLogging(t *testing.T) {
	tests := []struct {
		name       string
		network    string
		wantReason string
	}{
		{"localhost IPv4", "127.0.0.1", "localhost_access"},
		{"localhost IPv6", "::1", "localhost_access"},
		{"localhost name", "localhost", "localhost_access"},
		{"private class A", "10.1.1.1", "private_ipv4_class_a"},
		{"private class B", "172.16.1.1", "private_ipv4_class_b"},
		{"private class C", "192.168.1.1", "private_ipv4_class_c"},
		{"link-local IPv6", "fe80::1", "private_ipv6_link_local"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))
			validator := NewValidatorWithLogger(logger)

			_ = validator.validateNetwork(tt.network)

			logOutput := buf.String()
			if !strings.Contains(logOutput, tt.wantReason) {
				t.Errorf("Expected log to contain reason %q, got: %s", tt.wantReason, logOutput)
			}
		})
	}
}

// TestIsValidDomain verifies domain validation logic
func TestIsValidDomain(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		domain string
		want   bool
	}{
		// Valid
		{"example.com", true},
		{"api.github.com", true},
		{"a.b.c.example.com", true},
		{"my-site.com", true},
		{"cdn1.example.com", true},

		// Invalid
		{"", false},
		{"example", false},
		{".com", false},
		{"example.", false},
		{"-example.com", false},
		{"example-.com", false},
		{"exam ple.com", false},
		{"example@.com", false},
		{"example.c", false}, // TLD too short
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			got := validator.isValidDomain(tt.domain)
			if got != tt.want {
				t.Errorf("isValidDomain(%q) = %v, want %v", tt.domain, got, tt.want)
			}
		})
	}
}

// TestClassifyNetworkPermission verifies permission type classification
func TestClassifyNetworkPermission(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		network  string
		wantType NetworkPermissionType
		wantErr  bool
	}{
		{"github.com", PermTypeDomain, false},
		{"1.1.1.1", PermTypeIPv4, false},
		{"2001:db8::1", PermTypeIPv6, false},
		{"192.168.1.0/24", PermTypeCIDR, false},
		{"", "", true},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.network, func(t *testing.T) {
			gotType, err := validator.classifyNetworkPermission(tt.network)
			if (err != nil) != tt.wantErr {
				t.Errorf("classifyNetworkPermission(%q) error = %v, wantErr %v", tt.network, err, tt.wantErr)
				return
			}
			if gotType != tt.wantType {
				t.Errorf("classifyNetworkPermission(%q) type = %q, want %q", tt.network, gotType, tt.wantType)
			}
		})
	}
}

// TestIsLocalhostAddress verifies localhost detection
func TestIsLocalhostAddress(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		network string
		want    bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"127.0.0.2", true},
		{"127.1.2.3", true},
		{"::1", true},
		{"1.1.1.1", false},
		{"github.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.network, func(t *testing.T) {
			got := validator.isLocalhostAddress(tt.network)
			if got != tt.want {
				t.Errorf("isLocalhostAddress(%q) = %v, want %v", tt.network, got, tt.want)
			}
		})
	}
}

// TestIsPrivateIPRange verifies private IP range detection
func TestIsPrivateIPRange(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		network string
		want    string // reason (empty if not private)
	}{
		{"10.1.1.1", "private_ipv4_class_a"},
		{"172.16.1.1", "private_ipv4_class_b"},
		{"192.168.1.1", "private_ipv4_class_c"},
		{"fe80::1", "private_ipv6_link_local"},
		{"fc00::1", "private_ipv6_ula"},
		{"1.1.1.1", ""},
		{"github.com", ""},
	}

	for _, tt := range tests {
		t.Run(tt.network, func(t *testing.T) {
			got := validator.isPrivateIPRange(tt.network)
			if got != tt.want {
				t.Errorf("isPrivateIPRange(%q) = %q, want %q", tt.network, got, tt.want)
			}
		})
	}
}
