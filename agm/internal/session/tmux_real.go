package session

import "github.com/vbonnet/dear-agent/agm/internal/tmux"

// RealTmux wraps the internal/tmux package to provide TmuxInterface implementation
type RealTmux struct{}

// NewRealTmux creates a new RealTmux instance
func NewRealTmux() *RealTmux {
	return &RealTmux{}
}

// HasSession checks if a tmux session exists
func (t *RealTmux) HasSession(name string) (bool, error) {
	return tmux.HasSession(name)
}

// ListSessions returns all active tmux session names
func (t *RealTmux) ListSessions() ([]string, error) {
	return tmux.ListSessions()
}

// ListSessionsWithInfo returns all active tmux sessions with attachment info
func (t *RealTmux) ListSessionsWithInfo() ([]SessionInfo, error) {
	tmuxSessions, err := tmux.ListSessionsWithInfo()
	if err != nil {
		return nil, err
	}
	// Convert tmux.SessionInfo to session.SessionInfo
	sessions := make([]SessionInfo, len(tmuxSessions))
	for i, s := range tmuxSessions {
		sessions[i] = SessionInfo{
			Name:            s.Name,
			AttachedClients: s.AttachedClients,
			AttachedList:    s.AttachedList,
		}
	}
	return sessions, nil
}

// CreateSession creates a new tmux session
func (t *RealTmux) CreateSession(name, workdir string) error {
	return tmux.NewSession(name, workdir)
}

// KillSession removes a tmux session created by a failed lifecycle operation.
// It implements TmuxSessionKiller without widening TmuxInterface.
func (t *RealTmux) KillSession(name string) error {
	return tmux.KillSessionChecked(name)
}

// AttachSession attaches to a tmux session
func (t *RealTmux) AttachSession(name string) error {
	return tmux.AttachSession(name)
}

// SendKeys sends keys to a tmux session
func (t *RealTmux) SendKeys(session, keys string) error {
	return tmux.SendCommand(session, keys)
}

// HarnessLiveness scans the session's pane process tree for a live harness
// process (ce-axsr). Implements HarnessLivenessChecker.
func (t *RealTmux) HarnessLiveness(sessionName string) (LivenessInfo, error) {
	pl, err := tmux.CheckPaneLiveness(sessionName, tmux.GetSocketPath())
	if err != nil {
		return LivenessInfo{}, err
	}
	return LivenessInfo{
		SessionExists: pl.SessionExists,
		HarnessAlive:  pl.HarnessAlive,
		ZombieWriter:  pl.ZombieWriter,
		Evidence:      pl.Evidence,
	}, nil
}

// HarnessLivenessBatch scans many sessions with a constant number of
// subprocesses (one `tmux list-panes -a`, one `ps`). Implements
// HarnessLivenessBatchChecker.
func (t *RealTmux) HarnessLivenessBatch(sessionNames []string) (map[string]LivenessInfo, error) {
	batch, err := tmux.CheckPaneLivenessBatch(sessionNames, tmux.GetSocketPath())
	if err != nil {
		return nil, err
	}
	out := make(map[string]LivenessInfo, len(batch))
	for name, pl := range batch {
		out[name] = LivenessInfo{
			SessionExists: pl.SessionExists,
			HarnessAlive:  pl.HarnessAlive,
			ZombieWriter:  pl.ZombieWriter,
			Evidence:      pl.Evidence,
		}
	}
	return out, nil
}

// ListClients returns all clients attached to a specific session
func (t *RealTmux) ListClients(sessionName string) ([]ClientInfo, error) {
	tmuxClients, err := tmux.ListClients(sessionName)
	if err != nil {
		return nil, err
	}
	// Convert tmux.ClientInfo to session.ClientInfo
	clients := make([]ClientInfo, len(tmuxClients))
	for i, c := range tmuxClients {
		clients[i] = ClientInfo{
			SessionName: c.SessionName,
			TTY:         c.TTY,
			PID:         c.PID,
		}
	}
	return clients, nil
}
