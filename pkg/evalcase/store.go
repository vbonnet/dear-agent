package evalcase

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// casesSubdir is the directory under a store root that holds one JSON file per
// eval case.
const casesSubdir = "cases"

// FileStore persists EvalCases as one JSON file per case under a root directory
// (the evals/ dataset). It is the discoverable, version-controllable home the
// pipeline writes to.
type FileStore struct {
	root string
}

// NewFileStore returns a store rooted at dir. Cases live in dir/cases/<id>.json.
func NewFileStore(dir string) *FileStore {
	return &FileStore{root: dir}
}

// Root returns the store root directory.
func (s *FileStore) Root() string { return s.root }

// CasesDir returns the directory eval-case files are written to.
func (s *FileStore) CasesDir() string { return filepath.Join(s.root, casesSubdir) }

func (s *FileStore) pathFor(id string) string {
	return filepath.Join(s.CasesDir(), id+".json")
}

// Has reports whether a case with the given (already-sanitised) ID is stored.
func (s *FileStore) Has(id string) bool {
	_, err := os.Stat(s.pathFor(id))
	return err == nil
}

// Save writes c to the store and returns its path. It is idempotent and
// non-destructive: if a case with the same ID already exists it is left
// untouched (existed=true), so a human-curated edit to a generated case is never
// clobbered by a re-run over the same trace.
func (s *FileStore) Save(c EvalCase) (path string, existed bool, err error) {
	if err := os.MkdirAll(s.CasesDir(), 0o755); err != nil {
		return "", false, fmt.Errorf("create cases dir: %w", err)
	}
	path = s.pathFor(c.ID)
	if _, statErr := os.Stat(path); statErr == nil {
		return path, true, nil
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", false, fmt.Errorf("marshal eval case %s: %w", c.ID, err)
	}
	data = append(data, '\n')
	// Write atomically via a temp file + rename so a crash mid-write never
	// leaves a half-written case the loader would choke on.
	tmp, err := os.CreateTemp(s.CasesDir(), c.ID+".*.tmp")
	if err != nil {
		return "", false, fmt.Errorf("create temp case file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", false, fmt.Errorf("write temp case file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", false, fmt.Errorf("close temp case file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return "", false, fmt.Errorf("rename case file: %w", err)
	}
	return path, false, nil
}

// List loads every eval case in the store, sorted by ID. A missing store is an
// empty list, not an error.
func (s *FileStore) List() ([]EvalCase, error) {
	entries, err := os.ReadDir(s.CasesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read cases dir: %w", err)
	}
	var cases []EvalCase
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.CasesDir(), e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read case %s: %w", e.Name(), err)
		}
		var c EvalCase
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("unmarshal case %s: %w", e.Name(), err)
		}
		cases = append(cases, c)
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].ID < cases[j].ID })
	return cases, nil
}
