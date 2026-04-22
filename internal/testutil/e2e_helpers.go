package testutil

// MockAnthropicAPIKey returns a mock API key for E2E testing.
// This can be used when testing retrieve/ranking functionality
// without requiring a real Anthropic API key.
func MockAnthropicAPIKey() string {
	return "test-mock-api-key"
}

// E2ETestPort returns a safe port number for E2E testing
// to avoid conflicts with development servers.
func E2ETestPort() int {
	return 18767
}
