package research

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GetAPIKey retrieves the Gemini API key
// First checks GEMINI_API_KEY environment variable
// If not set and projectID is provided, retrieves from gcloud
func GetAPIKey(projectID string) (string, error) {
	// Try environment variable first
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey != "" {
		return apiKey, nil
	}

	// If no project ID provided, return error
	if projectID == "" {
		return "", NewAPIKeyNotSetError()
	}

	// Try to get API key from gcloud
	return getAPIKeyFromGCloud(projectID)
}

// getAPIKeyFromGCloud retrieves the API key from Google Cloud
// This matches the Python reference implementation
func getAPIKeyFromGCloud(projectID string) (string, error) {
	// List API keys to find "Gemini CLI API Key"
	listCmd := exec.Command(
		"gcloud", "services", "api-keys", "list",
		fmt.Sprintf("--project=%s", projectID),
		"--filter=displayName:'Gemini CLI API Key'",
		"--format=value(name)",
	)

	// Suppress stderr from gcloud
	listCmd.Stderr = nil

	output, err := listCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to list API keys: %w", err)
	}

	keyName := strings.TrimSpace(string(output))
	if keyName == "" {
		return "", fmt.Errorf("Gemini CLI API Key not found in project %s", projectID)
	}

	// Get the actual key string
	getKeyCmd := exec.Command(
		"gcloud", "services", "api-keys", "get-key-string",
		keyName,
	)

	// Suppress stderr from gcloud
	getKeyCmd.Stderr = nil

	keyOutput, err := getKeyCmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get API key string: %w", err)
	}

	// Parse "keyString: AIza..." format
	keyString := strings.TrimSpace(string(keyOutput))
	parts := strings.SplitN(keyString, "keyString: ", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("unexpected API key format: %s", keyString)
	}

	return strings.TrimSpace(parts[1]), nil
}
