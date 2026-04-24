package corpus

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// isCorpusCallosumAvailable checks if the cc CLI is installed
func isCorpusCallosumAvailable() bool {
	_, err := exec.LookPath("cc")
	return err == nil
}

// RegisterWayfinderSchemas registers all Wayfinder schemas with corpus callosum
// Gracefully degrades if cc CLI is not available
func RegisterWayfinderSchemas(workspace string) error {
	if !isCorpusCallosumAvailable() {
		// Graceful degradation: corpus callosum is optional
		return nil
	}

	schemas := GetAllSchemas()

	for _, schema := range schemas {
		if err := registerSchema(workspace, schema); err != nil {
			// Log error but don't fail if registration fails
			fmt.Fprintf(os.Stderr, "Warning: failed to register Wayfinder schema: %v\n", err)
		}
	}

	return nil
}

// registerSchema registers a single schema with corpus callosum
func registerSchema(workspace string, schema map[string]interface{}) error {
	// Add workspace to schema metadata
	schema["workspace"] = workspace

	// Marshal schema to JSON
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	// Call cc register command
	cmd := exec.Command("cc", "register", "--workspace", workspace, "--schema", "-")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	// Write schema to stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start cc register: %w", err)
	}

	// Write schema to stdin
	if _, err := stdin.Write(schemaJSON); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to write schema: %w", err)
	}
	stdin.Close()

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("cc register failed: %w", err)
	}

	return nil
}

// UnregisterWayfinderSchemas removes Wayfinder schemas from corpus callosum
// Gracefully degrades if cc CLI is not available
func UnregisterWayfinderSchemas(workspace string) error {
	if !isCorpusCallosumAvailable() {
		return nil
	}

	component := "wayfinder"

	cmd := exec.Command("cc", "unregister", "--workspace", workspace, "--component", component)
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Log error but don't fail
		fmt.Fprintf(os.Stderr, "Warning: failed to unregister Wayfinder schemas: %v\n", err)
	}

	return nil
}

// GetRegistrationStatus checks if Wayfinder schemas are registered
// Returns list of registered entity types, or empty list if not registered or cc not available
func GetRegistrationStatus(workspace string) ([]string, error) {
	if !isCorpusCallosumAvailable() {
		return []string{}, nil
	}

	cmd := exec.Command("cc", "list-schemas", "--workspace", workspace, "--component", "wayfinder", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		// Not registered or error - return empty list
		return []string{}, nil
	}

	var schemas []struct {
		Entity string `json:"entity"`
	}

	if err := json.Unmarshal(output, &schemas); err != nil {
		return []string{}, nil
	}

	entities := make([]string, len(schemas))
	for i, schema := range schemas {
		entities[i] = schema.Entity
	}

	return entities, nil
}

// PublishProject publishes a Wayfinder project to corpus callosum
// Gracefully degrades if cc not available
func PublishProject(workspace string, project map[string]interface{}) error {
	if !isCorpusCallosumAvailable() {
		return nil
	}

	// Ensure workspace field is set
	project["workspace"] = workspace
	project["_component"] = "wayfinder"
	project["_entity"] = "project"

	// Marshal to JSON
	projectJSON, err := json.Marshal(project)
	if err != nil {
		return fmt.Errorf("failed to marshal project: %w", err)
	}

	// Publish to corpus callosum
	cmd := exec.Command("cc", "publish", "--workspace", workspace, "--data", "-")
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start cc publish: %w", err)
	}

	if _, err := stdin.Write(projectJSON); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to write project data: %w", err)
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		// Log warning but don't fail
		fmt.Fprintf(os.Stderr, "Warning: failed to publish project to corpus callosum: %v\n", err)
	}

	return nil
}

// PublishPhase publishes a Wayfinder phase to corpus callosum
func PublishPhase(workspace string, phase map[string]interface{}) error {
	if !isCorpusCallosumAvailable() {
		return nil
	}

	phase["workspace"] = workspace
	phase["_component"] = "wayfinder"
	phase["_entity"] = "phase"

	phaseJSON, err := json.Marshal(phase)
	if err != nil {
		return fmt.Errorf("failed to marshal phase: %w", err)
	}

	cmd := exec.Command("cc", "publish", "--workspace", workspace, "--data", "-")
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start cc publish: %w", err)
	}

	if _, err := stdin.Write(phaseJSON); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to write phase data: %w", err)
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to publish phase to corpus callosum: %v\n", err)
	}

	return nil
}

// PublishValidation publishes a Wayfinder validation result to corpus callosum
func PublishValidation(workspace string, validation map[string]interface{}) error {
	if !isCorpusCallosumAvailable() {
		return nil
	}

	validation["workspace"] = workspace
	validation["_component"] = "wayfinder"
	validation["_entity"] = "validation"

	validationJSON, err := json.Marshal(validation)
	if err != nil {
		return fmt.Errorf("failed to marshal validation: %w", err)
	}

	cmd := exec.Command("cc", "publish", "--workspace", workspace, "--data", "-")
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start cc publish: %w", err)
	}

	if _, err := stdin.Write(validationJSON); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to write validation data: %w", err)
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to publish validation to corpus callosum: %v\n", err)
	}

	return nil
}
