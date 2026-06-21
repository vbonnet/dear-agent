// Package logrotate bounds disk usage for append-only agent and session logs
// (e.g. ~/.agm/vroom/trail.jsonl, ~/.local/state/dear-agent/*.log) while
// keeping older data queryable on disk.
//
// A Rotator does two things, each independently callable:
//
//   - Rotate(path): if the live log has grown past MaxSizeBytes, rename it to a
//     timestamped sibling (path.YYYYMMDD-HHMMSS), optionally gzip it, then
//     re-create an empty live log preserving the original mode. Writers that
//     hold the path (not an fd) pick up the fresh file on their next open.
//   - Prune(dir): delete rotated siblings that are older than MaxAge or that
//     exceed MaxFiles (oldest first), keeping the newest within both bounds.
//
// DryRun makes every method report intended actions to stderr and change
// nothing, so an operator can preview a sweep before arming it.
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

// Default configuration values used when a Config field is left at its zero
// value by New.
const (
	DefaultMaxSizeBytes int64         = 10 * 1024 * 1024 // 10MB
	DefaultMaxAge       time.Duration = 7 * 24 * time.Hour
	DefaultMaxFiles     int           = 10
)

// timestampLayout is the suffix format appended to a rotated file. It sorts
// lexicographically in chronological order, which Prune relies on.
const timestampLayout = "20060102-150405"

// Config controls rotation and pruning thresholds.
type Config struct {
	MaxSizeBytes int64         // rotate when the live file exceeds this (default 10MB)
	MaxAge       time.Duration // prune rotated files older than this (default 7 days)
	MaxFiles     int           // keep at most N rotated files per directory (default 10)
	Compress     bool          // gzip rotated files
	DryRun       bool          // report intended actions to stderr, change nothing

	// now returns the current time. It exists so tests can pin timestamps;
	// production code leaves it nil and gets time.Now.
	now func() time.Time

	// stderr is where DryRun and best-effort warnings are written. Tests may
	// redirect it; production code leaves it nil and gets os.Stderr.
	stderr io.Writer
}

// Rotator performs size/age/count-bounded log rotation per its Config.
type Rotator struct {
	cfg Config
}

// New returns a Rotator. Zero-valued size/age/count fields fall back to the
// package defaults so callers can pass a sparse Config (e.g. just Compress).
func New(cfg Config) *Rotator {
	if cfg.MaxSizeBytes <= 0 {
		cfg.MaxSizeBytes = DefaultMaxSizeBytes
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = DefaultMaxAge
	}
	if cfg.MaxFiles <= 0 {
		cfg.MaxFiles = DefaultMaxFiles
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	if cfg.stderr == nil {
		cfg.stderr = os.Stderr
	}
	return &Rotator{cfg: cfg}
}

// Rotate rotates a single log file if it has grown past MaxSizeBytes.
//
// A missing file or one at/under the threshold is a no-op (nil error): rotation
// is meant to run on a schedule, so absent or small logs are normal. After a
// successful rename (and optional gzip) a fresh empty file is created at path
// with the original file's mode, so the writer keeps appending to the same name.
func (r *Rotator) Rotate(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil // nothing to rotate yet
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() <= r.cfg.MaxSizeBytes {
		return nil // under threshold
	}

	dest := fmt.Sprintf("%s.%s", path, r.cfg.now().Format(timestampLayout))
	if r.cfg.Compress {
		dest += ".gz"
	}

	if r.cfg.DryRun {
		r.logf("would rotate %s (%d bytes) -> %s", path, info.Size(), dest)
		return nil
	}

	if r.cfg.Compress {
		if err := compressFile(path, dest, info.Mode()); err != nil {
			return fmt.Errorf("compress %s: %w", path, err)
		}
		// Truncate the live file in place rather than remove+recreate so any
		// process holding the path keeps writing to the same inode.
		if err := os.Truncate(path, 0); err != nil {
			return fmt.Errorf("truncate %s after compress: %w", path, err)
		}
		return nil
	}

	// Uncompressed: rename the live file aside, then re-create it empty with
	// the original mode.
	if err := os.Rename(path, dest); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", path, dest, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("recreate %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close recreated %s: %w", path, err)
	}
	return nil
}

// Prune deletes rotated log files in dir that exceed MaxAge or MaxFiles.
//
// A rotated file is any sibling matching "<base>.<timestamp>" or
// "<base>.<timestamp>.gz" for some live log base in dir. Pruning is grouped per
// base: each live log's rotated set is bounded independently so a busy log
// can't evict another log's history. Files past MaxAge go first; then, if a
// group still exceeds MaxFiles, the oldest extras are removed.
func (r *Rotator) Prune(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	// Group rotated files by their base log name.
	groups := map[string][]rotatedFile{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		base, ts, ok := parseRotatedName(e.Name())
		if !ok {
			continue
		}
		groups[base] = append(groups[base], rotatedFile{
			path: filepath.Join(dir, e.Name()),
			ts:   ts,
		})
	}

	cutoff := r.cfg.now().Add(-r.cfg.MaxAge)
	var firstErr error
	for _, files := range groups {
		// Newest first so MaxFiles keeps the most recent.
		sort.Slice(files, func(i, j int) bool { return files[i].ts.After(files[j].ts) })

		kept := 0
		for _, f := range files {
			remove := false
			switch {
			case f.ts.Before(cutoff):
				remove = true // too old
			case kept >= r.cfg.MaxFiles:
				remove = true // too many
			default:
				kept++
			}
			if !remove {
				continue
			}
			if r.cfg.DryRun {
				r.logf("would prune %s", f.path)
				continue
			}
			if err := os.Remove(f.path); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("remove %s: %w", f.path, err)
			}
		}
	}
	return firstErr
}

// rotatedFile is one rotated log sibling with its parsed timestamp.
type rotatedFile struct {
	path string
	ts   time.Time
}

// parseRotatedName splits a rotated filename into its base log name and
// timestamp. It accepts "<base>.<timestamp>" and "<base>.<timestamp>.gz",
// returning ok=false for anything that is not a recognized rotated file (e.g.
// the live log itself, or an unrelated file).
func parseRotatedName(name string) (base string, ts time.Time, ok bool) {
	trimmed := strings.TrimSuffix(name, ".gz")
	dot := strings.LastIndex(trimmed, ".")
	if dot < 0 {
		return "", time.Time{}, false
	}
	stamp := trimmed[dot+1:]
	parsed, err := time.ParseInLocation(timestampLayout, stamp, time.Local)
	if err != nil {
		return "", time.Time{}, false
	}
	return trimmed[:dot], parsed, true
}

// compressFile gzips src into dest (which must not already exist as a non-gz
// caller convention), giving dest the source file's permission bits.
func compressFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}

	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		gz.Close()
		out.Close()
		os.Remove(dest)
		return err
	}
	if err := gz.Close(); err != nil {
		out.Close()
		os.Remove(dest)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dest)
		return err
	}
	return nil
}

// logf writes a line to the configured stderr, prefixed for grep-ability.
func (r *Rotator) logf(format string, args ...any) {
	prefix := "logrotate: "
	if r.cfg.DryRun {
		prefix = "logrotate (dry-run): "
	}
	fmt.Fprintf(r.cfg.stderr, prefix+format+"\n", args...)
}
