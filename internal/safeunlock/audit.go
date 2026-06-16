package safeunlock

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// AuditEntry is one JSONL record describing a single lock decision. Every Clean
// call appends one line per lock it inspected to the audit log, giving an
// append-only, machine-readable trail of what safe-unlock did and why — the
// audit/OTel-feed counterpart to the human log written to Cleaner.Log.
type AuditEntry struct {
	Timestamp string `json:"timestamp"`        // RFC3339 UTC
	Repo      string `json:"repo"`             // checkout the lock belonged to
	LockPath  string `json:"lock_path"`        // absolute path inspected
	Kind      string `json:"kind"`             // index | ref:… | worktree:… | …
	Outcome   string `json:"outcome"`          // removed | would-remove | active | vanished
	Reason    string `json:"reason,omitempty"` // why, when active or vanished
	AgeSecond int64  `json:"age_seconds"`      // lock age at inspection, whole seconds
}

// appendAudit writes a single JSON line describing one lock decision. Failures
// are logged via slog but never block the unlock: a broken audit sink must not
// wedge an agent that is already stuck on a lock.
func appendAudit(repo string, res Result, dryRun bool) {
	dir := auditLogDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		slog.Warn("safe-unlock: cannot create audit dir", "dir", dir, "error", err)
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "safe-unlock-audit.jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		slog.Warn("safe-unlock: cannot open audit log", "error", err)
		return
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("safe-unlock: failed to close audit log", "error", cerr)
		}
	}()

	entry := AuditEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Repo:      repo,
		LockPath:  res.LockPath,
		Kind:      res.Kind,
		Outcome:   outcome(res, dryRun),
		Reason:    res.Reason,
		AgeSecond: int64(res.Age.Seconds()),
	}
	// Append the newline to the marshalled record and write it in a single
	// Write on the O_APPEND handle: a lone write under PIPE_BUF is atomic on
	// POSIX, so concurrent safe-unlock runs cannot interleave a half-line into
	// the shared audit log (which fmt.Fprintln's two writes could).
	b, _ := json.Marshal(entry)
	b = append(b, '\n')
	if _, werr := f.Write(b); werr != nil {
		slog.Warn("safe-unlock: failed to write audit entry", "error", werr)
	}
}

// outcome maps a Result to a stable audit verb.
func outcome(res Result, dryRun bool) string {
	switch {
	case res.Active:
		return "active"
	case res.Removed:
		return "removed"
	case dryRun:
		return "would-remove"
	default:
		return "vanished"
	}
}

// auditLogDir returns the directory for the safe-unlock audit log. It mirrors
// the other safe-* tools (~/.local/state/dear-agent), and is overridable via
// SAFE_UNLOCK_AUDIT_DIR for tests and sandboxes.
func auditLogDir() string {
	if d := os.Getenv("SAFE_UNLOCK_AUDIT_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "dear-agent")
	}
	return filepath.Join(home, ".local", "state", "dear-agent")
}
