// Package ops — sandbox_gc.go implements the periodic sandbox sweep (ce-uxju).
//
// SandboxGC walks ~/.agm/sandboxes and reaps every entry that fails ALL
// liveness checks (no non-archived session references it, no process holds a
// cwd/fd inside it, no mount point survives inside it). It is the periodic
// complement to the archive-time reap in runArchiveCleanup: the sweep catches
// sandboxes whose archive-time cleanup was skipped, refused, or predates the
// guarded reaper (the 2.3T / 541-dir backlog of 2026-07-03).
//
// Safety posture (see internal/sandboxgc):
//   - dry-run by default; the caller must set Reap=true to delete anything
//   - refuses to run when the session store is unreachable or empty (an empty
//     store is indistinguishable from a broken one — fail closed)
//   - skips sandboxes younger than MinAge to avoid racing session creation
//   - safety-gate refusals are reported per entry; caller cancellation aborts
//     the sweep with a partial result so it can never look like healthy work
//   - non-git and partial dirs are ordinary reapable content (ce-nd1z)
package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/gclog"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/sandboxgc"
)

// DefaultSandboxMinAge is how old a sandbox dir must be before the sweep will
// touch it. It protects the window between a session row being created and
// our storage snapshot: a sandbox created mid-sweep is never a reap candidate.
const DefaultSandboxMinAge = time.Hour

// SandboxGCRequest defines input for the sandbox sweep.
type SandboxGCRequest struct {
	// Reap actually deletes eligible sandboxes. Default false = dry-run.
	Reap bool `json:"reap,omitempty"`
	// MinAge skips sandboxes modified more recently than this
	// (default DefaultSandboxMinAge).
	MinAge time.Duration `json:"min_age,omitempty"`
	// LiveSessionIDs overrides the session-store source used by the periodic
	// sweep. Nil uses ctx.Storage. This is for CLI surfaces that must aggregate
	// all configured workspace stores before touching the global sandbox pool.
	LiveSessionIDs func() (map[string]bool, error) `json:"-"`
	// Warnings records non-fatal degraded inventory facts, such as a configured
	// workspace database that does not exist. Warnings are surfaced in JSON and
	// text output; they do not authorize deleting without a live-session source.
	Warnings []string `json:"warnings,omitempty"`
	// Source identifies the runner on every durable record produced by this
	// sweep. It is observational metadata, never deletion authority.
	Source string `json:"-"`
}

// SandboxGCEntry records the decision for one sandbox dir.
type SandboxGCEntry struct {
	Name   string `json:"name"`
	Action string `json:"action"` // reaped | would-reap | kept | error
	Reason string `json:"reason,omitempty"`
}

// SandboxGCResult summarises a sweep.
type SandboxGCResult struct {
	Operation string `json:"operation"`
	DryRun    bool   `json:"dry_run"`
	Scanned   int    `json:"scanned"`
	Reaped    int    `json:"reaped"` // deleted (or would-reap in dry-run)
	Kept      int    `json:"kept"`   // refused by a safety gate or too young
	Errors    int    `json:"errors"` // removal attempted and failed
	// ProbeFailures counts entries kept because a safety gate could not be
	// EVALUATED (lsof timed out, the mount table or session store was
	// unreadable) rather than because it positively found the sandbox in
	// use. These are a subset of Kept: a sweep can report zero Errors while
	// every entry was actually a probe failure, which looks identical to a
	// healthy idle sweep unless a reader checks this field too.
	ProbeFailures int `json:"probe_failures,omitempty"`
	// ReapRefused explains why a caller that explicitly asked to delete got a
	// scan instead. Without it the refusal is only inferable from `dry_run`,
	// which an automated caller reads as "this run was a dry run" — an ordinary
	// outcome — rather than "the deletion you requested did not happen". A
	// refusal that reads as a normal outcome is a failure reported as a healthy
	// state, which is precisely how a reap-nothing sweep passed for a working
	// one for a month (ce-uxju). Empty means the run did what it was asked.
	ReapRefused string           `json:"reap_refused,omitempty"`
	Warnings    []string         `json:"warnings,omitempty"`
	Entries     []SandboxGCEntry `json:"entries,omitempty"`
}

// SandboxGC sweeps ~/.agm/sandboxes for reapable sandbox dirs.
func SandboxGC(opCtx *OpContext, req *SandboxGCRequest) (*SandboxGCResult, error) {
	if req == nil {
		req = &SandboxGCRequest{}
	}
	base, err := sandboxgc.DefaultBase()
	if err != nil {
		return nil, err
	}
	liveSessionIDs := req.LiveSessionIDs
	if liveSessionIDs == nil {
		liveSessionIDs = liveSessionIDsFromStorage(opCtx)
	}
	checker := sandboxgc.NewChecker(base, liveSessionIDs)
	return sandboxGCWithChecker(requestContext(opCtx), req, base, checker)
}

// sandboxGCWithChecker is the testable core of SandboxGC.
func sandboxGCWithChecker(ctx context.Context, req *SandboxGCRequest, base string, checker *sandboxgc.Checker) (*SandboxGCResult, error) {
	minAge := req.MinAge
	if minAge <= 0 {
		minAge = DefaultSandboxMinAge
	}

	if err := sandboxGCContextError(ctx, "before live-session inventory"); err != nil {
		return nil, err
	}

	// Fail closed on storage health BEFORE touching the filesystem. An
	// unreachable or empty session store must abort the sweep: with no live-ID
	// set every sandbox would look orphaned.
	if checker.LiveSessionIDs == nil {
		return nil, fmt.Errorf("sandbox gc requires a live-session source; refusing to sweep without one")
	}
	_, inventoryErr := checker.LiveSessionIDs()
	if ctxErr := sandboxGCContextError(ctx, "after live-session inventory"); ctxErr != nil {
		if inventoryErr != nil {
			return nil, errors.Join(ctxErr, fmt.Errorf("session store unreachable, refusing to sweep: %w", inventoryErr))
		}
		return nil, ctxErr
	}
	if inventoryErr != nil {
		return nil, fmt.Errorf("session store unreachable, refusing to sweep: %w", inventoryErr)
	}

	result := &SandboxGCResult{
		Operation: "sandbox_gc",
		DryRun:    !req.Reap,
		Warnings:  append([]string(nil), req.Warnings...),
	}

	entries, readErr := os.ReadDir(base)
	if ctxErr := sandboxGCContextError(ctx, "after reading the sandbox base"); ctxErr != nil {
		if readErr != nil {
			return result, errors.Join(ctxErr, fmt.Errorf("reading sandbox base %s: %w", base, readErr))
		}
		return result, ctxErr
	}
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return result, nil
		}
		return nil, fmt.Errorf("reading sandbox base %s: %w", base, readErr)
	}

	now := time.Now()
	for _, entry := range entries {
		if err := sweepSandboxGCEntry(ctx, req, base, minAge, now, entry, checker, result); err != nil {
			return result, err
		}
	}

	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Name < result.Entries[j].Name })
	if err := sandboxGCContextError(ctx, "before reporting completion"); err != nil {
		return result, err
	}
	return result, nil
}

func sweepSandboxGCEntry(
	ctx context.Context,
	req *SandboxGCRequest,
	base string,
	minAge time.Duration,
	now time.Time,
	entry os.DirEntry,
	checker *sandboxgc.Checker,
	result *SandboxGCResult,
) error {
	if err := sandboxGCContextError(ctx, "before the next sandbox candidate"); err != nil {
		return err
	}
	result.Scanned++
	name := entry.Name()
	dir := filepath.Join(base, name)

	// Age gate: never touch fresh entries (racing session creation).
	info, infoErr := entry.Info()
	if ctxErr := sandboxGCContextError(ctx, "after inspecting sandbox "+name); ctxErr != nil {
		return joinSandboxGCOperationError(ctxErr, wrapSandboxGCEntryError(name, "inspecting", infoErr))
	}
	if infoErr == nil && now.Sub(info.ModTime()) < minAge {
		result.Kept++
		result.Entries = append(result.Entries, SandboxGCEntry{
			Name: name, Action: "kept", Reason: fmt.Sprintf("younger than %s", minAge),
		})
		return nil
	}

	// Non-directories directly under the base (stray files, dead symlinks, and
	// partial provisioning debris) go through the same gates. They are reapable
	// content, not errors (ce-nd1z).
	if !req.Reap {
		return classifySandboxGCDryRunCandidate(ctx, name, dir, checker, result)
	}
	return reapSandboxGCCandidate(ctx, name, dir, req.Source, checker, result)
}

func classifySandboxGCDryRunCandidate(
	ctx context.Context,
	name string,
	dir string,
	checker *sandboxgc.Checker,
	result *SandboxGCResult,
) error {
	// Reap would re-run these gates itself; one classification per entry avoids
	// duplicate lsof and mount scans in preview mode.
	checkErr := checker.CheckReapableContext(ctx, dir)
	if ctxErr := sandboxGCContextError(ctx, "while checking sandbox "+name); ctxErr != nil {
		return joinSandboxGCOperationError(ctxErr, checkErr)
	}
	if checkErr != nil {
		recordSandboxGCRefusal(result, name, checkErr)
		return nil
	}
	result.Reaped++
	result.Entries = append(result.Entries, SandboxGCEntry{Name: name, Action: "would-reap"})
	return nil
}

func reapSandboxGCCandidate(
	ctx context.Context,
	name string,
	dir string,
	source string,
	checker *sandboxgc.Checker,
	result *SandboxGCResult,
) error {
	reapErr := checker.ReapContext(ctx, dir)
	if reapErr != nil {
		if ctxErr := sandboxGCContextError(ctx, "while reaping sandbox "+name); ctxErr != nil {
			if !isSandboxGCRefusal(reapErr) {
				// Removal was attempted and may have partially mutated the tree.
				recordSandboxGCRemovalError(result, name, reapErr)
			}
			return errors.Join(ctxErr, reapErr)
		}
		if isSandboxGCRefusal(reapErr) {
			recordSandboxGCRefusal(result, name, reapErr)
		} else {
			recordSandboxGCRemovalError(result, name, reapErr)
		}
		return nil
	}

	result.Reaped++
	result.Entries = append(result.Entries, SandboxGCEntry{Name: name, Action: "reaped"})
	logGCEntry(gclog.Entry{
		Operation:      "sandbox_gc_reap",
		Source:         source,
		SessionID:      name,
		SandboxRemoved: dir,
	})
	return sandboxGCContextError(ctx, "after reaping sandbox "+name)
}

func isSandboxGCRefusal(err error) bool {
	var refusal *sandboxgc.RefusalError
	return errors.As(err, &refusal)
}

func recordSandboxGCRefusal(result *SandboxGCResult, name string, err error) {
	result.Kept++
	if isProbeFailure(err) {
		result.ProbeFailures++
	}
	result.Entries = append(result.Entries, SandboxGCEntry{
		Name: name, Action: "kept", Reason: refusalReason(err),
	})
}

func recordSandboxGCRemovalError(result *SandboxGCResult, name string, err error) {
	result.Errors++
	result.Entries = append(result.Entries, SandboxGCEntry{
		Name: name, Action: "error", Reason: err.Error(),
	})
}

func wrapSandboxGCEntryError(name, operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s sandbox %s: %w", operation, name, err)
}

func joinSandboxGCOperationError(contextErr, operationErr error) error {
	if operationErr == nil {
		return contextErr
	}
	return errors.Join(contextErr, operationErr)
}

func sandboxGCContextError(ctx context.Context, phase string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("sandbox gc context ended %s: %w", phase, err)
	}
	return nil
}

// liveSessionIDsFromStorage returns a closure yielding the IDs of all
// non-archived sessions. It fails closed: a nil context/storage, a storage
// error, OR a store with zero sessions (indistinguishable from a
// wiped/broken store on this host, which always has archived history)
// aborts the sweep.
//
// The result is memoized: the checker consults the live-session gate once
// per sandbox, and a 500-dir backlog must not turn into 500 storage queries.
// Memoization is safe because a session that goes live MID-sweep gets a NEW
// session ID and a NEW sandbox dir (protected by the MinAge gate), and any
// actively used sandbox is still refused by the live-process gate.
func liveSessionIDsFromStorage(ctx *OpContext) func() (map[string]bool, error) {
	var cached map[string]bool
	var cachedErr error
	var done bool
	return func() (map[string]bool, error) {
		if done {
			return cached, cachedErr
		}
		done = true
		cached, cachedErr = queryLiveSessionIDs(ctx)
		return cached, cachedErr
	}
}

// queryLiveSessionIDs performs the actual (fail-closed) storage query.
func queryLiveSessionIDs(ctx *OpContext) (map[string]bool, error) {
	if ctx == nil || ctx.Storage == nil {
		return nil, fmt.Errorf("no session storage configured — refusing to enumerate live sessions")
	}
	sessions, err := ctx.Storage.ListSessions(nil)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("session store returned zero sessions — refusing to treat all sandboxes as orphaned")
	}
	live := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		if s.Lifecycle != manifest.LifecycleArchived {
			live[s.SessionID] = true
		}
	}
	return live, nil
}

// refusalReason renders a compact reason for report entries.
func refusalReason(err error) string {
	if ref, ok := errors.AsType[*sandboxgc.RefusalError](err); ok {
		return fmt.Sprintf("%s: %s", ref.Reason, ref.Detail)
	}
	return err.Error()
}

// isProbeFailure reports whether err is a RefusalError raised because a
// safety gate could not be evaluated, as opposed to one that positively
// detected a live session/process/mount.
func isProbeFailure(err error) bool {
	ref, ok := errors.AsType[*sandboxgc.RefusalError](err)
	return ok && ref.ProbeFailure
}
