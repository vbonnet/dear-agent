package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// MinUnstickReason is the minimum length of the free-form justification required
// to unstick a session's input. Mirrors the override-guard convention used by
// break-glass and other sanctioned-override commands: a recovery action that can
// touch another worker's input line must carry an auditable reason.
const MinUnstickReason = 5

var unstickReason string

var sessionUnstickCmd = &cobra.Command{
	Use:   "unstick <session-name>",
	Short: "Clear a session's stuck input (sandbox-safe self-recovery)",
	Long: `Recover a worker whose input line is stuck — a ghost-text / human_typing block
or a stuck AGM paste-buffer — without needing direct tmux socket access.

This is the sanctioned, auditable counterpart to 'agm send clear-input'. It is
callable from any sandbox that can reach the agm CLI: it uses the same default
tmux socket path as every other agm tmux op (override via AGM_TMUX_SOCKET if a
sandbox must point at the host socket).

It captures the target session's pane, classifies the queued input, and:
  - stuck AGM message ([From: ...] header) → sends Enter to submit it
  - freeform human text                    → sends Ctrl+S to stash it
    (the stashed text is restored automatically on the worker's next interaction)
  - no queued input                        → no-op

A --reason (min 5 chars) is required and every clear action is appended to
~/.local/state/dear-agent/unstick-audit.jsonl (honoring XDG_STATE_HOME).

Examples:
  agm session unstick worker-42 --reason "ghost-text block, supervisor recovery"
  agm session unstick my-session --reason "stuck paste-buffer after restart"`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionUnstick,
}

func init() {
	sessionUnstickCmd.Flags().StringVar(&unstickReason, "reason", "",
		"Reason for unsticking the session (required, min 5 chars)")
	_ = sessionUnstickCmd.MarkFlagRequired("reason")
	sessionCmd.AddCommand(sessionUnstickCmd)
}

func runSessionUnstick(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	if err := validateUnstickReason(unstickReason); err != nil {
		return err
	}

	// Capture the pane to inspect queued input.
	paneContent, err := tmux.CapturePaneOutput(sessionName, 50)
	if err != nil {
		return fmt.Errorf("failed to capture pane for session '%s': %w", sessionName, err)
	}

	inputType, description := tmux.ClassifyQueuedInput(paneContent)

	switch inputType {
	case tmux.QueuedInputNone:
		// Nothing queued — surface it but don't fabricate an audit record for a
		// clear that never happened.
		ui.PrintSuccess(fmt.Sprintf("no queued input detected in session '%s' — nothing to unstick", sessionName))
		return nil

	case tmux.QueuedInputAGM:
		// Stuck AGM message — safe to submit by sending Enter.
		if err := tmux.SendEnterReliable(sessionName); err != nil {
			return fmt.Errorf("failed to send Enter to session '%s': %w", sessionName, err)
		}
		if err := appendUnstickAudit(unstickAuditRecord{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Session:   sessionName,
			Action:    "submit-agm",
			Reason:    unstickReason,
			Caller:    unstickCaller(),
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write unstick audit record: %v\n", err)
		}
		ui.PrintSuccess(fmt.Sprintf("input cleared on %s (submitted stuck AGM message)", sessionName))
		return nil

	case tmux.QueuedInputHuman:
		// Freeform human text — stash it (Ctrl+S) so it is preserved and restored
		// on the worker's next interaction, rather than discarded.
		if err := tmux.SendKeys(sessionName, "C-s"); err != nil {
			return fmt.Errorf("failed to send Ctrl+S to session '%s': %w", sessionName, err)
		}
		if err := appendUnstickAudit(unstickAuditRecord{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Session:   sessionName,
			Action:    "stash-human",
			Reason:    unstickReason,
			Caller:    unstickCaller(),
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write unstick audit record: %v\n", err)
		}
		ui.PrintSuccess(fmt.Sprintf("input cleared on %s (stashed human input; restored on next interaction)", sessionName))
		return nil

	default:
		return fmt.Errorf("unexpected input type in session '%s': %s", sessionName, description)
	}
}

// validateUnstickReason enforces the minimum justification length. cobra's
// MarkFlagRequired only rejects the empty string, so the length floor lives here.
func validateUnstickReason(reason string) error {
	if len(strings.TrimSpace(reason)) < MinUnstickReason {
		return fmt.Errorf("reason too short (min %d chars)", MinUnstickReason)
	}
	return nil
}

// unstickAuditRecord is one line in the unstick audit log.
type unstickAuditRecord struct {
	Timestamp string `json:"timestamp"`
	Session   string `json:"session"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	Caller    string `json:"caller"`
}

// unstickCaller identifies the session that invoked the unstick, falling back to
// the OS user when AGM_SESSION_NAME is unset.
func unstickCaller() string {
	if s := os.Getenv("AGM_SESSION_NAME"); s != "" {
		return s
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "unknown"
}

// unstickAuditPath resolves the audit log path, honoring XDG_STATE_HOME so tests
// (and sandboxes) can isolate it.
func unstickAuditPath() string {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return filepath.Join(base, "dear-agent", "unstick-audit.jsonl")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Last-resort relative path; the append below will surface any error.
		return filepath.Join(".local", "state", "dear-agent", "unstick-audit.jsonl")
	}
	return filepath.Join(home, ".local", "state", "dear-agent", "unstick-audit.jsonl")
}

// appendUnstickAudit appends a record to the unstick audit log as JSONL,
// creating the parent directory if needed.
func appendUnstickAudit(rec unstickAuditRecord) error {
	return appendUnstickAuditTo(unstickAuditPath(), rec)
}

// appendUnstickAuditTo is the testable core: it appends rec to the given path.
func appendUnstickAuditTo(path string, rec unstickAuditRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create audit dir: %w", err)
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to marshal audit record: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open audit log: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("failed to write audit record: %w", err)
	}
	return nil
}
