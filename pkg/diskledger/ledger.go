package diskledger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Op names the two halves of a ledger entry.
type Op string

const (
	// OpOpen records that a run took ownership of a path, with its usage at
	// that moment as the baseline.
	OpOpen Op = "disk_run_opened"
	// OpClose records that a run ended, with the usage still present. Whatever
	// remains is charged to the run as leaked.
	OpClose Op = "disk_run_closed"
	// OpLeak records a leak found by reconciliation rather than by a close:
	// the owning session is gone but the bytes are not.
	OpLeak Op = "disk_run_leaked"
)

// Kind classifies what sort of disk a run allocated. Grouping leaks by kind is
// what turns "the host is losing disk" into "sandbox teardown is the leak".
type Kind string

const (
	// KindSandbox is a per-session sandbox directory (APFS clone).
	KindSandbox Kind = "sandbox"
	// KindWorktree is a git worktree provisioned for a run.
	KindWorktree Kind = "worktree"
	// KindCache is a build or lint cache.
	KindCache Kind = "cache"
	// KindScratch is per-run temporary space (TMPDIR, preflight run dirs).
	KindScratch Kind = "scratch"
	// KindLog is an append-only log or transcript.
	KindLog Kind = "log"
)

// Run identifies one unit of disk ownership.
type Run struct {
	// RunID is unique per allocation. Reusing one across paths would make the
	// close ambiguous.
	RunID string `json:"run_id"`
	// Owner is the session/agent the bytes are charged to. Reconciliation asks
	// whether THIS is still alive.
	Owner string `json:"owner"`
	Kind  Kind   `json:"kind"`
	Path  string `json:"path"`
}

// Record is one line of the ledger.
type Record struct {
	Op    Op        `json:"op"`
	At    time.Time `json:"at"`
	RunID string    `json:"run_id"`
	Owner string    `json:"owner,omitempty"`
	Kind  Kind      `json:"kind,omitempty"`
	Path  string    `json:"path,omitempty"`
	Usage Usage     `json:"usage"`
	// LeakedBytes is the allocated bytes still present when the run ended.
	// Zero is the healthy case and is written explicitly: a reader that can
	// only see leaks cannot tell "nothing leaked" from "nothing ran", and that
	// ambiguity is what let a silent GC look healthy for weeks.
	LeakedBytes int64 `json:"leaked_bytes"`
	// BaselineBytes is what the path measured at open, so a reader can tell a
	// run that grew a shared cache from one that never cleaned its own.
	BaselineBytes int64 `json:"baseline_bytes,omitempty"`
}

// Ledger appends run records to a JSONL file.
//
// It deliberately keeps the open baselines in memory as well as on disk: the
// in-memory map serves Close within a process, and the on-disk log is what
// Reconcile reads after a crash, when there is no memory left to consult.
type Ledger struct {
	path string

	mu   sync.Mutex
	open map[string]Record
}

// DefaultPath returns ~/.agm/logs/diskledger.jsonl.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agm", "logs", "diskledger.jsonl"), nil
}

// New creates a Ledger writing to path, creating the parent directory.
func New(path string) (*Ledger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create ledger dir: %w", err)
	}
	return &Ledger{path: path, open: make(map[string]Record)}, nil
}

// NewDefault creates a Ledger at DefaultPath.
func NewDefault() (*Ledger, error) {
	p, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return New(p)
}

// Path returns the ledger file path.
func (l *Ledger) Path() string { return l.path }

// Open measures run.Path and records it as the run's baseline.
func (l *Ledger) Open(run Run) error {
	if run.RunID == "" {
		return fmt.Errorf("diskledger: RunID is required")
	}
	u, err := Measure(run.Path)
	if err != nil {
		// Record the attempt even when measurement degraded, so a run is never
		// invisible to reconciliation just because its baseline was unreadable.
		u.Partial = true
	}
	rec := Record{
		Op: OpOpen, At: time.Now(), RunID: run.RunID, Owner: run.Owner,
		Kind: run.Kind, Path: run.Path, Usage: u,
		BaselineBytes: u.AllocatedBytes,
	}
	l.mu.Lock()
	l.open[run.RunID] = rec
	l.mu.Unlock()
	return l.append(rec)
}

// Close re-measures the run's path and charges whatever remains as leaked.
func (l *Ledger) Close(runID string) error {
	l.mu.Lock()
	base, ok := l.open[runID]
	if ok {
		delete(l.open, runID)
	}
	l.mu.Unlock()
	if !ok {
		return fmt.Errorf("diskledger: close of unknown run %q", runID)
	}

	u, err := Measure(base.Path)
	if err != nil {
		u.Partial = true
	}
	return l.append(Record{
		Op: OpClose, At: time.Now(), RunID: base.RunID, Owner: base.Owner,
		Kind: base.Kind, Path: base.Path, Usage: u,
		LeakedBytes:   u.AllocatedBytes,
		BaselineBytes: base.BaselineBytes,
	})
}

// LogLeak appends a reconciliation-discovered leak.
func (l *Ledger) LogLeak(rec Record) error {
	rec.Op = OpLeak
	if rec.At.IsZero() {
		rec.At = time.Now()
	}
	return l.append(rec)
}

func (l *Ledger) append(rec Record) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal ledger record: %w", err)
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open ledger: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write ledger: %w", err)
	}
	return nil
}

// ReadRecords parses a ledger file. A malformed line is skipped rather than
// failing the whole read, so one bad append cannot blind the reconciler.
func ReadRecords(path string) ([]Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	// A torn append (a crash mid-write) truncates the last record. Stop there
	// and keep everything that parsed: refusing the whole file would blind the
	// reconciler to every run recorded before the crash, which is exactly the
	// history it needs after a crash.
	var out []Record
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var r Record
		if derr := dec.Decode(&r); derr != nil {
			return out, nil //nolint:nilerr // partial parse is the intended outcome
		}
		out = append(out, r)
	}
	return out, nil
}

// OpenEntries returns the open records that were never closed, newest wins.
func OpenEntries(recs []Record) []Record {
	state := make(map[string]Record)
	for _, r := range recs {
		switch r.Op {
		case OpOpen:
			state[r.RunID] = r
		case OpClose, OpLeak:
			delete(state, r.RunID)
		}
	}
	out := make([]Record, 0, len(state))
	for _, r := range state {
		out = append(out, r)
	}
	return out
}

// AliveFunc reports whether an owner is still live.
type AliveFunc func(owner string) bool

// Reconcile finds entries whose owner is gone but whose bytes are not.
//
// grace protects runs that are merely slow: an entry younger than grace is
// never charged, because a reaper that cannot tell "still working" from
// "leaked" is the reaper that deletes live work.
func Reconcile(open []Record, alive AliveFunc, grace time.Duration) []Record {
	now := time.Now()
	var leaks []Record
	for _, r := range open {
		if now.Sub(r.At) < grace {
			continue
		}
		if alive != nil && alive(r.Owner) {
			continue
		}
		u, err := Measure(r.Path)
		if err != nil {
			u.Partial = true
		}
		if u.Missing || u.AllocatedBytes == 0 {
			// Already gone: the run leaked nothing even though it never closed.
			continue
		}
		leaks = append(leaks, Record{
			Op: OpLeak, At: now, RunID: r.RunID, Owner: r.Owner, Kind: r.Kind,
			Path: r.Path, Usage: u, LeakedBytes: u.AllocatedBytes,
			BaselineBytes: r.BaselineBytes,
		})
	}
	return leaks
}
