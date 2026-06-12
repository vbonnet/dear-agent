// dead-links scans Markdown files in the repository for broken relative links.
// HTTP/HTTPS links are excluded — they change too frequently and have network
// dependencies that make CI checks unreliable. Only file-system links
// (./path, ../path, anchored paths without a scheme) are checked.
//
// Usage:
//
//	dead-links [--root <dir>] [--verbose]
//
// Exit codes: 0=clean, 1=broken links found, 2=usage error.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// linkRe matches Markdown links [text](target) and image links ![alt](target).
// Captures the link target in group 1.
var linkRe = regexp.MustCompile(`!?\[(?:[^\]]*)\]\(([^)]+)\)`)

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

	var findings []finding

	for _, mdFile := range mdFiles {
		bk, err := checkFile(mdFile, dir, *verbose)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
		findings = append(findings, bk...)
	}

	if len(findings) == 0 {
		fmt.Printf("ok: %d markdown file(s) checked — no broken relative links\n", len(mdFiles))
		return 0
	}

	fmt.Printf("dead-links: %d broken relative link(s) found\n\n", len(findings))
	for _, f := range findings {
		fmt.Printf("  %s:%d: broken link → %s\n", f.file, f.line, f.target)
	}
	return 1
}

func checkFile(mdFile, root string, verbose bool) ([]finding, error) {
	// Read the whole file rather than scanning line-by-line: bufio.Scanner
	// has a 64KB token limit and would silently stop on long lines (large
	// tables, embedded SVG, base64 images).
	content, err := os.ReadFile(mdFile)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", mdFile, err)
	}

	dir := filepath.Dir(mdFile)
	rel, _ := filepath.Rel(root, mdFile)

	var findings []finding

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		lineNum := i + 1
		matches := linkRe.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			target := m[1]
			if isExternalLink(target) {
				continue
			}
			// Strip anchor fragment (#section) from file path.
			path, _, _ := strings.Cut(target, "#")
			if path == "" {
				continue // pure anchor reference is always valid
			}
			// A leading "/" is a repository-root-relative link on GitHub
			// and similar platforms; resolve it against root, not the
			// file's own directory.
			var abs string
			if strings.HasPrefix(path, "/") {
				abs = filepath.Join(root, path)
			} else {
				abs = filepath.Join(dir, path)
			}
			abs = filepath.Clean(abs)
			if verbose {
				fmt.Printf("  check: %s:%d: %s\n", rel, lineNum, path)
			}
			if _, err := os.Stat(abs); os.IsNotExist(err) {
				findings = append(findings, finding{file: rel, line: lineNum, target: target})
			}
		}
	}
	return findings, nil
}

// isExternalLink reports whether target is an HTTP/HTTPS/mailto/other
// schemed URL. These are not checked — only relative file paths are.
func isExternalLink(target string) bool {
	lower := strings.ToLower(target)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "ftp://") ||
		strings.HasPrefix(lower, "//")
}

// findMarkdown returns a sorted list of all *.md files under root,
// excluding hidden directories and common generated-content paths.
func findMarkdown(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable entries; WalkDir reports them as warnings
		}
		name := d.Name()
		if d.IsDir() {
			// Skip hidden dirs, vendor, node_modules, .git — but never the
			// root itself, whose base name may legitimately be "." (e.g.
			// when invoked with --root .), which would otherwise abort the
			// entire walk and scan zero files.
			if path != root && (strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(strings.ToLower(name), ".md") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
