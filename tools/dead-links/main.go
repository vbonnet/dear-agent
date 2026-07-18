// dead-links scans tracked Markdown for broken local paths and anchors.
// Schemed links are excluded because network-dependent validation is not a
// deterministic CI contract. A shrink-only baseline can ratchet existing debt.
//
// Usage:
//
//	dead-links [--root <dir>] [--baseline <file>] [--verbose]
//
// Exit codes: 0=clean, 1=integrity findings, 2=usage or operational error.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// finding is a broken-link report entry.
type finding struct {
	file   string
	line   int
	target string
}

func main() {
	os.Exit(run())
}

func run() int {
	root := flag.String("root", "", "repo root to scan (default: git toplevel of cwd)")
	baselinePath := flag.String("baseline", "", "shrink-only baseline of source<TAB>target findings")
	verbose := flag.Bool("verbose", false, "print each checked link")
	flag.Parse()

	dir := *root
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "error: %q is not a directory\n", dir)
		return 2
	}

	mdFiles, err := findMarkdown(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error scanning markdown files: %v\n", err)
		return 2
	}

	checker := newLinkChecker(dir, *verbose)
	var findings []finding

	for _, mdFile := range mdFiles {
		bk, err := checker.checkFile(mdFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error checking markdown: %v\n", err)
			return 2
		}
		findings = append(findings, bk...)
	}

	var stale []string
	baselined := 0
	if *baselinePath != "" {
		path := *baselinePath
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		baseline, err := loadBaseline(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading baseline: %v\n", err)
			return 2
		}
		findings, stale, baselined = applyBaseline(findings, baseline)
	}

	if len(findings) == 0 && len(stale) == 0 {
		fmt.Printf("ok: %d tracked markdown file(s) checked — no new broken local links", len(mdFiles))
		if baselined > 0 {
			fmt.Printf(" (%d baselined relationship(s))", baselined)
		}
		fmt.Println()
		return 0
	}

	fmt.Printf("dead-links: %d new broken local link(s), %d stale baseline entries\n\n", len(findings), len(stale))
	for _, f := range findings {
		fmt.Printf("  %s:%d: broken link → %s\n", f.file, f.line, f.target)
	}
	for _, entry := range stale {
		fmt.Printf("  stale baseline → %s\n", entry)
	}
	return 1
}

// checkFile preserves the small test-facing helper while the command reuses a
// checker across files so parsed target documents are cached.
func checkFile(mdFile, root string, verbose bool) ([]finding, error) {
	return newLinkChecker(root, verbose).checkFile(mdFile)
}
