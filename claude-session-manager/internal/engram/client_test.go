package engram

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// Helper for mocking exec.Command
var testExecCommand = exec.Command

func mockExecCommand(stdout string, stderr string, exitCode int) func(string, ...string) *exec.Cmd {
	return func(command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{
			"GO_WANT_HELPER_PROCESS=1",
			fmt.Sprintf("MOCK_STDOUT=%s", stdout),
			fmt.Sprintf("MOCK_STDERR=%s", stderr),
			fmt.Sprintf("MOCK_EXIT_CODE=%d", exitCode),
		}
		return cmd
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	stdout := os.Getenv("MOCK_STDOUT")
	stderr := os.Getenv("MOCK_STDERR")
	exitCode := 0
	if code := os.Getenv("MOCK_EXIT_CODE"); code != "" {
		fmt.Sscanf(code, "%d", &exitCode)
	}

	if stdout != "" {
		fmt.Fprint(os.Stdout, stdout)
	}
	if stderr != "" {
		fmt.Fprint(os.Stderr, stderr)
	}
	os.Exit(exitCode)
}

func TestIsAvailable_BinaryInPath(t *testing.T) {
	// This test assumes 'engram' or a test binary exists
	// In real testing, you'd mock exec.LookPath
	cfg := EngramConfig{BinaryPath: ""}
	client := NewClient(cfg).(*cliClient)

	// Test will pass if engram in PATH, otherwise fail gracefully
	available := client.IsAvailable()
	t.Logf("Engram available in PATH: %v", available)
}

func TestParseResults_ValidJSON(t *testing.T) {
	jsonData := `[
		{
			"path": "/test/engram.md",
			"title": "Test",
			"score": 0.95,
			"tags": ["test"],
			"content": "Test content",
			"hash": "sha256:abc123"
		}
	]`

	results, err := parseResults([]byte(jsonData))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0].Title != "Test" {
		t.Errorf("Expected Title=Test, got %s", results[0].Title)
	}
	if results[0].Score != 0.95 {
		t.Errorf("Expected Score=0.95, got %.2f", results[0].Score)
	}
}

func TestParseResults_EmptyArray(t *testing.T) {
	jsonData := `[]`
	results, err := parseResults([]byte(jsonData))
	if err != nil {
		t.Fatalf("Expected no error for empty array, got %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestParseResults_MalformedJSON(t *testing.T) {
	jsonData := `{"error": "not an array"}`
	results, err := parseResults([]byte(jsonData))
	if err == nil {
		t.Errorf("Expected error for malformed JSON")
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results on error, got %d", len(results))
	}
}

func TestParseResults_InvalidRecords(t *testing.T) {
	jsonData := `[
		{"title": "Missing hash and content"},
		{"hash": "sha256:abc", "content": "Valid", "title": "Valid"}
	]`

	results, err := parseResults([]byte(jsonData))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	// Should filter out invalid record (missing hash/content)
	if len(results) != 1 {
		t.Errorf("Expected 1 valid result after filtering, got %d", len(results))
	}
	if results[0].Title != "Valid" {
		t.Errorf("Expected valid record to remain")
	}
}

func TestFilterByScore(t *testing.T) {
	results := []EngramResult{
		{Score: 0.95, Title: "High"},
		{Score: 0.65, Title: "Low"},
		{Score: 0.75, Title: "Medium"},
	}

	filtered := filterByScore(results, 0.7)
	if len(filtered) != 2 {
		t.Errorf("Expected 2 results with score ≥0.7, got %d", len(filtered))
	}
	for _, r := range filtered {
		if r.Score < 0.7 {
			t.Errorf("Expected all filtered results to have score ≥0.7, got %.2f", r.Score)
		}
	}
}

func TestFilterByScore_AllBelowThreshold(t *testing.T) {
	results := []EngramResult{
		{Score: 0.5, Title: "Low1"},
		{Score: 0.6, Title: "Low2"},
	}

	filtered := filterByScore(results, 0.7)
	if len(filtered) != 0 {
		t.Errorf("Expected 0 results when all below threshold, got %d", len(filtered))
	}
}
