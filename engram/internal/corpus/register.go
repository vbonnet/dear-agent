package corpus

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// RegisterEngramSchemas registers Engram schemas with corpus callosum.
// This function is called during engram init to make Engram's data
// discoverable by other tools (AGM, Wayfinder, Swarm).
//
// It uses the `cc register` CLI command if available, otherwise
// silently skips registration (graceful degradation).
func RegisterEngramSchemas(workspace string) error {
	// Check if cc (corpus callosum) CLI is available
	if !isCorpusCallosumAvailable() {
		// Silently skip if CC not installed - optional integration
		return nil
	}

	schema := GetEngramSchema()

	// Add workspace context to schema
	if workspace != "" {
		schema["workspace"] = workspace
	}

	// Serialize schema to JSON
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("failed to serialize schema: %w", err)
	}

	// Write schema to temporary file
	tmpFile, err := os.CreateTemp("", "engram-schema-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(schemaJSON); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write schema: %w", err)
	}
	tmpFile.Close()

	// Register with corpus callosum
	cmd := exec.Command("cc", "register",
		"--component", EngramComponentName,
		"--version", EngramComponentVersion,
		"--schema", tmpFile.Name(),
	)

	// Include workspace if specified
	if workspace != "" {
		cmd.Args = append(cmd.Args, "--workspace", workspace)
	}

	// Capture output for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cc register failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// UnregisterEngramSchemas removes Engram schemas from corpus callosum.
// Used for cleanup or when switching workspaces.
func UnregisterEngramSchemas(workspace string) error {
	if !isCorpusCallosumAvailable() {
		return nil
	}

	cmd := exec.Command("cc", "unregister",
		"--component", EngramComponentName,
	)

	if workspace != "" {
		cmd.Args = append(cmd.Args, "--workspace", workspace)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cc unregister failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// isCorpusCallosumAvailable checks if the cc CLI tool is installed.
func isCorpusCallosumAvailable() bool {
	_, err := exec.LookPath("cc")
	return err == nil
}

// GetRegistrationStatus checks if Engram schemas are registered with CC.
func GetRegistrationStatus(workspace string) (bool, error) {
	if !isCorpusCallosumAvailable() {
		return false, nil
	}

	cmd := exec.Command("cc", "discover")

	if workspace != "" {
		cmd.Args = append(cmd.Args, "--workspace", workspace)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("cc discover failed: %w", err)
	}

	// Check if output contains "engram"
	// This is a simple check - in production, parse JSON output
	outputStr := string(output)
	return contains(outputStr, "engram"), nil
}

// contains checks if a string contains a substring (helper function).
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				anyMatch(s, substr)))
}

func anyMatch(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
