package auth_test

import (
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// Example demonstrates basic usage of the auth hierarchy detection.
func Example() {
	// Simulate a GCP environment with Vertex AI
	os.Setenv("GOOGLE_CLOUD_PROJECT", "my-project")

	// Detect auth for Anthropic (will use Vertex AI Claude)
	method := auth.DetectAuthMethod("anthropic")
	fmt.Printf("Anthropic auth: %s\n", method)

	// Clean up and simulate API key environment
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Setenv("GEMINI_API_KEY", "test-key")

	// Detect auth for Gemini (will use API key)
	method = auth.DetectAuthMethod("gemini")
	fmt.Printf("Gemini auth: %s\n", method)

	// Clean up
	os.Unsetenv("GEMINI_API_KEY")

	// Output:
	// Anthropic auth: VertexAI
	// Gemini auth: APIKey
}

// ExampleDetectAuthMethod_vertexAI demonstrates Vertex AI detection.
func ExampleDetectAuthMethod_vertexAI() {
	// Set up GCP environment
	os.Setenv("GOOGLE_CLOUD_PROJECT", "my-gcp-project")
	defer os.Unsetenv("GOOGLE_CLOUD_PROJECT")

	// Both Anthropic and Gemini will use Vertex AI
	anthropicAuth := auth.DetectAuthMethod("anthropic")
	geminiAuth := auth.DetectAuthMethod("gemini")

	fmt.Printf("Anthropic: %s\n", anthropicAuth)
	fmt.Printf("Gemini: %s\n", geminiAuth)

	// Output:
	// Anthropic: VertexAI
	// Gemini: VertexAI
}

// ExampleDetectAuthMethod_apiKey demonstrates API key detection.
func ExampleDetectAuthMethod_apiKey() {
	// Set up API keys
	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	os.Setenv("GEMINI_API_KEY", "ai-test")
	os.Setenv("OPENROUTER_API_KEY", "sk-or-test")

	defer func() {
		os.Unsetenv("ANTHROPIC_API_KEY")
		os.Unsetenv("GEMINI_API_KEY")
		os.Unsetenv("OPENROUTER_API_KEY")
	}()

	// All providers will use API keys
	fmt.Printf("Anthropic: %s\n", auth.DetectAuthMethod("anthropic"))
	fmt.Printf("Gemini: %s\n", auth.DetectAuthMethod("gemini"))
	fmt.Printf("OpenRouter: %s\n", auth.DetectAuthMethod("openrouter"))

	// Output:
	// Anthropic: APIKey
	// Gemini: APIKey
	// OpenRouter: APIKey
}

// ExampleDetectAuthMethod_precedence demonstrates auth hierarchy precedence.
func ExampleDetectAuthMethod_precedence() {
	// Set up both Vertex AI and API key
	os.Setenv("GOOGLE_CLOUD_PROJECT", "my-project")
	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")

	defer func() {
		os.Unsetenv("GOOGLE_CLOUD_PROJECT")
		os.Unsetenv("ANTHROPIC_API_KEY")
	}()

	// Vertex AI takes precedence over API key
	method := auth.DetectAuthMethod("anthropic")
	fmt.Printf("Auth method: %s\n", method)

	// Output:
	// Auth method: VertexAI
}

// ExampleDetectAuthMethod_noAuth demonstrates behavior when no auth is available.
func ExampleDetectAuthMethod_noAuth() {
	// No environment variables set
	method := auth.DetectAuthMethod("anthropic")
	fmt.Printf("Auth method: %s\n", method)

	// Output:
	// Auth method: None
}

// ExampleDetectAuthMethod_providerAliases demonstrates provider name aliases.
func ExampleDetectAuthMethod_providerAliases() {
	os.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	defer os.Unsetenv("ANTHROPIC_API_KEY")

	// "anthropic" and "claude" are aliases
	method1 := auth.DetectAuthMethod("anthropic")
	method2 := auth.DetectAuthMethod("claude")

	fmt.Printf("anthropic: %s\n", method1)
	fmt.Printf("claude: %s\n", method2)

	// Output:
	// anthropic: APIKey
	// claude: APIKey
}
