package corpus

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// QueryAGMSessions queries AGM for session information via corpus callosum.
// Returns session data from the current workspace.
func QueryAGMSessions(workspace string, filter map[string]interface{}) ([]map[string]interface{}, error) {
	if !isCorpusCallosumAvailable() {
		return nil, fmt.Errorf("corpus callosum not available")
	}

	// Build query
	query := map[string]interface{}{
		"component": "agm",
		"schema":    "session",
		"workspace": workspace,
	}

	if filter != nil {
		query["filter"] = filter
	}

	return executeQuery(query)
}

// QueryWayfinderProjects queries Wayfinder for project information.
// Returns active projects in the workspace.
func QueryWayfinderProjects(workspace string, filter map[string]interface{}) ([]map[string]interface{}, error) {
	if !isCorpusCallosumAvailable() {
		return nil, fmt.Errorf("corpus callosum not available")
	}

	query := map[string]interface{}{
		"component": "wayfinder",
		"schema":    "project",
		"workspace": workspace,
	}

	if filter != nil {
		query["filter"] = filter
	}

	return executeQuery(query)
}

// QuerySwarmProjects queries Wayfinder for swarm project information.
// Note: The former engram-swarm component has been merged into wayfinder.
func QuerySwarmProjects(workspace string, filter map[string]interface{}) ([]map[string]interface{}, error) {
	if !isCorpusCallosumAvailable() {
		return nil, fmt.Errorf("corpus callosum not available")
	}

	query := map[string]interface{}{
		"component": "wayfinder",
		"schema":    "swarm",
		"workspace": workspace,
	}

	if filter != nil {
		query["filter"] = filter
	}

	return executeQuery(query)
}

// GetCurrentAGMSession retrieves information about the current AGM session.
// Useful for understanding the context in which Engram is running.
func GetCurrentAGMSession(workspace string) (map[string]interface{}, error) {
	sessions, err := QueryAGMSessions(workspace, map[string]interface{}{
		"state": "READY",
	})

	if err != nil {
		return nil, err
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no active AGM session found")
	}

	// Return first active session (should typically be only one)
	return sessions[0], nil
}

// GetWayfinderProjectForPath finds the Wayfinder project containing a given path.
func GetWayfinderProjectForPath(workspace string, path string) (map[string]interface{}, error) {
	projects, err := QueryWayfinderProjects(workspace, map[string]interface{}{
		"path_prefix": path,
	})

	if err != nil {
		return nil, err
	}

	if len(projects) == 0 {
		return nil, fmt.Errorf("no Wayfinder project found for path: %s", path)
	}

	return projects[0], nil
}

// executeQuery is a helper that executes a corpus callosum query.
func executeQuery(query map[string]interface{}) ([]map[string]interface{}, error) {
	// Serialize query to JSON
	queryJSON, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize query: %w", err)
	}

	// Execute cc query command
	cmd := exec.Command("cc", "query", "--query", string(queryJSON), "--format", "json")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("cc query failed: %w\nOutput: %s", err, string(output))
	}

	// Parse response
	var response struct {
		Status string                   `json:"status"`
		Data   []map[string]interface{} `json:"data"`
		Error  string                   `json:"error"`
	}

	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Status != "success" {
		return nil, fmt.Errorf("query failed: %s", response.Error)
	}

	return response.Data, nil
}

// DiscoverComponents returns a list of all components registered with corpus callosum.
func DiscoverComponents(workspace string) ([]string, error) {
	if !isCorpusCallosumAvailable() {
		return nil, fmt.Errorf("corpus callosum not available")
	}

	cmd := exec.Command("cc", "discover", "--format", "json")

	if workspace != "" {
		cmd.Args = append(cmd.Args, "--workspace", workspace)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("cc discover failed: %w", err)
	}

	// Parse response
	var response struct {
		Components []struct {
			Name    string `json:"component"`
			Version string `json:"version"`
		} `json:"components"`
	}

	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract component names
	names := make([]string, len(response.Components))
	for i, comp := range response.Components {
		names[i] = comp.Name
	}

	return names, nil
}
