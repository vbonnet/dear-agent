// Package backend provides backend functionality.
package backend

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// Compile-time check to ensure TmuxBackend implements Backend interface
var _ Backend = (*TmuxBackend)(nil)
var _ session.HarnessReadinessWaiter = (*TmuxBackend)(nil)
var _ session.InputReadinessChecker = (*TmuxBackend)(nil)

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

// KillSession preserves the optional destructive tmux capability through the
// backend layer used by the CLI.
func (b *TmuxBackend) KillSession(name string) error {
	killer, ok := b.tmux.(session.TmuxSessionKiller)
	if !ok {
		return fmt.Errorf("tmux client does not implement session deletion")
	}
	return killer.KillSession(name)
}

// HasSessionStrict preserves strict failure-vs-absence semantics through the
// backend layer used by destructive operations.
func (b *TmuxBackend) HasSessionStrict(ctx context.Context, name string) (bool, error) {
	if checker, ok := b.tmux.(session.StrictSessionExistenceChecker); ok {
		return checker.HasSessionStrict(ctx, name)
	}
	return b.tmux.HasSession(name)
}

// HarnessLiveness preserves process-level liveness through the backend layer.
func (b *TmuxBackend) HarnessLiveness(name string) (session.LivenessInfo, error) {
	checker, ok := b.tmux.(session.HarnessLivenessChecker)
	if !ok {
		return session.LivenessInfo{}, fmt.Errorf("tmux client does not implement harness liveness")
	}
	return checker.HarnessLiveness(name)
}

// HarnessLivenessBatch preserves efficient batch liveness through the backend layer.
func (b *TmuxBackend) HarnessLivenessBatch(names []string) (map[string]session.LivenessInfo, error) {
	checker, ok := b.tmux.(session.HarnessLivenessBatchChecker)
	if !ok {
		return nil, fmt.Errorf("tmux client does not implement batch harness liveness")
	}
	return checker.HarnessLivenessBatch(names)
}

// AttachSession attaches to or switches to the given session
func (b *TmuxBackend) AttachSession(name string) error {
	return b.tmux.AttachSession(name)
}

// SendKeys sends keys (command) to the given session
func (b *TmuxBackend) SendKeys(session, keys string) error {
	return b.tmux.SendKeys(session, keys)
}

// SendKeysToPane forwards verified exact-pane delivery.
func (b *TmuxBackend) SendKeysToPane(ctx context.Context, paneID, keys string) error {
	sender, ok := b.tmux.(session.VerifiedPaneSender)
	if !ok {
		return fmt.Errorf("tmux implementation %T does not expose verified pane delivery", b.tmux)
	}
	return sender.SendKeysToPane(ctx, paneID, keys)
}

// WaitForHarnessReady forwards the shared startup-readiness capability.
func (b *TmuxBackend) WaitForHarnessReady(ctx context.Context, sessionName, harness string, timeout time.Duration) error {
	waiter, ok := b.tmux.(session.HarnessReadinessWaiter)
	if !ok {
		return fmt.Errorf("tmux implementation %T does not expose harness readiness", b.tmux)
	}
	return waiter.WaitForHarnessReady(ctx, sessionName, harness, timeout)
}

// CheckInputReadiness forwards exact, harness-aware send safety.
func (b *TmuxBackend) CheckInputReadiness(ctx context.Context, sessionName, harness string) (session.InputReadiness, error) {
	checker, ok := b.tmux.(session.InputReadinessChecker)
	if !ok {
		return session.InputReadiness{}, fmt.Errorf("tmux implementation %T does not expose input readiness", b.tmux)
	}
	return checker.CheckInputReadiness(ctx, sessionName, harness)
}

// SendKeysIfInputReady forwards the atomic readiness-and-delivery boundary.
func (b *TmuxBackend) SendKeysIfInputReady(ctx context.Context, sessionName, harness, keys string, options session.InputDeliveryOptions) (session.InputReadiness, error) {
	sender, ok := b.tmux.(session.AtomicInputSender)
	if !ok {
		return session.InputReadiness{}, fmt.Errorf("tmux implementation %T does not expose atomic input delivery", b.tmux)
	}
	return sender.SendKeysIfInputReady(ctx, sessionName, harness, keys, options)
}

func init() {
	if err := Register("tmux", func() (Backend, error) {
		return NewTmuxBackend(), nil
	}); err != nil {
		slog.Error("backend: failed to register tmux", "error", err)
		os.Exit(1)
	}
}
