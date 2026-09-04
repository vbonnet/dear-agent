// Package sandboxgc implements provably-safe garbage collection of session
// sandbox directories under ~/.agm/sandboxes (ce-uxju).
//
// Background: on 2026-07-03 ~/.agm/sandboxes grew to 2.3T across 541 dirs and
// filled the data volume, crashing the host twice. Sandboxes use overlay-style
// merged/ + upper/ layouts; deleting THROUGH a live overlay mount can destroy
// the underlying source repo (see ~/.agm/cleanup-runbook.md). This package
// therefore refuses to touch a sandbox unless ALL of the following hold:
//
//  1. Path validation (allowlist-style): the target is DIRECTLY under the
//     sandbox base dir (~/.agm/sandboxes/<name>) — no nesting, no "..", no
//     absolute-path tricks, and the base itself must end in .agm/sandboxes.
//  2. No live session references it: the session store must be reachable and
//     the sandbox's name must not match any non-archived session ID.
//  3. No live process is inside it: no process has its cwd or an open fd on a
//     path inside the sandbox.
//  4. No mount point is inside it: the mount table must show no mount point at
//     or under the sandbox. A best-effort unmount of <dir>/merged is attempted
//     first; the mount table is then RE-READ and the reap refuses if anything
//     is still mounted inside.
//
// Unlike `agm session gc` worktree pruning (ce-nd1z), this package makes no
// git assumptions: non-git, partial, or corrupt sandbox dirs are ordinary
// reapable content, never errors.
package sandboxgc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Subprocess timeouts: a hung FUSE/NFS mount can wedge `mount` or `lsof`
// indefinitely (the 2026-07-03 incident had sandbox walks timing out). A
// timeout surfaces as an error, and every caller fails CLOSED on error — the
// sandbox is kept, never reaped blind.
const (
	mountCmdTimeout = 10 * time.Second
	lsofCmdTimeout  = 60 * time.Second
)

// Reason codes for refusing to reap a sandbox.
const (
	ReasonBadPath     = "invalid-path"
	ReasonLiveSession = "live-session"
	ReasonLiveProcess = "live-process"
	ReasonMountInside = "mount-inside"
	ReasonDeadline    = "deadline-expired"
)

// RefusalError explains why a sandbox was NOT reaped. It is a refusal, not a
// failure: callers should skip the sandbox and move on.
type RefusalError struct {
	Path   string
	Reason string // one of the Reason* constants
	Detail string
	// ProbeFailure is true when this refusal came from being UNABLE to run a
	// safety check (lsof timed out, the mount table was unreadable, the
	// session store was unreachable) rather than from the check actually
	// detecting a live session/process/mount. Both fail closed and refuse
	// identically, but callers that report sweep health must be able to tell
	// them apart: a probe failure means the sweep could not evaluate this
	// sandbox at all, not that it correctly found the sandbox in use.
	ProbeFailure bool
	ProcessID    int // non-zero only when a specific live process caused the refusal
	cause        error
}

func (e *RefusalError) Error() string {
	return fmt.Sprintf("refusing to reap %s: %s (%s)", e.Path, e.Reason, e.Detail)
}

// Unwrap retains a probe's underlying cause, especially caller cancellation,
// while RefusalError continues to carry the fail-closed decision metadata.
func (e *RefusalError) Unwrap() error {
	return e.cause
}

// ProcPath is one (pid, path) pair from the process table: a cwd or an open fd.
type ProcPath struct {
	PID  int
	Path string
}

// Checker holds the injectable dependencies for the safety gates. Production
// code should construct it with NewChecker; tests inject fakes.
type Checker struct {
	// Base is the absolute sandbox base directory (~/.agm/sandboxes).
	Base string
	// ListMounts returns the mount points currently in the mount table.
	ListMounts func(context.Context) ([]string, error)
	// ListProcPaths returns every (pid, path) the process table currently
	// holds open (cwd + open fds).
	ListProcPaths func(context.Context) ([]ProcPath, error)
	// LiveSessionIDs returns the set of session IDs that are NOT archived.
	// A nil function disables the live-session gate (only valid for the
	// archive path, where the owning session was just archived by the caller).
	LiveSessionIDs func() (map[string]bool, error)
	// Unmount best-effort unmounts a path (nil error if not mounted).
	Unmount func(path string) error
	// Remove recursively removes a path.
	Remove func(path string) error
}

// NewChecker returns a Checker wired to the real host (mount table, lsof,
// syscall.Unmount, os.RemoveAll). liveIDs may be nil ONLY for the
// archive-time reap of a session that the caller has already archived.
func NewChecker(base string, liveIDs func() (map[string]bool, error)) *Checker {
	return &Checker{
		Base:           filepath.Clean(base),
		ListMounts:     listMountsFromMountCmd,
		ListProcPaths:  listProcPathsFromLsof,
		LiveSessionIDs: liveIDs,
		Unmount:        unmountBestEffort,
		Remove:         os.RemoveAll,
	}
}

// DefaultBase returns ~/.agm/sandboxes for the current user.
func DefaultBase() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".agm", "sandboxes"), nil
}

// ValidateSandboxPath enforces the allowlist-style path gate: p must be an
// absolute, clean path DIRECTLY under base (exactly one component below it),
// and base itself must be an absolute path ending in .agm/sandboxes.
func ValidateSandboxPath(base, p string) error {
	if !filepath.IsAbs(base) {
		return fmt.Errorf("sandbox base %q is not absolute", base)
	}
	cleanBase := filepath.Clean(base)
	// Belt-and-suspenders: never operate outside a *.agm/sandboxes base, so a
	// miswired caller can't point the reaper at / or at a repo.
	if !strings.HasSuffix(cleanBase, string(filepath.Separator)+filepath.Join(".agm", "sandboxes")) {
		return fmt.Errorf("sandbox base %q does not end in .agm/sandboxes", base)
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("sandbox path %q is not absolute", p)
	}
	cleanP := filepath.Clean(p)
	if cleanP != p && cleanP+string(filepath.Separator) != p {
		// Reject raw paths that only become safe after cleaning ("a/../b").
		return fmt.Errorf("sandbox path %q is not a clean path", p)
	}
	if filepath.Dir(cleanP) != cleanBase {
		return fmt.Errorf("sandbox path %q is not directly under %q", p, cleanBase)
	}
	name := filepath.Base(cleanP)
	if name == "." || name == ".." || name == "" || name == string(filepath.Separator) {
		return fmt.Errorf("sandbox path %q has invalid final component", p)
	}
	return nil
}

// pathWithin reports whether p is inside (or equal to) dir.
func pathWithin(dir, p string) bool {
	dir = filepath.Clean(dir)
	p = filepath.Clean(p)
	return p == dir || strings.HasPrefix(p, dir+string(filepath.Separator))
}

// MountInside returns the first mount point at or under dir, if any.
func MountInside(mounts []string, dir string) (string, bool) {
	for _, m := range mounts {
		if pathWithin(dir, m) {
			return m, true
		}
	}
	return "", false
}

// ProcessInside returns the first process holding a cwd/fd at or under dir.
func ProcessInside(procs []ProcPath, dir string) (ProcPath, bool) {
	for _, pp := range procs {
		if pathWithin(dir, pp.Path) {
			return pp, true
		}
	}
	return ProcPath{}, false
}

// CheckReapable runs the non-destructive liveness gates (path validation,
// live-session, live-process) against dir. It returns nil when the sandbox is
// eligible for reaping, or a *RefusalError explaining which gate refused.
//
// The mount gate is NOT here: a dead sandbox may legitimately still have its
// overlay mounted, and Reap unmounts it first. The mount-inside hard gate is
// enforced in Reap, on a mount table RE-READ after the unmount attempt —
// immediately before the only RemoveAll in this package.
func (c *Checker) CheckReapable(dir string) error {
	return c.CheckReapableContext(context.Background(), dir)
}

// CheckReapableContext is CheckReapable with caller cancellation propagated
// through host process-table inspection.
func (c *Checker) CheckReapableContext(ctx context.Context, dir string) error {
	if err := contextRefusal(ctx, dir, "before safety checks"); err != nil {
		return err
	}
	if err := ValidateSandboxPath(c.Base, dir); err != nil {
		return &RefusalError{Path: dir, Reason: ReasonBadPath, Detail: err.Error()}
	}

	if c.LiveSessionIDs != nil {
		live, err := c.LiveSessionIDs()
		if err != nil {
			// Fail CLOSED: if we cannot enumerate live sessions we must
			// assume the sandbox is referenced.
			refusal := &RefusalError{Path: dir, Reason: ReasonLiveSession,
				Detail:       fmt.Sprintf("cannot enumerate live sessions: %v", err),
				ProbeFailure: true,
				cause:        err}
			if ctxErr := contextRefusal(ctx, dir, "during live-session inspection"); ctxErr != nil {
				return errors.Join(ctxErr, refusal)
			}
			return refusal
		}
		if err := contextRefusal(ctx, dir, "after live-session inspection"); err != nil {
			return err
		}
		if live[filepath.Base(dir)] {
			return &RefusalError{Path: dir, Reason: ReasonLiveSession,
				Detail: fmt.Sprintf("session %s is not archived", filepath.Base(dir))}
		}
	}

	procs, err := c.ListProcPaths(ctx)
	if err != nil {
		// Fail CLOSED: unknown process state means the dir may be in use.
		refusal := &RefusalError{Path: dir, Reason: ReasonLiveProcess,
			Detail:       fmt.Sprintf("cannot enumerate process paths: %v", err),
			ProbeFailure: true,
			cause:        err}
		if ctxErr := contextRefusal(ctx, dir, "during process inspection"); ctxErr != nil {
			return errors.Join(ctxErr, refusal)
		}
		return refusal
	}
	if err := contextRefusal(ctx, dir, "after process inspection"); err != nil {
		return err
	}
	if pp, found := ProcessInside(procs, dir); found {
		return &RefusalError{Path: dir, Reason: ReasonLiveProcess,
			Detail: fmt.Sprintf("pid %d holds %s", pp.PID, pp.Path), ProcessID: pp.PID}
	}

	return nil
}

// MountedInside reports whether the mount table currently shows a mount point
// at or under dir. Fails closed: an unreadable mount table reports mounted,
// with probeFailure=true so callers can tell "table unreadable" apart from
// "table read fine and a mount was actually found".
func (c *Checker) MountedInside(dir string) (mountPoint string, mounted bool, probeFailure bool) {
	return c.MountedInsideContext(context.Background(), dir)
}

// MountedInsideContext is MountedInside with caller cancellation propagated
// through host mount-table inspection.
func (c *Checker) MountedInsideContext(ctx context.Context, dir string) (mountPoint string, mounted bool, probeFailure bool) {
	mountPoint, mounted, probeFailure, _ = c.mountedInsideContext(ctx, dir)
	return mountPoint, mounted, probeFailure
}

// mountedInsideContext retains the probe's error for ReapContext while the
// legacy public classifier above keeps its three-value compatibility surface.
func (c *Checker) mountedInsideContext(ctx context.Context, dir string) (mountPoint string, mounted bool, probeFailure bool, cause error) {
	mounts, err := c.ListMounts(ctx)
	if err != nil {
		return fmt.Sprintf("unreadable mount table: %v", err), true, true, err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Sprintf("mount inspection deadline: %v", err), true, true, err
	}
	mp, found := MountInside(mounts, dir)
	return mp, found, false, nil
}

// Reap deletes the sandbox at dir after all safety gates pass. Sequence:
//
//  1. CheckReapable (path, live-session, live-process gates).
//  2. Best-effort unmount of <dir>/merged and <dir> themselves (overlay
//     providers mount at merged/).
//  3. RE-READ the mount table; refuse if any mount point is still inside dir.
//     This is the hard gate the cleanup runbook requires: never rm through a
//     live mount.
//  4. Recursive removal.
//
// A *RefusalError means the sandbox was intentionally skipped; any other
// error means removal was attempted and failed.
func (c *Checker) Reap(dir string) error {
	return c.ReapContext(context.Background(), dir)
}

// ReapContext is Reap with one caller-owned lifetime for every safety scan.
// Cancellation always fails closed, and the context is checked again
// immediately before recursive removal so an expired scan budget can never
// authorize a late delete.
func (c *Checker) ReapContext(ctx context.Context, dir string) error {
	if err := c.CheckReapableContext(ctx, dir); err != nil {
		return err
	}

	// Best-effort unmount: overlay providers mount at <dir>/merged.
	for _, mp := range []string{filepath.Join(dir, "merged"), dir} {
		if err := contextRefusal(ctx, dir, "before unmount"); err != nil {
			return err
		}
		if err := c.Unmount(mp); err != nil {
			// Not fatal by itself — the re-read below decides.
			continue
		}
	}

	// HARD GATE: re-read the mount table after unmounting. If anything is
	// still mounted at or under dir, refuse — deleting through a live mount
	// can destroy the mount's source (per ~/.agm/cleanup-runbook.md).
	// MountedInside fails closed on an unreadable mount table.
	m, mounted, probeFailure, mountErr := c.mountedInsideContext(ctx, dir)
	if ctxErr := contextRefusal(ctx, dir, "after mount inspection"); ctxErr != nil {
		if mounted {
			return errors.Join(ctxErr, &RefusalError{Path: dir, Reason: ReasonMountInside,
				Detail:       fmt.Sprintf("mount survived unmount: %s", m),
				ProbeFailure: probeFailure,
				cause:        mountErr})
		}
		return ctxErr
	}
	if mounted {
		return &RefusalError{Path: dir, Reason: ReasonMountInside,
			Detail:       fmt.Sprintf("mount survived unmount: %s", m),
			ProbeFailure: probeFailure,
			cause:        mountErr}
	}

	if err := contextRefusal(ctx, dir, "before removal"); err != nil {
		return err
	}
	if err := c.Remove(dir); err != nil {
		return fmt.Errorf("removing sandbox %s: %w", dir, err)
	}
	return nil
}

// --- real host implementations ---

func contextRefusal(ctx context.Context, dir, phase string) error {
	if err := ctx.Err(); err != nil {
		return &RefusalError{
			Path:         dir,
			Reason:       ReasonDeadline,
			Detail:       fmt.Sprintf("%s: %v", phase, err),
			ProbeFailure: true,
			cause:        err,
		}
	}
	return nil
}

// listMountsFromMountCmd shells out to `mount` and parses the mount points.
// Timeout-bounded: a hung mount-table read must not wedge GC, and the
// resulting error fails closed at the caller.
func listMountsFromMountCmd(parent context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(parent, mountCmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "mount").Output()
	if err != nil {
		return nil, fmt.Errorf("running mount: %w", err)
	}
	return ParseMountOutput(string(out)), nil
}

// ParseMountOutput extracts mount points from BSD/macOS `mount` output.
// Lines look like:
//
//	/dev/disk3s5 on /System/Volumes/Data (apfs, local, journaled, nobrowse)
//	bindfs@... on /Users/x/.agm/sandboxes/<id>/merged (osxfuse, nodev, ...)
//
// The mount point is the text between the FIRST " on " and the LAST " (".
// Using the last " (" tolerates mount points that themselves contain " (".
func ParseMountOutput(out string) []string {
	var mounts []string
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		_, rest, found := strings.Cut(line, " on ")
		if !found {
			continue
		}
		parenIdx := strings.LastIndex(rest, " (")
		if parenIdx < 0 {
			// Linux `mount` style: "... on /path type ext4 (rw)".
			if typeIdx := strings.LastIndex(rest, " type "); typeIdx >= 0 {
				rest = rest[:typeIdx]
			}
			mp := strings.TrimSpace(rest)
			if mp != "" {
				mounts = append(mounts, mp)
			}
			continue
		}
		mp := strings.TrimSpace(rest[:parenIdx])
		// Linux lines end "type ext4 (rw)": strip the trailing " type X".
		if typeIdx := strings.LastIndex(mp, " type "); typeIdx >= 0 {
			mp = strings.TrimSpace(mp[:typeIdx])
		}
		if mp != "" {
			mounts = append(mounts, mp)
		}
	}
	return mounts
}

// listProcPathsFromLsof shells out to lsof for every path any process holds
// (cwd + open fds). lsof exits non-zero when it cannot stat some files, so a
// non-zero exit with usable output is tolerated; no output at all is an error
// (fail closed at the caller). Timeout-bounded so a hung filesystem cannot
// wedge GC; a timeout also fails closed — partial output is never trusted.
func listProcPathsFromLsof(parent context.Context) ([]ProcPath, error) {
	ctx, cancel := context.WithTimeout(parent, lsofCmdTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lsof", "-n", "-P", "-F", "pn").Output()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, fmt.Errorf("running lsof: %w", ctxErr)
	}
	if len(out) == 0 && err != nil {
		return nil, fmt.Errorf("running lsof: %w", err)
	}
	return ParseLsofOutput(string(out)), nil
}

// ParseLsofOutput parses `lsof -F pn` field output: lines starting with 'p'
// carry the PID for all following 'n' (name/path) lines.
func ParseLsofOutput(out string) []ProcPath {
	var procs []ProcPath
	pid := 0
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			if n, err := strconv.Atoi(line[1:]); err == nil {
				pid = n
			}
		case 'n':
			path := line[1:]
			if strings.HasPrefix(path, "/") {
				procs = append(procs, ProcPath{PID: pid, Path: path})
			}
		}
	}
	return procs
}

// unmountBestEffort attempts to unmount path; not-mounted is success.
func unmountBestEffort(path string) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	}
	err := syscall.Unmount(path, 0)
	if err == nil || err == syscall.EINVAL || err == syscall.ENOENT {
		return nil
	}
	return err
}
