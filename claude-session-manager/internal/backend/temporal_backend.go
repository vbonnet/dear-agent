package backend

import (
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/temporal"
)

// Compile-time check to ensure TemporalBackend implements Backend interface
var _ Backend = (*TemporalBackend)(nil)

// TemporalBackend wraps temporal.TemporalInterface to implement the Backend interface
// This adapter allows the Temporal implementation to work with the new backend system
type TemporalBackend struct {
	client temporal.TemporalInterface
}

// NewTemporalBackend creates a new TemporalBackend instance
func NewTemporalBackend() *TemporalBackend {
	return &TemporalBackend{
		client: temporal.NewTemporalClient(),
	}
}

// NewTemporalBackendWithClient creates a new TemporalBackend with a custom TemporalInterface
// This is useful for testing with mock implementations
func NewTemporalBackendWithClient(client temporal.TemporalInterface) *TemporalBackend {
	return &TemporalBackend{
		client: client,
	}
}

// HasSession checks if a session with the given name exists
func (b *TemporalBackend) HasSession(name string) (bool, error) {
	return b.client.HasSession(name)
}

// ListSessions returns all active session names
func (b *TemporalBackend) ListSessions() ([]string, error) {
	return b.client.ListSessions()
}

// ListSessionsWithInfo returns all active sessions with attachment info
func (b *TemporalBackend) ListSessionsWithInfo() ([]SessionInfo, error) {
	temporalSessions, err := b.client.ListSessionsWithInfo()
	if err != nil {
		return nil, err
	}

	// Convert temporal.SessionInfo to backend.SessionInfo
	sessions := make([]SessionInfo, len(temporalSessions))
	for i, s := range temporalSessions {
		sessions[i] = SessionInfo{
			Name:            s.Name,
			AttachedClients: s.AttachedClients,
			AttachedList:    s.AttachedList,
		}
	}
	return sessions, nil
}

// ListClients returns all clients attached to a specific session
func (b *TemporalBackend) ListClients(sessionName string) ([]ClientInfo, error) {
	temporalClients, err := b.client.ListClients(sessionName)
	if err != nil {
		return nil, err
	}

	// Convert temporal.ClientInfo to backend.ClientInfo
	clients := make([]ClientInfo, len(temporalClients))
	for i, c := range temporalClients {
		clients[i] = ClientInfo{
			SessionName: c.SessionName,
			TTY:         c.TTY,
			PID:         c.PID,
		}
	}
	return clients, nil
}

// CreateSession creates a new session with the given name and working directory
func (b *TemporalBackend) CreateSession(name, workdir string) error {
	return b.client.CreateSession(name, workdir)
}

// AttachSession attaches to or switches to the given session
func (b *TemporalBackend) AttachSession(name string) error {
	return b.client.AttachSession(name)
}

// SendKeys sends keys (command) to the given session
func (b *TemporalBackend) SendKeys(session, keys string) error {
	return b.client.SendKeys(session, keys)
}

func init() {
	// Register temporal backend on package initialization
	Register("temporal", func() (Backend, error) {
		return NewTemporalBackend(), nil
	})
}
