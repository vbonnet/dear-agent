package research

import (
	"errors"
	"os"
	"testing"
)

func TestGetAPIKey_FromEnvVar(t *testing.T) {
	// Set environment variable
	testKey := "AIza-test-key-from-env"
	os.Setenv("GEMINI_API_KEY", testKey)
	defer os.Unsetenv("GEMINI_API_KEY")

	apiKey, err := GetAPIKey("")
	if err != nil {
		t.Fatalf("GetAPIKey() error = %v, want nil", err)
	}

	if apiKey != testKey {
		t.Errorf("apiKey = %q, want %q", apiKey, testKey)
	}
}

func TestGetAPIKey_NoEnvVarNoProject(t *testing.T) {
	// Ensure no environment variable
	os.Unsetenv("GEMINI_API_KEY")

	_, err := GetAPIKey("")
	if err == nil {
		t.Error("GetAPIKey() error = nil, want error for missing API key")
	}

	var researchErr *ResearchError
	if !errors.As(err, &researchErr) {
		t.Errorf("error type = %T, want *ResearchError", err)
	}

	// Check it's specifically the API key not set error
	if !errors.Is(err, ErrAPIKeyNotSet) {
		t.Errorf("error = %v, want ErrAPIKeyNotSet", err)
	}
}

func TestGetAPIKey_EnvVarTakesPrecedence(t *testing.T) {
	// Set environment variable
	testKey := "AIza-env-key"
	os.Setenv("GEMINI_API_KEY", testKey)
	defer os.Unsetenv("GEMINI_API_KEY")

	// Even with project ID, env var should take precedence
	apiKey, err := GetAPIKey("some-project-id")
	if err != nil {
		t.Fatalf("GetAPIKey() error = %v, want nil", err)
	}

	if apiKey != testKey {
		t.Errorf("apiKey = %q, want %q (env var should take precedence)", apiKey, testKey)
	}
}

// Note: Testing getAPIKeyFromGCloud requires gcloud CLI and real GCP project
// We skip integration testing of that function in unit tests
// Integration tests with real gcloud will be done separately
