package bus

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Queue persists frames bound for offline sessions and replays them on
// reconnect. Each session has its own file at {dir}/{sessionID}.jsonl; frames
// are appended as single-line JSON. Replay is atomic: Drain reads the file,
// truncates it to zero length, and returns the parsed frames. A crash between
// read and truncate means a session sees the same frame twice on its next
// reconnect — acceptable tradeoff, since duplicate deliveries are a lesser
// problem than lost messages for this system (the receiver can dedup by
// frame ID).
//
// The per-session file is protected by a sync.Mutex to serialize Append and
// Drain from multiple server goroutines; file-level serialization is also
// needed so offline-queue replay is ordered.
type Queue struct {
	Dir string

	mu    sync.Mutex
	locks map[string]*sync.Mutex // per-session lock for fine-grained concurrency
}

// NewQueue returns a Queue rooted at dir. The directory is created with
// 0o755 if it doesn't exist; returns an error only if creation fails.
func NewQueue(dir string) (*Queue, error) {
	if dir == "" {
		return nil, errors.New("queue: empty dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("queue: mkdir %s: %w", dir, err)
	}
	return &Queue{
		Dir:   dir,
		locks: make(map[string]*sync.Mutex),
	}, nil
}

// lockFor returns the per-session mutex, creating it on first use.
// The outer Queue.mu protects the locks map itself.
func (q *Queue) lockFor(sessionID string) *sync.Mutex {
	q.mu.Lock()
	defer q.mu.Unlock()
	if m, ok := q.locks[sessionID]; ok {
		return m
	}
	m := &sync.Mutex{}
	q.locks[sessionID] = m
	return m
}

// pathFor returns the on-disk file for sessionID. The id is sanitized so
// that a malicious session id can't escape the queue dir via ../ or /.
// Session ids in practice are short alphanumeric names, but defence in
// depth is cheap.
func (q *Queue) pathFor(sessionID string) (string, error) {
	clean := sanitizeSessionID(sessionID)
	if clean == "" {
		return "", fmt.Errorf("queue: invalid session id %q", sessionID)
	}
	return filepath.Join(q.Dir, clean+".jsonl"), nil
}

// sanitizeSessionID returns a session id stripped of characters that could
// produce a path-traversal filename. Returns empty string if the result
// would be empty or unsafe.
func sanitizeSessionID(id string) string {
	if id == "" {
		return ""
	}
	// Reject any id containing path separators or parent refs outright.
	if strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return ""
	}
	// Keep letters, digits, dash, underscore, dot.
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			return ""
		}
	}
	return b.String()
}

// Append writes a frame to sessionID's queue file. Frames are fsynced after
// each append — durability under crash is worth the ~ms per message for a
// low-volume supervisor bus.
func (q *Queue) Append(sessionID string, f *Frame) error {
	if f == nil {
		return errors.New("queue: nil frame")
	}
	path, err := q.pathFor(sessionID)
	if err != nil {
		return err
	}
	m := q.lockFor(sessionID)
	m.Lock()
	defer m.Unlock()

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("queue: open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	b, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("queue: encode frame: %w", err)
	}
	if _, err := file.Write(b); err != nil {
		return fmt.Errorf("queue: write: %w", err)
	}
	if _, err := file.Write([]byte{'\n'}); err != nil {
		return fmt.Errorf("queue: write terminator: %w", err)
	}
	return file.Sync()
}

// Drain reads and returns all queued frames for sessionID, then truncates
// the file to zero length. Returns (nil, nil) if no file exists (no queued
// messages). Malformed lines are skipped and logged via the returned error
// aggregation — callers who want strict semantics should fail on non-nil
// error, but lenient callers can replay what parsed successfully.
func (q *Queue) Drain(sessionID string) ([]*Frame, error) {
	path, err := q.pathFor(sessionID)
	if err != nil {
		return nil, err
	}
	m := q.lockFor(sessionID)
	m.Lock()
	defer m.Unlock()

	file, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("queue: open %s: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	frames, parseErr := readAllFrames(file)

	// Truncate regardless of parse errors — we've returned what we could
	// parse; leaving the file in place risks replaying bad data forever.
	if err := file.Truncate(0); err != nil {
		return frames, fmt.Errorf("queue: truncate: %w", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		return frames, fmt.Errorf("queue: seek: %w", err)
	}
	return frames, parseErr
}

// readAllFrames parses every JSON line in r and returns the frames.
// Parse errors are aggregated into a single error; successfully-parsed
// frames are still returned so the caller can decide whether to drop or
// replay them.
func readAllFrames(r io.Reader) ([]*Frame, error) {
	br := bufio.NewReader(r)
	var out []*Frame
	var errs []string
	line := 0
	for {
		line++
		b, err := br.ReadBytes('\n')
		if len(b) > 0 {
			trimmed := strings.TrimSpace(string(b))
			if trimmed != "" {
				var f Frame
				if jerr := json.Unmarshal([]byte(trimmed), &f); jerr != nil {
					errs = append(errs, fmt.Sprintf("line %d: %v", line, jerr))
				} else {
					out = append(out, &f)
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			errs = append(errs, fmt.Sprintf("line %d: read: %v", line, err))
			break
		}
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("queue: parse errors: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

// Len returns the number of queued frames for sessionID without consuming
// them. Useful for tests and diagnostics. Returns 0 if no file exists.
func (q *Queue) Len(sessionID string) (int, error) {
	path, err := q.pathFor(sessionID)
	if err != nil {
		return 0, err
	}
	m := q.lockFor(sessionID)
	m.Lock()
	defer m.Unlock()

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	defer func() { _ = file.Close() }()

	count := 0
	br := bufio.NewReader(file)
	for {
		b, err := br.ReadBytes('\n')
		if len(b) > 0 && strings.TrimSpace(string(b)) != "" {
			count++
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return count, err
		}
	}
	return count, nil
}
