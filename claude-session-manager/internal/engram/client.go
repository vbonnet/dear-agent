package engram

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// EngramResult represents a single engram result from retrieval
type EngramResult struct {
	Path    string   `json:"path"`
	Title   string   `json:"title"`
	Score   float64  `json:"score"`
	Tags    []string `json:"tags"`
	Content string   `json:"content"`
	Hash    string   `json:"hash"`
}

// Client provides interface for Engram CLI interaction
type Client interface {
	Query(query string, tags []string) ([]EngramResult, error)
	IsAvailable() bool
}

type cliClient struct {
	config      EngramConfig
	execCommand func(string, ...string) *exec.Cmd
}

// NewClient creates a new Engram CLI client
func NewClient(cfg EngramConfig) Client {
	return &cliClient{
		config:      cfg,
		execCommand: exec.Command,
	}
}

// IsAvailable checks if Engram binary is available
func (c *cliClient) IsAvailable() bool {
	binaryPath := c.config.BinaryPath
	if binaryPath == "" {
		path, err := exec.LookPath("engram")
		if err != nil {
			return false
		}
		binaryPath = path
	}

	// Verify binary exists and is executable
	info, err := os.Stat(binaryPath)
	if err != nil {
		return false
	}

	return info.Mode()&0111 != 0
}

// Query executes Engram retrieval with given query and tags
func (c *cliClient) Query(query string, tags []string) ([]EngramResult, error) {
	// Check availability first
	if !c.IsAvailable() {
		return nil, fmt.Errorf("engram binary not found (install or set AGM_ENGRAM_PATH)")
	}

	// Build command args
	args := []string{"retrieve", query, "--format", "json", "--limit", strconv.Itoa(c.config.Limit)}
	for _, tag := range tags {
		args = append(args, "--tag", tag)
	}

	// Execute with timeout
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Timeout)
	defer cancel()

	cmd := c.execCommand("engram", args...)
	cmdWithCtx := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)

	output, err := cmdWithCtx.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("engram query timed out after %v", c.config.Timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("engram command failed: %w", err)
	}

	// Parse JSON
	results, err := parseResults(output)
	if err != nil {
		return nil, err
	}

	// Filter by score
	filtered := filterByScore(results, c.config.ScoreThreshold)
	return filtered, nil
}

// parseResults parses JSON output from Engram CLI
func parseResults(data []byte) ([]EngramResult, error) {
	if len(data) == 0 {
		return []EngramResult{}, nil
	}

	var results []EngramResult
	if err := json.Unmarshal(data, &results); err != nil {
		return []EngramResult{}, fmt.Errorf("failed to parse engram JSON: %w", err)
	}

	// Filter out invalid results (missing required fields)
	valid := []EngramResult{}
	for _, r := range results {
		if r.Hash != "" && r.Content != "" {
			valid = append(valid, r)
		}
	}

	return valid, nil
}

// filterByScore filters results by minimum score threshold
func filterByScore(results []EngramResult, threshold float64) []EngramResult {
	filtered := []EngramResult{}
	for _, r := range results {
		if r.Score >= threshold {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
