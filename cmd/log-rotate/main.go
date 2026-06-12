// log-rotate bounds the on-disk size of agent and session logs by rotating
// live log files that have grown past a size threshold, then pruning the
// rotated copies by count and age. Rotated copies are kept (optionally
// gzip-compressed) rather than discarded, so historical logs stay queryable.
//
// Usage:
//
//	log-rotate [flags] [dir]
//
// dir defaults to ~/.agm/logs, the canonical agent/session log root. With no
// flags it runs in dry-run mode and prints exactly what it would do, so a
// caller can preview before committing with --apply.
//
// Examples:
//
//	log-rotate                                  # dry-run over ~/.agm/logs
//	log-rotate --apply --recursive              # rotate the whole tree
//	log-rotate --apply --max-size 50MB --compress --max-backups 5 ~/.agm/logs
//	log-rotate --apply --max-age 720h           # also prune backups older than 30d
//
// It is intentionally narrow: it never deletes a live log, only rotates it and
// prunes already-rotated copies. See internal/logrotate for the mechanics.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/internal/logrotate"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "log-rotate: %v\n", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	fs := flag.NewFlagSet("log-rotate", flag.ContinueOnError)
	var (
		apply      = fs.Bool("apply", false, "perform changes (default is dry-run preview)")
		recursive  = fs.Bool("recursive", false, "rotate logs in subdirectories too")
		compress   = fs.Bool("compress", false, "gzip rotated copies")
		maxSize    = fs.String("max-size", "100MB", "rotate a live log once it reaches this size (e.g. 50MB, 1GB)")
		maxBackups = fs.Int("max-backups", 7, "rotated copies retained per log (0 = unlimited)")
		maxAge     = fs.Duration("max-age", 0, "prune rotated copies older than this (0 = disabled, e.g. 720h)")
	)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: log-rotate [flags] [dir]\n\n"+
			"dir defaults to ~/.agm/logs. Runs in dry-run mode unless --apply is given.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(argv); err != nil {
		return err
	}

	sizeBytes, err := parseSize(*maxSize)
	if err != nil {
		return err
	}

	dir := fs.Arg(0)
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home dir: %w", err)
		}
		dir = filepath.Join(home, ".agm", "logs")
	}

	policy := logrotate.Policy{
		MaxSizeBytes: sizeBytes,
		MaxBackups:   *maxBackups,
		MaxAge:       *maxAge,
		Compress:     *compress,
	}
	dryRun := !*apply

	res, err := logrotate.New(policy).RotateDir(dir, *recursive, dryRun)
	if err != nil {
		return err
	}

	printSummary(dir, res)
	return nil
}

func printSummary(dir string, res logrotate.Result) {
	mode := "applied"
	if res.DryRun {
		mode = "dry-run (no changes written; use --apply)"
	}
	fmt.Printf("log-rotate %s — %s\n", dir, mode)
	if len(res.Actions) == 0 {
		fmt.Println("  nothing to do (no logs over threshold, no backups to prune)")
		return
	}
	var rotated, compressed, pruned int
	for _, a := range res.Actions {
		switch a.Kind {
		case "rotate":
			rotated++
		case "compress":
			compressed++
		case "prune":
			pruned++
		}
		fmt.Printf("  %-9s %s  (%s)\n", a.Kind, a.Path, a.Note)
	}
	fmt.Printf("summary: %d rotated, %d compressed, %d pruned\n", rotated, compressed, pruned)
}

// parseSize parses a human-friendly byte size such as "100MB", "1GB", "512KB",
// or a bare byte count. Units are powers of 1024 and case-insensitive.
func parseSize(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty --max-size")
	}
	units := []struct {
		suffix string
		mult   int64
	}{
		{"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10},
		{"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10}, {"B", 1},
	}
	upper := s
	for i := 0; i < len(upper); i++ {
		if upper[i] >= 'a' && upper[i] <= 'z' {
			upper = upper[:i] + string(upper[i]-32) + upper[i+1:]
		}
	}
	for _, u := range units {
		if len(upper) > len(u.suffix) && hasSuffix(upper, u.suffix) {
			numStr := upper[:len(upper)-len(u.suffix)]
			var num float64
			if _, err := fmt.Sscanf(numStr, "%g", &num); err != nil {
				return 0, fmt.Errorf("invalid --max-size %q: %w", s, err)
			}
			if num < 0 {
				return 0, fmt.Errorf("invalid --max-size %q: negative", s)
			}
			return int64(num * float64(u.mult)), nil
		}
	}
	var num int64
	if _, err := fmt.Sscanf(upper, "%d", &num); err != nil {
		return 0, fmt.Errorf("invalid --max-size %q (use e.g. 100MB, 1GB, or a byte count): %w", s, err)
	}
	return num, nil
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
