package escalation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	// ErrNotFound is returned when no escalation has the given ID.
	ErrNotFound = errors.New("escalation: not found")
	// ErrAlreadyExists is returned when Create would replace an escalation.
	ErrAlreadyExists = errors.New("escalation: already exists")
)

const (
	storeLockFilename     = ".lock"
	defaultStoreLockWait  = 10 * time.Second
	storeLockPollInterval = 25 * time.Millisecond
)

// Mutation changes one private copy of the latest committed escalation.
//
// A Mutation is called at most once while the Store holds its single-record
// transition lock. It must only mutate state: it must not perform external
// effects, spawn work, or call back into the Store. Returning an error aborts
// the update and leaves the committed record unchanged.
type Mutation func(current *Escalation) error

// Filter narrows List results. Zero value matches everything.
type Filter struct {
	// Pending, if true, returns only escalations that are not terminal.
	Pending bool
	// CurrentSessionID, if set, returns only escalations whose current holder is
	// that session (a supervisor's "my inbox" query).
	CurrentSessionID string
}

// Store holds the mutable current state of escalations. It is the shared point
// of truth between the asking worker and the answering supervisor, which run in
// separate processes — so the production implementation ([FileStore]) persists
// to disk. The append-only event log ([Sink]) is separate: the Store is "what
// is true now", the log is "everything that happened".
type Store interface {
	// Create inserts a new escalation without replacing an existing record.
	Create(ctx context.Context, e *Escalation) error
	// Update atomically reads, mutates, and publishes one existing escalation.
	// The returned value is a private copy of the committed record.
	Update(ctx context.Context, id string, mutate Mutation) (*Escalation, error)
	Get(ctx context.Context, id string) (*Escalation, error)
	List(ctx context.Context, f Filter) ([]*Escalation, error)
}

// MemStore is an in-memory Store for tests and single-process use.
type MemStore struct {
	mu sync.RWMutex
	m  map[string]*Escalation
}

// NewMemStore builds an empty MemStore.
func NewMemStore() *MemStore { return &MemStore{m: make(map[string]*Escalation)} }

// Create implements Store. It stores a deep copy so the caller cannot mutate
// committed state through an alias.
func (s *MemStore) Create(ctx context.Context, e *Escalation) error {
	if e == nil {
		return fmt.Errorf("escalation: Create requires a non-nil escalation")
	}
	if err := validateEscalationID(e.ID); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}

	created := cloneEscalation(e)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.m[e.ID]; exists {
		return fmt.Errorf("escalation %q: %w", e.ID, ErrAlreadyExists)
	}
	s.m[e.ID] = created
	return nil
}

// Update implements Store. The mutex covers the complete read, mutation, and
// replacement so competing goroutines always mutate the latest record.
func (s *MemStore) Update(ctx context.Context, id string, mutate Mutation) (*Escalation, error) {
	if err := validateEscalationID(id); err != nil {
		return nil, err
	}
	if mutate == nil {
		return nil, fmt.Errorf("escalation: Update requires a mutation")
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	committed, exists := s.m[id]
	if !exists {
		return nil, ErrNotFound
	}
	working := cloneEscalation(committed)
	if err := mutate(working); err != nil {
		return nil, err
	}
	if working.ID != id {
		return nil, fmt.Errorf("escalation: Update cannot change id %q to %q", id, working.ID)
	}

	// Clone twice: the callback may have retained working, and the returned value
	// must not alias the copy retained by the Store.
	s.m[id] = cloneEscalation(working)
	return cloneEscalation(working), nil
}

// Get implements Store.
func (s *MemStore) Get(ctx context.Context, id string) (*Escalation, error) {
	if err := validateEscalationID(id); err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.m[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneEscalation(e), nil
}

// List implements Store.
func (s *MemStore) List(ctx context.Context, f Filter) ([]*Escalation, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Escalation, 0, len(s.m))
	for _, e := range s.m {
		if matches(e, f) {
			out = append(out, cloneEscalation(e))
		}
	}
	sortEscalations(out)
	return out, nil
}

// FileStore persists each escalation as one JSON file (<id>.json) under a
// directory. Readers observe a complete old or new record. Create and Update
// hold a stable store-wide OS lock across their complete read/mutate/sync/rename
// transaction, including across separate processes. Platforms whose rename is
// not guaranteed atomic also take that lock for Get.
type FileStore struct {
	dir string
}

// NewFileStore creates (if needed) dir and returns a FileStore rooted there.
func NewFileStore(dir string) (*FileStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("escalation: empty store dir")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("escalation: resolve store dir %q: %w", dir, err)
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return nil, fmt.Errorf("escalation: mkdir %q: %w", absDir, err)
	}
	return &FileStore{dir: absDir}, nil
}

func (s *FileStore) path(id string) string { return filepath.Join(s.dir, id+".json") }

func (s *FileStore) lockPath() string { return filepath.Join(s.dir, storeLockFilename) }

// Create implements Store.
func (s *FileStore) Create(ctx context.Context, e *Escalation) error {
	if e == nil {
		return fmt.Errorf("escalation: Create requires a non-nil escalation")
	}
	if err := validateEscalationID(e.ID); err != nil {
		return err
	}
	created := cloneEscalation(e)
	lock, err := acquireStoreFileLock(ctx, s.lockPath())
	if err != nil {
		return err
	}
	// Closing the descriptor releases the advisory lock. State publication has
	// already succeeded or failed before this runs, so a release error must not
	// misreport a committed transition as safe to retry.
	defer func() { _ = lock.Close() }()

	if _, err := os.Lstat(s.path(e.ID)); err == nil {
		return fmt.Errorf("escalation %q: %w", e.ID, ErrAlreadyExists)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("escalation: inspect %q: %w", e.ID, err)
	}
	return s.writeRecord(created)
}

// Update implements Store. Its stable sidecar lock spans the complete
// read/mutate/sync/rename transaction; mutate is never retried.
func (s *FileStore) Update(ctx context.Context, id string, mutate Mutation) (*Escalation, error) {
	if err := validateEscalationID(id); err != nil {
		return nil, err
	}
	if mutate == nil {
		return nil, fmt.Errorf("escalation: Update requires a mutation")
	}
	lock, err := acquireStoreFileLock(ctx, s.lockPath())
	if err != nil {
		return nil, err
	}
	defer func() { _ = lock.Close() }()

	committed, err := s.readRecord(id)
	if err != nil {
		return nil, err
	}
	working := cloneEscalation(committed)
	if err := mutate(working); err != nil {
		return nil, err
	}
	if working.ID != id {
		return nil, fmt.Errorf("escalation: Update cannot change id %q to %q", id, working.ID)
	}
	// Detach committed state from any pointer retained by the callback before
	// serializing it and before returning another private copy.
	updated := cloneEscalation(working)
	if err := s.writeRecord(updated); err != nil {
		return nil, err
	}
	return cloneEscalation(updated), nil
}

// Get implements Store. Unix reads are lock-free because same-directory rename
// is atomic. Platforms without that guarantee share the transition lock so a
// reader cannot observe the replacement in flight.
func (s *FileStore) Get(ctx context.Context, id string) (*Escalation, error) {
	if err := validateEscalationID(id); err != nil {
		return nil, err
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if storeReadsRequireLock {
		lock, err := acquireStoreFileLock(ctx, s.lockPath())
		if err != nil {
			return nil, err
		}
		defer func() { _ = lock.Close() }()
	}
	return s.readRecord(id)
}

// List implements Store. It is intentionally not a multi-record snapshot.
func (s *FileStore) List(ctx context.Context, f Filter) ([]*Escalation, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	if storeReadsRequireLock {
		lock, err := acquireStoreFileLock(ctx, s.lockPath())
		if err != nil {
			return nil, err
		}
		defer func() { _ = lock.Close() }()
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("escalation: readdir: %w", err)
	}
	out := make([]*Escalation, 0, len(entries))
	for _, de := range entries {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		name := de.Name()
		if de.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		id := name[:len(name)-len(".json")]
		if err := validateEscalationID(id); err != nil {
			continue
		}
		e, err := s.readRecord(id)
		if err != nil {
			if ctxErr := contextError(ctx); ctxErr != nil {
				return nil, ctxErr
			}
			continue // skip unreadable/partial entries rather than fail the whole list
		}
		if matches(e, f) {
			out = append(out, e)
		}
	}
	sortEscalations(out)
	return out, nil
}

func (s *FileStore) readRecord(id string) (*Escalation, error) {
	// #nosec G703 -- every caller validates id as one portable filename before
	// reaching this internal helper; s.dir is the FileStore's fixed root.
	b, err := os.ReadFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("escalation: read %q: %w", id, err)
	}
	var e Escalation
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, fmt.Errorf("escalation: unmarshal %q: %w", id, err)
	}
	if e.ID != id {
		return nil, fmt.Errorf("escalation: record %q contains mismatched id %q", id, e.ID)
	}
	return &e, nil
}

func (s *FileStore) writeRecord(e *Escalation) error {
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("escalation: marshal: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, e.ID+".*.tmp")
	if err != nil {
		return fmt.Errorf("escalation: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(b); err != nil {
		return fmt.Errorf("escalation: write temp: %w", errors.Join(err, tmp.Close()))
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("escalation: sync temp: %w", errors.Join(err, tmp.Close()))
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("escalation: close temp: %w", err)
	}
	// #nosec G703 -- Create/Update validate e.ID before this helper, and Update
	// rejects ID mutation; the temp and destination remain under s.dir.
	if err := os.Rename(tmpName, s.path(e.ID)); err != nil {
		return fmt.Errorf("escalation: rename: %w", err)
	}
	return nil
}

func cloneEscalation(e *Escalation) *Escalation {
	if e == nil {
		return nil
	}
	cp := *e
	cp.Chain = append([]string(nil), e.Chain...)
	if e.Confer != nil {
		confer := *e.Confer
		confer.Members = append([]string(nil), e.Confer.Members...)
		confer.Ballots = append([]Ballot(nil), e.Confer.Ballots...)
		cp.Confer = &confer
	}
	return &cp
}

func validateEscalationID(id string) error {
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id || !isPortableEscalationID(id) {
		return fmt.Errorf("escalation: invalid id %q", id)
	}
	return nil
}

func isPortableEscalationID(id string) bool {
	for i := range len(id) {
		if !isPortableEscalationIDByte(id[i]) {
			return false
		}
	}
	stem := id
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	return !isReservedWindowsRecordName(stem)
}

func isPortableEscalationIDByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.'
}

func isReservedWindowsRecordName(stem string) bool {
	upper := strings.ToUpper(stem)
	switch upper {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	if len(upper) == 4 && (upper[:3] == "COM" || upper[:3] == "LPT") && upper[3] >= '1' && upper[3] <= '9' {
		return true
	}
	return false
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("escalation: nil context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("escalation: operation canceled: %w", err)
	}
	return nil
}

func waitForStoreFileLock(ctx context.Context, try func() (bool, error)) error {
	if ctx == nil {
		return fmt.Errorf("escalation: nil context")
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, defaultStoreLockWait)
	defer cancel()
	ticker := time.NewTicker(storeLockPollInterval)
	defer ticker.Stop()
	for {
		acquired, err := try()
		if err != nil {
			return err
		}
		if acquired {
			return nil
		}
		select {
		case <-waitCtx.Done():
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("escalation: store lock wait canceled: %w", err)
			}
			return fmt.Errorf("escalation: timed out after %s waiting for store lock: %w", defaultStoreLockWait, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func sortEscalations(escalations []*Escalation) {
	sort.Slice(escalations, func(i, j int) bool {
		return escalations[i].CreatedAt.Before(escalations[j].CreatedAt)
	})
}

func matches(e *Escalation, f Filter) bool {
	if f.Pending && e.resolved() {
		return false
	}
	if f.CurrentSessionID != "" && e.CurrentSessionID != f.CurrentSessionID {
		return false
	}
	return true
}

type storeFileLock interface {
	Close() error
}

var (
	_ Store = (*MemStore)(nil)
	_ Store = (*FileStore)(nil)
)
