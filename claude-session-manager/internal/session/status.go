package session

import "github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"

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

	// Check tmux state
	exists, err := tmux.HasSession(m.Tmux.SessionName)
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
// Returns a map of manifest Name → status
func ComputeStatusBatch(manifests []*manifest.Manifest, tmux TmuxInterface) map[string]string {
	statuses := make(map[string]string, len(manifests))

	// Get all tmux sessions in one call (optimization)
	existingSessions, err := tmux.ListSessions()
	if err != nil {
		// On error, assume no sessions exist
		existingSessions = []string{}
	}

	// Build set of existing sessions for O(1) lookup
	sessionSet := make(map[string]bool, len(existingSessions))
	for _, name := range existingSessions {
		sessionSet[name] = true
	}

	// Compute status for each manifest
	for _, m := range manifests {
		if m.Lifecycle == manifest.LifecycleArchived {
			statuses[m.Name] = "archived"
		} else if sessionSet[m.Tmux.SessionName] {
			statuses[m.Name] = "active"
		} else {
			statuses[m.Name] = "stopped"
		}
	}

	return statuses
}
