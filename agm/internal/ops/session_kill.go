package ops

import (
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
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
	// HarnessDead reports that the tmux session exists but the process check
	// proved no harness process is running in the pane tree (ce-axsr): the
	// session is a zombie pane, not an active session, so --confirmed-stuck
	// is not required to kill it.
	HarnessDead bool `json:"harness_dead,omitempty"`
	// ZombieWriter reports the ce-qkf7 mode: the only surviving pane-tree
	// process is an orphaned agm — likely still writing a heartbeat file.
	ZombieWriter bool `json:"zombie_writer,omitempty"`
	// LivenessEvidence lists the pane's process names when the harness was
	// proven dead, so callers can say why the session was treated as dead.
	LivenessEvidence string `json:"liveness_evidence,omitempty"`
}

// TmuxKiller is the subset of tmux operations needed by KillSession.
// The ops layer uses this interface to kill tmux sessions without
// importing the tmux package directly.
type TmuxKiller interface {
	HasSession(name string) (bool, error)
	SendKeys(session, keys string) error
}

// isTmuxSessionRunning queries the tmux backend (when available) for the
// liveness of name.
func isTmuxSessionRunning(ctx *OpContext, name string) bool {
	if ctx.Tmux == nil {
		return false
	}
	ti, ok := ctx.Tmux.(interface {
		HasSession(name string) (bool, error)
	})
	if !ok {
		return false
	}
	has, err := ti.HasSession(name)
	return err == nil && has
}

// sessionLiveness combines the two liveness signals for name: tmux session
// existence AND a harness process actually running in the pane tree
// (ce-axsr). `tmux has-session` alone is false-green — the session keeps
// existing after the harness exits and the pane falls back to a shell.
//
// Active is only true when both signals agree. When the tmux backend cannot
// prove process liveness (no HarnessLivenessChecker capability, or the scan
// errors), Active falls back to session existence — a scan failure must not
// let an active session be killed without confirmation.
type sessionLiveness struct {
	SessionExists bool
	Active        bool
	HarnessDead   bool
	ZombieWriter  bool
	Evidence      string
}

func assessSessionLiveness(ctx *OpContext, name string) sessionLiveness {
	exists := isTmuxSessionRunning(ctx, name)
	lv := sessionLiveness{SessionExists: exists, Active: exists}
	if !exists {
		return lv
	}
	checker, ok := ctx.Tmux.(session.HarnessLivenessChecker)
	if !ok {
		return lv
	}
	info, err := checker.HarnessLiveness(name)
	if err != nil || !info.SessionExists {
		// Scan failure (or a session that vanished mid-check) proves nothing:
		// keep the conservative "exists = active" verdict.
		return lv
	}
	if !info.HarnessAlive {
		lv.Active = false
		lv.HarnessDead = true
		lv.ZombieWriter = info.ZombieWriter
		lv.Evidence = info.Evidence
	}
	return lv
}

// assessRecentActivity returns the last-activity timestamp pointer (or nil)
// and a flag for whether the session was active within recentActivityThreshold.
func assessRecentActivity(m *manifest.Manifest, recentActivityThreshold time.Duration) (*time.Time, bool) {
	if m.UpdatedAt.IsZero() {
		return nil, false
	}
	last := m.UpdatedAt
	return &last, time.Since(m.UpdatedAt) < recentActivityThreshold
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

	tmuxName := m.Tmux.SessionName
	if tmuxName == "" {
		tmuxName = m.Name
	}
	// "Running" means a harness process is provably alive in the pane tree,
	// not merely that tmux has a session by this name (ce-axsr). A zombie
	// pane (tmux session exists, harness dead) is killable without
	// --confirmed-stuck — that flag protects live work, and there is none.
	liveness := assessSessionLiveness(ctx, tmuxName)
	wasRunning := liveness.Active
	if wasRunning && !req.ConfirmedStuck {
		return &KillSessionResult{
			Operation:  "kill_session",
			SessionID:  m.SessionID,
			Name:       m.Name,
			WasRunning: true,
		}, ErrActiveSessionKill(m.Name)
	}
	lastActivity, recentlyActive := assessRecentActivity(m, recentActivityThreshold)

	// If recently active and not forced, return result with warning
	// The CLI layer uses this to prompt for confirmation
	result := &KillSessionResult{
		Operation:        "kill_session",
		SessionID:        m.SessionID,
		Name:             m.Name,
		WasRunning:       wasRunning,
		RecentlyActive:   recentlyActive,
		LastActivity:     lastActivity,
		HarnessDead:      liveness.HarnessDead,
		ZombieWriter:     liveness.ZombieWriter,
		LivenessEvidence: liveness.Evidence,
	}

	// Dry run
	if ctx.DryRun {
		result.DryRun = true
		return result, nil
	}

	// Kill-protect: if recently active and neither bypass flag is set,
	// return an error so the CLI layer can prompt for confirmation.
	// --confirmed-stuck counts as confirmation here too: demanding --force
	// on top of --confirmed-stuck (and vice versa) created a flag ping-pong
	// where no single re-run command could kill a stuck session (ce-axsr).
	// One flag path must be sufficient.
	if recentlyActive && !req.Force && !req.ConfirmedStuck {
		return result, ErrKillProtected(m.Name, *lastActivity)
	}

	return result, nil
}
