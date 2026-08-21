// Package prconcern detects pull requests that mix a mechanical refactor with
// net-new logic, so the deterministic size-and-scope signal can ask for a split
// on shape rather than only on size.
//
// # The gap this closes
//
// .github/workflows/pr-size-scope.yml already posts a split suggestion, but
// only once a PR crosses a raw size threshold (1,000 changed lines, 50 changed
// files, 4 top-level areas). A 300-line PR that renames a package AND adds a
// feature on top of the new names is well under every one of those thresholds
// and sails through — yet it is exactly the shape CONTRIBUTING.md's
// "Small, stacked PRs" section tells authors to split, because the reviewer
// cannot tell a mechanical move from a behaviour change when both arrive in
// one diff.
//
// # The signal
//
// Two independent facts have to hold before this reports a mix:
//
//  1. a move-only record exists — a rename or copy of a source file whose
//     content did not change at all (git reports 0 added and 0 deleted lines
//     for it), and
//  2. substantial net-new logic exists elsewhere in the diff — added lines in
//     non-test source files that are not themselves part of a move.
//
// Requiring both is what keeps an ordinary rename quiet. Renaming a package
// means touching its call sites, but an import fix-up is one or two added
// lines per file; it cannot reach the net-new-logic threshold. A PR that only
// moves files reports nothing, and a PR that only adds a feature reports
// nothing. Only the genuine mix trips it.
package prconcern

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

// DefaultNewLogicLines is the number of added non-test source lines that must
// accompany a move-only record before the diff counts as mixed. It sits well
// above the import fix-ups a rename drags along and well below a real feature.
const DefaultNewLogicLines = 50

// Change is one file record from a diff, with its rename pairing and line
// counts already resolved.
type Change struct {
	// Path is the post-image path; for a rename it is the destination.
	Path string
	// OldPath is the pre-image path of a rename or copy, empty otherwise.
	OldPath string
	// Added and Deleted are the line counts. They are -1 for a binary file,
	// which git reports as "-" and which carries no reviewable line content.
	Added, Deleted int
}

// IsMove reports whether the record is a rename or copy of a SOURCE file that
// changed no content — the mechanical half of a mixed diff.
//
// The source restriction matters. Renaming test fixtures or documentation
// alongside a feature is part of that feature, not a separable refactor, so
// telling the author to "land the move first" would be wrong advice. Against
// 600 commits of this repository's history it is also the difference between
// one true positive and three findings, two of which were testdata renames.
// Moved test CODE still counts: relocating a package moves its tests with it.
func (c Change) IsMove() bool {
	return c.OldPath != "" && c.Added == 0 && c.Deleted == 0 && isSource(c.Path)
}

// Analysis is the verdict for one diff.
type Analysis struct {
	// MoveOnly lists the pure rename/copy records, as "old => new".
	MoveOnly []string
	// NewLogic lists the non-move, non-test source files carrying added lines.
	NewLogic []string
	// NewLogicLines is the total added line count across NewLogic.
	NewLogicLines int
	// Mixed reports whether both signals fired.
	Mixed bool
}

// Reason renders the split request for a review comment, or "" when the diff
// is not mixed.
func (a Analysis) Reason() string {
	if !a.Mixed {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- This PR mixes a mechanical refactor with net-new logic: %s, "+
		"alongside %d added line(s) of new logic in %s.",
		countNoun(len(a.MoveOnly), "file moved or renamed with no content change",
			"files moved or renamed with no content change"),
		a.NewLogicLines, countNoun(len(a.NewLogic), "other source file", "other source files"))
	b.WriteString("\n  Land the move on its own first, then build on the new names in a follow-up —")
	b.WriteString(" a reviewer cannot tell a mechanical move from a behaviour change when both arrive in one diff.")
	b.WriteString("\n  Moved: ")
	b.WriteString(strings.Join(preview(a.MoveOnly, 5), ", "))
	b.WriteString("\n  New logic: ")
	b.WriteString(strings.Join(preview(a.NewLogic, 5), ", "))
	return b.String()
}

func countNoun(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(n) + " " + plural
}

// preview caps a list for a comment and says how many were omitted, so the
// signal never pastes a hundred paths into a PR thread.
func preview(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	out := append([]string(nil), items[:limit]...)
	return append(out, fmt.Sprintf("and %d more", len(items)-limit))
}

// Analyze applies the two-signal rule to a parsed diff. newLogicLines <= 0
// selects DefaultNewLogicLines.
func Analyze(changes []Change, newLogicLines int) Analysis {
	if newLogicLines <= 0 {
		newLogicLines = DefaultNewLogicLines
	}
	var a Analysis
	for _, c := range changes {
		if c.IsMove() {
			a.MoveOnly = append(a.MoveOnly, c.OldPath+" => "+c.Path)
			continue
		}
		// A moved-and-edited file is still refactor work, not the net-new
		// logic this looks for, so it contributes to neither signal.
		if c.OldPath != "" {
			continue
		}
		if c.Added <= 0 || !isSource(c.Path) || isTest(c.Path) {
			continue
		}
		a.NewLogic = append(a.NewLogic, c.Path)
		a.NewLogicLines += c.Added
	}
	sort.Strings(a.MoveOnly)
	sort.Strings(a.NewLogic)
	a.Mixed = len(a.MoveOnly) > 0 && a.NewLogicLines >= newLogicLines
	return a
}

// sourceExts are the extensions whose added lines count as new logic. Markdown,
// fixtures, and generated data are excluded: a docs change that rides along
// with a rename is not the reviewability problem this detects.
var sourceExts = map[string]bool{
	".go": true, ".rs": true, ".ts": true, ".tsx": true,
	".js": true, ".jsx": true, ".sh": true, ".py": true,
}

func isSource(p string) bool { return sourceExts[strings.ToLower(path.Ext(p))] }

// isTest excludes test code from the new-logic signal. Tests legitimately grow
// alongside a refactor — renaming a package means its tests move and adapt with
// it — so counting them would flag ordinary, well-tested refactors.
func isTest(p string) bool {
	base := path.Base(p)
	switch {
	case strings.HasSuffix(base, "_test.go"),
		strings.HasSuffix(base, ".test.ts"), strings.HasSuffix(base, ".test.js"),
		strings.HasSuffix(base, ".spec.ts"), strings.HasSuffix(base, ".spec.js"),
		strings.HasPrefix(base, "test_"):
		return true
	}
	for seg := range strings.SplitSeq(path.Dir(p), "/") {
		if seg == "testdata" || seg == "tests" || seg == "__tests__" {
			return true
		}
	}
	return false
}

// ParseNumstatZ parses `git diff -M --numstat -z` output.
//
// In -z mode each ordinary record is "added\tdeleted\tpath\0". A rename or copy
// record instead ends its tab-separated field with an EMPTY path and then emits
// two more NUL-terminated fields: "added\tdeleted\t\0oldpath\0newpath\0". The
// two shapes have to be distinguished explicitly; treating every record as
// three tab fields silently mis-reads every rename, which are exactly the
// records this package exists to find.
//
// -z is also what makes non-ASCII paths safe: without it git quotes and
// backslash-escapes them, so `docs/café.md` would arrive as an escaped literal.
func ParseNumstatZ(out string) ([]Change, error) {
	fields := strings.Split(out, "\x00")
	var changes []Change
	for i := 0; i < len(fields); i++ {
		rec := fields[i]
		if strings.TrimSpace(rec) == "" {
			continue
		}
		parts := strings.SplitN(rec, "\t", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("malformed numstat record %q", rec)
		}
		added, err := parseCount(parts[0])
		if err != nil {
			return nil, fmt.Errorf("record %q: %w", rec, err)
		}
		deleted, err := parseCount(parts[1])
		if err != nil {
			return nil, fmt.Errorf("record %q: %w", rec, err)
		}
		c := Change{Added: added, Deleted: deleted}
		if parts[2] == "" {
			// Rename/copy: the next two NUL-separated fields are old and new.
			// Both must be present AND non-empty: splitting on NUL always
			// yields a trailing empty element, so a record truncated after its
			// old path still offers an in-range but empty destination.
			if i+2 >= len(fields) || fields[i+1] == "" || fields[i+2] == "" {
				return nil, fmt.Errorf("truncated rename record %q", rec)
			}
			c.OldPath, c.Path = fields[i+1], fields[i+2]
			i += 2
		} else {
			c.Path = parts[2]
		}
		changes = append(changes, c)
	}
	return changes, nil
}

// parseCount reads one numstat count. Git writes "-" for a binary file, which
// has no line content to review; -1 marks it so it counts toward neither
// signal.
func parseCount(s string) (int, error) {
	if s == "-" {
		return -1, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("unparseable line count %q: %w", s, err)
	}
	return n, nil
}
