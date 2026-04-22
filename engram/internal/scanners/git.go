package scanners

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/engram/internal/metacontext"
)

// GitScanner analyzes git history for project signals.
// Priority: 20 (low, supplementary information).
type GitScanner struct {
	name     string
	priority int
}

// NewGitScanner creates a new GitScanner.
func NewGitScanner() *GitScanner {
	return &GitScanner{
		name:     "git",
		priority: 20,
	}
}

func (s *GitScanner) Name() string {
	return s.name
}

func (s *GitScanner) Priority() int {
	return s.priority
}

// Scan analyzes git repository for signals.
func (s *GitScanner) Scan(ctx context.Context, req *metacontext.AnalyzeRequest) ([]metacontext.Signal, error) {
	// Check if .git directory exists
	gitDir := filepath.Join(req.WorkingDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// Not a git repo, skip gracefully
		return []metacontext.Signal{}, nil
	}

	signals := []metacontext.Signal{}

	// Run git log --oneline -n 10
	// Security: exec.CommandContext prevents shell injection (E1)
	// Only WorkingDir from user input (already validated)
	cmd := exec.CommandContext(ctx, "git", "log", "--oneline", "-n", "10")
	cmd.Dir = req.WorkingDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Graceful degradation: corrupted repo, detached HEAD, etc.
		// Log warning, emit telemetry, but continue analysis
		return signals, nil
	}

	// Parse output for signals (currently minimal, could extract commit patterns)
	lines := strings.Split(string(output), "\n")
	commitCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			commitCount++
		}
	}

	// Add metadata signal (not a primary signal, just context)
	if commitCount > 0 {
		signals = append(signals, metacontext.Signal{
			Name:       "Git",
			Confidence: 1.0,
			Source:     "git",
			Metadata: map[string]string{
				"recent_commits": fmt.Sprintf("%d", commitCount),
			},
		})
	}

	return signals, nil
}
