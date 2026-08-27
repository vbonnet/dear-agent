package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/compaction"
	"github.com/vbonnet/dear-agent/agm/internal/fileutil"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// SessionCompactionDeliveryRequest identifies one durable session and the
// complete command that should be submitted to its interactive harness.
type SessionCompactionDeliveryRequest struct {
	// Identifier is the exact stable AGM session ID established by the caller's
	// resolution/preflight step. This operation intentionally performs no name,
	// tmux-name, UUID-prefix, or replacement fallback.
	Identifier string `json:"session_id"`
	Command    string `json:"command"`
	Forced     bool   `json:"forced,omitempty"`
	// ExpectedPreservation marks Command as a prompt composed from mutable
	// session preservation context. DeliverSessionCompaction recomposes the
	// command from the locked manifest and current state file before allocating
	// an audit record, and rejects any mismatch as definite non-delivery.
	ExpectedPreservation *SessionCompactionPreservationExpectation `json:"-"`
}

// SessionCompactionPreservationExpectation carries the immutable caller input
// needed to recompose a generated preservation prompt. The expected snapshot
// itself is Request.Command, keeping the mutation interface small.
type SessionCompactionPreservationExpectation struct {
	Focus string
}

// SessionCompactionDeliveryResult is the exact runtime identity that accepted
// a compaction command. SessionID is the durable lock/reload identity; PaneID
// and TargetPID are the pinned tmux process identity used for delivery.
type SessionCompactionDeliveryResult struct {
	Operation            string                    `json:"operation"`
	SessionID            string                    `json:"session_id"`
	Name                 string                    `json:"name"`
	TmuxName             string                    `json:"tmux_name"`
	Harness              string                    `json:"harness"`
	PaneID               string                    `json:"pane_id"`
	PanePID              int                       `json:"pane_pid"`
	TargetPID            int                       `json:"target_pid"`
	HarnessStartTime     string                    `json:"harness_start_time"`
	TmuxSessionID        string                    `json:"tmux_session_id"`
	PromptFile           string                    `json:"prompt_file"`
	AttemptID            string                    `json:"attempt_id"`
	AttemptOutcome       compaction.AttemptOutcome `json:"attempt_outcome"`
	Delivered            bool                      `json:"delivered"`
	MayHaveStarted       bool                      `json:"may_have_started"`
	PostSubmitProcessing bool                      `json:"post_submit_processing_observed,omitempty"`
	AccountingPending    bool                      `json:"accounting_pending"`
}

type compactionDeliveryAttempt interface {
	ID() string
	Mark(compaction.AttemptOutcome) error
}

// compactionDeliveryAccounting keeps ledger ordering behind one operations
// boundary. Callers cannot load, mutate, and save accounting state themselves.
type compactionDeliveryAccounting interface {
	AllocatePrompt(baseDir, sessionID, content string) (compaction.PromptAllocation, error)
	BeginAttempt(baseDir, sessionID, displayName, promptFile string, forced bool) (compactionDeliveryAttempt, error)
}

type durableCompactionDeliveryAccounting struct{}

func (durableCompactionDeliveryAccounting) AllocatePrompt(baseDir, sessionID, content string) (compaction.PromptAllocation, error) {
	return compaction.AllocatePromptExclusive(baseDir, sessionID, content)
}

func (durableCompactionDeliveryAccounting) BeginAttempt(baseDir, sessionID, displayName, promptFile string, forced bool) (compactionDeliveryAttempt, error) {
	return compaction.BeginAttempt(baseDir, sessionID, displayName, promptFile, forced)
}

// DeliverSessionCompaction resolves one stable session identity, serializes
// delivery with lifecycle changes, reloads the authoritative manifest, and
// durably begins one stable-ID-keyed attempt, and submits the command only
// through the tmux runtime's atomic exact-pane readiness-and-delivery
// capability. The stable-session lock is retained through terminal accounting.
func DeliverSessionCompaction(opCtx *OpContext, req *SessionCompactionDeliveryRequest) (*SessionCompactionDeliveryResult, error) {
	if err := validateSessionCompactionDeliveryRequest(opCtx, req); err != nil {
		return nil, err
	}
	resolved, err := resolveExactCompactionSession(opCtx, req.Identifier)
	if err != nil {
		return nil, err
	}

	tx := &sessionCompactionDelivery{
		opCtx:    opCtx,
		request:  req,
		callCtx:  requestContext(opCtx),
		resolved: resolved,
	}
	if err := tx.callCtx.Err(); err != nil {
		return nil, err
	}
	err = WithSessionLockContext(tx.callCtx, resolved.SessionID, tx.runLocked)
	return tx.result, err
}

func validateSessionCompactionDeliveryRequest(opCtx *OpContext, req *SessionCompactionDeliveryRequest) error {
	if req == nil || req.Identifier == "" {
		return ErrInvalidInput("identifier", "Session identifier is required.")
	}
	if req.Command == "" {
		return ErrInvalidInput("command", "Compaction command is required.")
	}
	if err := ValidateCompactionCommandText(req.Command); err != nil {
		return ErrInvalidInput("command", err.Error())
	}
	if req.Command != "/compact" && !strings.HasPrefix(req.Command, "/compact ") {
		return ErrInvalidInput("command", "Compaction delivery accepts only a /compact command.")
	}
	if opCtx == nil || opCtx.Storage == nil {
		return ErrStorageError("session_compaction_delivery storage", errors.New("session storage is required"))
	}
	if opCtx.CompactionBaseDir == "" || !filepath.IsAbs(opCtx.CompactionBaseDir) {
		return ErrStorageError("session_compaction_delivery accounting", errors.New("an absolute trusted compaction base directory is required"))
	}
	return nil
}

func resolveExactCompactionSession(opCtx *OpContext, identifier string) (*manifest.Manifest, error) {
	resolved, err := opCtx.Storage.GetSession(identifier)
	if err != nil {
		return nil, ErrStorageError("session_compaction_delivery_exact_resolve", err)
	}
	if resolved == nil {
		return nil, ErrSessionNotFound(identifier)
	}
	if resolved.SessionID != identifier {
		return nil, ErrStorageError(
			"session_compaction_delivery_exact_resolve",
			fmt.Errorf("resolved stable session ID %q, expected %q", stableSessionID(resolved), identifier),
		)
	}
	return resolved, nil
}

type sessionCompactionDelivery struct {
	opCtx    *OpContext
	request  *SessionCompactionDeliveryRequest
	callCtx  context.Context
	resolved *manifest.Manifest
	result   *SessionCompactionDeliveryResult
}

func (d *sessionCompactionDelivery) runLocked() error {
	current, err := d.reloadAndValidateCurrent()
	if err != nil {
		return err
	}
	sender, accounting, err := d.prepareRuntime(current)
	if err != nil {
		return err
	}
	attempt, err := d.beginAttempt(current, accounting)
	if err != nil {
		return err
	}
	return d.deliverAttempt(current, sender, attempt)
}

func (d *sessionCompactionDelivery) reloadAndValidateCurrent() (*manifest.Manifest, error) {
	if err := d.callCtx.Err(); err != nil {
		return nil, err
	}
	current, err := d.opCtx.Storage.GetSession(d.resolved.SessionID)
	if err != nil {
		return nil, ErrStorageError("session_compaction_delivery_reload", err)
	}
	if current == nil {
		return nil, ErrSessionNotFound(d.resolved.SessionID)
	}
	if current.SessionID == "" || current.SessionID != d.resolved.SessionID {
		return nil, ErrStorageError("session_compaction_delivery_reload", errors.New("reloaded session did not preserve its stable session ID"))
	}

	d.result = newSessionCompactionDeliveryResult(current)
	if err := requireActiveDeliverySession(current, d.resolved.Name); err != nil {
		return nil, err
	}
	// A concurrent rename makes the caller's display context stale even when
	// the durable identity is unchanged. Reject it before accounting or input.
	if current.Name != d.resolved.Name {
		return nil, ErrSessionNotReady(current.Name, "SESSION_RENAMED_DURING_COMPACTION")
	}
	if isAPISessionManifest(current) {
		return nil, ErrSessionNotReady(current.Name, "PURE_API_SESSION")
	}
	if err := d.validatePreservation(current); err != nil {
		return nil, err
	}
	return current, nil
}

func (d *sessionCompactionDelivery) validatePreservation(current *manifest.Manifest) error {
	if d.request.ExpectedPreservation == nil {
		return nil
	}
	currentCommand, err := ComposeSessionCompactionPreservation(
		d.opCtx.CompactionBaseDir,
		toSessionDetail(current, ""),
		d.request.ExpectedPreservation.Focus,
	)
	if err != nil || currentCommand != d.request.Command {
		return ErrSessionNotReady(current.Name, "PRESERVATION_CONTEXT_CHANGED")
	}
	return nil
}

func (d *sessionCompactionDelivery) prepareRuntime(current *manifest.Manifest) (session.AtomicInputSender, compactionDeliveryAccounting, error) {
	tmuxName := current.Tmux.SessionName
	if tmuxName == "" {
		tmuxName = current.Name
	}
	harness := current.Harness
	if harness == "" {
		harness = manifest.DefaultHarness
	}
	d.result.TmuxName = tmuxName
	d.result.Harness = agent.NormalizeHarnessName(harness)

	sender, ok := d.opCtx.Tmux.(session.AtomicInputSender)
	if !ok {
		return nil, nil, ErrSessionNotReady(current.Name, "ATOMIC_DELIVERY_UNAVAILABLE")
	}
	accounting := d.opCtx.compactionAccounting
	if accounting == nil {
		accounting = durableCompactionDeliveryAccounting{}
	}
	return sender, accounting, nil
}

func (d *sessionCompactionDelivery) beginAttempt(current *manifest.Manifest, accounting compactionDeliveryAccounting) (compactionDeliveryAttempt, error) {
	prompt, err := accounting.AllocatePrompt(d.opCtx.CompactionBaseDir, current.SessionID, d.request.Command)
	if err != nil {
		return nil, ErrStorageError("compaction_prompt_allocate", err)
	}
	d.result.PromptFile = prompt.Path

	attempt, err := accounting.BeginAttempt(
		d.opCtx.CompactionBaseDir,
		current.SessionID,
		current.Name,
		prompt.Path,
		d.request.Forced,
	)
	if err != nil {
		return nil, d.handleAttemptRejection(current.Name, prompt.Path, err)
	}
	d.result.AttemptID = attempt.ID()
	d.result.AttemptOutcome = compaction.AttemptOutcomePending
	return attempt, nil
}

func (d *sessionCompactionDelivery) handleAttemptRejection(name, promptPath string, beginErr error) error {
	cleanupErr := removeUnboundCompactionPrompt(promptPath)
	if cleanupErr != nil {
		return ErrStorageError("compaction_prompt_cleanup_after_rejection", errors.Join(beginErr, cleanupErr))
	}
	d.result.PromptFile = ""
	if errors.Is(beginErr, compaction.ErrAntiLoopRejected) {
		return ErrCompactionPolicy(name, beginErr)
	}
	return ErrStorageError("compaction_attempt_begin", beginErr)
}

func (d *sessionCompactionDelivery) deliverAttempt(
	current *manifest.Manifest,
	sender session.AtomicInputSender,
	attempt compactionDeliveryAttempt,
) error {
	if cancelErr := d.callCtx.Err(); cancelErr != nil {
		if markErr := markCompactionAttempt(d.result, attempt, compaction.AttemptOutcomeDefiniteNotSent); markErr != nil {
			return ErrStorageError("compaction_attempt_cancel", errors.Join(cancelErr, markErr))
		}
		return cancelErr
	}

	readiness, sendErr := sender.SendKeysIfInputReady(
		d.callCtx,
		d.result.TmuxName,
		d.result.Harness,
		d.request.Command,
		session.InputDeliveryOptions{
			AllowQueuedAGM:                false,
			RequireSubmissionConfirmation: true,
			RawBracketedPaste:             strings.Contains(d.request.Command, "\n"),
			ExpectedStableSessionID:       current.SessionID,
		},
	)
	recordCompactionReadiness(d.result, readiness)
	if err := finalizeRejectedCompactionSend(d.result, attempt, current.Name, readiness, sendErr); err != nil {
		return err
	}
	if err := validateExactCompactionReceipt(readiness, current.SessionID); err != nil {
		d.result.MayHaveStarted = true
		return finalizeUncertainCompaction(d.result, attempt, current.Name, err)
	}

	d.result.Delivered = true
	d.result.MayHaveStarted = true
	if markErr := markCompactionAttempt(d.result, attempt, compaction.AttemptOutcomeConfirmed); markErr != nil {
		return ErrDeliveryAccounting(current.Name, markErr)
	}
	return nil
}

func recordCompactionReadiness(result *SessionCompactionDeliveryResult, readiness session.InputReadiness) {
	result.PaneID = readiness.PaneID
	result.PanePID = readiness.PanePID
	result.TargetPID = readiness.TargetPID
	result.HarnessStartTime = readiness.HarnessStartTime
	result.TmuxSessionID = readiness.TargetSessionID
	result.MayHaveStarted = readiness.MayHaveStarted
	result.PostSubmitProcessing = readiness.PostSubmitProcessing
}

func finalizeRejectedCompactionSend(
	result *SessionCompactionDeliveryResult,
	attempt compactionDeliveryAttempt,
	name string,
	readiness session.InputReadiness,
	sendErr error,
) error {
	if readiness.MayHaveStarted {
		cause := sendErr
		if cause == nil {
			cause = errors.New("atomic sender crossed the submission boundary without confirming delivery")
		}
		return finalizeUncertainCompaction(result, attempt, name, cause)
	}
	if sendErr != nil {
		if markErr := markCompactionAttempt(result, attempt, compaction.AttemptOutcomeDefiniteNotSent); markErr != nil {
			return ErrStorageError("compaction_attempt_not_sent", errors.Join(sendErr, markErr))
		}
		return ErrStorageError("tmux.SendKeysIfInputReady", sendErr)
	}
	if !readiness.Ready {
		if markErr := markCompactionAttempt(result, attempt, compaction.AttemptOutcomeDefiniteNotSent); markErr != nil {
			return ErrStorageError("compaction_attempt_not_sent", markErr)
		}
		return ErrSessionNotReady(name, readiness.State)
	}
	return nil
}

func validateExactCompactionReceipt(readiness session.InputReadiness, expectedStableID string) error {
	if readiness.PaneID != "" && readiness.PanePID > 0 && readiness.TargetPID > 0 && readiness.HarnessStartTime != "" &&
		readiness.TargetSessionID != "" && readiness.StableSessionID == expectedStableID {
		return nil
	}
	return fmt.Errorf(
		"atomic sender confirmed delivery without an exact stable-bound runtime receipt (pane=%q pane_pid=%d harness_pid=%d harness_start=%q tmux_session=%q stable_session=%q expected_stable=%q)",
		readiness.PaneID, readiness.PanePID, readiness.TargetPID, readiness.HarnessStartTime, readiness.TargetSessionID,
		readiness.StableSessionID, expectedStableID,
	)
}

// ValidateCompactionCommandText rejects terminal control bytes before a
// compaction command is audited, displayed, or delivered. LF is the sole
// allowed control because multiline preservation text is pasted atomically.
func ValidateCompactionCommandText(command string) error {
	if !utf8.ValidString(command) {
		return errors.New("compaction delivery requires valid UTF-8")
	}
	for _, r := range command {
		if r != '\n' && unicode.IsControl(r) {
			return fmt.Errorf("compaction delivery rejects control character %U; only line feeds are allowed for multiline preservation text", r)
		}
	}
	return nil
}

// ComposeSessionCompactionPreservation builds the command from one resolved
// session snapshot and its stable-ID-bound state file. A missing or invalid
// state file is usable only when focus supplies an explicit fallback.
func ComposeSessionCompactionPreservation(baseDir string, target SessionDetail, focus string) (string, error) {
	state, stateFilePath, err := compaction.LoadSessionState(baseDir, target.ID, target.Name)
	if err == nil {
		return compaction.GeneratePreservePrompt(state, stateFilePath, focus, target.Name), nil
	}
	if focus == "" {
		return "", err
	}

	harness := target.Harness
	if harness == "" {
		harness = manifest.DefaultHarness
	}
	return compaction.GeneratePrompt(&compaction.PromptInput{
		SessionName: target.Name,
		Project:     target.Project,
		Purpose:     target.Purpose,
		Tags:        target.Tags,
		Harness:     agent.NormalizeHarnessName(harness),
		FocusText:   focus,
	}), nil
}

func removeUnboundCompactionPrompt(path string) error {
	return removeUnboundCompactionPromptWithDirSync(path, fileutil.SyncDir)
}

func removeUnboundCompactionPromptWithDirSync(path string, syncDir func(string) error) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove unbound compaction prompt %s: %w", path, err)
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("persist unbound compaction prompt removal %s: %w", path, err)
	}
	return nil
}

func markCompactionAttempt(result *SessionCompactionDeliveryResult, attempt compactionDeliveryAttempt, outcome compaction.AttemptOutcome) error {
	if err := attempt.Mark(outcome); err != nil {
		result.AccountingPending = true
		return err
	}
	result.AttemptOutcome = outcome
	return nil
}

func finalizeUncertainCompaction(
	result *SessionCompactionDeliveryResult,
	attempt compactionDeliveryAttempt,
	name string,
	cause error,
) error {
	result.MayHaveStarted = true
	if markErr := markCompactionAttempt(result, attempt, compaction.AttemptOutcomeUncertain); markErr != nil {
		cause = errors.Join(cause, fmt.Errorf("persist uncertain compaction outcome: %w", markErr))
	}
	return ErrDeliveryUncertain(name, cause)
}

func newSessionCompactionDeliveryResult(m *manifest.Manifest) *SessionCompactionDeliveryResult {
	result := &SessionCompactionDeliveryResult{Operation: "deliver_session_compaction"}
	if m != nil {
		result.SessionID = m.SessionID
		result.Name = m.Name
	}
	return result
}

func stableSessionID(m *manifest.Manifest) string {
	if m == nil {
		return ""
	}
	return m.SessionID
}
