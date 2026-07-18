// ears-to-bdd converts EARS SPEC.md files into Gherkin BDD feature stubs.
// It is the second stage in the EARS → BDD → code pipeline:
//
//	SPEC.md → ears-lint (validate) → ears-to-bdd (generate) → .feature stubs
//
// Usage:
//
//	ears-to-bdd [flags] [paths...]
//
// Each path may be a SPEC.md file or a directory to search recursively for
// SPEC.md files. With no paths, the current directory is searched.
//
// Flags:
//
//	-out dir    Write .feature files into dir (default: print to stdout).
//	            Files are named <spec-dir-basename>.feature.
//	-dry-run    Print what would be written without writing files.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/internal/earsbdd"
	"github.com/vbonnet/dear-agent/internal/repoinventory"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs2 := flag.NewFlagSet("ears-to-bdd", flag.ContinueOnError)
	outDir := fs2.String("out", "", "write .feature files to this directory (default: stdout)")
	dryRun := fs2.Bool("dry-run", false, "print what would be written without writing")
	fs2.SetOutput(stderr)

	if err := fs2.Parse(args); err != nil {
		return 2
	}

	paths := fs2.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	specs, err := expandPaths(paths)
	if err != nil {
		fmt.Fprintf(stderr, "ears-to-bdd: %v\n", err)
		return 1
	}
	if len(specs) == 0 {
		fmt.Fprintln(stderr, "ears-to-bdd: no SPEC.md files found")
		return 1
	}

	exitCode := 0
	for _, spec := range specs {
		reqs, err := earsbdd.ExtractFile(spec)
		if err != nil {
			fmt.Fprintf(stderr, "ears-to-bdd: %v\n", err)
			exitCode = 1
			continue
		}

		ff := earsbdd.Generate(spec, reqs)
		if ff.Content == "" {
			fmt.Fprintf(stderr, "ears-to-bdd: %s: no EARS requirements found, skipping\n", spec)
			continue
		}

		if *outDir == "" && !*dryRun {
			// Stdout mode: print all features separated by a blank line.
			fmt.Fprintln(stdout, ff.Content)
			continue
		}

		outPath := featureOutPath(*outDir, spec)
		if *dryRun {
			fmt.Fprintf(stdout, "would write: %s (%d requirements)\n", outPath, len(reqs))
			continue
		}

		if err := writeFeature(outPath, ff.Content); err != nil {
			fmt.Fprintf(stderr, "ears-to-bdd: %v\n", err)
			exitCode = 1
		} else {
			fmt.Fprintf(stdout, "wrote: %s (%d requirements)\n", outPath, len(reqs))
		}
	}
	return exitCode
}

// expandPaths accepts a list of paths (files or directories) and returns all
// SPEC.md files found within them.
func expandPaths(paths []string) ([]string, error) {
	var specs []string
	seen := map[string]bool{}

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}

		if !info.IsDir() {
			abs, _ := filepath.Abs(p)
			if !seen[abs] {
				seen[abs] = true
				specs = append(specs, p)
			}
			continue
		}

		inventory, err := repoinventory.Scan(p)
		if err != nil {
			return nil, fmt.Errorf("inventory %s: %w", p, err)
		}
		for _, file := range inventory {
			if !strings.EqualFold(file.Name(), "SPEC.md") || seen[file.Absolute] {
				continue
			}
			seen[file.Absolute] = true
			specs = append(specs, file.Absolute)
		}
	}
	return specs, nil
}

// featureOutPath builds the output .feature path under outDir.
// "internal/fsguard/SPEC.md" → "<outDir>/fsguard.feature"
func featureOutPath(outDir, specPath string) string {
	dir := filepath.Dir(specPath)
	base := filepath.Base(dir)
	return filepath.Join(outDir, base+".feature")
}

// writeFeature writes content to path, creating parent directories as needed.
func writeFeature(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
