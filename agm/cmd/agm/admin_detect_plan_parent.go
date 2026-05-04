package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

var (
	detectSessionID string
	detectSlug      string
	detectCWD       string
	detectTimeout   time.Duration
)

var detectPlanParentCmd = &cobra.Command{
	Use:   "detect-plan-parent",
	Short: "Detect parent planning session for an execution session",
	Long: `Detect parent planning session based on timing, CWD, and slug metadata.

This command searches for a planning session that likely spawned the given
execution session when Claude Code used "Clear Context and Execute Plan".

It looks for sessions created within the timeout window before the given session,
matching on CWD and optionally slug, and having a non-empty name (not "Unknown").`,
	Example: `  # Detect parent for an execution session
  agm admin detect-plan-parent --session-id 80c10f57-d114-477b-ad6e-53cfe23a580f \
    --cwd ~/src --slug reflective-doodling-manatee --timeout 60s

  # Minimal detection (CWD only)
  agm admin detect-plan-parent --session-id 80c10f57-d114-477b-ad6e-53cfe23a580f \
    --cwd ~/src`,
	RunE: runDetectPlanParent,
}

func init() {
	adminCmd.AddCommand(detectPlanParentCmd)

	detectPlanParentCmd.Flags().StringVar(&detectSessionID, "session-id", "",
		"Child session UUID (required)")
	detectPlanParentCmd.Flags().StringVar(&detectSlug, "slug", "",
		"Slug identifier to match (optional, improves accuracy)")
	detectPlanParentCmd.Flags().StringVar(&detectCWD, "cwd", "",
		"Current working directory path (required)")
	detectPlanParentCmd.Flags().DurationVar(&detectTimeout, "timeout",
		60*time.Second, "Maximum time window to search backwards")

	detectPlanParentCmd.MarkFlagRequired("session-id")
	detectPlanParentCmd.MarkFlagRequired("cwd")
}

func runDetectPlanParent(cmd *cobra.Command, args []string) error {
	// Connect to Dolt storage
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	// Get the child session to extract its creation time
	child, err := adapter.GetSession(detectSessionID)
	if err != nil {
		return fmt.Errorf("failed to get child session: %w", err)
	}

	// Search window: child.CreatedAt - timeout to child.CreatedAt
	windowEnd := child.CreatedAt
	windowStart := windowEnd.Add(-detectTimeout)

	// Get all sessions
	allSessions, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	candidates := scoreParentCandidates(allSessions, child.CreatedAt, windowStart, windowEnd)
	if len(candidates) == 0 {
		// Output empty (not an error - just no parent found)
		return nil
	}

	best := pickBestCandidate(candidates)
	// Output parent session ID (for shell script consumption)
	fmt.Println(best.SessionID)

	return nil
}

// detectCandidate is a flattened, scored view of a session used during parent
// detection scoring.
type detectCandidate struct {
	SessionID string
	Name      string
	CreatedAt time.Time
	Score     int
}

// scoreParentCandidates filters allSessions down to those eligible to be a
// planning parent for the orphan and assigns each a relevance score.
func scoreParentCandidates(allSessions []*manifest.Manifest, childCreatedAt, windowStart, windowEnd time.Time) []detectCandidate {
	var candidates []detectCandidate
	for _, session := range allSessions {
		if session.CreatedAt.Before(windowStart) || session.CreatedAt.After(windowEnd) {
			continue
		}
		if session.Name == "" || session.Name == "Unknown" {
			continue
		}
		if session.SessionID == detectSessionID {
			continue
		}
		if session.Context.Project != detectCWD {
			continue
		}
		score := scoreCandidate(session, childCreatedAt)
		candidates = append(candidates, detectCandidate{
			SessionID: session.SessionID,
			Name:      session.Name,
			CreatedAt: session.CreatedAt,
			Score:     score,
		})
	}
	return candidates
}

// scoreCandidate assigns a relevance score for a parent candidate based on
// time proximity and slug-matching heuristics.
func scoreCandidate(session *manifest.Manifest, childCreatedAt time.Time) int {
	score := 1
	if detectSlug != "" {
		score++
	}
	timeDiff := childCreatedAt.Sub(session.CreatedAt)
	switch {
	case timeDiff < 10*time.Second:
		score += 3
	case timeDiff < 30*time.Second:
		score += 2
	default:
		score++
	}
	return score
}

// pickBestCandidate returns the highest-scoring candidate, breaking ties by
// most-recent CreatedAt.
func pickBestCandidate(candidates []detectCandidate) detectCandidate {
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.Score > best.Score ||
			(c.Score == best.Score && c.CreatedAt.After(best.CreatedAt)) {
			best = c
		}
	}
	return best
}
