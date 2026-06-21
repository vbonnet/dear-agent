package session

import (
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// StatusInfo holds status and attachment information for a session
type StatusInfo struct {
	Status          string // "active", "stopped", or "archived"
	AttachedClients int    // Number of attached clients (0 if not active)
	LocallyAttached bool   // True if attached on this terminal/machine
}

// TmuxSessionName returns the tmux session name for a manifest with fallback logic.
// If Tmux.SessionName is empty, falls back to the sanitized session name. This is
// the single source of truth for "which tmux session name does this manifest map
// to" — status computation and archive safety checks MUST agree on it, otherwise a
// live detached pane (whose manifest has an empty Tmux.SessionName) can be archived
// while status reports it active.
func TmuxSessionName(m *manifest.Manifest) string {
	if m.Tmux.SessionName != "" {
		return m.Tmux.SessionName
	}

	// Fallback: use sanitized session name
	sanitized := tmux.SanitizeSessionName(m.Name)
	if sanitized == "" {
		return "session" // Last resort fallback
	}
	return sanitized
}

// ComputeStatus determines the current status of a session
// Returns one of: "active", "stopped", or "archived"
//
// Status logic:
// - If Lifecycle == "archived" → "archived"
// - If tmux session exists → "active"
// - Otherwise → "stopped"
func ComputeStatus(m *manifest.Manifest, tmux TmuxInterface) string {
	// Check lifecycle first - archived takes precedence
	if m.Lifecycle == manifest.LifecycleArchived {
		return "archived"
	}

	// If tmux interface is nil, conservatively report stopped
	if tmux == nil {
		return "stopped"
	}

	// Check tmux state (use fallback logic for empty tmux_session_name)
	tmuxSessionName := TmuxSessionName(m)
	exists, err := tmux.HasSession(tmuxSessionName)
	if err != nil {
		// On tmux error, assume stopped (conservative choice)
		return "stopped"
	}

	if exists {
		return "active"
	}

	return "stopped"
}

// ComputeStatusBatch computes status for multiple manifests efficiently
// Makes a single call to tmux.ListSessions() instead of N calls to HasSession()
//
// Returns a map of manifest Name → status.
//
// WARNING: keying by Name is lossy when two manifests share a Name (e.g. an
// archived shadow and a live worker). The last writer wins and can hide a live
// session. Display callers tolerate this; any code that makes lifecycle
// decisions (e.g. GC) MUST use ComputeStatusBatchByID instead.
func ComputeStatusBatch(manifests []*manifest.Manifest, tmux TmuxInterface) map[string]string {
	return computeStatusBatchKeyed(manifests, tmux, func(m *manifest.Manifest) string { return m.Name })
}

// ComputeStatusBatchByID computes status for multiple manifests efficiently and
// keys the result map by SessionID (UUID) instead of Name. Use this anywhere the
// status drives a decision per session, so name collisions can't shadow a live
// session under an archived (or otherwise stale) duplicate name.
func ComputeStatusBatchByID(manifests []*manifest.Manifest, tmux TmuxInterface) map[string]string {
	return computeStatusBatchKeyed(manifests, tmux, func(m *manifest.Manifest) string { return m.SessionID })
}

// computeStatusBatchKeyed is the shared core of the batch status helpers. It
// lists tmux sessions once and computes each manifest's status, keying the
// result map by key(m).
func computeStatusBatchKeyed(manifests []*manifest.Manifest, tmux TmuxInterface, key func(*manifest.Manifest) string) map[string]string {
	statuses := make(map[string]string, len(manifests))

	// If tmux interface is nil, conservatively report all as stopped (or archived if lifecycle set)
	var existingSessions []string
	if tmux == nil {
		existingSessions = []string{}
	} else {
		// Get all tmux sessions in one call (optimization)
		var err error
		existingSessions, err = tmux.ListSessions()
		if err != nil {
			// On error, assume no sessions exist
			existingSessions = []string{}
		}
	}

	// Build set of existing sessions for O(1) lookup
	sessionSet := make(map[string]bool, len(existingSessions))
	for _, name := range existingSessions {
		sessionSet[name] = true
	}

	// Compute status for each manifest
	for _, m := range manifests {
		if m.Lifecycle == manifest.LifecycleArchived {
			statuses[key(m)] = "archived"
		} else {
			// Use fallback logic for empty tmux_session_name
			tmuxSessionName := TmuxSessionName(m)
			if sessionSet[tmuxSessionName] {
				statuses[key(m)] = "active"
			} else {
				statuses[key(m)] = "stopped"
			}
		}
	}

	return statuses
}

// ComputeStatusBatchWithInfo computes status and attachment info for multiple manifests efficiently
// Makes a single call to tmux.ListSessionsWithInfo() instead of N calls
//
// Returns a map of manifest Name → StatusInfo
func ComputeStatusBatchWithInfo(manifests []*manifest.Manifest, tmux TmuxInterface) map[string]StatusInfo {
	statuses := make(map[string]StatusInfo, len(manifests))

	// Get all tmux sessions with attachment info in one call (optimization)
	existingSessions, err := tmux.ListSessionsWithInfo()
	if err != nil {
		// On error, assume no sessions exist
		existingSessions = []SessionInfo{}
	}

	// Build map of session name → SessionInfo for O(1) lookup
	sessionMap := make(map[string]SessionInfo, len(existingSessions))
	for _, session := range existingSessions {
		sessionMap[session.Name] = session
	}

	// Compute status for each manifest
	for _, m := range manifests {
		if m.Lifecycle == manifest.LifecycleArchived {
			statuses[m.Name] = StatusInfo{
				Status:          "archived",
				AttachedClients: 0,
				LocallyAttached: false,
			}
		} else {
			// Use fallback logic for empty tmux_session_name
			tmuxSessionName := TmuxSessionName(m)
			if sessionInfo, exists := sessionMap[tmuxSessionName]; exists {
				statuses[m.Name] = StatusInfo{
					Status:          "active",
					AttachedClients: sessionInfo.AttachedClients,
					LocallyAttached: false, // No longer used, but kept for compatibility
				}
			} else {
				statuses[m.Name] = StatusInfo{
					Status:          "stopped",
					AttachedClients: 0,
					LocallyAttached: false,
				}
			}
		}
	}

	return statuses
}
