// Package beads provides a client for the bd CLI bead operations.
package beads

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Client wraps the bd CLI for bead operations.
type Client struct {
	bdPath string
}

// NewClient creates a new Client, locating the bd CLI on the system PATH.
func NewClient() *Client {
	path, err := exec.LookPath("bd")
	if err != nil {
		path = "" // bd CLI not available
	}
	return &Client{bdPath: path}
}

// IsAvailable returns true if the bd CLI was found on the system PATH.
func (c *Client) IsAvailable() bool {
	return c.bdPath != ""
}

// GetBeadByUUID queries bd list --label "uuid:<uuid>" and returns first bead ID
func (c *Client) GetBeadByUUID(uuid string) (string, error) {
	if !c.IsAvailable() {
		return "", fmt.Errorf("bd CLI not available")
	}

	cmd := exec.CommandContext(context.Background(), c.bdPath, "list", "--label", fmt.Sprintf("uuid:%s", uuid)) //nolint:gosec // bdPath from exec.LookPath, args controlled
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bd list failed: %w", err)
	}

	// Parse JSON array response
	var beads []BeadSummary
	if err := json.Unmarshal(output, &beads); err != nil {
		return "", fmt.Errorf("JSON parse failed: %w", err)
	}

	if len(beads) == 0 {
		return "", nil // No bead found (not an error)
	}

	return beads[0].ID, nil
}

// GetBeadTitle fetches bead title using bd show
func (c *Client) GetBeadTitle(beadID string) (string, error) {
	cmd := exec.CommandContext(context.Background(), c.bdPath, "show", beadID) //nolint:gosec // bdPath from exec.LookPath, args controlled
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bd show failed: %w", err)
	}

	// Parse output to extract "Title: ..." line
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Title:") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "Title:"))
			return title, nil
		}
	}

	return "", fmt.Errorf("title not found in bd show output")
}

// CreateAutoBean creates a new auto-generated bead with metadata labels
func (c *Client) CreateAutoBean(title, description, sessionUUID, priority string) (string, error) {
	if !c.IsAvailable() {
		return "", fmt.Errorf("bd CLI not available")
	}

	args := []string{
		"create",
		"--title", title,
		"--type", "feature",
		"--priority", priority,
		"--label", "auto-created",
		"--label", fmt.Sprintf("uuid:%s", sessionUUID),
		"--label", "session-end",
		"--description", description,
	}

	cmd := exec.CommandContext(context.Background(), c.bdPath, args...) //nolint:gosec // bdPath from exec.LookPath, args controlled
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bd create failed: %w", err)
	}

	// Parse output to extract bead ID
	// Expected format: "Created bead: bd-123"
	outputStr := strings.TrimSpace(string(output))
	parts := strings.Split(outputStr, " ")
	if len(parts) >= 3 {
		return parts[2], nil
	}

	return "", fmt.Errorf("failed to parse bead ID from output: %s", outputStr)
}

// ListBeadsBySession returns all beads associated with a session UUID.
func (c *Client) ListBeadsBySession(sessionUUID string) ([]BeadSummary, error) {
	if !c.IsAvailable() {
		return nil, fmt.Errorf("bd CLI not available")
	}

	cmd := exec.CommandContext(context.Background(), c.bdPath, "list", "--label", fmt.Sprintf("uuid:%s", sessionUUID)) //nolint:gosec // bdPath from exec.LookPath, args controlled
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd list failed: %w", err)
	}

	var beadList []BeadSummary
	if err := json.Unmarshal(output, &beadList); err != nil {
		return nil, fmt.Errorf("JSON parse failed: %w", err)
	}

	return beadList, nil
}

// UpdateBeadDescription updates an existing bead's description
func (c *Client) UpdateBeadDescription(beadID, description string) error {
	if !c.IsAvailable() {
		return fmt.Errorf("bd CLI not available")
	}

	cmd := exec.CommandContext(context.Background(), c.bdPath, "update", beadID, "--description", description) //nolint:gosec // bdPath from exec.LookPath, args controlled
	_, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("bd update failed: %w", err)
	}

	return nil
}
