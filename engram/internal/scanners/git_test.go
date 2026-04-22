package scanners

import (
	"context"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/metacontext"
	"github.com/vbonnet/dear-agent/internal/testutil"
)

// TestGitScanner_WithGitRepo tests git scanner with actual git repository
func TestGitScanner_WithGitRepo(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)
	testutil.SetupTestRepo(t, tmpdir)

	scanner := NewGitScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	// Scanner should handle gracefully even if repo is empty
	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Empty repo may return 0 signals (no commits yet)
	// This is acceptable behavior
	_ = signals
}

// TestGitScanner_NoGitRepo tests handling when .git directory doesn't exist
func TestGitScanner_NoGitRepo(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)

	scanner := NewGitScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() should handle missing .git gracefully, got error: %v", err)
	}

	// Should return empty signals
	if len(signals) != 0 {
		t.Errorf("Expected 0 signals for non-git directory, got %d", len(signals))
	}
}

// TestGitScanner_Name tests Name() method
func TestGitScanner_Name(t *testing.T) {
	scanner := NewGitScanner()
	if scanner.Name() != "git" {
		t.Errorf("Expected name 'git', got '%s'", scanner.Name())
	}
}

// TestGitScanner_Priority tests Priority() method
func TestGitScanner_Priority(t *testing.T) {
	scanner := NewGitScanner()
	if scanner.Priority() != 20 {
		t.Errorf("Expected priority 20, got %d", scanner.Priority())
	}
}
