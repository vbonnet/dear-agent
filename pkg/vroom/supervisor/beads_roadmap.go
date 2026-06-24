package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// BeadsRoadmap is the production Roadmap adapter. It reads pending work
// proposals from a beads database via "bd ready --json" and records
// Meta-Orchestrator decisions as bead notes.
//
// Accept and Reject write bead notes (soft audit records). Accept also
// forwards the proposal to an optional Queue so the Orchestrator can see
// newly admitted work without an extra bd read.
//
// State transitions (in_progress / closed) are NOT triggered here — those
// are owned by the worker that actually claims and executes the bead.
type BeadsRoadmap struct {
	// DB is the path passed to "bd --db". Defaults to
	// ~/beads/context-engine/.beads (the canonical CLAUDE.md path).
	DB string

	// BDBin is the bd binary to invoke. Empty resolves "bd" on PATH.
	BDBin string

	// Timeout bounds a single bd invocation. Zero uses defaultBeadsTimeout.
	Timeout time.Duration

	// Queue receives a Task for each accepted proposal (optional). When set,
	// Accept enqueues the proposal so the Orchestrator can dispatch it
	// without a separate beads read.
	Queue interface {
		Enqueue(Task) error
	}

	// run is an injection seam for tests; nil uses exec.CommandContext.
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

const defaultBeadsTimeout = 30 * time.Second

// maxDescriptionBytes is the maximum number of bytes kept from a bead's
// description before it is truncated. Bead descriptions can be thousands
// of characters; WorkProposal.Reason only needs the gist.
const maxDescriptionBytes = 500

// bdReadyItem mirrors the subset of "bd ready --json" array elements that
// BeadsRoadmap consumes. Additional fields are ignored.
type bdReadyItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
	IssueType   string `json:"issue_type"`
}

// PendingProposals shells out to "bd --db <DB> ready --json" and maps each
// ready bead to a WorkProposal. The description is truncated to avoid
// bloating the decision trail with multi-KB bead bodies.
func (r *BeadsRoadmap) PendingProposals(ctx context.Context) ([]WorkProposal, error) {
	out, err := r.exec(ctx, "--db", r.dbPath(), "ready", "--json")
	if err != nil {
		return nil, fmt.Errorf("BeadsRoadmap.PendingProposals: bd ready: %w", err)
	}
	var items []bdReadyItem
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("BeadsRoadmap.PendingProposals: parse output: %w", err)
	}
	proposals := make([]WorkProposal, 0, len(items))
	for _, item := range items {
		reason := item.Description
		if reason == "" {
			reason = "(no description)"
		} else if len(reason) > maxDescriptionBytes {
			reason = reason[:maxDescriptionBytes] + "…"
		}
		scope := scopeFromIssueType(item.IssueType)
		proposals = append(proposals, WorkProposal{
			ID:        item.ID,
			Title:     item.Title,
			Reason:    reason,
			ScopeHint: scope,
		})
	}
	return proposals, nil
}

// Accept appends a note to the bead recording the Meta-Orchestrator's
// decision, then optionally enqueues the accepted proposal to r.Queue.
func (r *BeadsRoadmap) Accept(ctx context.Context, proposalID string) error {
	_, err := r.exec(ctx, "--db", r.dbPath(), "note", proposalID,
		"accepted into vroom roadmap by Meta-Orchestrator (vroom-mesh)")
	if err != nil {
		return fmt.Errorf("BeadsRoadmap.Accept %s: bd note: %w", proposalID, err)
	}
	if r.Queue != nil {
		if enqErr := r.Queue.Enqueue(Task{
			ID:    proposalID,
			Title: proposalID,
		}); enqErr != nil {
			// Log but don't fail — the bd note is the durable record.
			return fmt.Errorf("BeadsRoadmap.Accept %s: enqueue: %w", proposalID, enqErr)
		}
	}
	return nil
}

// Reject appends a note to the bead recording the Meta-Orchestrator's
// rejection with the stated reason.
func (r *BeadsRoadmap) Reject(ctx context.Context, proposalID, reason string) error {
	msg := fmt.Sprintf("rejected by vroom Meta-Orchestrator: %s", reason)
	_, err := r.exec(ctx, "--db", r.dbPath(), "note", proposalID, msg)
	if err != nil {
		return fmt.Errorf("BeadsRoadmap.Reject %s: bd note: %w", proposalID, err)
	}
	return nil
}

// dbPath returns the beads database path, defaulting to the canonical path.
func (r *BeadsRoadmap) dbPath() string {
	if r.DB != "" {
		return r.DB
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "beads", "context-engine", ".beads")
}

func (r *BeadsRoadmap) exec(ctx context.Context, args ...string) ([]byte, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultBeadsTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if r.run != nil {
		bin := r.BDBin
		if bin == "" {
			bin = "bd"
		}
		return r.run(ctx, bin, args...)
	}

	bin := r.BDBin
	if bin == "" {
		bin = "bd"
	}
	return exec.CommandContext(ctx, bin, args...).Output() //#nosec G204 -- bin is a fixed binary path; args are controlled caller values
}

// scopeFromIssueType maps a beads issue_type to the nearest ScopeHint.
func scopeFromIssueType(t string) ScopeHint {
	switch t {
	case "bug", "defect":
		return ScopeFix
	case "feature", "enhancement":
		return ScopeFeature
	case "research", "spike":
		return ScopeResearch
	case "refactor":
		return ScopeRefactor
	case "infra", "chore", "ci":
		return ScopeInfra
	default:
		return ScopeUnspecified
	}
}
