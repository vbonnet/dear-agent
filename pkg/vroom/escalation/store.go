package escalation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// ErrNotFound is returned by Store.Get when no escalation has the given ID.
var ErrNotFound = errors.New("escalation: not found")

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
	Put(ctx context.Context, e *Escalation) error
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

// Put implements Store (stores a copy to avoid aliasing the caller's value).
func (s *MemStore) Put(_ context.Context, e *Escalation) error {
	if e == nil || e.ID == "" {
		return fmt.Errorf("escalation: Put requires a non-nil escalation with an ID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *e
	cp.Chain = append([]string(nil), e.Chain...)
	s.m[e.ID] = &cp
	return nil
}

// Get implements Store.
func (s *MemStore) Get(_ context.Context, id string) (*Escalation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.m[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *e
	cp.Chain = append([]string(nil), e.Chain...)
	return &cp, nil
}

// List implements Store.
func (s *MemStore) List(_ context.Context, f Filter) ([]*Escalation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Escalation, 0, len(s.m))
	for _, e := range s.m {
		if !matches(e, f) {
			continue
		}
		cp := *e
		cp.Chain = append([]string(nil), e.Chain...)
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// FileStore persists each escalation as one JSON file (<id>.json) under a
// directory. Writes are atomic (temp file + rename) so a reader never sees a
// half-written record. Last-writer-wins on concurrent updates to the same
// escalation; this is acceptable for v1 (concurrent answers to one escalation
// are rare) and is documented as a known limitation in WAYFINDER.md.
type FileStore struct {
	dir string
	mu  sync.Mutex // serialises this process's writes; cross-process is rename-atomic
}

// NewFileStore creates (if needed) dir and returns a FileStore rooted there.
func NewFileStore(dir string) (*FileStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("escalation: empty store dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("escalation: mkdir %q: %w", dir, err)
	}
	return &FileStore{dir: dir}, nil
}

func (s *FileStore) path(id string) string { return filepath.Join(s.dir, id+".json") }

// Put implements Store.
func (s *FileStore) Put(_ context.Context, e *Escalation) error {
	if e == nil || e.ID == "" {
		return fmt.Errorf("escalation: Put requires a non-nil escalation with an ID")
	}
	if filepath.Base(e.ID) != e.ID {
		return fmt.Errorf("escalation: invalid id %q", e.ID)
	}
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("escalation: marshal: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tmp, err := os.CreateTemp(s.dir, e.ID+".*.tmp")
	if err != nil {
		return fmt.Errorf("escalation: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("escalation: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("escalation: close temp: %w", err)
	}
	if err := os.Rename(tmpName, s.path(e.ID)); err != nil {
		return fmt.Errorf("escalation: rename: %w", err)
	}
	return nil
}

// Get implements Store.
func (s *FileStore) Get(_ context.Context, id string) (*Escalation, error) {
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
	return &e, nil
}

// List implements Store.
func (s *FileStore) List(ctx context.Context, f Filter) ([]*Escalation, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("escalation: readdir: %w", err)
	}
	out := make([]*Escalation, 0, len(entries))
	for _, de := range entries {
		name := de.Name()
		if de.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		id := name[:len(name)-len(".json")]
		e, err := s.Get(ctx, id)
		if err != nil {
			continue // skip unreadable/partial entries rather than fail the whole list
		}
		if matches(e, f) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
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

var (
	_ Store = (*MemStore)(nil)
	_ Store = (*FileStore)(nil)
)
