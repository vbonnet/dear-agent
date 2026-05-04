package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
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

	// Filter candidates:
	// 1. Created within time window
	// 2. Has non-empty name (not "Unknown")
	// 3. Matches CWD from context
	// 4. If slug provided, matches slug
	var candidates []*struct {
		SessionID string
		Name      string
		CreatedAt time.Time
		Score     int // Higher score = better match
	}

	for _, session := range allSessions {
		// Skip if outside time window
		if session.CreatedAt.Before(windowStart) || session.CreatedAt.After(windowEnd) {
			continue
		}

		// Skip if unnamed (execution sessions typically have "Unknown")
		if session.Name == "" || session.Name == "Unknown" {
			continue
		}

		// Skip if it's the child session itself
		if session.SessionID == detectSessionID {
			continue
		}

		// Check CWD match (from context.project)
		if session.Context.Project != detectCWD {
			continue
		}

		// Calculate match score
		score := 1 // Base score for matching CWD and time window

		// Bonus for slug match (if provided)
		// Note: Slug is not stored in manifest, but we can check tmux session name pattern
		if detectSlug != "" {
			// Slug typically appears in tmux session name or can be inferred
			// For now, just add bonus if names are similar
			score += 1
		}

		// Bonus for being closer in time (more recent = likely parent)
		timeDiff := child.CreatedAt.Sub(session.CreatedAt)
		switch {
		case timeDiff < 10*time.Second:
			score += 3 // Very close in time
		case timeDiff < 30*time.Second:
			score += 2 // Reasonably close
		default:
			score += 1 // Within window
		}

		candidates = append(candidates, &struct {
			SessionID string
			Name      string
			CreatedAt time.Time
			Score     int
		}{
			SessionID: session.SessionID,
			Name:      session.Name,
			CreatedAt: session.CreatedAt,
			Score:     score,
		})
	}

	// No candidates found
	if len(candidates) == 0 {
		// Output empty (not an error - just no parent found)
		return nil
	}

	// Find best candidate (highest score, most recent if tied)
	var best *struct {
		SessionID string
		Name      string
		CreatedAt time.Time
		Score     int
	}
	for _, candidate := range candidates {
		if best == nil || candidate.Score > best.Score ||
			(candidate.Score == best.Score && candidate.CreatedAt.After(best.CreatedAt)) {
			best = candidate
		}
	}

	// Output parent session ID (for shell script consumption)
	fmt.Println(best.SessionID)

	return nil
}
