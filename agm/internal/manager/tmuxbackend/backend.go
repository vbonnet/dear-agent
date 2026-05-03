// Package tmuxbackend implements the manager.Backend interfaces using tmux
// as the session runtime. This is a refactor of existing tmux logic from
// agm/internal/tmux and agm/internal/session into the new interface layer.
package tmuxbackend

import (
	"context"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manager"
	"github.com/vbonnet/dear-agent/agm/internal/state"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// Compile-time interface checks.
var (
	_ manager.Backend           = (*TmuxBackend)(nil)
	_ manager.AttachableBackend = (*TmuxBackend)(nil)
)

// TmuxBackend manages agent sessions via tmux.
// It delegates to the existing agm/internal/tmux package for all
// tmux operations, adapting them to the manager.Backend interface.
type TmuxBackend struct{}

// New creates a new TmuxBackend.
func New() *TmuxBackend {
	return &TmuxBackend{}
}

// Name returns the backend identifier.
func (b *TmuxBackend) Name() string { return "tmux" }

// Capabilities returns what the tmux backend supports.
func (b *TmuxBackend) Capabilities() manager.BackendCapabilities {
	return manager.BackendCapabilities{
		SupportsAttach:        true,
		SupportsStructuredIO:  false, // tmux uses terminal scraping
		SupportsInterrupt:     true,
		MaxConcurrentSessions: 0, // unlimited
	}
}

// --- SessionManager ---

// CreateSession creates a new tmux session.
func (b *TmuxBackend) CreateSession(_ context.Context, config manager.SessionConfig) (manager.SessionID, error) {
	if config.Name == "" {
		return "", fmt.Errorf("session name is required")
	}
	workdir := config.WorkingDirectory
	if workdir == "" {
		workdir = "."
	}
	if err := tmux.NewSession(config.Name, workdir); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	// tmux uses the session name as the ID
	return manager.SessionID(config.Name), nil
}

// TerminateSession kills a tmux session.
func (b *TmuxBackend) TerminateSession(_ context.Context, id manager.SessionID) error {
	tmux.KillSession(string(id))
	return nil
}

// ListSessions returns all active tmux sessions.
func (b *TmuxBackend) ListSessions(_ context.Context, filter manager.SessionFilter) ([]manager.SessionInfo, error) {
	tmuxSessions, err := tmux.ListSessionsWithInfo()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	results := make([]manager.SessionInfo, 0, len(tmuxSessions))
	for _, s := range tmuxSessions {
		info := manager.SessionInfo{
			ID:       manager.SessionID(s.Name),
			Name:     s.Name,
			State:    manager.StateIdle, // Default; callers can use GetState for accuracy
			Attached: s.AttachedClients > 0,
		}

		// Apply name filter if specified
		if filter.NameMatch != "" && s.Name != filter.NameMatch {
			continue
		}

		results = append(results, info)
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}
	return results, nil
}

// GetSession returns info for a single tmux session.
func (b *TmuxBackend) GetSession(_ context.Context, id manager.SessionID) (manager.SessionInfo, error) {
	name := string(id)
	exists, err := tmux.HasSession(name)
	if err != nil {
		return manager.SessionInfo{}, fmt.Errorf("check session: %w", err)
	}
	if !exists {
		return manager.SessionInfo{}, fmt.Errorf("session %q not found", name)
	}

	// Get attachment info
	sessions, err := tmux.ListSessionsWithInfo()
	if err != nil {
		return manager.SessionInfo{ //nolint:nilerr // intentional: caller signals via separate bool/optional
			ID:    id,
			Name:  name,
			State: manager.StateIdle,
		}, nil
	}

	for _, s := range sessions {
		if s.Name == name || s.Name == tmux.NormalizeTmuxSessionName(name) {
			return manager.SessionInfo{
				ID:       id,
				Name:     s.Name,
				Attached: s.AttachedClients > 0,
				State:    manager.StateIdle,
			}, nil
		}
	}

	return manager.SessionInfo{
		ID:    id,
		Name:  name,
		State: manager.StateIdle,
	}, nil
}

// RenameSession renames a tmux session.
func (b *TmuxBackend) RenameSession(ctx context.Context, id manager.SessionID, name string) error {
	socketPath := tmux.GetSocketPath()
	normalizedOld := tmux.NormalizeTmuxSessionName(string(id))
	sanitizedNew := tmux.SanitizeSessionName(name)
	_, err := tmux.RunWithTimeout(ctx, 5*time.Second,
		"tmux", "-S", socketPath, "rename-session",
		"-t", tmux.FormatSessionTarget(normalizedOld), sanitizedNew)
	if err != nil {
		return fmt.Errorf("rename session: %w", err)
	}
	return nil
}

// AttachSession attaches the terminal to a tmux session.
func (b *TmuxBackend) AttachSession(_ context.Context, id manager.SessionID) error {
	return tmux.AttachSession(string(id))
}

// --- MessageBroker ---

// SendMessage sends a message to a tmux session via send-keys.
func (b *TmuxBackend) SendMessage(_ context.Context, id manager.SessionID, message string) (manager.SendResult, error) {
	err := tmux.SendCommand(string(id), message)
	if err != nil {
		return manager.SendResult{Delivered: false, Error: err}, fmt.Errorf("send message: %w", err)
	}
	return manager.SendResult{Delivered: true}, nil
}

// ReadOutput captures recent terminal output from a tmux session pane.
func (b *TmuxBackend) ReadOutput(_ context.Context, id manager.SessionID, lines int) (string, error) {
	if lines <= 0 {
		lines = 30
	}
	output, err := tmux.CapturePaneOutput(string(id), lines)
	if err != nil {
		return "", fmt.Errorf("read output: %w", err)
	}
	return output, nil
}

// Interrupt sends Ctrl-C to the tmux session to cancel the current operation.
func (b *TmuxBackend) Interrupt(ctx context.Context, id manager.SessionID) error {
	socketPath := tmux.GetSocketPath()
	normalizedName := tmux.NormalizeTmuxSessionName(string(id))

	// Verify session is reachable via capture-pane before sending Ctrl-C.
	// Bug fix: must confirm session exists and responds before injecting keys.
	_, captureErr := tmux.RunWithTimeout(ctx, 5*time.Second,
		"tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p")
	if captureErr != nil {
		return fmt.Errorf("capture-pane failed before interrupt: %w (session may be down)", captureErr)
	}

	_, err := tmux.RunWithTimeout(ctx, 5*time.Second,
		"tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "C-c")
	if err != nil {
		return fmt.Errorf("interrupt session: %w", err)
	}
	return nil
}

// --- StateReader ---

// GetState detects the current state of a tmux session by parsing terminal content.
func (b *TmuxBackend) GetState(_ context.Context, id manager.SessionID) (manager.StateResult, error) {
	name := string(id)

	exists, err := tmux.HasSession(name)
	if err != nil {
		return manager.StateResult{State: manager.StateOffline}, fmt.Errorf("check session: %w", err)
	}
	if !exists {
		return manager.StateResult{
			State:      manager.StateOffline,
			Confidence: 1.0,
			Evidence:   "session does not exist in tmux",
		}, nil
	}

	paneContent, err := tmux.CapturePaneOutput(name, 30)
	if err != nil {
		return manager.StateResult{ //nolint:nilerr // intentional: caller signals via separate bool/optional
			State:      manager.StateIdle,
			Confidence: 0.5,
			Evidence:   "cannot read terminal content, defaulting to IDLE",
		}, nil
	}

	detector := state.NewDetector()
	result := detector.DetectState(paneContent, time.Now())
	mappedState := mapTerminalState(result.State)

	var confidence float64
	switch result.Confidence {
	case "high":
		confidence = 0.95
	case "medium":
		confidence = 0.7
	default:
		confidence = 0.4
	}

	return manager.StateResult{
		State:      mappedState,
		Confidence: confidence,
		Evidence:   fmt.Sprintf("terminal parsing: %s (%s)", result.State, result.Evidence),
	}, nil
}

// CheckDelivery determines if a session can receive input right now.
func (b *TmuxBackend) CheckDelivery(_ context.Context, id manager.SessionID) (manager.CanReceive, error) {
	name := string(id)

	exists, err := tmux.HasSession(name)
	if err != nil || !exists {
		return manager.CanReceiveNotFound, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	paneContent, err := tmux.CapturePaneOutput(name, 30)
	if err != nil {
		return manager.CanReceiveQueue, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	detector := state.NewDetector()
	canReceive := detector.CheckCanReceive(paneContent)

	// Map state.CanReceive to manager.CanReceive
	switch canReceive {
	case state.CanReceiveYes:
		return manager.CanReceiveYes, nil
	case state.CanReceiveNo:
		return manager.CanReceiveNo, nil
	case state.CanReceiveNotFound:
		return manager.CanReceiveNotFound, nil
	default:
		return manager.CanReceiveQueue, nil
	}
}

// HealthCheck verifies tmux is available and responsive.
func (b *TmuxBackend) HealthCheck(_ context.Context) error {
	_, err := tmux.Version()
	if err != nil {
		return fmt.Errorf("tmux health check failed: %w", err)
	}
	return nil
}

// mapTerminalState converts state.State to manager.State.
func mapTerminalState(s state.State) manager.State {
	switch s {
	case state.StateReady:
		return manager.StateIdle
	case state.StateThinking:
		return manager.StateWorking
	case state.StateBlockedAuth, state.StateBlockedInput:
		return manager.StatePermissionPrompt
	case state.StateBlockedPermission:
		return manager.StatePermissionPrompt
	case state.StateStuck:
		return manager.StateWorking
	case state.StateUnknown:
		return manager.StateIdle
	default:
		return manager.StateIdle
	}
}

func init() {
	_ = manager.DefaultRegistry.Register("tmux", func() (manager.Backend, error) {
		return New(), nil
	})
}
