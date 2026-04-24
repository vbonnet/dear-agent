package main

import (
	"os"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// auditHandled is set to true when a command logs its own enriched audit event,
// preventing PersistentPostRunE from double-logging.
var (
	auditHandled   bool
	auditHandledMu sync.Mutex
)

// logCommandAudit logs an enriched audit event for a specific command.
// It should be called at the end of each command's RunE with the command-specific
// metadata. Both success and failure are logged (cobra's PersistentPostRunE only
// fires on success, so this ensures failed commands are also audited).
func logCommandAudit(command, session string, args map[string]string, cmdErr error) {
	if auditLogger == nil {
		return
	}

	result := "success"
	errMsg := ""
	if cmdErr != nil {
		result = "error"
		errMsg = cmdErr.Error()
	}

	event := ops.AuditEvent{
		Timestamp:  time.Now(),
		Command:    command,
		Session:    session,
		User:       os.Getenv("AGM_SESSION_NAME"),
		Args:       args,
		Result:     result,
		DurationMs: time.Since(commandStartTime).Milliseconds(),
		Error:      errMsg,
	}

	// Best-effort: don't fail the command on audit errors
	_ = auditLogger.Log(event)

	auditHandledMu.Lock()
	auditHandled = true
	auditHandledMu.Unlock()
}
