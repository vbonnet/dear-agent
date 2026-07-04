// Package beadstore is a bd-CLI-backed bead store with verified writes.
//
// It exists because of ce-ctsi: the legacy engram MCP beads_create tool
// appended to a private JSONL file (~/.beads/issues.jsonl by default) and
// reported success, while the beads database everything else reads is the
// bd/dolt store. Four P0 disk-retro action items were acknowledged and then
// silently lost that way on 2026-07-03.
//
// Design rules enforced here:
//
//  1. No fallback store, ever. The store path (--db) must be configured
//     explicitly; an unconfigured store is a hard error, not a default.
//  2. Every invocation passes --db explicitly. bd's auto-discovery of
//     .beads/*.db from the working directory is the wrong-database foot-gun.
//  3. Writes are verified read-after-write: a create only returns success
//     after `bd show <id>` against the same --db proves the row landed.
//     Note that `bd --json show <missing-id>` exits 0 and prints an error
//     object, so exit codes alone cannot prove anything.
package beadstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ErrStoreNotConfigured is returned when no database path is configured.
// Callers must treat this as fatal — there is deliberately no fallback store.
var ErrStoreNotConfigured = errors.New(
	"beads store not configured: set BEADS_DB (or --beads-db) to the bd database path " +
		"(e.g. ~/beads/context-engine/.beads); refusing to write to a fallback store")

// ErrNotFound is returned by Show when the bead does not exist in the store.
var ErrNotFound = errors.New("bead not found in store")

// Runner executes a command and returns its stdout, stderr, and error.
// It is injectable for tests.
type Runner func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)

// execRunner is the default Runner backed by os/exec.
func execRunner(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return []byte(outBuf.String()), []byte(errBuf.String()), err
}

// defaultTimeout bounds each bd invocation.
const defaultTimeout = 30 * time.Second

// Store is a handle to one explicit bd database.
type Store struct {
	// BDPath is the bd binary to invoke. Empty means "bd" from PATH.
	BDPath string
	// DBPath is the explicit --db value. Required; there is no fallback.
	DBPath string
	// Run executes commands; defaults to os/exec when nil.
	Run Runner
}

// Bead is the subset of bd's issue JSON this package consumes.
type Bead struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Status           string   `json:"status"`
	Priority         int      `json:"priority"`
	IssueType        string   `json:"issue_type,omitempty"`
	Labels           []string `json:"labels,omitempty"`
	EstimatedMinutes int      `json:"estimated_minutes,omitempty"`
	CreatedAt        string   `json:"created_at,omitempty"`
}

// CreateRequest describes a bead to create.
type CreateRequest struct {
	Title            string
	Description      string
	Priority         int // 0 (highest) … 4
	Labels           []string
	EstimatedMinutes int
	IssueType        string // defaults to "task"
}

func (r *CreateRequest) validate() error {
	if strings.TrimSpace(r.Title) == "" {
		return errors.New("title must be a non-empty string")
	}
	if strings.TrimSpace(r.Description) == "" {
		return errors.New("description must be a non-empty string")
	}
	if r.Priority < 0 || r.Priority > 4 {
		return fmt.Errorf("priority must be 0-4 (0=highest), got %d", r.Priority)
	}
	if r.EstimatedMinutes < 0 {
		return fmt.Errorf("estimated_minutes must be >= 0, got %d", r.EstimatedMinutes)
	}
	return nil
}

// bd runs one bd command against the configured store.
func (s *Store) bd(ctx context.Context, args ...string) ([]byte, error) {
	if s.DBPath == "" {
		return nil, ErrStoreNotConfigured
	}
	bin := s.BDPath
	if bin == "" {
		bin = "bd"
	}
	run := s.Run
	if run == nil {
		run = execRunner
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	full := append([]string{"--db", s.DBPath, "--json"}, args...)
	stdout, stderr, err := run(ctx, bin, full...)
	if err != nil {
		return nil, fmt.Errorf("bd %s failed: %w (stderr: %s)",
			args[0], err, strings.TrimSpace(string(stderr)))
	}
	return stdout, nil
}

// VerifiedCreate creates a bead and verifies it landed in the store before
// returning success. Any failure — including an acknowledged write whose row
// cannot be read back — is a hard error.
func (s *Store) VerifiedCreate(ctx context.Context, req CreateRequest) (*Bead, error) {
	if s.DBPath == "" {
		return nil, ErrStoreNotConfigured
	}
	if err := req.validate(); err != nil {
		return nil, err
	}

	issueType := req.IssueType
	if issueType == "" {
		issueType = "task"
	}
	args := []string{
		"create", req.Title,
		"--description", req.Description,
		"--priority", strconv.Itoa(req.Priority),
		"--type", issueType,
	}
	if len(req.Labels) > 0 {
		args = append(args, "--labels", strings.Join(req.Labels, ","))
	}
	if req.EstimatedMinutes > 0 {
		args = append(args, "--estimate", strconv.Itoa(req.EstimatedMinutes))
	}

	out, err := s.bd(ctx, args...)
	if err != nil {
		return nil, err
	}

	var created Bead
	if jsonErr := json.Unmarshal(bytesTrim(out), &created); jsonErr != nil || created.ID == "" {
		return nil, fmt.Errorf(
			"bd create did not return a bead ID (write cannot be verified, treating as failed): output %q",
			strings.TrimSpace(string(out)))
	}

	// Read-after-write: the write is only a success once the row is
	// readable from the same store. This is the ce-ctsi fix.
	verified, err := s.Show(ctx, created.ID)
	if err != nil {
		return nil, fmt.Errorf(
			"beads write NOT verified: bd acknowledged bead %s but it cannot be read back from store %s: %w",
			created.ID, s.DBPath, err)
	}
	return verified, nil
}

// Show fetches one bead by ID. Returns ErrNotFound when the store has no such
// row. bd exits 0 for missing IDs and prints {"error": ...}, so the payload —
// not the exit code — is authoritative.
func (s *Store) Show(ctx context.Context, id string) (*Bead, error) {
	out, err := s.bd(ctx, "show", id)
	if err != nil {
		return nil, err
	}

	trimmed := bytesTrim(out)
	var beads []Bead
	if json.Unmarshal(trimmed, &beads) == nil {
		for i := range beads {
			if beads[i].ID == id {
				return &beads[i], nil
			}
		}
		return nil, fmt.Errorf("%w: %s (store %s)", ErrNotFound, id, s.DBPath)
	}

	// Not an array: bd reports missing IDs as an error object with exit 0.
	var errObj struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(trimmed, &errObj) == nil && errObj.Error != "" {
		return nil, fmt.Errorf("%w: %s (store %s): %s", ErrNotFound, id, s.DBPath, errObj.Error)
	}
	return nil, fmt.Errorf("unparseable bd show output for %s: %q", id, strings.TrimSpace(string(out)))
}

// List returns beads in the store. When all is true, closed beads are
// included and the default result limit is lifted.
func (s *Store) List(ctx context.Context, all bool) ([]Bead, error) {
	args := []string{"list", "-n", "0"}
	if all {
		args = append(args, "--all")
	}
	out, err := s.bd(ctx, args...)
	if err != nil {
		return nil, err
	}
	var beads []Bead
	if err := json.Unmarshal(bytesTrim(out), &beads); err != nil {
		return nil, fmt.Errorf("unparseable bd list output: %w", err)
	}
	return beads, nil
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
