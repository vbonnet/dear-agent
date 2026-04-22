package ops

import (
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

// KillSessionRequest defines the input for killing a session's tmux process.
type KillSessionRequest struct {
	// Identifier is a session ID, name, or UUID prefix.
	Identifier string `json:"identifier"`
	// Force bypasses the last-activity safety check.
	Force bool `json:"force,omitempty"`
	// ConfirmedStuck is REQUIRED to kill an active (running) session.
	// Without this flag, only stopped sessions can be killed.
	ConfirmedStuck bool `json:"confirmed_stuck,omitempty"`
}

// KillSessionResult is the output of KillSession.
type KillSessionResult struct {
	Operation      string     `json:"operation"`
	SessionID      string     `json:"session_id"`
	Name           string     `json:"name"`
	WasRunning     bool       `json:"was_running"`
	DryRun         bool       `json:"dry_run,omitempty"`
	RecentlyActive bool       `json:"recently_active,omitempty"`
	LastActivity   *time.Time `json:"last_activity,omitempty"`
}

// TmuxKiller is the subset of tmux operations needed by KillSession.
// The ops layer uses this interface to kill tmux sessions without
// importing the tmux package directly.
type TmuxKiller interface {
	HasSession(name string) (bool, error)
	SendKeys(session, keys string) error
}

// KillSession terminates the tmux session for an AGM session.
// If ctx.DryRun is true, returns what would happen without executing.
func KillSession(ctx *OpContext, req *KillSessionRequest) (*KillSessionResult, error) {
	if req == nil || req.Identifier == "" {
		return nil, ErrInvalidInput("identifier", "Session identifier is required. Provide a session ID, name, or UUID prefix.")
	}

	recentActivityThreshold := contracts.Load().SessionLifecycle.RecentActivityThreshold.Duration

	// Resolve session
	m, err := ctx.Storage.GetSession(req.Identifier)
	if err != nil {
		m, err = findByName(ctx, req.Identifier)
		if err != nil {
			return nil, err
		}
	}
	if m == nil {
		return nil, ErrSessionNotFound(req.Identifier)
	}

	// Check if already archived
	if m.Lifecycle == "archived" {
		return nil, ErrSessionArchived(m.Name)
	}

	// Determine tmux session name
	tmuxName := m.Tmux.SessionName
	if tmuxName == "" {
		tmuxName = m.Name
	}

	// Check if session is running in tmux
	wasRunning := false
	if ctx.Tmux != nil {
		ti, ok := ctx.Tmux.(interface {
			HasSession(name string) (bool, error)
		})
		if ok {
			has, hasErr := ti.HasSession(tmuxName)
			if hasErr == nil && has {
				wasRunning = true
			}
		}
	}

	// Active session safety: refuse to kill running sessions without --confirmed-stuck
	if wasRunning && !req.ConfirmedStuck {
		return &KillSessionResult{
			Operation:  "kill_session",
			SessionID:  m.SessionID,
			Name:       m.Name,
			WasRunning: true,
		}, ErrActiveSessionKill(m.Name)
	}

	// Check last activity timestamp for kill-protect
	var lastActivity *time.Time
	recentlyActive := false
	if !m.UpdatedAt.IsZero() {
		lastActivity = &m.UpdatedAt
		if time.Since(m.UpdatedAt) < recentActivityThreshold {
			recentlyActive = true
		}
	}

	// If recently active and not forced, return result with warning
	// The CLI layer uses this to prompt for confirmation
	result := &KillSessionResult{
		Operation:      "kill_session",
		SessionID:      m.SessionID,
		Name:           m.Name,
		WasRunning:     wasRunning,
		RecentlyActive: recentlyActive,
		LastActivity:   lastActivity,
	}

	// Dry run
	if ctx.DryRun {
		result.DryRun = true
		return result, nil
	}

	// Kill-protect: if recently active and not forced, return an error
	// so the CLI layer can prompt for confirmation
	if recentlyActive && !req.Force {
		return result, ErrKillProtected(m.Name, *lastActivity)
	}

	return result, nil
}
