// Command log-rotate bounds disk usage for append-only agent and session logs.
//
// For each target path it rotates the live log when it grows past --max-size
// (renaming to path.YYYYMMDD-HHMMSS, optionally gzipped), then prunes the
// target's directory of rotated siblings older than --max-age or beyond
// --max-files. With no positional args and no --targets it sweeps a default
// set covering the VROOM trail and dear-agent state logs.
//
// Usage:
//
//	log-rotate                              # rotate + prune the default targets
//	log-rotate ~/.agm/vroom/trail.jsonl     # specific files
//	log-rotate --dry-run                    # show intended actions, change nothing
//	log-rotate --max-size 50MB --max-age 14d --max-files 20
//	log-rotate --no-compress                # rename without gzip
//	log-rotate --targets '~/.local/state/dear-agent/*.log,*.jsonl'
//
// Exit codes: 0 = success; 1 = one or more targets failed; 2 = usage/flag error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/logrotate"
)

func main() {
	code, err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "log-rotate:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

// defaultTargets are the glob patterns swept when the user names no paths.
// Best-effort: patterns that match nothing are silently skipped.
func defaultTargets() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".agm", "vroom", "trail.jsonl"),
		filepath.Join(home, ".local", "state", "dear-agent", "*.log"),
		filepath.Join(home, ".local", "state", "dear-agent", "*.jsonl"),
	}
}

func run(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("log-rotate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		maxSize    = fs.String("max-size", "10MB", "Maximum file size before rotation")
		maxAge     = fs.String("max-age", "7d", "Delete rotated files older than this")
		maxFiles   = fs.Int("max-files", 10, "Keep at most N rotated copies")
		noCompress = fs.Bool("no-compress", false, "Disable gzip compression of rotated files")
		dryRun     = fs.Bool("dry-run", false, "Show what would be rotated/pruned without acting")
		targets    = fs.String("targets", "", "Comma-separated glob patterns (overrides positional args)")
	)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil // -h/-help: usage already printed
		}
		return 2, err
	}

	sizeBytes, err := parseSize(*maxSize)
	if err != nil {
		return 2, fmt.Errorf("--max-size: %w", err)
	}
	age, err := parseDuration(*maxAge)
	if err != nil {
		return 2, fmt.Errorf("--max-age: %w", err)
	}

	paths, err := resolvePaths(selectPatterns(*targets, fs.Args()))
	if err != nil {
		return 2, err
	}
	if len(paths) == 0 {
		fmt.Fprintln(stdout, "log-rotate: no matching log files")
		return 0, nil
	}

	r := logrotate.New(logrotate.Config{
		MaxSizeBytes: sizeBytes,
		MaxAge:       age,
		MaxFiles:     *maxFiles,
		Compress:     !*noCompress,
		DryRun:       *dryRun,
	})

	// Rotate every file, then prune each distinct directory once.
	failed := false
	dirs := map[string]struct{}{}
	for _, p := range paths {
		if err := r.Rotate(p); err != nil {
			fmt.Fprintf(stderr, "log-rotate: rotate %s: %v\n", p, err)
			failed = true
		}
		dirs[filepath.Dir(p)] = struct{}{}
	}
	for _, d := range sortedKeys(dirs) {
		if err := r.Prune(d); err != nil {
			fmt.Fprintf(stderr, "log-rotate: prune %s: %v\n", d, err)
			failed = true
		}
	}

	if failed {
		return 1, nil
	}
	fmt.Fprintf(stdout, "log-rotate: processed %d file(s)\n", len(paths))
	return 0, nil
}

// selectPatterns chooses the target glob patterns: --targets wins (comma-split),
// else positional args, else the built-in default set. User-supplied patterns
// get leading-~ expansion; defaults are already absolute.
func selectPatterns(targets string, posArgs []string) []string {
	var patterns []string
	switch {
	case targets != "":
		for p := range strings.SplitSeq(targets, ",") {
			if p = strings.TrimSpace(p); p != "" {
				patterns = append(patterns, expandHome(p))
			}
		}
	case len(posArgs) > 0:
		for _, p := range posArgs {
			patterns = append(patterns, expandHome(p))
		}
	default:
		patterns = defaultTargets()
	}
	return patterns
}

// resolvePaths expands glob patterns to a deduplicated, sorted list of regular
// files. A literal (non-glob) path that doesn't exist is dropped silently so a
// scheduled sweep over default targets never errors on absent logs.
func resolvePaths(patterns []string) ([]string, error) {
	seen := map[string]struct{}{}
	for _, pat := range patterns {
		matches, err := filepath.Glob(pat)
		if err != nil {
			return nil, fmt.Errorf("bad pattern %q: %w", pat, err)
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil || info.IsDir() {
				continue
			}
			seen[m] = struct{}{}
		}
	}
	return sortedKeys(seen), nil
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// parseSize parses a human byte size like "10MB", "512KB", "2GB", or a bare
// byte count ("1048576"). It is case-insensitive and accepts both "MB" and "M".
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		factor int64
	}{
		{"GB", 1 << 30}, {"G", 1 << 30},
		{"MB", 1 << 20}, {"M", 1 << 20},
		{"KB", 1 << 10}, {"K", 1 << 10},
		{"B", 1},
	} {
		if strings.HasSuffix(s, u.suffix) {
			mult = u.factor
			s = strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
			break
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("size must be non-negative")
	}
	return n * mult, nil
}

// parseDuration extends time.ParseDuration with a "d" (day) suffix, since logs
// are pruned on a days-to-weeks scale. "7d" -> 7*24h. Plain Go durations
// ("48h", "30m") still work.
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if d, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.ParseFloat(d, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return time.Duration(days * float64(24*time.Hour)), nil
	}
	return time.ParseDuration(s)
}

// sortedKeys returns the keys of a set sorted lexicographically.
func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
