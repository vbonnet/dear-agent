package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileClaimStore is a ClaimStore backed by per-supervisor claim files on disk,
// so leader-election claims survive across separate AGM sessions (the retro's
// claim/verify/assume/announce protocol runs between supervisors that are
// distinct processes). Layout under Root:
//
//	<root>/<by>/claim-<target>.json
//
// This mirrors the retro's `~/.agm/supervisors/<self>/meta-claim.json` sketch,
// generalised so a supervisor can claim more than one target. Each file holds
// one JSON-encoded Claim. Get scans every supervisor's directory for the
// target and returns the winning claim.
type FileClaimStore struct {
	root string
}

// fileClaim is the on-disk JSON shape (Role is a string).
type fileClaim struct {
	Target    string    `json:"target"`
	By        string    `json:"by"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// DefaultClaimStoreRoot is the on-disk root the retro names for supervisor
// claim files.
func DefaultClaimStoreRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "agm", "supervisors")
	}
	return filepath.Join(home, ".agm", "supervisors")
}

// NewFileClaimStore returns a FileClaimStore rooted at root. If root is empty
// it falls back to DefaultClaimStoreRoot. The root directory is created lazily
// on first Put.
func NewFileClaimStore(root string) *FileClaimStore {
	if root == "" {
		root = DefaultClaimStoreRoot()
	}
	return &FileClaimStore{root: root}
}

func (s *FileClaimStore) claimPath(target, by Role) string {
	return filepath.Join(s.root, string(by), fmt.Sprintf("claim-%s.json", string(target)))
}

// Put implements ClaimStore. It writes atomically (temp file + rename) so a
// reader never observes a partially-written claim.
func (s *FileClaimStore) Put(_ context.Context, c Claim) error {
	if !c.Target.Valid() || !c.By.Valid() {
		return fmt.Errorf("FileClaimStore: invalid claim target=%q by=%q", c.Target, c.By)
	}
	path := s.claimPath(c.Target, c.By)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("FileClaimStore: mkdir: %w", err)
	}
	data, err := json.Marshal(fileClaim{
		Target:    string(c.Target),
		By:        string(c.By),
		Reason:    c.Reason,
		Timestamp: c.Timestamp,
	})
	if err != nil {
		return fmt.Errorf("FileClaimStore: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("FileClaimStore: write temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("FileClaimStore: rename: %w", err)
	}
	return nil
}

// Get implements ClaimStore. It scans every supervisor directory under root
// for a claim on target and returns the winner per claimWins.
func (s *FileClaimStore) Get(_ context.Context, target Role) (Claim, bool, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return Claim{}, false, nil
		}
		return Claim{}, false, fmt.Errorf("FileClaimStore: read root: %w", err)
	}
	var winner Claim
	have := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(s.root, e.Name(), fmt.Sprintf("claim-%s.json", string(target)))
		data, err := os.ReadFile(path)
		if err != nil {
			continue // no claim from this supervisor (or unreadable) — skip
		}
		var fc fileClaim
		if err := json.Unmarshal(data, &fc); err != nil {
			continue // ignore a corrupt claim file rather than fail the whole read
		}
		c := Claim{
			Target:    Role(fc.Target),
			By:        Role(fc.By),
			Reason:    fc.Reason,
			Timestamp: fc.Timestamp,
		}
		if !have || claimWins(c, winner) {
			winner = c
			have = true
		}
	}
	return winner, have, nil
}

// Delete implements ClaimStore. Removing a claim that does not exist is not an
// error.
func (s *FileClaimStore) Delete(_ context.Context, target Role, by Role) error {
	if err := os.Remove(s.claimPath(target, by)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("FileClaimStore: remove: %w", err)
	}
	return nil
}
