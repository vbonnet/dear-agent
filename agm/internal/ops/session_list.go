package ops

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// ListSessionsRequest defines the input for listing sessions.
type ListSessionsRequest struct {
	// Status filters by lifecycle: "active" (default), "archived", "all".
	Status string `json:"status,omitempty"`

	// Harness filters by agent type: "claude-code", "gemini-cli", etc.
	Harness string `json:"harness,omitempty"`

	// Tags filters by context tags (all must match).
	Tags []string `json:"tags,omitempty"`

	// ExcludeStopped hides sessions without an active tmux session from
	// results. When true, only sessions with running tmux are shown.
	ExcludeStopped bool `json:"exclude_stopped,omitempty"`

	// Limit caps the number of results (default: 100, max: 1000).
	Limit int `json:"limit,omitempty"`

	// Offset for pagination.
	Offset int `json:"offset,omitempty"`
}

// SessionSummary is the per-session output for list operations.
// Designed for token efficiency — only includes fields agents typically need.
//
// NOTE: State field was removed — it produced false positives (PERMISSION_PROMPT,
// DONE, OFFLINE) that caused cascading bad decisions. State detection will be
// reimplemented with capture-pane-based ground truth. See cross_check.go.
type SessionSummary struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Attached      bool     `json:"attached"`
	Harness       string   `json:"harness"`
	Project       string   `json:"project"`
	Tags          []string `json:"tags,omitempty"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	EstimatedCost float64  `json:"estimated_cost,omitempty"`
}

// ListSessionsResult is the output of ListSessions.
type ListSessionsResult struct {
	Operation           string           `json:"operation"`
	Sessions            []SessionSummary `json:"sessions"`
	Total               int              `json:"total"`
	Limit               int              `json:"limit"`
	Offset              int              `json:"offset"`
	OrphanTmuxSessions  []string         `json:"orphan_tmux_sessions,omitempty"`
}

// ListSessions returns sessions matching the given filters.
func ListSessions(ctx *OpContext, req *ListSessionsRequest) (*ListSessionsResult, error) {
	if req == nil {
		req = &ListSessionsRequest{}
	}

	// Apply defaults
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return nil, ErrInvalidInput("limit", "Limit must be between 1 and 1000.")
	}

	// Build storage filter
	filter := &dolt.SessionFilter{
		Harness: req.Harness,
		Tags:    req.Tags,
		Limit:   limit,
		Offset:  req.Offset,
	}

	switch req.Status {
	case "", "active":
		filter.ExcludeArchived = true
	case "archived":
		filter.Lifecycle = "archived"
	case "all":
		// No filter
	default:
		return nil, ErrInvalidInput("status", "Status must be one of: active, archived, all.")
	}

	manifests, err := ctx.Storage.ListSessions(filter)
	if err != nil {
		return nil, ErrStorageError("list_sessions", err)
	}

	// Fetch tmux session info once (single tmux call) and reuse for
	// both status computation and orphan detection.
	var tmuxSessions []session.SessionInfo
	if ctx.Tmux != nil {
		if ti, ok := ctx.Tmux.(tmuxInfoProvider); ok {
			tmuxSessions, _ = ti.ListSessionsWithInfo()
		}
	}

	// Compute statuses and attachment from cached tmux data
	statuses, attached := computeStatusesFromInfo(manifests, tmuxSessions)

	// Transform to summaries
	sessions := make([]SessionSummary, 0, len(manifests))
	for _, m := range manifests {
		s := toSessionSummary(m, statuses, attached)
		if req.ExcludeStopped && s.Status == "stopped" {
			continue
		}
		sessions = append(sessions, s)
	}

	// Detect orphan tmux sessions (tmux sessions with no AGM counterpart)
	orphans := findOrphanTmuxSessions(manifests, tmuxSessions)

	return &ListSessionsResult{
		Operation:          "list_sessions",
		Sessions:           sessions,
		Total:              len(sessions),
		Limit:              limit,
		Offset:             req.Offset,
		OrphanTmuxSessions: orphans,
	}, nil
}

func toSessionSummary(m *manifest.Manifest, statuses map[string]string, attached map[string]bool) SessionSummary {
	status := "active"
	if m.Lifecycle == manifest.LifecycleArchived {
		status = "archived"
	} else if m.Lifecycle == manifest.LifecycleReaping {
		status = "stopped" // reaping sessions display as stopped
	} else if s, ok := statuses[m.Name]; ok {
		status = s
	}

	estCost := m.LastKnownCost
	if estCost == 0 && m.CostTracking != nil {
		inCost := float64(m.CostTracking.TokensIn) / 1_000_000 * 15.0  // Opus input
		outCost := float64(m.CostTracking.TokensOut) / 1_000_000 * 75.0 // Opus output
		estCost = inCost + outCost
	}

	return SessionSummary{
		ID:            m.SessionID,
		Name:          m.Name,
		Status:        status,
		Attached:      attached[m.Name],
		Harness:       m.Harness,
		Project:       m.Context.Project,
		Tags:          m.Context.Tags,
		CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     m.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		EstimatedCost: estCost,
	}
}

// autoRecoverOverlay attempts to dismiss a Background Tasks UI overlay and
// re-resolves state. This prevents session list from showing a stale
// BACKGROUND_TASKS state when the overlay can be automatically dismissed.
//
// This is the Fix 4 component of the pipeline deadlock prevention: when
// agm session list detects BACKGROUND_TASKS_VIEW state, it auto-recovers
// by sending Left/Escape keys to dismiss the overlay.
func autoRecoverOverlay(tmuxName string, m *manifest.Manifest, fallbackState string) string {
	fmt.Fprintf(os.Stderr, "Auto-recovering overlay on '%s'...\n", tmuxName)

	// Step 1: Verify overlay is actually visible via capture-pane before sending keys.
	// Bug fix: previously sent Left key without verifying overlay state first.
	canReceive := session.CheckSessionDelivery(tmuxName)
	if canReceive != state.CanReceiveOverlay {
		// No overlay detected — don't send dismiss keys blindly
		return fallbackState
	}

	// Step 2: Send Left arrow key to dismiss overlay (verified visible)
	if err := tmux.SendKeys(tmuxName, "Left"); err != nil {
		return fallbackState
	}

	// Step 3: Wait for overlay to close
	time.Sleep(200 * time.Millisecond)

	// Step 4: Re-check state
	canReceive = session.CheckSessionDelivery(tmuxName)
	if canReceive == state.CanReceiveYes {
		// Overlay dismissed — re-resolve state
		newState := session.ResolveSessionState(tmuxName, m.State, m.Claude.UUID, m.StateUpdatedAt)
		fmt.Fprintf(os.Stderr, "Overlay dismissed on '%s' (now: %s)\n", tmuxName, newState)
		return newState
	}

	if canReceive == state.CanReceiveOverlay {
		// Try Escape as fallback (overlay still visible — verified)
		if err := tmux.SendKeys(tmuxName, "Escape"); err != nil {
			return fallbackState
		}
		time.Sleep(200 * time.Millisecond)

		canReceive = session.CheckSessionDelivery(tmuxName)
		if canReceive == state.CanReceiveYes {
			newState := session.ResolveSessionState(tmuxName, m.State, m.Claude.UUID, m.StateUpdatedAt)
			fmt.Fprintf(os.Stderr, "Overlay dismissed on '%s' with Escape (now: %s)\n", tmuxName, newState)
			return newState
		}
	}

	// Could not dismiss — return original state
	return fallbackState
}



// tmuxInfoProvider is the interface needed for attachment-aware status computation.
type tmuxInfoProvider interface {
	ListSessions() ([]string, error)
	ListSessionsWithInfo() ([]session.SessionInfo, error)
}

// computeStatuses wraps computeStatusesWithAttachment for callers that only need status.
func computeStatuses(manifests []*manifest.Manifest, tmux interface{}) map[string]string {
	statuses, _ := computeStatusesWithAttachment(manifests, tmux)
	return statuses
}

// computeStatusesWithAttachment computes both status and attachment info in a single tmux call.
func computeStatusesWithAttachment(manifests []*manifest.Manifest, tmux interface{}) (map[string]string, map[string]bool) {
	statuses := make(map[string]string, len(manifests))
	attached := make(map[string]bool, len(manifests))

	// Try the richer interface first (with attachment info)
	if ti, ok := tmux.(tmuxInfoProvider); ok {
		sessions, err := ti.ListSessionsWithInfo()
		if err != nil {
			return statuses, attached
		}

		sessionMap := make(map[string]session.SessionInfo, len(sessions))
		for _, s := range sessions {
			sessionMap[s.Name] = s
		}

		for _, m := range manifests {
			tmuxName := m.Tmux.SessionName
			if tmuxName == "" {
				tmuxName = m.Name
			}
			if info, exists := sessionMap[tmuxName]; exists {
				statuses[m.Name] = "active"
				attached[m.Name] = info.AttachedClients > 0
			} else {
				statuses[m.Name] = "stopped"
			}
		}
		return statuses, attached
	}

	// Fallback: basic interface without attachment info
	ti, ok := tmux.(interface {
		ListSessions() ([]string, error)
	})
	if !ok {
		return statuses, attached
	}

	tmuxSessions, err := ti.ListSessions()
	if err != nil {
		return statuses, attached
	}

	tmuxSet := make(map[string]bool, len(tmuxSessions))
	for _, s := range tmuxSessions {
		tmuxSet[s] = true
	}

	for _, m := range manifests {
		tmuxName := m.Tmux.SessionName
		if tmuxName == "" {
			tmuxName = m.Name
		}
		if tmuxSet[tmuxName] {
			statuses[m.Name] = "active"
		} else {
			statuses[m.Name] = "stopped"
		}
	}
	return statuses, attached
}

// computeStatusesFromInfo computes status and attachment from pre-fetched tmux session info.
func computeStatusesFromInfo(manifests []*manifest.Manifest, tmuxSessions []session.SessionInfo) (map[string]string, map[string]bool) {
	statuses := make(map[string]string, len(manifests))
	attached := make(map[string]bool, len(manifests))

	sessionMap := make(map[string]session.SessionInfo, len(tmuxSessions))
	for _, s := range tmuxSessions {
		sessionMap[s.Name] = s
	}

	for _, m := range manifests {
		tmuxName := m.Tmux.SessionName
		if tmuxName == "" {
			tmuxName = m.Name
		}
		if info, exists := sessionMap[tmuxName]; exists {
			statuses[m.Name] = "active"
			attached[m.Name] = info.AttachedClients > 0
		} else {
			statuses[m.Name] = "stopped"
		}
	}
	return statuses, attached
}

// findOrphanTmuxSessions returns tmux session names that have no corresponding
// AGM session. These are tmux sessions on the AGM socket that were never
// registered or whose AGM records were deleted.
func findOrphanTmuxSessions(manifests []*manifest.Manifest, tmuxSessions []session.SessionInfo) []string {
	if len(tmuxSessions) == 0 {
		return nil
	}

	// Build set of known AGM tmux session names
	agmNames := make(map[string]bool, len(manifests)*2)
	for _, m := range manifests {
		tmuxName := m.Tmux.SessionName
		if tmuxName == "" {
			tmuxName = m.Name
		}
		agmNames[tmuxName] = true
		agmNames[m.Name] = true
	}

	// Find tmux sessions not in AGM
	var orphans []string
	for _, s := range tmuxSessions {
		if !agmNames[s.Name] {
			orphans = append(orphans, s.Name)
		}
	}

	sort.Strings(orphans)
	return orphans
}
