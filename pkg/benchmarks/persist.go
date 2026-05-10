package benchmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteResults serializes results as JSON under dir/<run_id>.json. The dir is
// created if it does not exist. Permissions are 0o750 / 0o600 to match the
// rest of the dear-agent codebase.
func WriteResults(results *Results, dir string) (string, error) {
	if results == nil {
		return "", fmt.Errorf("benchmarks: WriteResults requires non-nil results")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create results dir: %w", err)
	}
	path := filepath.Join(dir, results.RunID+".json")
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal results: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write results: %w", err)
	}
	return path, nil
}

// ReadResults reads a Results JSON file written by WriteResults.
func ReadResults(path string) (*Results, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read results: %w", err)
	}
	var r Results
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse results: %w", err)
	}
	return &r, nil
}
