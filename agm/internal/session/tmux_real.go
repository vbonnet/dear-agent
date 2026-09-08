package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// The production composition root injects *RealTmux directly. Keep every
// safety-critical optional capability proven on that concrete value so a
// forwarding adapter cannot silently weaken the runtime contract.
var (
	_ TmuxInterface                 = (*RealTmux)(nil)
	_ TmuxSessionKiller             = (*RealTmux)(nil)
	_ StableSessionIdentityManager  = (*RealTmux)(nil)
	_ StrictSessionExistenceChecker = (*RealTmux)(nil)
	_ HarnessLivenessChecker        = (*RealTmux)(nil)
	_ HarnessLivenessBatchChecker   = (*RealTmux)(nil)
	_ HarnessReadinessWaiter        = (*RealTmux)(nil)
	_ InputReadinessChecker         = (*RealTmux)(nil)
	_ PaneOutputCapturer            = (*RealTmux)(nil)
	_ AtomicInputSender             = (*RealTmux)(nil)
	_ VerifiedPaneSender            = (*RealTmux)(nil)
)

// RealTmux wraps the internal/tmux package to provide TmuxInterface implementation
type RealTmux struct{}

var (
	_ TmuxInterface                  = (*RealTmux)(nil)
	_ ResumeTmuxIdentityManager      = (*RealTmux)(nil)
	_ ExpectedHarnessLivenessChecker = (*RealTmux)(nil)
	_ ResumeReadinessWaiter          = (*RealTmux)(nil)
	_ SafePromptSender               = (*RealTmux)(nil)
	_ LiteralKeySender               = (*RealTmux)(nil)
)

// NewRealTmux creates a new RealTmux instance
func NewRealTmux() *RealTmux {
	return &RealTmux{}
}

// HasSession checks if a tmux session exists
func (t *RealTmux) HasSession(name string) (bool, error) {
	return tmux.HasSession(name)
}

// CapturePaneTail returns the last lines of the session's pane content,
// including scrollback, from the AGM tmux socket.
func (t *RealTmux) CapturePaneTail(sessionName string, lines int) (string, error) {
	return tmux.CapturePaneOutput(sessionName, lines)
}

// HasSessionStrict checks an exact target without collapsing backend failures
// into a missing-session result.
func (t *RealTmux) HasSessionStrict(ctx context.Context, name string) (bool, error) {
	return tmux.HasSessionStrictContext(ctx, name)
}

// ListSessions returns all active tmux session names
func (t *RealTmux) ListSessions() ([]string, error) {
	return tmux.ListSessions()
}

// ListSessionsWithInfo returns all active tmux sessions with attachment info
func (t *RealTmux) ListSessionsWithInfoStrict() ([]SessionInfo, error) {
	tmuxSessions, err := tmux.ListSessionsWithInfoStrict()
	if err != nil {
		return nil, err
	}
	return convertSessionInfo(tmuxSessions), nil
}

func (t *RealTmux) ListSessionsWithInfo() ([]SessionInfo, error) {
	tmuxSessions, err := tmux.ListSessionsWithInfo()
	if err != nil {
		return nil, err
	}
	return convertSessionInfo(tmuxSessions), nil
}

// convertSessionInfo maps tmux.SessionInfo onto the session-package type.
func convertSessionInfo(in []tmux.SessionInfo) []SessionInfo {
	out := make([]SessionInfo, len(in))
	for i, s := range in {
		out[i] = SessionInfo{
			Name:            s.Name,
			AttachedClients: s.AttachedClients,
			AttachedList:    s.AttachedList,
		}
	}
	return out
}

// CreateSession creates a new tmux session
func (t *RealTmux) CreateSession(name, workdir string) error {
	return tmux.NewSession(name, workdir)
}

// CreateSessionBound creates a session whose tmux incarnation is bound to the
// durable AGM session identity before the command queue returns.
func (t *RealTmux) CreateSessionBound(name, workdir, stableSessionID string) error {
	return tmux.NewSessionBound(name, workdir, stableSessionID)
}

// BindSessionStableID adopts one explicitly reused tmux session.
func (t *RealTmux) BindSessionStableID(ctx context.Context, name, stableSessionID string) (bool, error) {
	return tmux.BindStableSessionIDContext(ctx, name, stableSessionID)
}

// ClearSessionStableID compensates only the exact binding written during a
// failed reused-session creation transaction.
func (t *RealTmux) ClearSessionStableID(ctx context.Context, name, stableSessionID string) error {
	return tmux.ClearStableSessionIDContext(ctx, name, stableSessionID)
}

// CreateSessionWithIdentity creates one exact tmux identity for transactional
// resume rollback.
func (t *RealTmux) CreateSessionWithIdentity(name, workdir, stableSessionID string) (tmux.SessionIdentity, error) {
	return tmux.NewSessionWithIdentityBound(name, workdir, stableSessionID)
}

// KillSession removes a tmux session created by a failed lifecycle operation.
// It implements TmuxSessionKiller without widening TmuxInterface.
func (t *RealTmux) KillSession(name string) error {
	return tmux.KillSessionChecked(name)
}

// KillSessionIdentityChecked removes only the exact creation owned by the
// current resume transaction.
func (t *RealTmux) KillSessionIdentityChecked(identity tmux.SessionIdentity) error {
	return tmux.KillSessionIdentityChecked(identity)
}

// HasSessionIdentityStrict verifies exact identity cleanup without collapsing
// tmux backend failures into absence.
func (t *RealTmux) HasSessionIdentityStrict(identity tmux.SessionIdentity) (bool, error) {
	return tmux.HasSessionIdentityStrict(identity)
}

// AttachSession attaches to a tmux session
func (t *RealTmux) AttachSession(name string) error {
	return tmux.AttachSession(name)
}

// SendKeys sends keys to a tmux session
func (t *RealTmux) SendKeys(session, keys string) error {
	return tmux.SendCommand(session, keys)
}

// SendLiteralKeys sends a key token without appending Enter.
func (t *RealTmux) SendLiteralKeys(sessionName, keys string) error {
	return tmux.SendKeys(sessionName, keys)
}

// SendKeysToPane sends to an exact pane previously returned by readiness.
func (t *RealTmux) SendKeysToPane(ctx context.Context, paneID, keys string) error {
	return tmux.SendCommandToPaneContext(ctx, paneID, keys)
}

// WaitForHarnessReady observes the harness-specific interactive boundary used
// by shared creation before registration or prompt delivery.
func (t *RealTmux) WaitForHarnessReady(ctx context.Context, sessionName, harness string, timeout time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return tmux.WaitForExpectedHarnessReady(ctx, sessionName, harness, timeout)
}

// WaitForResumeReady preserves the established native resume readiness policy
// while keeping terminal rendering outside the shared operation.
func (t *RealTmux) WaitForResumeReady(ctx context.Context, sessionName, harness, piLaunchID string, timeout time.Duration) (ResumeReadiness, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ResumeReadiness{}, err
	}
	switch agent.NormalizeHarnessName(harness) {
	case "claude-code":
		result := ResumeReadiness{}
		if err := tmux.WaitForProcessReadyContext(ctx, sessionName, "claude", 15*time.Second); err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			result.Warnings = append(result.Warnings, "Claude process is taking longer than expected")
		}
		err := tmux.WaitForPromptOrResumeFailureContext(ctx, sessionName, timeout)
		if err == nil {
			return result, nil
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		var resumeFailure *tmux.ResumeFailureError
		if errors.As(err, &resumeFailure) {
			return result, fmt.Errorf("claude could not resume this conversation: %s", resumeFailure.Detail)
		}
		result.Warnings = append(result.Warnings, "Claude conversation is taking longer than expected to load")
		return result, nil
	case "codex-cli":
		return waitForCodexResumeReady(
			ctx,
			sessionName,
			timeout,
			tmux.WaitForProcessReadyContext,
			tmux.WaitForCodexPromptContext,
		)
	case "agy":
		return waitForAgyResumeReady(ctx, sessionName, timeout, tmux.WaitForAgyPromptOnResume)
	case "pi-cli":
		if err := tmux.WaitForPiLaunchPromptContext(ctx, sessionName, piLaunchID, timeout); err != nil {
			return ResumeReadiness{}, fmt.Errorf("pi exact resume did not reach managed readiness: %w", err)
		}
		return ResumeReadiness{}, nil
	case "opencode-cli":
		return waitForOpenCodeResumeReady(ctx, sessionName, timeout, tmux.WaitForExpectedHarnessReady)
	default:
		return ResumeReadiness{}, nil
	}
}

func waitForOpenCodeResumeReady(
	ctx context.Context,
	sessionName string,
	timeout time.Duration,
	waitForHarness func(context.Context, string, string, time.Duration) error,
) (ResumeReadiness, error) {
	if err := waitForHarness(ctx, sessionName, "opencode-cli", timeout); err != nil {
		return ResumeReadiness{}, fmt.Errorf("opencode composer did not become ready: %w", err)
	}
	return ResumeReadiness{}, nil
}

func waitForCodexResumeReady(
	ctx context.Context,
	sessionName string,
	timeout time.Duration,
	waitForProcess func(context.Context, string, string, time.Duration) error,
	waitForComposer func(context.Context, string, time.Duration) error,
) (ResumeReadiness, error) {
	if err := waitForProcess(ctx, sessionName, "codex", timeout); err != nil {
		return ResumeReadiness{}, fmt.Errorf("codex process did not become ready: %w", err)
	}
	if err := waitForComposer(ctx, sessionName, timeout); err != nil {
		return ResumeReadiness{}, fmt.Errorf("codex composer did not become ready: %w", err)
	}
	return ResumeReadiness{}, nil
}

func waitForAgyResumeReady(
	ctx context.Context,
	sessionName string,
	timeout time.Duration,
	waitForPrompt func(context.Context, string, time.Duration) error,
) (ResumeReadiness, error) {
	err := waitForPrompt(ctx, sessionName, timeout)
	if err == nil {
		return ResumeReadiness{}, nil
	}
	if ctx.Err() != nil || errors.Is(err, tmux.ErrAgyOnboardingRequired) {
		return ResumeReadiness{}, err
	}
	return ResumeReadiness{Warnings: []string{"AGY conversation is taking longer than expected to load"}}, nil
}

// CheckInputReadiness captures the current pane and classifies whether an
// interactive harness composer can safely receive input.
func (t *RealTmux) CheckInputReadiness(ctx context.Context, sessionName, harness string) (InputReadiness, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	readiness, err := tmux.CheckExpectedHarnessInput(ctx, sessionName, harness)
	if err != nil {
		return InputReadiness{}, err
	}
	return InputReadiness{
		Ready: readiness.Ready, State: readiness.State,
		PaneID: readiness.TargetPane, PanePID: readiness.TargetPanePID, TargetPID: readiness.TargetPID,
		HarnessStartTime: readiness.HarnessStartTime,
		TargetSessionID:  readiness.TargetSessionID, StableSessionID: readiness.StableSessionID,
	}, nil
}

// SendPrompt submits one prompt using the active harness's native composer
// semantics and preserves acknowledgement uncertainty.
func (t *RealTmux) SendPrompt(ctx context.Context, sessionName, harness, prompt string) (PromptSubmission, error) {
	err := tmux.SendMultiLinePromptSafeForHarnessContext(ctx, sessionName, prompt, false, agent.NormalizeHarnessName(harness))
	if err != nil {
		return PromptSubmission{MayHaveStarted: tmux.PromptSubmissionMayHaveOccurred(err)}, err
	}
	return PromptSubmission{MayHaveStarted: true}, nil
}

// SendKeysIfInputReady serializes exact harness readiness with exact-pane
// delivery so another AGM sender cannot invalidate the observation.
func (t *RealTmux) SendKeysIfInputReady(ctx context.Context, sessionName, harness, keys string, options InputDeliveryOptions) (InputReadiness, error) {
	readiness, err := tmux.CheckExpectedHarnessInputAndSend(ctx, sessionName, harness, keys, tmux.InputDeliveryOptions{
		AllowQueuedAGM:                options.AllowQueuedAGM,
		RequireSubmissionConfirmation: options.RequireSubmissionConfirmation,
		RawBracketedPaste:             options.RawBracketedPaste,
		ExpectedStableSessionID:       options.ExpectedStableSessionID,
	})
	result := InputReadiness{
		Ready: readiness.Ready, State: readiness.State,
		PaneID: readiness.TargetPane, PanePID: readiness.TargetPanePID, TargetPID: readiness.TargetPID,
		HarnessStartTime: readiness.HarnessStartTime,
		TargetSessionID:  readiness.TargetSessionID, StableSessionID: readiness.StableSessionID,
		MayHaveStarted:       readiness.MayHaveStarted,
		PostSubmitProcessing: readiness.PostSubmitProcessing,
		Forced:               readiness.Forced,
	}
	if err != nil {
		return result, err
	}
	return result, nil
}

// HarnessLiveness scans the session's pane process tree for a live harness
// process (ce-axsr). Implements HarnessLivenessChecker.
func (t *RealTmux) HarnessLiveness(sessionName string) (LivenessInfo, error) {
	pl, err := tmux.CheckPaneLiveness(sessionName, tmux.GetSocketPath())
	if err != nil {
		return LivenessInfo{}, err
	}
	return LivenessInfo{
		SessionExists:    pl.SessionExists,
		HarnessAlive:     pl.HarnessAlive,
		ZombieWriter:     pl.ZombieWriter,
		RestartableShell: pl.RestartableShell,
		Evidence:         pl.Evidence,
	}, nil
}

// ExpectedHarnessLiveness scans for the configured harness rather than any
// generic agent process.
func (t *RealTmux) ExpectedHarnessLiveness(ctx context.Context, sessionName, harness string) (LivenessInfo, error) {
	pl, err := tmux.CheckExpectedHarnessLiveness(ctx, sessionName, harness)
	if err != nil {
		return LivenessInfo{}, err
	}
	return LivenessInfo{
		SessionExists:    pl.SessionExists,
		HarnessAlive:     pl.HarnessAlive,
		ZombieWriter:     pl.ZombieWriter,
		RestartableShell: pl.RestartableShell,
		Evidence:         pl.Evidence,
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
			SessionExists:    pl.SessionExists,
			HarnessAlive:     pl.HarnessAlive,
			ZombieWriter:     pl.ZombieWriter,
			RestartableShell: pl.RestartableShell,
			Evidence:         pl.Evidence,
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
