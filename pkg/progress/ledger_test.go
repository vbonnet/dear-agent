package progress

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// Test helpers
func setupTestRepo(t *testing.T) string {
	// Create a temporary directory for the test repo
	tmpDir := t.TempDir()

	// Initialize git repo
	cmd := gittest.Command(t, tmpDir, "init")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}
	gittest.HardenRepo(t, tmpDir)

	// Configure git user (required for commits by the code under test, which
	// runs git outside the sandbox).
	for _, cfg := range [][]string{
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		cmd := gittest.Command(t, tmpDir, cfg...)
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to configure git: %v", err)
		}
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = gittest.Command(t, tmpDir, "add", "test.txt")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}

	cmd = gittest.Command(t, tmpDir, "commit", "-m", "Initial commit")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	return tmpDir
}

func TestLedgerCommit(t *testing.T) {
	repoPath := setupTestRepo(t)

	ledger := NewLedger(repoPath, "test-bead-123")

	// Record a progress milestone
	sha, err := ledger.Commit("design", "test-design-phase", "done", "Completed API design review")
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	if sha == "" {
		t.Fatal("Expected non-empty SHA")
	}

	// Verify commit was created
	cmd := gittest.Command(t, repoPath, "log", "--oneline", "-1")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get git log: %v", err)
	}

	logLine := strings.TrimSpace(string(output))
	if !strings.Contains(logLine, "progress: design phase completed") {
		t.Fatalf("Expected commit message not found in log: %s", logLine)
	}
}

func TestLedgerLastPhase(t *testing.T) {
	repoPath := setupTestRepo(t)

	ledger := NewLedger(repoPath, "test-bead-456")

	// Commit a milestone
	_, err := ledger.Commit("exploration", "test-explore-phase", "in-progress", "Starting codebase exploration")
	if err != nil {
		t.Fatalf("First commit failed: %v", err)
	}

	// Give it a moment to ensure different timestamp
	time.Sleep(100 * time.Millisecond)

	// Commit another milestone
	_, err = ledger.Commit("implementation", "test-impl-phase", "done", "Implementation phase complete")
	if err != nil {
		t.Fatalf("Second commit failed: %v", err)
	}

	// Query for last phase
	phase, milestone, status, ts, sha, err := ledger.LastPhase()
	if err != nil {
		t.Fatalf("LastPhase failed: %v", err)
	}

	if phase != "implementation" {
		t.Fatalf("Expected phase 'implementation', got '%s'", phase)
	}

	if milestone != "test-impl-phase" {
		t.Fatalf("Expected milestone 'test-impl-phase', got '%s'", milestone)
	}

	if status != "done" {
		t.Fatalf("Expected status 'done', got '%s'", status)
	}

	if sha == "" {
		t.Fatal("Expected non-empty SHA")
	}

	if ts.IsZero() {
		t.Fatal("Expected non-zero timestamp")
	}
}

func TestLedgerQueryLog(t *testing.T) {
	repoPath := setupTestRepo(t)

	ledger := NewLedger(repoPath, "test-bead-789")

	// Commit multiple milestones
	milestones := []struct {
		phase     string
		milestone string
		status    string
	}{
		{"exploration", "test-phase-1", "done"},
		{"design", "test-phase-2", "in-progress"},
		{"implementation", "test-phase-3", "done"},
	}

	for _, m := range milestones {
		_, err := ledger.Commit(m.phase, m.milestone, m.status, "")
		if err != nil {
			t.Fatalf("Commit failed: %v", err)
		}
		time.Sleep(50 * time.Millisecond) // Ensure different timestamps
	}

	// Query all progress commits
	entries, err := ledger.QueryLog("")
	if err != nil {
		t.Fatalf("QueryLog failed: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	// Verify entries (in reverse chronological order)
	expected := []struct {
		phase     string
		milestone string
		status    string
	}{
		{"implementation", "test-phase-3", "done"},
		{"design", "test-phase-2", "in-progress"},
		{"exploration", "test-phase-1", "done"},
	}

	for i, exp := range expected {
		if entries[i].Phase != exp.phase {
			t.Fatalf("Entry %d: expected phase '%s', got '%s'", i, exp.phase, entries[i].Phase)
		}
		if entries[i].Milestone != exp.milestone {
			t.Fatalf("Entry %d: expected milestone '%s', got '%s'", i, exp.milestone, entries[i].Milestone)
		}
		if entries[i].Status != exp.status {
			t.Fatalf("Entry %d: expected status '%s', got '%s'", i, exp.status, entries[i].Status)
		}
		if entries[i].BeadID != "test-bead-789" {
			t.Fatalf("Entry %d: expected bead ID 'test-bead-789', got '%s'", i, entries[i].BeadID)
		}
	}
}

func TestLedgerEmptyRepo(t *testing.T) {
	repoPath := setupTestRepo(t)

	ledger := NewLedger(repoPath, "test-bead-empty")

	// Query before any progress commits
	phase, milestone, status, ts, sha, err := ledger.LastPhase()
	if err != nil {
		t.Fatalf("LastPhase should not error on empty ledger: %v", err)
	}

	if phase != "" || milestone != "" || status != "" {
		t.Fatalf("Expected empty values, got phase=%s, milestone=%s, status=%s", phase, milestone, status)
	}

	if !ts.IsZero() || sha != "" {
		t.Fatalf("Expected zero timestamp and empty SHA, got ts=%v, sha=%s", ts, sha)
	}
}

func TestLedgerValidation(t *testing.T) {
	repoPath := setupTestRepo(t)

	ledger := NewLedger(repoPath, "test-bead-validation")

	tests := []struct {
		name      string
		phase     string
		milestone string
		status    string
		wantErr   bool
	}{
		{"valid", "design", "phase-1", "done", false},
		{"missing phase", "", "phase-1", "done", true},
		{"missing milestone", "design", "", "done", true},
		{"missing status", "design", "phase-1", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ledger.Commit(tt.phase, tt.milestone, tt.status, "")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Commit validation: expected error=%v, got err=%v", tt.wantErr, err)
			}
		})
	}
}

func TestLedgerTrailerFormatting(t *testing.T) {
	repoPath := setupTestRepo(t)

	ledger := NewLedger(repoPath, "test-bead-format")

	// Commit with all details
	_, err := ledger.Commit("verification", "test-verify-phase", "blocked", "Waiting for code review approval")
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Read commit message directly from git
	cmd := gittest.Command(t, repoPath, "log", "-1", "--format=%B")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to read commit message: %v", err)
	}

	msg := string(output)

	// Verify all trailers are present
	expectedTrailers := []string{
		"Progress-Phase: verification",
		"Progress-Milestone: test-verify-phase",
		"Progress-Status: blocked",
		"Bead-ID: test-bead-format",
	}

	for _, trailer := range expectedTrailers {
		if !strings.Contains(msg, trailer) {
			t.Fatalf("Expected trailer '%s' not found in commit message:\n%s", trailer, msg)
		}
	}

	// Verify Progress-Timestamp format (ISO8601)
	if !strings.Contains(msg, "Progress-Timestamp:") {
		t.Fatal("Expected Progress-Timestamp trailer not found")
	}

	// Check timestamp format (rough check for ISO8601)
	for line := range strings.SplitSeq(msg, "\n") {
		if after, ok := strings.CutPrefix(line, "Progress-Timestamp:"); ok {
			ts := strings.TrimSpace(after)
			if _, err := time.Parse(time.RFC3339, ts); err != nil {
				t.Fatalf("Progress-Timestamp is not valid RFC3339: %s (error: %v)", ts, err)
			}
		}
	}
}

func TestLedgerMultipleLedgers(t *testing.T) {
	repoPath := setupTestRepo(t)

	// Create two ledgers for different beads in same repo
	ledger1 := NewLedger(repoPath, "bead-1")
	ledger2 := NewLedger(repoPath, "bead-2")

	// Commit to ledger1
	_, err := ledger1.Commit("design", "bead-1-phase", "done", "")
	if err != nil {
		t.Fatalf("Ledger1 commit failed: %v", err)
	}

	// Commit to ledger2
	_, err = ledger2.Commit("implementation", "bead-2-phase", "done", "")
	if err != nil {
		t.Fatalf("Ledger2 commit failed: %v", err)
	}

	// Both should be able to query all commits
	entries1, err := ledger1.QueryLog("")
	if err != nil {
		t.Fatalf("QueryLog1 failed: %v", err)
	}

	entries2, err := ledger2.QueryLog("")
	if err != nil {
		t.Fatalf("QueryLog2 failed: %v", err)
	}

	// Both should see 2 progress commits
	if len(entries1) != 2 {
		t.Fatalf("Ledger1: expected 2 entries, got %d", len(entries1))
	}

	if len(entries2) != 2 {
		t.Fatalf("Ledger2: expected 2 entries, got %d", len(entries2))
	}

	// Verify both ledgers can see entries tagged with different beads
	// Each ledger has its own beadID but both should see the bead ID in the entry
	found1In1 := false
	found2In2 := false

	for _, e := range entries1 {
		if e.BeadID == "bead-1" {
			found1In1 = true
		}
	}

	for _, e := range entries2 {
		if e.BeadID == "bead-2" {
			found2In2 = true
		}
	}

	if !found1In1 {
		t.Fatal("Ledger1 should see bead-1 entry")
	}

	if !found2In2 {
		t.Fatal("Ledger2 should see bead-2 entry")
	}
}
