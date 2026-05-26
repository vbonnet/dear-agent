// Package decisiontrail provides append-only persistence for consequential
// VROOM decisions. It is the on-disk substrate that CONTEXT.md §"Decision
// trail" promises and that pkg/vroom/vroom (the in-memory event emitter) did
// not previously provide.
//
// The contract is intentionally narrow:
//
//   - Append-only. Records are never rewritten or deleted in-place. Rotating
//     to a new file is allowed; mutating a written record is not.
//   - One record per line (JSONL / NDJSON). Each line is a self-contained
//     JSON object so a partial write at process death loses at most the
//     trailing partial line, never the preceding records.
//   - Concurrent safe. Multiple goroutines may Append; writes are serialised
//     internally.
//
// This package deliberately does NOT define what counts as a "consequential"
// decision. That is a policy decision for the supervisor or its caller. The
// trail will record whatever it is given.
package decisiontrail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Record is one entry in the decision trail. Fields are deliberately small
// and stable; richer payloads belong inside Payload (free-form JSON).
type Record struct {
	// EventID uniquely identifies this record. Populated by Append if empty.
	EventID string `json:"event_id"`

	// Timestamp is RFC3339 nanos in UTC. Populated by Append if zero.
	Timestamp time.Time `json:"timestamp"`

	// Role is the supervisor (or worker) role that made the decision.
	// Free-form string; values used by the supervisor package are the
	// supervisor.Role constants ("meta-orchestrator" / "orchestrator" /
	// "overseer").
	Role string `json:"role"`

	// Kind is a short topic-style identifier ("supervisor.tick.start",
	// "supervisor.check.peer", "supervisor.decision.gated", etc.). The
	// trail does not validate the kind; recording it is the writer's
	// responsibility.
	Kind string `json:"kind"`

	// Payload is free-form JSON-serialisable data. May be nil.
	Payload map[string]any `json:"payload,omitempty"`
}

// Trail is the append-only sink that the supervisor mesh writes to.
type Trail interface {
	// Append writes one record. The record's EventID and Timestamp are
	// populated if empty/zero. The call returns only after the record is
	// flushed to the underlying io.Writer.
	Append(ctx context.Context, rec Record) error

	// Close releases any underlying resources. Subsequent Append calls
	// return an error.
	Close() error
}

// JSONLTrail is a Trail backed by a single append-only file (or any
// io.WriteCloser). One line per record.
type JSONLTrail struct {
	mu     sync.Mutex
	w      io.WriteCloser
	closed bool
}

// OpenJSONL opens (or creates) path for append-only writes and returns a
// JSONLTrail. Parent directories are created with 0o755 if missing.
//
// The file is opened O_APPEND so multiple processes writing the same trail
// produce interleaved-but-line-atomic output on POSIX (lines ≤ PIPE_BUF;
// each Append call is one Write of one line). This is sufficient for the
// "multiple supervisor processes share a trail" deployment shape.
func OpenJSONL(path string) (*JSONLTrail, error) {
	if path == "" {
		return nil, fmt.Errorf("decisiontrail: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("decisiontrail: mkdir %q: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("decisiontrail: open %q: %w", path, err)
	}
	return NewJSONLTrail(f), nil
}

// NewJSONLTrail wraps an existing io.WriteCloser. Useful for tests
// (pass a *bytes.Buffer wrapped in a nopCloser, or use NewBufferTrail).
func NewJSONLTrail(w io.WriteCloser) *JSONLTrail {
	return &JSONLTrail{w: w}
}

// Append marshals rec to JSON and writes one line. The lock guarantees one
// goroutine's record is fully written before the next one begins, so
// readers never see interleaved JSON.
func (t *JSONLTrail) Append(ctx context.Context, rec Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if rec.EventID == "" {
		rec.EventID = uuid.New().String()
	}
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	} else {
		// Always store UTC so readers don't have to sort by inconsistent zones.
		rec.Timestamp = rec.Timestamp.UTC()
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("decisiontrail: marshal: %w", err)
	}
	line = append(line, '\n')

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return fmt.Errorf("decisiontrail: trail is closed")
	}
	if _, err := t.w.Write(line); err != nil {
		return fmt.Errorf("decisiontrail: write: %w", err)
	}
	return nil
}

// Close closes the underlying writer. Subsequent Append calls return an error.
func (t *JSONLTrail) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	return t.w.Close()
}
