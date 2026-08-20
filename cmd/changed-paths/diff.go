package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// revisionRe bounds what may reach the git command line. The SHAs come from
// the workflow event payload, which is attacker-influenced on a fork PR, and a
// value like `--upload-pack=…` would otherwise be read as a flag.
var revisionRe = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)

// ChangedFiles returns every path touched between base and head, counting
// *both* sides of a rename or copy.
//
// `--name-only` reports a detected rename as its destination path alone. That
// loses the type and the scope of the vanished source: renaming `pkg/x/a.go`
// to `docs/a.md` would report only the Markdown path, classify as `go=false`,
// and skip build and analysis even though a compiled source file was deleted.
// `--name-status -z` keeps both sides, and NUL termination keeps paths with
// spaces or non-ASCII bytes intact (git would otherwise quote them).
func ChangedFiles(repo, base, head string) ([]string, error) {
	if !revisionRe.MatchString(base) || !revisionRe.MatchString(head) {
		return nil, fmt.Errorf("refusing non-SHA revisions: base=%q head=%q", base, head)
	}
	//nolint:gosec // G702: both revisions are validated against revisionRe above.
	cmd := exec.Command("git", "diff", "--name-status", "-M", "-C", "-z", base+"..."+head)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git diff %s...%s: %w: %s", base, head, err, strings.TrimSpace(string(out)))
	}
	return ParseNameStatusZ(string(out)), nil
}

// ParseNameStatusZ parses `git diff --name-status -z` output. Records are
// NUL-terminated; a rename or copy record is three fields (status, source,
// destination) instead of two.
func ParseNameStatusZ(out string) []string {
	fields := strings.Split(out, "\x00")
	seen := map[string]bool{}
	for i := 0; i < len(fields); i++ {
		status := fields[i]
		if status == "" {
			continue
		}
		want := 1
		if status[0] == 'R' || status[0] == 'C' {
			want = 2
		}
		for n := 0; n < want && i+1 < len(fields); n++ {
			i++
			if fields[i] != "" {
				seen[fields[i]] = true
			}
		}
	}
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	sort.Strings(files)
	return files
}
