package quality

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Baseline represents a snapshot of go vet issue counts.
type Baseline struct {
	IssueCount  int      `json:"issue_count"`
	Timestamp   string   `json:"timestamp"`
	GoVetOutput []string `json:"go_vet_output,omitempty"`
	CommitHash  string   `json:"commit_hash,omitempty"`
}

// RunGoVet executes go vet ./... on the given repo path and returns output lines.
func RunGoVet(repoPath string) ([]string, error) {
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	lines := parseOutputLines(string(output))
	if err != nil {
		// go vet returns exit code 1 when issues are found; that's expected
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return lines, nil
		}
		return nil, fmt.Errorf("failed to run go vet: %w", err)
	}
	return lines, nil
}

// parseOutputLines splits output into non-empty trimmed lines.
func parseOutputLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// CountIssues returns the number of issues from go vet output lines.
func CountIssues(vetOutput []string) int {
	return len(vetOutput)
}

// LoadBaseline reads a baseline from a JSON file.
func LoadBaseline(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read baseline file: %w", err)
	}
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("failed to parse baseline JSON: %w", err)
	}
	return &b, nil
}

// SaveBaseline writes a baseline to a JSON file.
func SaveBaseline(path string, b *Baseline) error {
	if b.Timestamp == "" {
		b.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal baseline: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write baseline file: %w", err)
	}
	return nil
}

// CheckRegression returns an error if the current issue count exceeds the baseline.
func CheckRegression(current int, baseline *Baseline) error {
	if current > baseline.IssueCount {
		return fmt.Errorf("regression detected: %d issues (baseline: %d, increase: +%d)",
			current, baseline.IssueCount, current-baseline.IssueCount)
	}
	return nil
}
