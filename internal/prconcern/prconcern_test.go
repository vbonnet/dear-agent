package prconcern_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/prconcern"
)

// numstat builds a -z numstat stream. Ordinary records carry their path in the
// third tab field; rename records leave it empty and follow with old and new.
func numstat(records ...string) string { return strings.Join(records, "") }

func plain(added, deleted int, p string) string {
	return itoa(added) + "\t" + itoa(deleted) + "\t" + p + "\x00"
}

func rename(added, deleted int, old, dst string) string {
	return itoa(added) + "\t" + itoa(deleted) + "\t\x00" + old + "\x00" + dst + "\x00"
}

// itoa renders a numstat count, using git's "-" for a binary file.
func itoa(n int) string {
	if n < 0 {
		return "-"
	}
	return strconv.Itoa(n)
}

func TestParseNumstatZHandlesRenamesAndPlainRecords(t *testing.T) {
	in := numstat(
		plain(12, 3, "pkg/a/thing.go"),
		rename(0, 0, "agm/old.go", "pkg/new.go"),
		plain(-1, -1, "docs/diagram.png"),
	)
	got, err := prconcern.ParseNumstatZ(in)
	if err != nil {
		t.Fatalf("ParseNumstatZ: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d changes, want 3: %+v", len(got), got)
	}
	if got[0].Path != "pkg/a/thing.go" || got[0].Added != 12 || got[0].OldPath != "" {
		t.Errorf("plain record = %+v", got[0])
	}
	if got[1].OldPath != "agm/old.go" || got[1].Path != "pkg/new.go" {
		t.Errorf("rename record = %+v, want old/new pairing", got[1])
	}
	if !got[1].IsMove() {
		t.Error("a 0/0 rename must count as a move")
	}
	if got[2].Added != -1 {
		t.Errorf("binary record = %+v, want -1 counts", got[2])
	}
}

// Paths with non-ASCII bytes are exactly why the parser reads -z output.
func TestParseNumstatZHandlesNonASCIIPaths(t *testing.T) {
	got, err := prconcern.ParseNumstatZ(plain(3, 1, "docs/café.md"))
	if err != nil {
		t.Fatalf("ParseNumstatZ: %v", err)
	}
	if len(got) != 1 || got[0].Path != "docs/café.md" {
		t.Errorf("got %+v, want the path verbatim", got)
	}
}

func TestParseNumstatZRejectsMalformedInput(t *testing.T) {
	for _, bad := range []string{
		"12\tnope\tpkg/a.go\x00",
		"onlyonefield\x00",
		"0\t0\t\x00only-old-path\x00",
	} {
		if _, err := prconcern.ParseNumstatZ(bad); err == nil {
			t.Errorf("ParseNumstatZ(%q) = nil error, want a parse failure", bad)
		}
	}
}

func TestAnalyzeFlagsRefactorMixedWithNewLogic(t *testing.T) {
	changes, err := prconcern.ParseNumstatZ(numstat(
		rename(0, 0, "agm/session.go", "pkg/session/session.go"),
		rename(0, 0, "agm/session_util.go", "pkg/session/util.go"),
		plain(180, 0, "pkg/session/feature.go"),
	))
	if err != nil {
		t.Fatal(err)
	}
	got := prconcern.Analyze(changes, 0)
	if !got.Mixed {
		t.Fatalf("Analyze = not mixed, want mixed: %+v", got)
	}
	if len(got.MoveOnly) != 2 {
		t.Errorf("MoveOnly = %v, want 2 entries", got.MoveOnly)
	}
	if got.NewLogicLines != 180 {
		t.Errorf("NewLogicLines = %d, want 180", got.NewLogicLines)
	}
	reason := got.Reason()
	for _, want := range []string{"mixes a mechanical refactor", "Land the move on its own first", "pkg/session/feature.go"} {
		if !strings.Contains(reason, want) {
			t.Errorf("Reason() missing %q:\n%s", want, reason)
		}
	}
}

// The whole design rests on this: a rename drags import fix-ups along, and
// those must never look like a feature.
func TestAnalyzeStaysQuietForAPureRenameWithCallSiteFixups(t *testing.T) {
	changes, err := prconcern.ParseNumstatZ(numstat(
		rename(0, 0, "agm/session.go", "pkg/session/session.go"),
		plain(1, 1, "cmd/agm/main.go"),
		plain(1, 1, "cmd/vroom/main.go"),
		plain(2, 2, "internal/ops/run.go"),
	))
	if err != nil {
		t.Fatal(err)
	}
	if got := prconcern.Analyze(changes, 0); got.Mixed {
		t.Errorf("a rename with import fix-ups must not be flagged: %+v", got)
	}
}

func TestAnalyzeStaysQuietForSingleConcernDiffs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		records string
	}{
		{"moves only", numstat(rename(0, 0, "a/x.go", "b/x.go"), rename(0, 0, "a/y.go", "b/y.go"))},
		{"feature only", numstat(plain(400, 10, "pkg/new/feature.go"))},
		{"docs alongside a move", numstat(rename(0, 0, "a/x.go", "b/x.go"), plain(300, 0, "docs/guide.md"))},
		{"tests alongside a move", numstat(rename(0, 0, "a/x.go", "b/x.go"), plain(300, 0, "pkg/a/x_test.go"))},
		{"testdata alongside a move", numstat(rename(0, 0, "a/x.go", "b/x.go"), plain(900, 0, "pkg/a/testdata/big.go"))},
		{"moved and edited, no new files", numstat(rename(120, 40, "a/x.go", "b/x.go"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changes, err := prconcern.ParseNumstatZ(tc.records)
			if err != nil {
				t.Fatal(err)
			}
			got := prconcern.Analyze(changes, 0)
			if got.Mixed {
				t.Errorf("Analyze flagged a single-concern diff: %+v", got)
			}
			if got.Reason() != "" {
				t.Errorf("Reason() = %q, want empty", got.Reason())
			}
		})
	}
}

func TestAnalyzeRespectsThreshold(t *testing.T) {
	changes, err := prconcern.ParseNumstatZ(numstat(
		rename(0, 0, "a/x.go", "b/x.go"),
		plain(30, 0, "pkg/new.go"),
	))
	if err != nil {
		t.Fatal(err)
	}
	if prconcern.Analyze(changes, 0).Mixed {
		t.Error("30 added lines is below the default threshold; want quiet")
	}
	if !prconcern.Analyze(changes, 10).Mixed {
		t.Error("30 added lines is above a threshold of 10; want flagged")
	}
}

func TestAnalyzeCapsThePathPreview(t *testing.T) {
	var recs []string
	for i := range 12 {
		recs = append(recs, rename(0, 0, "a/f"+strconv.Itoa(i)+".go", "b/f"+strconv.Itoa(i)+".go"))
	}
	recs = append(recs, plain(200, 0, "pkg/new.go"))
	changes, err := prconcern.ParseNumstatZ(numstat(recs...))
	if err != nil {
		t.Fatal(err)
	}
	reason := prconcern.Analyze(changes, 0).Reason()
	if !strings.Contains(reason, "and 7 more") {
		t.Errorf("Reason() should cap the preview at 5 plus a count:\n%s", reason)
	}
}

func TestAnalyzeEmptyDiff(t *testing.T) {
	got := prconcern.Analyze(nil, 0)
	if got.Mixed || got.Reason() != "" {
		t.Errorf("empty diff = %+v, want quiet", got)
	}
}

// Renaming fixtures or docs alongside a feature is part of that feature, not a
// separable refactor. This is the rule that took the historical sweep from
// three findings to one true positive.
func TestAnalyzeIgnoresNonSourceMoves(t *testing.T) {
	changes, err := prconcern.ParseNumstatZ(numstat(
		rename(0, 0, "pkg/a/testdata/d2-valid.md", "pkg/a/testdata/research-valid.md"),
		rename(0, 0, ".github/ISSUE_TEMPLATE/bug_report.md", ".github/ISSUE_TEMPLATE/bug.md"),
		plain(600, 0, "pkg/a/feature.go"),
	))
	if err != nil {
		t.Fatal(err)
	}
	if got := prconcern.Analyze(changes, 0); got.Mixed {
		t.Errorf("fixture/doc renames must not count as a refactor: %+v", got)
	}
}

// Moving a package moves its tests with it; that is still code movement.
func TestAnalyzeCountsMovedTestCodeAsARefactor(t *testing.T) {
	changes, err := prconcern.ParseNumstatZ(numstat(
		rename(0, 0, "agm/session_test.go", "pkg/session/session_test.go"),
		plain(600, 0, "pkg/session/feature.go"),
	))
	if err != nil {
		t.Fatal(err)
	}
	if got := prconcern.Analyze(changes, 0); !got.Mixed {
		t.Errorf("a moved test file is code movement and should count: %+v", got)
	}
}
