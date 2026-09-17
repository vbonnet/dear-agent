package ops

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/lock"
)

// This file is the durable half of alert routing: the inter-process file
// lock, the append-only queue file itself (with rotation), and the bounded
// tail reads every routing decision and every `agm alerts` surface uses. It
// is split out of alert_router.go, which holds the routing decision logic,
// so that file stays under the repo's structural-health line budget as
// alert routing has grown more nuanced (DONE-vs-zombie liveness, canonical
// target resolution, list-error propagation).

// lockQueue takes the inter-process queue lock and returns its release.
//
// Acquisition is bounded: if a peer holds the lock past the timeout the
// caller proceeds anyway, accepting that dedupe may double-send, because
// blocking indefinitely on a wedged peer would stall the alert entirely.
func (r *AlertRouter) lockQueue() func() {
	if r.queuePath == "" {
		return func() {}
	}
	fl, err := lock.New(r.queuePath + ".lock")
	if err != nil {
		return func() {}
	}
	timeout := r.lockTimeout
	if timeout <= 0 {
		timeout = alertLockTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := fl.TryLock(); err == nil {
			return func() { _ = fl.Unlock() }
		}
		if time.Now().After(deadline) {
			_ = fl.Unlock()
			return func() {}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// appendLocked writes rec to the queue. Callers must already hold the queue
// lock; the name says so because the rotation below is only safe when no
// peer is mid-append.
func (r *AlertRouter) appendLocked(rec AlertRecord) error {
	return appendAlertRecord(r.queuePath, rec)
}

func appendAlertRecord(path string, rec AlertRecord) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create alert queue dir: %w", err)
	}
	if err := rotateAlertQueue(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open alert queue: %w", err)
	}
	if err := json.NewEncoder(f).Encode(rec); err != nil {
		_ = f.Close()
		return fmt.Errorf("write alert queue: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close alert queue: %w", err)
	}
	return nil
}

// rotateAlertQueue moves the queue aside once it exceeds the size cap, so a
// watcher that has run for months does not accumulate an unbounded history.
// One generation is kept; older history is not load-bearing because dedupe
// only looks back over its window.
func rotateAlertQueue(path string) error {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= alertQueueMaxBytes {
		return nil //nolint:nilerr // a missing queue is normal; it is about to be created
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return fmt.Errorf("rotate alert queue: %w", err)
	}
	return nil
}

// ReadAlertRecords reads up to limit most-recent alert records from path.
//
// It reads a bounded tail rather than the whole file: this runs on every
// routed alert, and the queue accumulates every dispatched, quiet, and
// queued record, so a full scan would make alert handling slower the longer
// the host has been up.
func ReadAlertRecords(path string, limit int) ([]AlertRecord, error) {
	return readAlertRecords(path, limit, nil)
}

// ReadAlertRecordsWithStatus reads up to limit most-recent records whose
// status is one of statuses.
//
// Filtering happens before the limit is applied. Reading the newest limit
// records and then filtering would answer "no queued alerts" whenever the
// tail happened to be full of dispatched completions, even though an
// undelivered critical alert was still sitting in the file.
func ReadAlertRecordsWithStatus(path string, limit int, statuses ...AlertStatus) ([]AlertRecord, error) {
	if len(statuses) == 0 {
		return readAlertRecords(path, limit, nil)
	}
	return readAlertRecords(path, limit, func(rec AlertRecord) bool {
		return slices.Contains(statuses, rec.Status)
	})
}

func readAlertRecords(path string, limit int, keep func(AlertRecord) bool) ([]AlertRecord, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open alert queue: %w", err)
	}
	defer func() { _ = f.Close() }()

	reader, err := boundedTail(f)
	if err != nil {
		return nil, err
	}

	var records []AlertRecord
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var rec AlertRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if keep != nil && !keep(rec) {
			continue
		}
		if limit > 0 && len(records) >= limit {
			// Shift in place so the backing array stays bounded rather than
			// growing to hold every line in the file.
			copy(records, records[1:])
			records[limit-1] = rec
		} else {
			records = append(records, rec)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read alert queue: %w", err)
	}
	return records, nil
}

// boundedTail returns a reader over at most alertQueueTailBytes of f,
// starting at a record boundary.
func boundedTail(f *os.File) (io.Reader, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat alert queue: %w", err)
	}
	if info.Size() <= alertQueueTailBytes {
		return f, nil
	}
	if _, err := f.Seek(info.Size()-alertQueueTailBytes, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek alert queue: %w", err)
	}
	// The seek almost certainly landed mid-line; drop that partial record so
	// the scanner starts on a whole one.
	buffered := bufio.NewReader(f)
	if _, err := buffered.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("align alert queue tail: %w", err)
	}
	return buffered, nil
}

// jsonMarshalAlert marshals one record; used by tests that build queue
// fixtures directly.
func jsonMarshalAlert(rec AlertRecord) ([]byte, error) {
	return json.Marshal(rec)
}
