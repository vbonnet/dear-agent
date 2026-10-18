package override

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"syscall"
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
	e = completeAuditEntry(e)

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

	b, err := marshalAuditEntry(e)
	if err != nil {
		return
	}
	_, _ = f.Write(b)
}

// appendAuditDurable persists one complete audit entry before a safety-critical
// override may proceed. It serializes writers, tightens existing permissions,
// syncs the file, and syncs every directory entry needed to reach it.
func appendAuditDurable(e auditEntry) error {
	e = completeAuditEntry(e)
	b, err := marshalAuditEntry(e)
	if err != nil {
		return fmt.Errorf("marshal override audit entry: %w", err)
	}

	dir := auditDir()
	if err := mkdirAuditDirDurable(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, "override-audit.jsonl")
	f, created, err := openDurableAuditFile(path)
	if err != nil {
		return err
	}
	openedInfo, err := persistAuditEntry(f, b)
	if err != nil {
		return err
	}
	return verifyPersistedAudit(path, dir, created, openedInfo)
}

func persistAuditEntry(f *os.File, b []byte) (os.FileInfo, error) {
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return nil, fmt.Errorf("secure override audit file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return nil, fmt.Errorf("lock override audit file: %w", err)
	}
	locked := true
	defer func() {
		if locked {
			_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		}
	}()
	openedInfo, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened override audit file: %w", err)
	}
	written, err := f.Write(b)
	if err != nil {
		return nil, fmt.Errorf("append override audit entry: %w", err)
	}
	if written != len(b) {
		return nil, fmt.Errorf("append override audit entry: %w", io.ErrShortWrite)
	}
	if err := f.Sync(); err != nil {
		return nil, fmt.Errorf("sync override audit file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		return nil, fmt.Errorf("unlock override audit file: %w", err)
	}
	locked = false
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close override audit file: %w", err)
	}
	closed = true
	return openedInfo, nil
}

func verifyPersistedAudit(path, dir string, created bool, openedInfo os.FileInfo) error {
	currentInfo, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("verify override audit path: %w", err)
	}
	if !currentInfo.Mode().IsRegular() || !os.SameFile(openedInfo, currentInfo) {
		return errors.New("override audit path changed while the entry was persisted")
	}
	// A newly created name needs the directory sync; syncing unconditionally also
	// persists the tightened directory and file mode metadata on existing paths.
	if err := syncAuditDir(dir); err != nil {
		return fmt.Errorf("sync override audit directory (created=%t): %w", created, err)
	}
	return nil
}

func completeAuditEntry(e auditEntry) auditEntry {
	e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	e.PID = os.Getpid()
	e.Caller = filepath.Base(os.Args[0])
	if cwd, err := os.Getwd(); err == nil {
		e.CWD = cwd
	}

	return e
}

func marshalAuditEntry(e auditEntry) ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	// Append the newline to the marshalled bytes and write in a single
	// f.Write call. A lone Write on an O_APPEND file is atomic for payloads
	// under PIPE_BUF (4KB) on POSIX, so concurrent agents cannot interleave
	// partial lines into the JSONL log. fmt.Fprintln would issue two writes.
	b = append(b, '\n')
	return b, nil
}

func mkdirAuditDirDurable(dir string) error {
	clean := filepath.Clean(dir)
	var missing []string
	for current := clean; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("override audit path component %s is not a directory", current)
			}
			break
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("inspect override audit directory %s: %w", current, err)
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("no existing parent for override audit directory %s", clean)
		}
	}
	if err := os.MkdirAll(clean, 0o700); err != nil {
		return fmt.Errorf("create override audit directory: %w", err)
	}
	// #nosec G302 -- directories require execute bits; 0700 is owner-only.
	if err := os.Chmod(clean, 0o700); err != nil {
		return fmt.Errorf("secure override audit directory: %w", err)
	}
	for _, created := range slices.Backward(missing) {
		if err := syncAuditDir(filepath.Dir(created)); err != nil {
			return fmt.Errorf("persist override audit directory %s: %w", created, err)
		}
	}
	if err := syncAuditDir(clean); err != nil {
		return fmt.Errorf("persist override audit directory permissions: %w", err)
	}
	return nil
}

func openDurableAuditFile(path string) (*os.File, bool, error) {
	for range 3 {
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			f, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0o600)
			if errors.Is(openErr, os.ErrExist) {
				continue
			}
			if openErr != nil {
				return nil, false, fmt.Errorf("create override audit file: %w", openErr)
			}
			return f, true, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("inspect override audit file: %w", err)
		}
		if !info.Mode().IsRegular() {
			return nil, false, fmt.Errorf("override audit path %s is not a regular file", path)
		}
		f, openErr := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if openErr != nil {
			return nil, false, fmt.Errorf("open override audit file: %w", openErr)
		}
		openedInfo, statErr := f.Stat()
		if statErr != nil {
			_ = f.Close()
			return nil, false, fmt.Errorf("inspect opened override audit file: %w", statErr)
		}
		if !os.SameFile(info, openedInfo) {
			_ = f.Close()
			continue
		}
		return f, false, nil
	}
	return nil, false, errors.New("override audit path changed repeatedly while opening")
}

func syncAuditDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
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
