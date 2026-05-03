// Package corpus provides corpus-related functionality.
package corpus

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// QueryAGMSessions queries AGM sessions in the current workspace
// Returns list of session metadata from corpus callosum
func QueryAGMSessions(workspace string, filters map[string]string) ([]map[string]interface{}, error) {
	if !isCorpusCallosumAvailable() {
		return []map[string]interface{}{}, nil
	}

	// Build query command
	args := []string{
		"query",
		"--workspace", workspace,
		"--component", "agm",
		"--entity", "session",
		"--format", "json",
	}

	// Add filters
	for key, value := range filters {
		args = append(args, "--filter", fmt.Sprintf("%s=%s", key, value))
	}

	cmd := exec.Command("cc", args...)
	output, err := cmd.Output()
	if err != nil {
		// No results or error - return empty list
		return []map[string]interface{}{}, nil
	}

	var sessions []map[string]interface{}
	if err := json.Unmarshal(output, &sessions); err != nil {
		return []map[string]interface{}{}, fmt.Errorf("failed to parse AGM sessions: %w", err)
	}

	return sessions, nil
}

// GetCurrentAGMSession retrieves the current AGM session for this workspace
// Returns session metadata or nil if not found
func GetCurrentAGMSession(workspace string) (map[string]interface{}, error) {
	sessions, err := QueryAGMSessions(workspace, map[string]string{
		"state": "READY",
	})
	if err != nil {
		return nil, err
	}

	if len(sessions) == 0 {
		return nil, nil
	}

	// Return most recently updated session
	// Assumes cc query returns results sorted by updated_at descending
	return sessions[0], nil
}

// QueryEngramBeads queries Engram beads in the current workspace
// Returns list of bead metadata from corpus callosum
func QueryEngramBeads(workspace string, filters map[string]string) ([]map[string]interface{}, error) {
	if !isCorpusCallosumAvailable() {
		return []map[string]interface{}{}, nil
	}

	args := []string{
		"query",
		"--workspace", workspace,
		"--component", "engram",
		"--entity", "bead",
		"--format", "json",
	}

	for key, value := range filters {
		args = append(args, "--filter", fmt.Sprintf("%s=%s", key, value))
	}

	cmd := exec.Command("cc", args...)
	output, err := cmd.Output()
	if err != nil {
		return []map[string]interface{}{}, nil
	}

	var beads []map[string]interface{}
	if err := json.Unmarshal(output, &beads); err != nil {
		return []map[string]interface{}{}, fmt.Errorf("failed to parse Engram beads: %w", err)
	}

	return beads, nil
}

// GetBeadsBySession retrieves all beads associated with a specific session
func GetBeadsBySession(workspace, sessionID string) ([]map[string]interface{}, error) {
	return QueryEngramBeads(workspace, map[string]string{
		"session_id": sessionID,
	})
}

// GetOpenBeads retrieves all open beads in the workspace
func GetOpenBeads(workspace string) ([]map[string]interface{}, error) {
	return QueryEngramBeads(workspace, map[string]string{
		"status": "open",
	})
}

// QueryWayfinderProjects queries Wayfinder projects in the current workspace
// Returns list of project metadata from corpus callosum
func QueryWayfinderProjects(workspace string, filters map[string]string) ([]map[string]interface{}, error) {
	if !isCorpusCallosumAvailable() {
		return []map[string]interface{}{}, nil
	}

	args := []string{
		"query",
		"--workspace", workspace,
		"--component", "wayfinder",
		"--entity", "project",
		"--format", "json",
	}

	for key, value := range filters {
		args = append(args, "--filter", fmt.Sprintf("%s=%s", key, value))
	}

	cmd := exec.Command("cc", args...)
	output, err := cmd.Output()
	if err != nil {
		return []map[string]interface{}{}, nil
	}

	var projects []map[string]interface{}
	if err := json.Unmarshal(output, &projects); err != nil {
		return []map[string]interface{}{}, fmt.Errorf("failed to parse Wayfinder projects: %w", err)
	}

	return projects, nil
}

// GetProjectBySession retrieves a Wayfinder project by session ID
func GetProjectBySession(workspace, sessionID string) (map[string]interface{}, error) {
	projects, err := QueryWayfinderProjects(workspace, map[string]string{
		"session_id": sessionID,
	})
	if err != nil {
		return nil, err
	}

	if len(projects) == 0 {
		return nil, nil
	}

	return projects[0], nil
}

// GetActiveProjects retrieves all active (in_progress) Wayfinder projects
func GetActiveProjects(workspace string) ([]map[string]interface{}, error) {
	return QueryWayfinderProjects(workspace, map[string]string{
		"status": "in_progress",
	})
}

// QueryPhases queries Wayfinder phases for a specific session
func QueryPhases(workspace, sessionID string) ([]map[string]interface{}, error) {
	if !isCorpusCallosumAvailable() {
		return []map[string]interface{}{}, nil
	}

	args := []string{
		"query",
		"--workspace", workspace,
		"--component", "wayfinder",
		"--entity", "phase",
		"--filter", fmt.Sprintf("session_id=%s", sessionID),
		"--format", "json",
	}

	cmd := exec.Command("cc", args...)
	output, err := cmd.Output()
	if err != nil {
		return []map[string]interface{}{}, nil
	}

	var phases []map[string]interface{}
	if err := json.Unmarshal(output, &phases); err != nil {
		return []map[string]interface{}{}, fmt.Errorf("failed to parse phases: %w", err)
	}

	return phases, nil
}

// GetCurrentPhase retrieves the current in-progress phase for a session
func GetCurrentPhase(workspace, sessionID string) (map[string]interface{}, error) {
	phases, err := QueryPhases(workspace, sessionID)
	if err != nil {
		return nil, err
	}

	// Find in_progress phase
	for _, phase := range phases {
		if status, ok := phase["status"].(string); ok && status == "in_progress" {
			return phase, nil
		}
	}

	return nil, nil
}

// CrossComponentQuery performs a cross-component query across multiple tools
// Returns aggregated results from AGM, Engram, and Wayfinder
func CrossComponentQuery(workspace string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Query AGM session
	agmSession, err := GetCurrentAGMSession(workspace)
	if err == nil && agmSession != nil {
		result["agm_session"] = agmSession
	}

	// Query Wayfinder projects
	projects, err := GetActiveProjects(workspace)
	if err == nil {
		result["active_projects"] = projects
	}

	// Query open beads
	beads, err := GetOpenBeads(workspace)
	if err == nil {
		result["open_beads"] = beads
	}

	return result, nil
}

// DiscoverComponents discovers all registered components in the workspace
// Returns list of component names
func DiscoverComponents(workspace string) ([]string, error) {
	if !isCorpusCallosumAvailable() {
		return []string{}, nil
	}

	cmd := exec.Command("cc", "discover", "--workspace", workspace, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return []string{}, nil
	}

	var components []struct {
		Component string `json:"component"`
	}

	if err := json.Unmarshal(output, &components); err != nil {
		return []string{}, nil
	}

	names := make([]string, len(components))
	for i, c := range components {
		names[i] = c.Component
	}

	return names, nil
}
