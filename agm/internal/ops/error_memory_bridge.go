package ops

import (
	"log/slog"
	"time"

	"github.com/vbonnet/dear-agent/engram/errormemory"
)

// Source constants for error memory records originating from AGM ops.
const (
	SourceAGMStall      = "agm-stall-detector"
	SourceAGMQualityGate = "agm-quality-gate"
	SourceAGMTrust      = "agm-trust"
	SourceAGMArchive    = "agm-archive"
	SourceAGMCrossCheck = "agm-cross-check"
)

// ErrorMemoryCategory constants for AGM-originated error patterns.
const (
	ErrMemCatPermissionPrompt = "permission-prompt"
	ErrMemCatStall            = "stall"
	ErrMemCatErrorLoop        = "error-loop"
	ErrMemCatQualityGate      = "quality-gate"
	ErrMemCatFalseCompletion  = "false-completion"
	ErrMemCatSessionDown      = "session-down"
	ErrMemCatEnterBug         = "enter-bug"
	ErrMemCatBuildFailure     = "build-failure"
)

// recordErrorMemory upserts an error pattern to the persistent error memory
// store. All errors are logged and swallowed — error memory recording must
// never block the calling operation.
func recordErrorMemory(pattern, category, command, remediation, source, sessionName string) {
	now := time.Now()
	store := errormemory.NewStore(errormemory.DefaultDBPath)
	_, err := store.Upsert(errormemory.ErrorRecord{
		Pattern:       pattern,
		ErrorCategory: category,
		CommandSample: command,
		Remediation:   remediation,
		Count:         1,
		FirstSeen:     now,
		LastSeen:      now,
		SessionIDs:    []string{sessionName},
		Source:        source,
	})
	if err != nil {
		slog.Warn("Failed to record error memory",
			"pattern", pattern,
			"category", category,
			"session", sessionName,
			"error", err,
		)
	}
}
