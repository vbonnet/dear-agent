package main

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// skipDirs are never walked when discovering //go:embed directives.
var skipDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true, "testdata": true,
}

// DiscoverEmbedRoots walks the checkout and returns every repo-relative file or
// directory reachable from a //go:embed directive.
//
// Discovering these instead of hard-coding an extension list is the point: a
// `.sql` schema, a `.yaml` contract, or a Markdown skill compiled into a binary
// changes the built program even though no `.go` file changed. A hand-written
// list of "Go-relevant" extensions drifts out of the tree the first time
// someone embeds a new kind of asset, and the failure mode is a skipped
// required check, not a red one.
//
// Errors are non-fatal: the caller treats an empty result as "discovered
// nothing", and the document-denylist polarity in classify.go still errs
// toward running.
func DiscoverEmbedRoots(repo string) ([]string, error) {
	seen := map[string]bool{}
	err := filepath.WalkDir(repo, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable subtree must not break detection
		}
		if d.IsDir() {
			if p != repo && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(p) != ".go" {
			return nil
		}
		for _, pattern := range embedPatterns(p) {
			for _, root := range resolvePattern(repo, filepath.Dir(p), pattern) {
				seen[root] = true
			}
		}
		return nil
	})
	roots := make([]string, 0, len(seen))
	for r := range seen {
		roots = append(roots, r)
	}
	sort.Strings(roots)
	return roots, err
}

// embedPatterns extracts the patterns from every //go:embed directive in a Go
// source file.
func embedPatterns(file string) []string {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "//go:embed") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "//go:embed"))
		if rest == "" || rest == line {
			continue
		}
		out = append(out, splitEmbedArgs(rest)...)
	}
	return out
}

// splitEmbedArgs splits a directive's arguments, honouring the double-quoted
// and backquoted forms the go command accepts.
func splitEmbedArgs(rest string) []string {
	var args []string
	for rest != "" {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			break
		}
		switch rest[0] {
		case '"':
			if end := strings.Index(rest[1:], `"`); end >= 0 {
				if v, err := strconv.Unquote(rest[:end+2]); err == nil {
					args = append(args, v)
				}
				rest = rest[end+2:]
				continue
			}
			return args
		case '`':
			if end := strings.Index(rest[1:], "`"); end >= 0 {
				args = append(args, rest[1:end+1])
				rest = rest[end+2:]
				continue
			}
			return args
		}
		field := rest
		if i := strings.IndexAny(rest, " \t"); i >= 0 {
			field = rest[:i]
			rest = rest[i:]
		} else {
			rest = ""
		}
		args = append(args, field)
	}
	return args
}

// resolvePattern turns one embed pattern into repo-relative roots, expanding
// any glob against the checkout on disk. The `all:` prefix (which only changes
// whether dot- and underscore-prefixed files are included) is stripped: for
// change detection the containing directory is the unit either way.
func resolvePattern(repo, dir, pattern string) []string {
	pattern = strings.TrimPrefix(pattern, "all:")
	if pattern == "" || filepath.IsAbs(pattern) || strings.HasPrefix(pattern, "..") {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil || len(matches) == 0 {
		// A pattern that matches nothing on disk (or a malformed one) still
		// names something the build cares about; keep the literal join so a
		// deleted embedded file is not silently dropped from the taxonomy.
		matches = []string{filepath.Join(dir, pattern)}
	}
	var roots []string
	for _, m := range matches {
		rel, err := filepath.Rel(repo, m)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		roots = append(roots, filepath.ToSlash(rel))
	}
	return roots
}
