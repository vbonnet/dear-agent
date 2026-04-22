// Package restbackend implements a REST/Process-based backend for AGM.
// Sessions are managed as subprocesses communicating via stdin/stdout JSON pipes,
// eliminating the dependency on tmux for session management.
package restbackend

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/vbonnet/dear-agent/agm/internal/backend"
)

// Compile-time check that RestBackend implements backend.Backend.
var _ backend.Backend = (*RestBackend)(nil)

// RestBackend manages agent sessions as subprocesses with JSON stdin/stdout I/O.
type RestBackend struct {
	mu         sync.RWMutex
	sessions   map[string]*managedProcess // keyed by session name
	claudePath string                     // path to claude binary
}

// New creates a new RestBackend. claudePath defaults to "claude" if empty.
func New(claudePath string) *RestBackend {
	if claudePath == "" {
		claudePath = "claude"
	}
	return &RestBackend{
		sessions:   make(map[string]*managedProcess),
		claudePath: claudePath,
	}
}

// HasSession checks if a session with the given name exists and is alive.
func (b *RestBackend) HasSession(name string) (bool, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	proc, exists := b.sessions[name]
	if !exists {
		return false, nil
	}
	return proc.isAlive(), nil
}

// ListSessions returns all active session names.
func (b *RestBackend) ListSessions() ([]string, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	names := make([]string, 0, len(b.sessions))
	for name, proc := range b.sessions {
		if proc.isAlive() {
			names = append(names, name)
		}
	}
	return names, nil
}

// ListSessionsWithInfo returns all active sessions with metadata.
func (b *RestBackend) ListSessionsWithInfo() ([]backend.SessionInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	infos := make([]backend.SessionInfo, 0, len(b.sessions))
	for name, proc := range b.sessions {
		if proc.isAlive() {
			infos = append(infos, backend.SessionInfo{
				Name:            name,
				AttachedClients: 0, // process backend has no attach concept
				AttachedList:    "",
			})
		}
	}
	return infos, nil
}

// ListClients returns clients attached to a session.
// Process backend doesn't support terminal attachment, so this always returns empty.
func (b *RestBackend) ListClients(sessionName string) ([]backend.ClientInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if _, exists := b.sessions[sessionName]; !exists {
		return nil, fmt.Errorf("session %q not found", sessionName)
	}
	return nil, nil
}

// CreateSession spawns a new Claude subprocess with the given name and working directory.
func (b *RestBackend) CreateSession(name, workdir string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if proc, exists := b.sessions[name]; exists && proc.isAlive() {
		return fmt.Errorf("session %q already exists", name)
	}

	proc, err := spawnProcess(context.Background(), name, name, b.claudePath, workdir, "", nil)
	if err != nil {
		return fmt.Errorf("create session %q: %w", name, err)
	}

	b.sessions[name] = proc
	return nil
}

// AttachSession is not supported by the process backend (no terminal to attach to).
func (b *RestBackend) AttachSession(name string) error {
	return fmt.Errorf("process backend does not support terminal attachment; use SendKeys/ReadOutput instead")
}

// SendKeys sends a message to the session's stdin pipe.
func (b *RestBackend) SendKeys(session, keys string) error {
	b.mu.RLock()
	proc, exists := b.sessions[session]
	b.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session %q not found", session)
	}
	if !proc.isAlive() {
		return fmt.Errorf("session %q is not running", session)
	}

	return proc.sendMessage(keys)
}

// ReadOutput returns the last n lines of output from the session.
func (b *RestBackend) ReadOutput(session string, lines int) (string, error) {
	b.mu.RLock()
	proc, exists := b.sessions[session]
	b.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("session %q not found", session)
	}

	return proc.readOutput(lines), nil
}

// GetProcessState returns the process lifecycle state for a session.
func (b *RestBackend) GetProcessState(session string) (ProcessState, error) {
	b.mu.RLock()
	proc, exists := b.sessions[session]
	b.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("session %q not found", session)
	}

	return proc.state.Load().(ProcessState), nil
}

// TerminateSession stops a running session.
func (b *RestBackend) TerminateSession(name string) error {
	b.mu.Lock()
	proc, exists := b.sessions[name]
	if !exists {
		b.mu.Unlock()
		return fmt.Errorf("session %q not found", name)
	}
	delete(b.sessions, name)
	b.mu.Unlock()

	return proc.stop()
}

func init() {
	if err := backend.Register("process", func() (backend.Backend, error) {
		return New(""), nil
	}); err != nil {
		log.Printf("restbackend: failed to register process backend: %v", err)
	}
}
