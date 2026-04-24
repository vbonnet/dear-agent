package sandbox

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseKernelVersion_Detailed(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard_kernel_version",
			input:    "Linux version 6.6.123+ (builder@host) (gcc version 11.2.0) #1 SMP",
			expected: "6.6.123",
		},
		{
			name:     "kernel_with_dash_suffix",
			input:    "Linux version 5.11.0-ubuntu1 (builder@host) #1 SMP",
			expected: "5.11.0-ubuntu1", // TrimRight only removes trailing +/-
		},
		{
			name:     "minimal_version",
			input:    "Linux version 5.4.0",
			expected: "5.4.0",
		},
		{
			name:     "version_word_elsewhere",
			input:    "Some random string without version keyword",
			expected: "keyword", // "version" found, next word returned
		},
		{
			name:     "version_at_end_no_value",
			input:    "Linux version",
			expected: "unknown",
		},
		{
			name:     "empty_string",
			input:    "",
			expected: "unknown",
		},
		{
			name:     "version_with_trailing_plus",
			input:    "Linux version 4.19.128+ (gcc) #1 SMP",
			expected: "4.19.128",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseKernelVersion(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsKernelVersionAtLeast_Detailed(t *testing.T) {
	tests := []struct {
		name    string
		version string
		major   int
		minor   int
		want    bool
	}{
		{"exact_match", "5.11.0", 5, 11, true},
		{"higher_major", "6.6.123", 5, 11, true},
		{"higher_minor", "5.15.0", 5, 11, true},
		{"lower_minor", "5.10.0", 5, 11, false},
		{"lower_major", "4.19.0", 5, 11, false},
		{"invalid_version", "unknown", 5, 11, false},
		{"malformed_version", "not.a.version", 5, 11, false},
		{"empty_version", "", 5, 11, false},
		{"equal_major_equal_minor", "5.11.99", 5, 11, true},
		{"zero_check", "0.0.0", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isKernelVersionAtLeast(tt.version, tt.major, tt.minor)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestRegisterProvider_CustomProvider(t *testing.T) {
	// Register a custom provider
	RegisterProvider("test-custom-provider", func() Provider {
		return NewMockProvider()
	})

	// Verify it can be looked up
	provider, err := NewProviderForPlatform("test-custom-provider")
	assert.NoError(t, err)
	assert.NotNil(t, provider)
	assert.Equal(t, "mock", provider.Name())
}

func TestMockProvider_GetSandbox(t *testing.T) {
	provider := NewMockProvider()

	// GetSandbox for nonexistent should return false
	sb, exists := provider.GetSandbox("nonexistent")
	assert.False(t, exists)
	assert.Nil(t, sb)
}

func TestMockProvider_GetSandbox_AfterCreate(t *testing.T) {
	provider := NewMockProvider()

	req := SandboxRequest{
		SessionID:    "test-get",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
	}

	created, err := provider.Create(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, created)

	sb, exists := provider.GetSandbox(created.ID)
	assert.True(t, exists)
	assert.NotNil(t, sb)
	assert.Equal(t, "test-get", sb.ID)
}

func TestMockProvider_GetSandbox_AfterDestroy(t *testing.T) {
	provider := NewMockProvider()

	req := SandboxRequest{
		SessionID:    "test-destroy-get",
		LowerDirs:    []string{t.TempDir()},
		WorkspaceDir: t.TempDir(),
	}

	created, err := provider.Create(context.Background(), req)
	assert.NoError(t, err)

	err = provider.Destroy(context.Background(), created.ID)
	assert.NoError(t, err)

	sb, exists := provider.GetSandbox(created.ID)
	assert.False(t, exists)
	assert.Nil(t, sb)
}
