package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BeadIDFromSession extracts the bead ID from an AGM session name.
// Examples: "worker-ce-11fi" → "ce-11fi", "worker_ce-z50t" → "ce-z50t".
// Returns "" for non-worker sessions (orchestrator, overseer, meta-*).
func BeadIDFromSession(name string) string {
	lower := strings.ToLower(name)
	for _, role := range SupervisorPatterns() {
		if strings.Contains(lower, role) {
			return ""
		}
	}
	for _, prefix := range []string{"worker-", "worker_"} {
		if strings.HasPrefix(lower, prefix) {
			return name[len(prefix):]
		}
	}
	return ""
}

// beadShowResult is the JSON shape returned by `bd show --json`.
type beadShowResult struct {
	Status string `json:"status"`
}

// beadDBPath returns the canonical path to the context-engine bead database.
func beadDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "beads", "context-engine", ".beads")
}

// queryBeadState runs `bd --db <db> show <id> --json` and returns the status field.
// Returns "not-found" when the bead does not exist.
func queryBeadState(id string) (string, error) {
	cmd := exec.Command("bd", "--db", beadDBPath(), "show", id, "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.ToLower(string(out))
		if strings.Contains(text, "not found") || strings.Contains(text, "no such") {
			return "not-found", nil
		}
		return "", fmt.Errorf("bd show %s: %w (%s)", id, err, strings.TrimSpace(string(out)))
	}
	var result beadShowResult
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("bd show %s: parse JSON: %w", id, err)
	}
	if result.Status == "" {
		return "unknown", nil
	}
	return strings.ToLower(result.Status), nil
}

// ClassifyGhostWorker determines whether a worker session is a ghost.
//
// A session is a GHOST when its bead is closed or done and it has been alive
// at least as long as threshold. A zero threshold means any closed/done bead
// is immediately classified as GHOST.
//
// Returns "GHOST", "ACTIVE", or "UNKNOWN".
func ClassifyGhostWorker(session, beadState string, age time.Duration, threshold time.Duration) string {
	switch beadState {
	case "closed", "done":
		if age >= threshold {
			return "GHOST"
		}
		return "ACTIVE" // still within grace period
	case "open":
		return "ACTIVE"
	default:
		return "UNKNOWN"
	}
}

// RunGhostWorkerCheck classifies all worker-* sessions by their bead state and
// logs any GHOSTs (sessions whose bead is closed/done but the tmux session is
// still alive). The action is observe-and-escalate only — no auto-archive.
//
// sessions is the list of live session names (from tmux or AGM list).
// threshold is the minimum age before a closed-bead session is declared a ghost
// (pass 0 to flag all closed/done beads immediately).
// Returns the names of ghost sessions.
func RunGhostWorkerCheck(ctx context.Context, sessions []string, threshold time.Duration, dryRun bool) ([]string, error) {
	var ghosts []string
	for _, name := range sessions {
		if ctx.Err() != nil {
			return ghosts, ctx.Err()
		}
		beadID := BeadIDFromSession(name)
		if beadID == "" {
			continue
		}
		state, err := queryBeadState(beadID)
		if err != nil {
			slog.Warn("ghost-worker: bead query failed",
				"session", name,
				"bead", beadID,
				"err", err,
			)
			continue
		}
		// Use age=0 with the caller-supplied threshold; callers with richer
		// session metadata can pre-filter by age before calling this function.
		class := ClassifyGhostWorker(name, state, 0, threshold)
		if class == "GHOST" {
			slog.Warn("ghost-worker: GHOST detected",
				"session", name,
				"bead_id", beadID,
				"bead_state", state,
				"action", "observe+escalate only — no auto-archive",
			)
			ghosts = append(ghosts, name)
		}
	}
	return ghosts, nil
}
