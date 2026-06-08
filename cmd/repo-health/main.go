// Command repo-health audits the dear-agent repository and reports on its
// code quality, architecture, agent-specific hygiene, and configuration
// drift. It is meant to run both locally (`make health-check`) and as a
// scheduled CI job (.github/workflows/health-check.yml).
//
// It emits a human-readable markdown summary (stdout by default) and,
// optionally, a JSON report and/or markdown file for CI artifacts. The
// process exit code encodes the verdict: 0 healthy, 1 degraded, 2 critical.
//
// Usage:
//
//	repo-health                          # markdown summary to stdout
//	repo-health --json                   # JSON report to stdout
//	repo-health --coverage               # also run the test suite for coverage
//	repo-health --json-out report.json --md-out report.md
//	repo-health --root /path/to/repo --exit-zero
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	os.Exit(cli(os.Args[1:]))
}

func cli(argv []string) int {
	fs := flag.NewFlagSet("repo-health", flag.ContinueOnError)
	var (
		root     = fs.String("root", "", "repo root to scan (default: git toplevel of cwd)")
		asJSON   = fs.Bool("json", false, "print the JSON report to stdout instead of markdown")
		jsonOut  = fs.String("json-out", "", "also write the JSON report to this file")
		mdOut    = fs.String("md-out", "", "also write the markdown summary to this file")
		coverage = fs.Bool("coverage", false, "run the test suite to measure coverage (slow)")
		exitZero = fs.Bool("exit-zero", false, "always exit 0 (report only, don't gate)")
	)
	if err := fs.Parse(argv); err != nil {
		return 2
	}

	repoRoot, err := resolveRoot(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "repo-health: %v\n", err)
		return 2
	}
	module := resolveModule(repoRoot)

	opts := defaultOptions(repoRoot, module)
	opts.coverage = *coverage

	report := scan(opts, time.Now())

	if *jsonOut != "" {
		if err := writeJSON(*jsonOut, report); err != nil {
			fmt.Fprintf(os.Stderr, "repo-health: writing %s: %v\n", *jsonOut, err)
			return 2
		}
	}
	if *mdOut != "" {
		if err := os.WriteFile(*mdOut, []byte(renderMarkdown(report)), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "repo-health: writing %s: %v\n", *mdOut, err)
			return 2
		}
	}

	if *asJSON {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Print(renderMarkdown(report))
	}

	if *exitZero {
		return 0
	}
	return report.Status.ExitCode()
}

// resolveRoot returns the absolute repo root: the flag if given, otherwise
// the git toplevel of the current directory.
func resolveRoot(flagRoot string) (string, error) {
	if flagRoot != "" {
		abs, err := filepath.Abs(flagRoot)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not in a git repo and --root not set: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveModule returns the Go module path, falling back to parsing go.mod
// directly if `go list -m` is unavailable.
func resolveModule(root string) string {
	cmd := exec.Command("go", "list", "-m")
	cmd.Dir = root
	if out, err := cmd.Output(); err == nil {
		if m := strings.TrimSpace(string(out)); m != "" {
			return m
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func writeJSON(path string, r Report) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
