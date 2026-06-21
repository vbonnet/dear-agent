// Package beads is a thin wrapper over the `bd` CLI that lets wayfinder file a
// tracking bead for a session. Wayfinder creates one bead per task at the SETUP
// phase so every task is tracked from the start and its PR can auto-close it on
// merge (see internal/safepr, which folds "Closes <bead>" into the PR body).
package beads

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DefaultPriority is the priority of an auto-created session bead. P1 mirrors
// the rest of the dear-agent bead-filing tooling (cmd/merge-audit).
const DefaultPriority = "P1"

// DefaultDB returns the beads database wayfinder files into. It honours the
// WAYFINDER_BEADS_DB override (used by tests and non-standard checkouts) and
// otherwise defaults to ~/beads/context-engine/.beads, the same path the
// merge-audit tooling uses.
func DefaultDB() string {
	if db := os.Getenv("WAYFINDER_BEADS_DB"); db != "" {
		return db
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "beads", "context-engine", ".beads")
}

// Available reports whether the bd CLI is on PATH. Callers skip auto-creation
// (with a warning) rather than failing the run when bd is absent — a missing
// tracker must never block a phase transition.
func Available() bool {
	_, err := exec.LookPath("bd")
	return err == nil
}

// createArgs builds the argv (after "bd") for filing a session bead. Split out
// from Create so the flag construction is unit-testable without invoking bd.
// --silent makes bd print only the new issue ID on stdout, which Create reads
// back as the bead id.
func createArgs(db, title string) []string {
	args := []string{}
	if db != "" {
		args = append(args, "--db", db)
	}
	args = append(args, "create", title,
		"--priority", DefaultPriority,
		"--type", "task",
		"--silent")
	return args
}

// Create files a new bead titled title into db and returns its id. db may be
// empty to use bd's own database resolution. A non-nil error means no bead was
// created; the caller should warn and continue.
func Create(ctx context.Context, db, title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", fmt.Errorf("cannot create bead with empty title")
	}
	// #nosec G204 — fixed "bd" binary; all args passed as argv (no shell).
	cmd := exec.CommandContext(ctx, "bd", createArgs(db, title)...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(errBuf.Bytes()); len(msg) > 0 {
			return "", fmt.Errorf("bd create failed: %w: %s", err, msg)
		}
		return "", fmt.Errorf("bd create failed: %w", err)
	}
	id := strings.TrimSpace(out.String())
	if id == "" {
		return "", fmt.Errorf("bd create produced no bead id (stdout empty)")
	}
	return id, nil
}
