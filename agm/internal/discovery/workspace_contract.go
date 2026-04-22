// Package discovery provides discovery functionality.
package discovery

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// WorkspaceInfo represents workspace information from workspace CLI contract
type WorkspaceInfo struct {
	Name            string         `json:"name"`
	Root            string         `json:"root"`
	Enabled         bool           `json:"enabled"`
	OutputDir       string         `json:"output_dir"`
	DetectionMethod string         `json:"detection_method"`
	Confidence      float64        `json:"confidence"`
	Settings        map[string]any `json:"settings"`
}

// DetectWorkspaceUsingContract uses the workspace CLI contract to detect current workspace
// This replaces direct filesystem scanning with contract-based detection
func DetectWorkspaceUsingContract(pwd string) (*WorkspaceInfo, error) {
	// Check if workspace CLI is available
	_, err := exec.LookPath("workspace")
	if err != nil {
		return nil, fmt.Errorf("workspace CLI not found: %w (install workspace protocol for multi-workspace support)", err)
	}

	// Execute workspace detect command
	args := []string{"detect", "--format=json"}
	if pwd != "" {
		args = append(args, "--pwd="+pwd)
	}

	cmd := exec.Command("workspace", args...)
	output, err := cmd.Output()
	if err != nil {
		// Graceful degradation: no workspace detected
		return nil, fmt.Errorf("workspace detection failed: %w", err)
	}

	// Parse JSON output
	var info WorkspaceInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("failed to parse workspace CLI output: %w", err)
	}

	return &info, nil
}

// ListWorkspacesUsingContract uses workspace CLI to list all configured workspaces
func ListWorkspacesUsingContract() ([]WorkspaceInfo, error) {
	// Check if workspace CLI is available
	_, err := exec.LookPath("workspace")
	if err != nil {
		return nil, fmt.Errorf("workspace CLI not found: %w", err)
	}

	// Execute workspace list command
	cmd := exec.Command("workspace", "list", "--format=json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("workspace list failed: %w", err)
	}

	// Parse JSON output
	var result struct {
		Workspaces []WorkspaceInfo `json:"workspaces"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse workspace list output: %w", err)
	}

	return result.Workspaces, nil
}

// IsWorkspaceContractAvailable checks if the workspace protocol CLI is installed
func IsWorkspaceContractAvailable() bool {
	_, err := exec.LookPath("workspace")
	return err == nil
}
