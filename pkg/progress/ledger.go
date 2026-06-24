package progress

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ProgressEntry represents a single progress milestone from git log.
type ProgressEntry struct {
	Phase      string    // e.g., "exploration", "design", "implementation", "verification", "complete"
	Milestone  string    // e.g., "ce-ek4f-explore-codebase"
	Status     string    // "in-progress", "blocked", "paused", "done"
	Timestamp  time.Time // ISO8601 timestamp from commit
	BeadID     string    // Link to bead (e.g., "ce-ek4f")
	CommitSHA  string    // Git commit SHA for this milestone
	Subject    string    // First line of commit message
}

// Ledger tracks work milestones via git commits.
// Each significant phase is anchored to a commit with structured Progress-* trailers.
type Ledger struct {
	repoPath string // Path to git repository
	beadID   string // Bead ID linking this work to outcome tracking
}

// NewLedger creates a new progress ledger for a git repository.
// repoPath should be the root of a git repository (default: current directory).
// beadID links this ledger to a bead outcome record (e.g., "ce-ek4f").
func NewLedger(repoPath, beadID string) *Ledger {
	if repoPath == "" {
		repoPath = "."
	}
	return &Ledger{
		repoPath: repoPath,
		beadID:   beadID,
	}
}

// Commit records a progress milestone to git.
// phase: current work phase (e.g., "exploration", "design", "implementation")
// milestone: unique milestone ID (e.g., "ce-ek4f-explore-codebase")
// status: state at commit time ("in-progress", "blocked", "paused", "done")
// body: optional commit message body for additional context
// Returns the commit SHA on success.
func (l *Ledger) Commit(phase, milestone, status, body string) (string, error) {
	if phase == "" || milestone == "" || status == "" {
		return "", fmt.Errorf("phase, milestone, and status are required")
	}

	// Build commit message with conventional commit format and Progress-* trailers
	now := time.Now().UTC()
	timestamp := now.Format(time.RFC3339)

	subject := fmt.Sprintf("progress: %s phase completed", phase)

	// Construct full message with trailers (RFC 2822 style)
	msg := subject
	if body != "" {
		msg += "\n\n" + body
	}

	// Add progress trailers
	msg += "\n\nProgress-Phase: " + phase
	msg += "\nProgress-Milestone: " + milestone
	msg += "\nProgress-Status: " + status
	msg += "\nProgress-Timestamp: " + timestamp
	msg += "\nBead-ID: " + l.beadID

	// Execute git commit
	cmd := exec.Command("git", "-C", l.repoPath, "commit", "--allow-empty", "-m", msg)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git commit failed: %w", err)
	}

	// Get the commit SHA of the just-created commit
	sha, err := l.getHeadSHA()
	if err != nil {
		return "", err
	}

	return sha, nil
}

// LastPhase reads git log and returns the last recorded progress state.
// Used on session resume to understand where work left off.
// Returns all zero values if no progress commits are found.
func (l *Ledger) LastPhase() (phase, milestone, status string, timestamp time.Time, sha string, err error) {
	// Query git log for commits with Progress-* trailers
	// Use git log --format with custom format to extract trailers
	cmd := exec.Command("git", "-C", l.repoPath, "log", "--format=%H%n%s%n%b", "-n", "50")
	output, err := cmd.Output()
	if err != nil {
		return "", "", "", time.Time{}, "", fmt.Errorf("git log failed: %w", err)
	}

	// Parse output to find last progress entry
	entries := l.parseLogOutput(string(output))
	if len(entries) == 0 {
		return "", "", "", time.Time{}, "", nil
	}

	last := entries[0] // Most recent is first
	return last.Phase, last.Milestone, last.Status, last.Timestamp, last.CommitSHA, nil
}

// QueryLog returns all progress commits since a given limit.
// If since is empty, returns all progress commits.
// Returns commits in reverse chronological order (newest first).
func (l *Ledger) QueryLog(since string) ([]ProgressEntry, error) {
	args := []string{"-C", l.repoPath, "log", "--format=%H%n%s%n%b"}

	if since != "" {
		// since can be a SHA, branch name, or timestamp range
		// For simplicity, append it as a revision spec
		args = append(args, since+"..HEAD")
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	return l.parseLogOutput(string(output)), nil
}

// getHeadSHA returns the SHA of the HEAD commit
func (l *Ledger) getHeadSHA() (string, error) {
	cmd := exec.Command("git", "-C", l.repoPath, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// parseLogOutput parses git log output and extracts ProgressEntry records.
// Format of input: "<SHA>\n<subject>\n<body>\n...\n<SHA>\n<subject>\n<body>"
// Body contains trailer lines like "Progress-Phase: ..."
func (l *Ledger) parseLogOutput(logOutput string) []ProgressEntry {
	var entries []ProgressEntry

	lines := strings.Split(logOutput, "\n")

	var currentSHA, currentSubject, currentPhase, currentMilestone, currentStatus string
	var currentTimestamp time.Time

	for _, line := range lines {
		// Raw line check for SHA (don't trim, SHA detection should be exact)
		if l.isSHA(line) {
			// If we were building an entry, save it first
			if currentSHA != "" && currentPhase != "" {
				entries = append(entries, ProgressEntry{
					Phase:      currentPhase,
					Milestone:  currentMilestone,
					Status:     currentStatus,
					Timestamp:  currentTimestamp,
					BeadID:     l.beadID,
					CommitSHA:  currentSHA,
					Subject:    currentSubject,
				})
			}

			currentSHA = line
			currentSubject = ""
			currentPhase = ""
			currentMilestone = ""
			currentStatus = ""
			currentTimestamp = time.Time{}
			continue
		}

		if currentSHA == "" {
			// Haven't found a SHA yet
			continue
		}

		line = strings.TrimSpace(line)

		// Skip empty lines in body
		if line == "" {
			continue
		}

		// First non-empty line after SHA is the subject
		if currentSubject == "" {
			currentSubject = line
			continue
		}

		// Parse trailers from body (after subject)
		if strings.HasPrefix(line, "Progress-Phase:") {
			currentPhase = strings.TrimSpace(strings.TrimPrefix(line, "Progress-Phase:"))
		} else if strings.HasPrefix(line, "Progress-Milestone:") {
			currentMilestone = strings.TrimSpace(strings.TrimPrefix(line, "Progress-Milestone:"))
		} else if strings.HasPrefix(line, "Progress-Status:") {
			currentStatus = strings.TrimSpace(strings.TrimPrefix(line, "Progress-Status:"))
		} else if strings.HasPrefix(line, "Progress-Timestamp:") {
			timeStr := strings.TrimSpace(strings.TrimPrefix(line, "Progress-Timestamp:"))
			if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
				currentTimestamp = t
			}
		}
	}

	// Don't forget the last entry
	if currentPhase != "" && currentSHA != "" {
		entries = append(entries, ProgressEntry{
			Phase:      currentPhase,
			Milestone:  currentMilestone,
			Status:     currentStatus,
			Timestamp:  currentTimestamp,
			BeadID:     l.beadID,
			CommitSHA:  currentSHA,
			Subject:    currentSubject,
		})
	}

	return entries
}

// isSHA returns true if the string is a valid Git SHA-1 hash (40 hex chars)
func (l *Ledger) isSHA(s string) bool {
	match, _ := regexp.MatchString(`^[0-9a-f]{40}$`, s)
	return match
}
