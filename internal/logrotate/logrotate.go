// Package logrotate provides size- and age-based rotation for agent and
// session log files, bounding on-disk usage while keeping rotated logs
// queryable (optionally gzip-compressed rather than discarded).
//
// It is deliberately narrow and side-effect-explicit, mirroring the
// internal/safesrc wrapper philosophy: a single Policy describes the desired
// retention, every action is reported in a Result, and a dry run performs no
// writes at all so a caller can preview exactly what would change.
//
// The canonical target is ~/.agm/logs (see cmd/log-rotate), which holds
// per-subsystem logs (messages/, daemon/, audit.jsonl, diary.jsonl, …) that
// otherwise grow without bound.
package logrotate

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// rotatedLayout is the timestamp suffix appended to a rotated file. It is
// lexicographically sortable and filesystem-safe (no colons), so a plain
// string sort of backup names matches chronological order.
const rotatedLayout = "20060102T150405Z"

// Policy describes when a log file is rotated and how many rotated copies are
// retained. A zero value rotates nothing (MaxSizeBytes <= 0 disables size
// rotation); set fields explicitly.
type Policy struct {
	// MaxSizeBytes rotates a live log once it reaches this size. Values <= 0
	// disable size-based rotation.
	MaxSizeBytes int64
	// MaxBackups is the maximum number of rotated copies retained per base log.
	// Values <= 0 keep an unlimited number (age may still prune them).
	MaxBackups int
	// MaxAge prunes rotated copies older than this. Values <= 0 disable
	// age-based pruning.
	MaxAge time.Duration
	// Compress gzips a rotated copy and removes the uncompressed original.
	Compress bool
}

// Action records a single mutation logrotate performed (or, in dry-run mode,
// would have performed).
type Action struct {
	Kind string // "rotate", "compress", or "prune"
	Path string // the file the action applied to
	Note string // human-readable detail (e.g. resulting backup name, size)
}

// Result is the full set of actions for a single RotateDir/RotateFile call.
type Result struct {
	Actions []Action
	DryRun  bool
}

// clock is overridable in tests so rotated-suffix timestamps are deterministic.
// It defaults to time.Now and is never nil in production.
type clock func() time.Time

// Rotator applies a Policy to log files. The zero value is not usable; use
// New.
type Rotator struct {
	policy Policy
	now    clock
}

// New returns a Rotator bound to policy, using the wall clock.
func New(policy Policy) *Rotator {
	return &Rotator{policy: policy, now: time.Now}
}

// RotateDir applies the policy to every regular file directly under dir whose
// name does not look like an already-rotated backup. When recursive is true,
// subdirectories are walked as well. Files are processed in a stable order so
// the action log is deterministic. A dry run performs no filesystem writes.
func (r *Rotator) RotateDir(dir string, recursive, dryRun bool) (Result, error) {
	res := Result{DryRun: dryRun}
	info, err := os.Stat(dir)
	if err != nil {
		return res, fmt.Errorf("stat log dir %q: %w", dir, err)
	}
	if !info.IsDir() {
		return res, fmt.Errorf("log dir %q is not a directory", dir)
	}

	var liveLogs []string
	walk := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if !recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || isRotated(d.Name()) {
			return nil
		}
		liveLogs = append(liveLogs, path)
		return nil
	}
	if err := filepath.WalkDir(dir, walk); err != nil {
		return res, fmt.Errorf("walk log dir %q: %w", dir, err)
	}
	sort.Strings(liveLogs)

	for _, path := range liveLogs {
		if err := r.rotateOne(path, dryRun, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// RotateFile applies the policy to a single live log file (plus pruning of its
// existing backups). It is the building block RotateDir uses per file.
func (r *Rotator) RotateFile(path string, dryRun bool) (Result, error) {
	res := Result{DryRun: dryRun}
	err := r.rotateOne(path, dryRun, &res)
	return res, err
}

func (r *Rotator) rotateOne(path string, dryRun bool, res *Result) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %q: %w", path, err)
	}

	if r.policy.MaxSizeBytes > 0 && fi.Size() >= r.policy.MaxSizeBytes {
		backup, err := r.rotate(path, dryRun, res)
		if err != nil {
			return err
		}
		if r.policy.Compress {
			if err := r.compress(backup, dryRun, res); err != nil {
				return err
			}
		}
	}

	return r.prune(path, dryRun, res)
}

// rotate moves path aside to a timestamped backup and recreates an empty live
// log with the original mode. It returns the backup path. In dry-run mode it
// records the intended action and returns the would-be backup path without
// touching disk.
func (r *Rotator) rotate(path string, dryRun bool, res *Result) (string, error) {
	backup := path + "." + r.now().UTC().Format(rotatedLayout)
	// Avoid clobbering an existing backup produced within the same second.
	for i := 1; ; i++ {
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			break
		} else if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("stat backup %q: %w", backup, err)
		}
		backup = fmt.Sprintf("%s.%s-%d", path, r.now().UTC().Format(rotatedLayout), i)
	}

	res.Actions = append(res.Actions, Action{Kind: "rotate", Path: path, Note: filepath.Base(backup)})
	if dryRun {
		return backup, nil
	}

	fi, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat %q: %w", path, err)
	}
	if err := os.Rename(path, backup); err != nil {
		return "", fmt.Errorf("rotate %q -> %q: %w", path, backup, err)
	}
	// Recreate the live log so producers that hold the path (not the fd) keep
	// writing to a fresh, empty file.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return "", fmt.Errorf("recreate %q: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close recreated %q: %w", path, err)
	}
	return backup, nil
}

// compress gzips src to src+".gz" and removes src. In dry-run mode it only
// records the intended action.
func (r *Rotator) compress(src string, dryRun bool, res *Result) error {
	dst := src + ".gz"
	res.Actions = append(res.Actions, Action{Kind: "compress", Path: src, Note: filepath.Base(dst)})
	if dryRun {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open for compress %q: %w", src, err)
	}
	defer in.Close()

	// Carry the rotated file's permissions onto the .gz so a backup never
	// becomes more readable than the log it came from.
	srcInfo, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat for compress %q: %w", src, err)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create %q: %w", dst, err)
	}
	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		_ = gz.Close()
		_ = out.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("gzip %q: %w", src, err)
	}
	if err := gz.Close(); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("flush gzip %q: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("close %q: %w", dst, err)
	}
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("remove uncompressed %q: %w", src, err)
	}
	return nil
}

// prune enforces MaxBackups and MaxAge against the backups of a single base
// log. Backups are the files whose name is base + a rotated suffix.
func (r *Rotator) prune(base string, dryRun bool, res *Result) error {
	backups, err := backupsFor(base)
	if err != nil {
		return err
	}
	// Newest first, so index >= MaxBackups are the surplus oldest copies.
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime.After(backups[j].modTime)
	})

	cutoff := r.now().Add(-r.policy.MaxAge)
	for i, b := range backups {
		overCount := r.policy.MaxBackups > 0 && i >= r.policy.MaxBackups
		tooOld := r.policy.MaxAge > 0 && b.modTime.Before(cutoff)
		if !overCount && !tooOld {
			continue
		}
		reason := "max-backups"
		if tooOld && !overCount {
			reason = "max-age"
		}
		res.Actions = append(res.Actions, Action{Kind: "prune", Path: b.path, Note: reason})
		if dryRun {
			continue
		}
		if err := os.Remove(b.path); err != nil {
			return fmt.Errorf("prune %q: %w", b.path, err)
		}
	}
	return nil
}

type backup struct {
	path    string
	modTime time.Time
}

// backupsFor returns the rotated copies of the live log at base, matching
// "<base>.<rotated-suffix>" optionally followed by ".gz".
func backupsFor(base string) ([]backup, error) {
	dir := filepath.Dir(base)
	prefix := filepath.Base(base) + "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %q: %w", dir, err)
	}
	var out []backup
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, prefix) {
			continue
		}
		if !isRotated(name) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("stat backup %q: %w", name, err)
		}
		out = append(out, backup{path: filepath.Join(dir, name), modTime: fi.ModTime()})
	}
	return out, nil
}

// isRotated reports whether name looks like a rotated backup this package
// produces: a base name followed by ".<14-digit-timestamp>Z" (with an optional
// "-N" collision suffix and/or ".gz" compression suffix). This is what keeps
// RotateDir from treating an already-rotated file as a fresh live log.
func isRotated(name string) bool {
	trimmed := strings.TrimSuffix(name, ".gz")
	dot := strings.LastIndex(trimmed, ".")
	if dot < 0 {
		return false
	}
	suffix := trimmed[dot+1:]
	// Strip an optional "-N" collision marker.
	if dash := strings.IndexByte(suffix, '-'); dash >= 0 {
		suffix = suffix[:dash]
	}
	_, err := time.Parse(rotatedLayout, suffix)
	return err == nil
}
