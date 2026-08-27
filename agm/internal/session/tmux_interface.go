package session

import (
	"context"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// SessionInfo holds information about a tmux session
type SessionInfo struct {
	Name            string
	AttachedClients int    // Number of clients attached to this session
	AttachedList    string // Comma-separated list of attached TTYs (e.g., "/dev/pts/3,/dev/pts/8")
}

// ClientInfo holds information about a tmux client
type ClientInfo struct {
	SessionName string
	TTY         string
	PID         int
}

// LivenessInfo is the harness-process liveness verdict for a tmux session
// (ce-axsr). `tmux has-session` alone is false-green: the session keeps
// existing after the harness exits and the pane falls back to a shell.
type LivenessInfo struct {
	// SessionExists reports whether the tmux session was found at all.
	SessionExists bool
	// HarnessAlive reports whether a harness process (claude, codex, agy,
	// node, …) is running in a pane's descendant process tree.
	HarnessAlive bool
	// ZombieWriter reports that no harness is alive but an agm process is
	// still in the pane tree — the ce-qkf7 orphaned-heartbeat-writer mode.
	ZombieWriter bool
	// RestartableShell reports that the pane contains only its root shell and
	// can safely accept a harness relaunch.
	RestartableShell bool
	// Evidence summarizes the pane's descendant process names.
	Evidence string
}

// HarnessLivenessChecker is an optional capability a TmuxInterface
// implementation can provide: proving (not assuming) that a harness process
// runs inside the session. Callers discover it by type assertion so existing
// mocks keep compiling.
type HarnessLivenessChecker interface {
	// HarnessLiveness scans the session's pane process tree for a live
	// harness process.
	HarnessLiveness(sessionName string) (LivenessInfo, error)
}

// HarnessLivenessBatchChecker is the batch form of HarnessLivenessChecker:
// one scan for many sessions using a constant number of subprocesses. List
// paths must prefer this capability so N sessions do not cost 3N subprocess
// spawns. The result map is keyed by the requested session names.
type HarnessLivenessBatchChecker interface {
	HarnessLivenessBatch(sessionNames []string) (map[string]LivenessInfo, error)
}

// TmuxSessionKiller is the optional destructive counterpart to
// TmuxInterface.CreateSession. Creation operations discover this capability
// during rollback so a failed post-create step cannot strand a fresh tmux
// session. Keeping it separate preserves compatibility with lightweight tmux
// implementations that only support the original interface.
type TmuxSessionKiller interface {
	KillSession(name string) error
}

// StableSessionIdentityManager creates new tmux sessions with a durable AGM
// identity binding and transactionally adopts an explicitly reused session.
// Creation requires this capability so later strict delivery never relies on
// a display name that a replacement can reuse.
type StableSessionIdentityManager interface {
	CreateSessionBound(name, workdir, stableSessionID string) error
	BindSessionStableID(ctx context.Context, name, stableSessionID string) (newlyBound bool, err error)
	ClearSessionStableID(ctx context.Context, name, stableSessionID string) error
}

// StrictSessionExistenceChecker distinguishes an absent exact target from
// socket, permission, timeout, and other tmux failures. Destructive operations
// use this capability so backend failure cannot be mistaken for absence.
type StrictSessionExistenceChecker interface {
	HasSessionStrict(ctx context.Context, name string) (bool, error)
}

// HarnessReadinessWaiter is the optional startup capability used by shared
// lifecycle operations that launch a harness without a surface runtime. A nil
// error means the named harness has reached its interactive prompt/composer.
type HarnessReadinessWaiter interface {
	WaitForHarnessReady(ctx context.Context, sessionName, harness string, timeout time.Duration) error
}

// ResumeTmuxIdentityManager creates and compensates one exact tmux session
// identity. Resume operations require this capability for cold-start rollback
// so a same-named replacement can never be destroyed.
type ResumeTmuxIdentityManager interface {
	CreateSessionWithIdentity(name, workdir, stableSessionID string) (tmux.SessionIdentity, error)
	KillSessionIdentityChecked(identity tmux.SessionIdentity) error
	HasSessionIdentityStrict(identity tmux.SessionIdentity) (bool, error)
}

// ExpectedHarnessLivenessChecker classifies the configured harness rather than
// accepting any generic agent process. Resume uses this capability to decide
// whether an existing Pi pane is a safe restartable shell.
type ExpectedHarnessLivenessChecker interface {
	ExpectedHarnessLiveness(ctx context.Context, sessionName, harness string) (LivenessInfo, error)
}

// ResumeReadiness is the native readiness outcome. Warnings preserve the
// existing permissive Claude and AGY slow-start policy without leaking
// terminal presentation into the operation adapter.
type ResumeReadiness struct {
	Warnings []string
}

// ResumeReadinessWaiter observes the native readiness boundary for a resumed
// harness. PiLaunchID binds Pi readiness to the exact command submitted by the
// current operation.
type ResumeReadinessWaiter interface {
	WaitForResumeReady(ctx context.Context, sessionName, harness, piLaunchID string, timeout time.Duration) (ResumeReadiness, error)
}

// PromptSubmission is the irreversible outcome of safe prompt delivery.
// MayHaveStarted is true both for confirmed delivery and for a lost
// acknowledgement after the final submission boundary.
type PromptSubmission struct {
	MayHaveStarted bool
}

// SafePromptSender preserves the active harness's native composer semantics
// while submitting one optional post-resume prompt.
type SafePromptSender interface {
	SendPrompt(ctx context.Context, sessionName, harness, prompt string) (PromptSubmission, error)
}

// LiteralKeySender sends one tmux key token without adding Enter. Resume uses
// it only to replay saved permission-mode Shift+Tab transitions.
type LiteralKeySender interface {
	SendLiteralKeys(sessionName, keys string) error
}

// InputReadiness is the observable result of inspecting a tmux pane before
// message delivery. State is the detector verdict (for example YES, NO,
// QUEUE, OVERLAY, or NOT_FOUND).
type InputReadiness struct {
	Ready  bool
	State  string
	PaneID string
	// PanePID is tmux's root process for PaneID, captured with the pane and
	// session incarnation. Pane IDs can be reused after a server restart.
	PanePID int
	// TargetPID is the foreground harness process captured in the same readiness proof.
	// Identity-bound follow-up observers require both PaneID and TargetPID.
	TargetPID int
	// HarnessStartTime is the operating-system birth identity for TargetPID.
	// It prevents PID reuse from attributing later work to the delivered turn.
	HarnessStartTime string
	// TargetSessionID is tmux's server-local session incarnation ID observed
	// with PaneID. StableSessionID is AGM's durable identity bound as a tmux
	// session option. Strict follow-up observers require both so a server
	// restart or same-named replacement cannot inherit completion evidence.
	TargetSessionID string
	StableSessionID string
	// MayHaveStarted records that delivery crossed the irreversible submission
	// boundary even though its final acknowledgement was lost. Callers must not
	// turn this into an ordinary retry-shaped failure.
	MayHaveStarted bool
	// PostSubmitProcessing is positive exact-target evidence that the harness
	// entered its native processing state after submission.
	PostSubmitProcessing bool
	Forced               bool
}

// InputDeliveryOptions controls narrowly scoped exceptions inside the atomic
// readiness-and-delivery boundary. AllowQueuedAGM accepts only a positively
// identified queued AGM paste; it never bypasses generic busy states, human
// drafts, process ownership, permission, overlays, onboarding, missing targets,
// or other fail-closed states.
type InputDeliveryOptions struct {
	AllowQueuedAGM bool
	// RequireSubmissionConfirmation asks the runtime to preserve uncertainty
	// when Enter was accepted but every post-submit observation failed, and to
	// re-prove the exact submitted target as live-ready or natively processing.
	RequireSubmissionConfirmation bool
	// RawBracketedPaste preserves multiline input as one composer paste instead
	// of allowing tmux to translate embedded newlines into submit keys.
	RawBracketedPaste bool
	// ExpectedStableSessionID binds the tmux incarnation to the durable AGM
	// session identity before any terminal mutation.
	ExpectedStableSessionID string
}

// InputReadinessChecker is the optional pre-delivery capability used by
// shared message operations. Delivery is safe only when Ready is true.
type InputReadinessChecker interface {
	CheckInputReadiness(ctx context.Context, sessionName, harness string) (InputReadiness, error)
}

// PaneOutputCapturer is the optional read capability used by result-surfacing
// operations to return the tail of a session's terminal output.
type PaneOutputCapturer interface {
	CapturePaneTail(sessionName string, lines int) (string, error)
}

// AtomicInputSender checks harness input ownership and delivers to the
// resulting exact pane while holding one tmux mutation boundary. If Ready is
// false, no input was sent unless MayHaveStarted is true; if Ready is true and
// error is nil, delivery completed successfully. PaneID/PanePID/TargetPID
// identify the exact runtime for confirmed and submission-uncertain outcomes.
// A forced result is valid only for a positively identified queued AGM paste
// and reports the post-clear empty-composer state YES.
type AtomicInputSender interface {
	SendKeysIfInputReady(ctx context.Context, sessionName, harness, keys string, options InputDeliveryOptions) (InputReadiness, error)
}

// VerifiedPaneSender delivers to the exact pane returned by
// InputReadinessChecker, preventing active-pane changes from redirecting input.
type VerifiedPaneSender interface {
	SendKeysToPane(ctx context.Context, paneID, keys string) error
}

// TmuxInterface provides an abstraction for tmux operations
// This allows mocking tmux in tests without requiring real tmux to be installed
type TmuxInterface interface {
	// HasSession checks if a tmux session with the given name exists
	HasSession(name string) (bool, error)

	// ListSessions returns all active tmux session names
	ListSessions() ([]string, error)

	// ListSessionsWithInfo returns all active tmux sessions with attachment info
	ListSessionsWithInfo() ([]SessionInfo, error)

	// ListClients returns all clients attached to a specific session
	ListClients(sessionName string) ([]ClientInfo, error)

	// CreateSession creates a new tmux session with the given name and working directory
	CreateSession(name, workdir string) error

	// AttachSession attaches to or switches to the given tmux session
	AttachSession(name string) error

	// SendKeys sends keys (command) to the given tmux session
	SendKeys(session, keys string) error
}
