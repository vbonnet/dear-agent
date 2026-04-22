// Package backend provides backend functionality.
package backend

import (
	"log"

	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// Compile-time check to ensure TmuxBackend implements Backend interface
var _ Backend = (*TmuxBackend)(nil)

// TmuxBackend wraps session.TmuxInterface to implement the Backend interface
// This adapter allows the existing tmux implementation to work with the new backend system
type TmuxBackend struct {
	tmux session.TmuxInterface
}

// NewTmuxBackend creates a new TmuxBackend instance
func NewTmuxBackend() *TmuxBackend {
	return &TmuxBackend{
		tmux: session.NewRealTmux(),
	}
}

// NewTmuxBackendWithClient creates a new TmuxBackend with a custom TmuxInterface
// This is useful for testing with mock implementations
func NewTmuxBackendWithClient(tmux session.TmuxInterface) *TmuxBackend {
	return &TmuxBackend{
		tmux: tmux,
	}
}

// HasSession checks if a session with the given name exists
func (b *TmuxBackend) HasSession(name string) (bool, error) {
	return b.tmux.HasSession(name)
}

// ListSessions returns all active session names
func (b *TmuxBackend) ListSessions() ([]string, error) {
	return b.tmux.ListSessions()
}

// ListSessionsWithInfo returns all active sessions with attachment info
func (b *TmuxBackend) ListSessionsWithInfo() ([]SessionInfo, error) {
	tmuxSessions, err := b.tmux.ListSessionsWithInfo()
	if err != nil {
		return nil, err
	}

	// Convert session.SessionInfo to backend.SessionInfo
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

// ListClients returns all clients attached to a specific session
func (b *TmuxBackend) ListClients(sessionName string) ([]ClientInfo, error) {
	tmuxClients, err := b.tmux.ListClients(sessionName)
	if err != nil {
		return nil, err
	}

	// Convert session.ClientInfo to backend.ClientInfo
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

// CreateSession creates a new session with the given name and working directory
func (b *TmuxBackend) CreateSession(name, workdir string) error {
	return b.tmux.CreateSession(name, workdir)
}

// AttachSession attaches to or switches to the given session
func (b *TmuxBackend) AttachSession(name string) error {
	return b.tmux.AttachSession(name)
}

// SendKeys sends keys (command) to the given session
func (b *TmuxBackend) SendKeys(session, keys string) error {
	return b.tmux.SendKeys(session, keys)
}

func init() {
	if err := Register("tmux", func() (Backend, error) {
		return NewTmuxBackend(), nil
	}); err != nil {
		log.Fatalf("backend: failed to register tmux: %v", err)
	}
}
