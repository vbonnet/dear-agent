package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/compaction"
	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// CompactTriggerRequest defines the input for auto-compaction checks.
type CompactTriggerRequest struct {
	// DryRun previews which sessions would be compacted without sending.
	DryRun bool `json:"dry_run,omitempty"`
}

// CompactTriggerResult is the output of CheckAndTriggerCompaction.
type CompactTriggerResult struct {
	Operation string                  `json:"operation"`
	Checked   int                     `json:"checked"`
	Triggered []CompactTriggerAction  `json:"triggered,omitempty"`
	Warnings  []CompactTriggerWarning `json:"warnings,omitempty"`
	Skipped   []CompactTriggerSkip    `json:"skipped,omitempty"`
	Errors    []string                `json:"errors,omitempty"`
}

// CompactTriggerAction records a compaction that was triggered (or would be in dry-run).
type CompactTriggerAction struct {
	SessionName    string  `json:"session_name"`
	SessionID      string  `json:"session_id"`
	ContextPercent float64 `json:"context_percent"`
	Level          string  `json:"level"` // "compact" or "critical"
	DryRun         bool    `json:"dry_run,omitempty"`
	Delivered      bool    `json:"delivered,omitempty"`
}

// CompactTriggerWarning records a session approaching context limits.
type CompactTriggerWarning struct {
	SessionName    string  `json:"session_name"`
	SessionID      string  `json:"session_id"`
	ContextPercent float64 `json:"context_percent"`
	Level          string  `json:"level"` // "warn" or "critical"
}

// CompactTriggerSkip records a session that was skipped due to anti-loop or other reasons.
type CompactTriggerSkip struct {
	SessionName string `json:"session_name"`
	SessionID   string `json:"session_id"`
	Reason      string `json:"reason"`
}

// agmBaseDir returns ~/.agm, or AGM_HOME if set.
func agmBaseDir() string {
	if d := os.Getenv("AGM_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agm")
}

// CheckAndTriggerCompaction scans all active sessions for context window usage
// and triggers /compact on sessions exceeding the compact threshold.
func CheckAndTriggerCompaction(ctx *OpContext, req *CompactTriggerRequest) (*CompactTriggerResult, error) {
	if req == nil {
		req = &CompactTriggerRequest{}
	}

	slo := contracts.Load()
	thresholdWarn := slo.Compaction.ContextThresholdWarn
	thresholdCompact := slo.Compaction.ContextThresholdCompact
	thresholdCritical := slo.Compaction.ContextThresholdCritical

	result := &CompactTriggerResult{
		Operation: "compact_trigger",
	}

	// List all active sessions
	manifests, err := ctx.Storage.ListSessions(nil)
	if err != nil {
		return nil, ErrStorageError("list_sessions", err)
	}

	// Filter to non-archived sessions
	var active []*manifest.Manifest
	for _, m := range manifests {
		if m.Lifecycle != manifest.LifecycleArchived {
			active = append(active, m)
		}
	}
	result.Checked = len(active)

	baseDir := agmBaseDir()

	for _, m := range active {
		usage, err := session.DetectContextFromManifestOrLog(m)
		if err != nil {
			// Can't detect context — skip silently (many sessions may not have data)
			continue
		}

		pct := usage.PercentageUsed

		switch {
		case pct >= thresholdCompact:
			level := "compact"
			if pct >= thresholdCritical {
				level = "critical"
			}

			// Check anti-loop safety
			compState, loadErr := compaction.LoadState(baseDir, m.Name)
			if loadErr != nil {
				result.Errors = append(result.Errors,
					fmt.Sprintf("session %q: failed to load compaction state: %v", m.Name, loadErr))
				continue
			}

			if antiErr := compaction.CheckAntiLoop(compState, false); antiErr != nil {
				result.Skipped = append(result.Skipped, CompactTriggerSkip{
					SessionName: m.Name,
					SessionID:   m.SessionID,
					Reason:      antiErr.Error(),
				})
				continue
			}

			action := CompactTriggerAction{
				SessionName:    m.Name,
				SessionID:      m.SessionID,
				ContextPercent: pct,
				Level:          level,
				DryRun:         req.DryRun,
			}

			if !req.DryRun {
				delivered := triggerCompaction(ctx, m, baseDir, compState)
				action.Delivered = delivered
			}

			result.Triggered = append(result.Triggered, action)

		case pct >= thresholdWarn:
			result.Warnings = append(result.Warnings, CompactTriggerWarning{
				SessionName:    m.Name,
				SessionID:      m.SessionID,
				ContextPercent: pct,
				Level:          "warn",
			})
		}
	}

	return result, nil
}

// triggerCompaction sends /compact to a session and records the event.
// Returns true if the message was delivered successfully.
func triggerCompaction(ctx *OpContext, m *manifest.Manifest, baseDir string, compState *compaction.CompactionState) bool {
	sendResult, err := SendMessage(ctx, &SendMessageRequest{
		Recipient: m.Name,
		Message:   "/compact",
	})
	if err != nil {
		return false
	}

	if sendResult.Delivered {
		// Record compaction event for anti-loop tracking
		compaction.RecordCompaction(compState, "auto-compact-trigger", false)
		_ = compaction.SaveState(baseDir, compState)

		// Write audit log entry
		writeCompactAuditLog(baseDir, m.Name, m.SessionID)
	}

	return sendResult.Delivered
}

// writeCompactAuditLog appends an entry to the auto-compact audit log.
func writeCompactAuditLog(baseDir, sessionName, sessionID string) {
	logDir := filepath.Join(baseDir, "auto-compact-logs")
	_ = os.MkdirAll(logDir, 0o755)

	logFile := filepath.Join(logDir, "triggers.log")
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	entry := fmt.Sprintf("%s session=%s id=%s action=compact_triggered\n",
		time.Now().Format(time.RFC3339), sessionName, sessionID)
	_, _ = f.WriteString(entry)
}
