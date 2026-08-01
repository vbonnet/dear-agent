// Command ears-lint validates EARS-formatted requirements in SPEC.md files.
//
// Usage:
//
//	ears-lint [flags] [path ...]
//
// Each path may be a SPEC.md file or a directory (searched recursively for
// SPEC.md). With no paths it lints ./SPEC.md.
//
// Exit codes:
//
//	0  all linted files passed
//	1  one or more files failed (zero valid requirements, or — with --strict —
//	   any non-conforming requirement)
//	2  usage or I/O error
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/internal/repoinventory"
	"github.com/vbonnet/dear-agent/spec-governance/earslint"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ears-lint", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		strict     = fs.Bool("strict", false, "fail on any non-conforming requirement, not just zero-requirement files")
		jsonOut    = fs.Bool("json", false, "emit machine-readable JSON results")
		configPath = fs.String("config", "", "path to an EARS config file (.earslint.yml); defaults to built-in patterns")
		quiet      = fs.Bool("quiet", false, "suppress per-file pass messages")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: ears-lint [flags] [path ...]\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg := earslint.DefaultConfig()
	if *configPath != "" {
		loaded, err := earslint.LoadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(stderr, "ears-lint: %v\n", err)
			return 2
		}
		cfg = loaded
	}

	linter, err := earslint.New(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "ears-lint: %v\n", err)
		return 2
	}

	paths := fs.Args()
	if len(paths) == 0 {
		paths = []string{"SPEC.md"}
	}

	files, err := expandPaths(paths)
	if err != nil {
		fmt.Fprintf(stderr, "ears-lint: %v\n", err)
		return 2
	}
	if len(files) == 0 {
		fmt.Fprintf(stderr, "ears-lint: no SPEC.md files found in: %v\n", paths)
		return 2
	}

	var results []earslint.Result
	failed := false
	for _, f := range files {
		res, err := linter.LintFile(f)
		if err != nil {
			fmt.Fprintf(stderr, "ears-lint: %v\n", err)
			return 2
		}
		results = append(results, res)
		if res.Failed(*strict) {
			failed = true
		}
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			fmt.Fprintf(stderr, "ears-lint: %v\n", err)
			return 2
		}
	} else {
		printText(stdout, results, *strict, *quiet)
	}

	if failed {
		return 1
	}
	return 0
}

func printText(out io.Writer, results []earslint.Result, strict, quiet bool) {
	for _, res := range results {
		for _, f := range res.Findings {
			fmt.Fprintf(out, "%s\n", f.String())
			if f.Text != "" {
				fmt.Fprintf(out, "    > %s\n", f.Text)
			}
		}
		if res.Failed(strict) {
			fmt.Fprintf(out, "✗ %s: FAIL (%d/%d valid, %d non-conforming)\n",
				res.File, res.ValidRequirements, res.TotalRequirements, res.NonConforming())
		} else if !quiet {
			extra := ""
			if res.NonConforming() > 0 {
				extra = fmt.Sprintf(", %d non-conforming (reported)", res.NonConforming())
			}
			fmt.Fprintf(out, "✓ %s: %d valid requirements%s\n", res.File, res.ValidRequirements, extra)
		}
	}
}

// expandPaths turns the given paths (files or directories) into a deduplicated
// list of SPEC.md files. Directories are walked recursively.
func expandPaths(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if !seen[abs] {
			seen[abs] = true
			out = append(out, p)
		}
	}

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(p)
			continue
		}
		inventory, err := repoinventory.Scan(p)
		if err != nil {
			return nil, err
		}
		for _, file := range inventory {
			if file.Name() == "SPEC.md" {
				add(file.Absolute)
			}
		}
	}
	return out, nil
}
