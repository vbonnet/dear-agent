package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/retrolint"
)

type config struct {
	repoRoot         string
	retrosDir        string
	baselinePath     string
	ratchet          bool
	absenceLookback  time.Duration
	changedSince     string
	files            string
	timeout          time.Duration
	jsonOutput       bool
	generateBaseline string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	timeout := cfg.timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if cfg.generateBaseline != "" {
		return handleGenerateBaseline(ctx, cfg, stdout, stderr)
	}

	return executeLint(ctx, cfg, stdout, stderr)
}

func parseFlags(args []string, stderr io.Writer) (*config, error) {
	fs := flag.NewFlagSet("retro-lint", flag.ContinueOnError)
	fs.SetOutput(stderr)

	cfg := &config{}
	var lookbackStr string

	fs.StringVar(&cfg.repoRoot, "repo", ".", "target repository root containing declared artifacts")
	fs.StringVar(&cfg.retrosDir, "retros-dir", "retrospectives", "directory containing retrospective markdown files")
	fs.StringVar(&cfg.baselinePath, "baseline", "", "path to grandfathered baseline store (JSONL)")
	fs.BoolVar(&cfg.ratchet, "ratchet", false, "reject baseline entries with valid guards or removed files")
	fs.StringVar(&lookbackStr, "absence-lookback", "", "lookback window for absence-alarm mode (e.g. 7d, 168h)")
	fs.StringVar(&cfg.changedSince, "changed-since", "", "only evaluate retrospectives modified since git ref")
	fs.StringVar(&cfg.files, "files", "", "comma-separated list of explicit retrospective files to evaluate")
	fs.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "overall execution timeout")
	fs.BoolVar(&cfg.jsonOutput, "json", false, "emit structured JSON output to stdout")
	fs.StringVar(&cfg.generateBaseline, "generate-baseline", "", "generate grandfathered baseline JSONL for current retrospectives")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if lookbackStr != "" {
		dur, err := parseDuration(lookbackStr)
		if err != nil {
			return nil, fmt.Errorf("invalid -absence-lookback: %w", err)
		}
		cfg.absenceLookback = dur
	}

	if cfg.timeout < 0 {
		return nil, fmt.Errorf("timeout cannot be negative")
	}

	cfg.repoRoot = expandHome(cfg.repoRoot)
	cfg.retrosDir = expandHome(cfg.retrosDir)
	cfg.baselinePath = expandHome(cfg.baselinePath)

	return cfg, nil
}

func expandHome(path string) string {
	if path == "~" {
		if h, err := os.UserHomeDir(); err == nil {
			return h
		}
	} else if strings.HasPrefix(path, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, path[2:])
		}
	}
	return path
}

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, "d") {
		daysStr := lower[:len(lower)-1]
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, fmt.Errorf("invalid day duration %q", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func handleGenerateBaseline(ctx context.Context, cfg *config, stdout, stderr io.Writer) int {
	entries, err := retrolint.GenerateBaseline(ctx, cfg.repoRoot, cfg.retrosDir)
	if err != nil {
		fmt.Fprintf(stderr, "error generating baseline: %v\n", err)
		return 2
	}

	w := io.Writer(stdout)
	if cfg.generateBaseline != "-" {
		f, err := os.Create(filepath.Clean(cfg.generateBaseline))
		if err != nil {
			fmt.Fprintf(stderr, "error creating baseline file: %v\n", err)
			return 2
		}
		defer f.Close()
		w = f
	}

	if err := retrolint.WriteBaseline(w, entries); err != nil {
		fmt.Fprintf(stderr, "error writing baseline: %v\n", err)
		return 2
	}
	return 0
}

func executeLint(ctx context.Context, cfg *config, stdout, stderr io.Writer) int {
	opts := retrolint.Options{
		RepoRoot:        cfg.repoRoot,
		RetrosDir:       cfg.retrosDir,
		BaselinePath:    cfg.baselinePath,
		Ratchet:         cfg.ratchet,
		AbsenceLookback: cfg.absenceLookback,
	}

	if cfg.files != "" {
		for f := range strings.SplitSeq(cfg.files, ",") {
			trimmed := strings.TrimSpace(f)
			if trimmed != "" {
				opts.Files = append(opts.Files, trimmed)
			}
		}
	} else if cfg.changedSince != "" {
		changedFiles, err := getChangedRetros(ctx, cfg.retrosDir, cfg.changedSince)
		if err != nil {
			fmt.Fprintf(stderr, "error determining changed retrospectives: %v\n", err)
			return 2
		}
		opts.Files = changedFiles
	} else if _, err := os.Stat(filepath.Clean(cfg.retrosDir)); err != nil {
		fmt.Fprintf(stderr, "error accessing retrospectives directory %q: %v\n", cfg.retrosDir, err)
		return 2
	}

	report, err := retrolint.Run(ctx, opts)
	if err != nil {
		fmt.Fprintf(stderr, "lint failure: %v\n", err)
		return 2
	}

	if cfg.jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(stderr, "error encoding JSON output: %v\n", err)
			return 2
		}
	} else {
		renderTextReport(stdout, report)
	}

	if report.Status == retrolint.StatusPass || report.Status == retrolint.StatusPresent {
		return 0
	}
	return 1
}

func getChangedRetros(ctx context.Context, retrosDir, ref string) ([]string, error) {
	//nolint:gosec // git diff invocation with controlled arguments
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "--diff-filter=ACMR", ref, "--", retrosDir)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && strings.HasSuffix(trimmed, ".md") {
			files = append(files, trimmed)
		}
	}
	return files, nil
}

func renderTextReport(w io.Writer, rep *retrolint.Report) {
	fmt.Fprintf(w, "Retrospective Guard Lint: %s (evaluated: %d, passed: %d, waived: %d, failed: %d)\n",
		rep.Status, rep.Evaluated, rep.Passed, rep.Waived, rep.Failed)

	for _, ratErr := range rep.RatchetErrors {
		fmt.Fprintf(w, "  [RATCHET] %s\n", ratErr)
	}

	for _, res := range rep.Results {
		if res.Status == retrolint.StatusFail || res.Status == retrolint.StatusAbsent {
			fmt.Fprintf(w, "  [%s] %s\n", res.Status, res.Path)
			for _, errStr := range res.Errors {
				fmt.Fprintf(w, "    - %s\n", errStr)
			}
		}
	}
}
