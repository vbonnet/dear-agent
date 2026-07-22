package backend

import (
	"context"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// Compile-time check to ensure BackendAdapter implements session.TmuxInterface
var _ session.TmuxInterface = (*BackendAdapter)(nil)
var _ session.TmuxSessionKiller = (*BackendAdapter)(nil)
var _ session.StrictSessionExistenceChecker = (*BackendAdapter)(nil)
var _ session.HarnessLivenessChecker = (*BackendAdapter)(nil)
var _ session.HarnessLivenessBatchChecker = (*BackendAdapter)(nil)
var _ session.HarnessReadinessWaiter = (*BackendAdapter)(nil)
var _ session.InputReadinessChecker = (*BackendAdapter)(nil)

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

// KillSession forwards the destructive capability required by shared
// lifecycle operations. Keeping this on the adapter is essential: the CLI
// passes BackendAdapter (not the underlying RealTmux) into ops.OpContext.
func (a *BackendAdapter) KillSession(name string) error {
	return a.backend.KillSession(name)
}

// HasSessionStrict preserves a backend's strict existence capability through
// the CLI adapter. Backends without a stronger probe retain the Backend
// contract that HasSession returns operational failures rather than absence.
func (a *BackendAdapter) HasSessionStrict(ctx context.Context, name string) (bool, error) {
	if checker, ok := a.backend.(session.StrictSessionExistenceChecker); ok {
		return checker.HasSessionStrict(ctx, name)
	}
	return a.backend.HasSession(name)
}

// HarnessLiveness preserves process-level liveness when the selected backend
// provides it. Callers treat an unsupported capability error conservatively.
func (a *BackendAdapter) HarnessLiveness(name string) (session.LivenessInfo, error) {
	if checker, ok := a.backend.(session.HarnessLivenessChecker); ok {
		return checker.HarnessLiveness(name)
	}
	return session.LivenessInfo{}, fmt.Errorf("backend does not implement harness liveness")
}

// HarnessLivenessBatch preserves the efficient batch liveness capability.
func (a *BackendAdapter) HarnessLivenessBatch(names []string) (map[string]session.LivenessInfo, error) {
	if checker, ok := a.backend.(session.HarnessLivenessBatchChecker); ok {
		return checker.HarnessLivenessBatch(names)
	}
	return nil, fmt.Errorf("backend does not implement batch harness liveness")
}

// AttachSession attaches to or switches to the given session
func (a *BackendAdapter) AttachSession(name string) error {
	return a.backend.AttachSession(name)
}

// SendKeys sends keys (command) to the given session
func (a *BackendAdapter) SendKeys(sessionName, keys string) error {
	return a.backend.SendKeys(sessionName, keys)
}

// SendKeysToPane preserves verified exact-pane delivery through the adapter.
func (a *BackendAdapter) SendKeysToPane(ctx context.Context, paneID, keys string) error {
	sender, ok := a.backend.(session.VerifiedPaneSender)
	if !ok {
		return fmt.Errorf("backend does not expose verified pane delivery")
	}
	return sender.SendKeysToPane(ctx, paneID, keys)
}

// WaitForHarnessReady preserves the optional readiness capability through the
// legacy backend adapter used by the production CLI.
func (a *BackendAdapter) WaitForHarnessReady(ctx context.Context, sessionName, harness string, timeout time.Duration) error {
	waiter, ok := a.backend.(session.HarnessReadinessWaiter)
	if !ok {
		return fmt.Errorf("backend %T does not expose harness readiness", a.backend)
	}
	return waiter.WaitForHarnessReady(ctx, sessionName, harness, timeout)
}

// CheckInputReadiness preserves exact, harness-aware send safety through the
// production adapter chain.
func (a *BackendAdapter) CheckInputReadiness(ctx context.Context, sessionName, harness string) (session.InputReadiness, error) {
	checker, ok := a.backend.(session.InputReadinessChecker)
	if !ok {
		return session.InputReadiness{}, fmt.Errorf("backend %T does not expose input readiness", a.backend)
	}
	return checker.CheckInputReadiness(ctx, sessionName, harness)
}

// SendKeysIfInputReady preserves atomic readiness and exact-pane delivery
// through the production backend adapter chain.
func (a *BackendAdapter) SendKeysIfInputReady(ctx context.Context, sessionName, harness, keys string, options session.InputDeliveryOptions) (session.InputReadiness, error) {
	sender, ok := a.backend.(session.AtomicInputSender)
	if !ok {
		return session.InputReadiness{}, fmt.Errorf("backend %T does not expose atomic input delivery", a.backend)
	}
	return sender.SendKeysIfInputReady(ctx, sessionName, harness, keys, options)
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
