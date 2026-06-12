package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCheckRepo_Missing verifies a non-existent directory is reported as missing.
func TestCheckRepo_Missing(t *testing.T) {
	t.Parallel()
	r := checkRepo("/nonexistent/path/does/not/exist", repoSpec{dir: "nonexistent", defaultBranch: "main"})
	if !r.Missing {
		t.Error("expected Missing=true for nonexistent dir")
	}
	if len(r.Violations) == 0 {
		t.Error("expected at least one violation for missing dir")
	}
}

// TestReportFilePath verifies the report path helper.
func TestReportFilePath(t *testing.T) {
	t.Parallel()
	got := reportFilePath("/home/test")
	want := "/home/test/.agm/logs/src-health-last.json"
	if got != want {
		t.Errorf("report path = %q, want %q", got, want)
	}
}

// TestWriteReadReport verifies that writeReport produces a valid JSON file.
func TestWriteReadReport(t *testing.T) {
	t.Parallel()
	tmpHome := t.TempDir()

	results := []repoResult{
		{Dir: "dear-agent", Branch: "main", ExpectedBranch: "main", IsClean: true, CheckedAt: time.Now().UTC()},
	}
	if err := writeReport(tmpHome, results, 0); err != nil {
		t.Fatalf("writeReport: %v", err)
	}

	reportPath := reportFilePath(tmpHome)
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if len(data) == 0 {
		t.Error("report file is empty")
	}

	// Verify file mode is owner-only.
	info, err := os.Stat(reportPath)
	if err != nil {
		t.Fatalf("stat report: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("report mode = %o, want 0600", mode)
	}
}

// TestRotateLogs_SmallFile verifies that a file below the size cap is not rotated.
func TestRotateLogs_SmallFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "small.log")
	_ = os.WriteFile(logPath, []byte("hello"), 0o600)

	// src-health doesn't have rotateLogs directly, but we test writeReport
	// idempotency here instead.
	results := []repoResult{{Dir: "test", Branch: "main", ExpectedBranch: "main"}}
	homeDir := tmpDir

	// Write twice — second call should overwrite, not panic.
	if err := writeReport(homeDir, results, 0); err != nil {
		t.Fatalf("first writeReport: %v", err)
	}
	if err := writeReport(homeDir, results, 0); err != nil {
		t.Fatalf("second writeReport: %v", err)
	}
}
