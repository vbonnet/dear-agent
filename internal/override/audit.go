package override

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// auditEntry is one JSON line in the override audit log.
type auditEntry struct {
	Timestamp string `json:"timestamp"`
	Tool      string `json:"tool"`
	Flag      string `json:"flag"`
	Gate      string `json:"gate"`
	Risk      string `json:"risk"`
	Reason    string `json:"reason"`
	Judge     string `json:"judge"`
	Allowed   bool   `json:"allowed"`
	Verdict   string `json:"verdict,omitempty"`
	Caller    string `json:"caller"`
	PID       int    `json:"pid"`
	CWD       string `json:"cwd,omitempty"`
}

// appendAudit writes a single JSON line to the override audit log. Failures are
// swallowed: an audit-log problem must never block (or silently allow) the
// gate decision — the decision is already made by the time we record it.
func appendAudit(e auditEntry) {
	e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	e.PID = os.Getpid()
	e.Caller = filepath.Base(os.Args[0])
	if cwd, err := os.Getwd(); err == nil {
		e.CWD = cwd
	}

	dir := auditDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "override-audit.jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	// Append the newline to the marshalled bytes and write in a single
	// f.Write call. A lone Write on an O_APPEND file is atomic for payloads
	// under PIPE_BUF (4KB) on POSIX, so concurrent agents cannot interleave
	// partial lines into the JSONL log. fmt.Fprintln would issue two writes.
	b = append(b, '\n')
	_, _ = f.Write(b)
}

// auditDir returns the directory holding the override audit log. It mirrors
// safe-merge's location (~/.local/state/dear-agent). Overridable via
// OVERRIDE_AUDIT_DIR for tests and sandboxed runs.
func auditDir() string {
	if d := os.Getenv("OVERRIDE_AUDIT_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	return filepath.Join(home, ".local", "state", "dear-agent")
}
