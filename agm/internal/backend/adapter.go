package backend

import (
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// Compile-time check to ensure BackendAdapter implements session.TmuxInterface
var _ session.TmuxInterface = (*BackendAdapter)(nil)

// BackendAdapter adapts a Backend to implement session.TmuxInterface
// This allows the backend system to be used with existing code that expects TmuxInterface
type BackendAdapter struct {
	backend Backend
}

// NewBackendAdapter creates a new BackendAdapter wrapping the given backend
func NewBackendAdapter(backend Backend) *BackendAdapter {
	return &BackendAdapter{
		backend: backend,
	}
}

// HasSession checks if a session with the given name exists
func (a *BackendAdapter) HasSession(name string) (bool, error) {
	return a.backend.HasSession(name)
}

// ListSessions returns all active session names
func (a *BackendAdapter) ListSessions() ([]string, error) {
	return a.backend.ListSessions()
}

// ListSessionsWithInfo returns all active sessions with attachment info
func (a *BackendAdapter) ListSessionsWithInfo() ([]session.SessionInfo, error) {
	backendInfos, err := a.backend.ListSessionsWithInfo()
	if err != nil {
		return nil, err
	}

	// Convert backend.SessionInfo to session.SessionInfo
	infos := make([]session.SessionInfo, len(backendInfos))
	for i, info := range backendInfos {
		infos[i] = session.SessionInfo{
			Name:            info.Name,
			AttachedClients: info.AttachedClients,
			AttachedList:    info.AttachedList,
		}
	}
	return infos, nil
}

// ListClients returns all clients attached to a specific session
func (a *BackendAdapter) ListClients(sessionName string) ([]session.ClientInfo, error) {
	backendClients, err := a.backend.ListClients(sessionName)
	if err != nil {
		return nil, err
	}

	// Convert backend.ClientInfo to session.ClientInfo
	clients := make([]session.ClientInfo, len(backendClients))
	for i, client := range backendClients {
		clients[i] = session.ClientInfo{
			SessionName: client.SessionName,
			TTY:         client.TTY,
			PID:         client.PID,
		}
	}
	return clients, nil
}

// CreateSession creates a new session with the given name and working directory
func (a *BackendAdapter) CreateSession(name, workdir string) error {
	return a.backend.CreateSession(name, workdir)
}

// AttachSession attaches to or switches to the given session
func (a *BackendAdapter) AttachSession(name string) error {
	return a.backend.AttachSession(name)
}

// SendKeys sends keys (command) to the given session
func (a *BackendAdapter) SendKeys(sessionName, keys string) error {
	return a.backend.SendKeys(sessionName, keys)
}

// GetDefaultBackendAdapter returns a BackendAdapter using the default backend
// The backend is selected based on the AGM_SESSION_BACKEND environment variable
// Defaults to tmux if not set
func GetDefaultBackendAdapter() (*BackendAdapter, error) {
	backend, err := GetBackend()
	if err != nil {
		return nil, err
	}
	return NewBackendAdapter(backend), nil
}
