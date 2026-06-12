package corpus

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// corpusCallosumBin returns the corpus-callosum CLI binary name.
// Reads CORPUS_CALLOSUM_BIN env var; defaults to "corpus-callosum".
// Never falls back to "cc" — on macOS/Linux that name resolves to the C
// compiler (clang/gcc), which silently accepts the invocation and then
// errors with "unknown argument --workspace ...".
func corpusCallosumBin() string {
	if bin := os.Getenv("CORPUS_CALLOSUM_BIN"); bin != "" {
		return bin
	}
	return "corpus-callosum"
}

// isCorpusCallosumAvailable checks if the corpus-callosum CLI is installed.
func isCorpusCallosumAvailable() bool {
	_, err := exec.LookPath(corpusCallosumBin())
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
	schema["workspace"] = workspace

	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	bin := corpusCallosumBin()
	cmd := exec.Command(bin, "register", "--workspace", workspace, "--schema", "-")
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s register: %w", bin, err)
	}

	if _, err := stdin.Write(schemaJSON); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to write schema: %w", err)
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("%s register failed: %w", bin, err)
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

	cmd := exec.Command(corpusCallosumBin(), "unregister", "--workspace", workspace, "--component", component)
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

	cmd := exec.Command(corpusCallosumBin(), "list-schemas", "--workspace", workspace, "--component", "wayfinder", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		// Not registered or error - return empty list
		return []string{}, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	var schemas []struct {
		Entity string `json:"entity"`
	}

	if err := json.Unmarshal(output, &schemas); err != nil {
		return []string{}, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	entities := make([]string, len(schemas))
	for i, schema := range schemas {
		entities[i] = schema.Entity
	}

	return entities, nil
}

// PublishProject publishes a Wayfinder project to corpus callosum.
// Always tags the map with workspace/_component/_entity for caller consistency.
// Gracefully degrades (no-op) if the corpus-callosum binary is not installed.
func PublishProject(workspace string, project map[string]interface{}) error {
	project["workspace"] = workspace
	project["_component"] = "wayfinder"
	project["_entity"] = "project"

	if !isCorpusCallosumAvailable() {
		return nil
	}

	// Marshal to JSON
	projectJSON, err := json.Marshal(project)
	if err != nil {
		return fmt.Errorf("failed to marshal project: %w", err)
	}

	// Publish to corpus callosum
	cmd := exec.Command(corpusCallosumBin(), "publish", "--workspace", workspace, "--data", "-")
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

// PublishPhase publishes a Wayfinder phase to corpus callosum.
// Always tags the map with workspace/_component/_entity for caller consistency.
// Gracefully degrades (no-op) if the corpus-callosum binary is not installed.
func PublishPhase(workspace string, phase map[string]interface{}) error {
	phase["workspace"] = workspace
	phase["_component"] = "wayfinder"
	phase["_entity"] = "phase"

	if !isCorpusCallosumAvailable() {
		return nil
	}

	phaseJSON, err := json.Marshal(phase)
	if err != nil {
		return fmt.Errorf("failed to marshal phase: %w", err)
	}

	cmd := exec.Command(corpusCallosumBin(), "publish", "--workspace", workspace, "--data", "-")
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

// PublishValidation publishes a Wayfinder validation result to corpus callosum.
// Always tags the map with workspace/_component/_entity for caller consistency.
// Gracefully degrades (no-op) if the corpus-callosum binary is not installed.
func PublishValidation(workspace string, validation map[string]interface{}) error {
	validation["workspace"] = workspace
	validation["_component"] = "wayfinder"
	validation["_entity"] = "validation"

	if !isCorpusCallosumAvailable() {
		return nil
	}

	validationJSON, err := json.Marshal(validation)
	if err != nil {
		return fmt.Errorf("failed to marshal validation: %w", err)
	}

	cmd := exec.Command(corpusCallosumBin(), "publish", "--workspace", workspace, "--data", "-")
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
