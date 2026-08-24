package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/craplens"
)

// TestRunRequiresRevisions pins the usage contract: a missing revision is the
// one thing that exits non-zero.
func TestRunRequiresRevisions(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no args", args: nil},
		{name: "base only", args: []string{"-base", "HEAD~1"}},
		{name: "head only", args: []string{"-head", "HEAD"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(t.Context(), tc.args, &stdout, &stderr); got != exitUsage {
				t.Errorf("exit = %d, want %d", got, exitUsage)
			}
			if !strings.Contains(stderr.String(), "-base") {
				t.Errorf("stderr should name the missing flags, got %q", stderr.String())
			}
		})
	}
}

// TestRunRejectsUnknownFlag pins the flag-error path.
func TestRunRejectsUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run(t.Context(), []string{"-nope"}, &stdout, &stderr); got != exitUsage {
		t.Errorf("exit = %d, want %d", got, exitUsage)
	}
}

// TestRunUnreadableDiffIsUsageNotFailure pins that an unusable revision pair
// exits with the usage status rather than pretending the diff was clean.
func TestRunUnreadableDiffIsUsageNotFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run(t.Context(), []string{"-base", "definitely-not-a-rev", "-head", "also-not-a-rev", "-repo", t.TempDir()}, &stdout, &stderr)
	if got != exitUsage {
		t.Errorf("exit = %d, want %d", got, exitUsage)
	}
	if stdout.Len() != 0 {
		t.Errorf("nothing should be printed to stdout on a usage failure, got %q", stdout.String())
	}
}

// TestWriteGitHubOutputCleanDiff pins the outputs the workflow reads when there
// is nothing to say. An empty crap_report is what keeps the comment step from
// posting an empty comment.
func TestWriteGitHubOutputCleanDiff(t *testing.T) {
	var out bytes.Buffer
	writeGitHubOutput(&out, cleanReport())

	got := out.String()
	if !strings.Contains(got, "crap_flagged=false") {
		t.Errorf("missing crap_flagged=false in %q", got)
	}
	if !strings.Contains(got, "crap_report=\n") {
		t.Errorf("missing empty crap_report in %q", got)
	}
	if strings.Contains(got, "crap_report<<") {
		t.Errorf("an empty crap_report should not open a heredoc block: %q", got)
	}
}

// TestWriteGitHubOutputHeredocIsTerminated pins that a flagged report is
// emitted as a properly opened and closed heredoc, which is what stops the
// multi-line body from corrupting every later output in GITHUB_OUTPUT.
func TestWriteGitHubOutputHeredocIsTerminated(t *testing.T) {
	var out bytes.Buffer
	writeGitHubOutput(&out, flaggedReport())

	got := out.String()
	if !strings.Contains(got, "crap_flagged=true") {
		t.Errorf("missing crap_flagged=true in %q", got)
	}
	const delim = "CRAP_LINT_REPORT_EOF"
	if strings.Count(got, delim) != 2 {
		t.Errorf("expected exactly one opening and one closing %s, got %d occurrences", delim, strings.Count(got, delim))
	}
	if !strings.Contains(got, "crap_report<<"+delim+"\n") {
		t.Errorf("heredoc was not opened correctly: %q", got)
	}
	if !strings.HasSuffix(got, delim+"\n") {
		t.Errorf("heredoc was not closed on the final line: %q", got)
	}
}

// TestUnflaggedSummaryDistinguishesUnmeasuredFromClean pins the honesty
// property the command and the skill both promise: a run that measured nothing
// must not read like a healthy diff. Reporting "clean" for an unusable
// measurement is the failure that would make this signal worth ignoring.
func TestUnflaggedSummaryDistinguishesUnmeasuredFromClean(t *testing.T) {
	tests := []struct {
		name      string
		report    craplens.Report
		wantHas   string
		wantNotIn string
	}{
		{
			name:      "checkout mismatch",
			report:    craplens.Report{CheckoutMismatch: true, Changed: 7},
			wantHas:   "not measured",
			wantNotIn: "clean",
		},
		{
			name:      "every package unmeasured",
			report:    craplens.Report{Changed: 4, Unknown: []string{"agm/internal/dolt"}},
			wantHas:   "not measured",
			wantNotIn: "clean",
		},
		{
			name:      "every changed function unmeasured",
			report:    craplens.Report{Changed: 2, Unmeasured: 2},
			wantHas:   "not measured",
			wantNotIn: "clean",
		},
		{
			name:      "genuinely clean",
			report:    craplens.Report{Scored: 5, WithinAgentTarget: 5, Threshold: craplens.DefaultThreshold},
			wantHas:   "clean",
			wantNotIn: "not measured",
		},
		{
			name: "clean but partially unmeasured still says so",
			report: craplens.Report{
				Scored: 5, WithinAgentTarget: 5, Threshold: craplens.DefaultThreshold,
				Unknown: []string{"agm/internal/tmux"},
			},
			wantHas:   "unmeasured",
			wantNotIn: "not measured",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := unflaggedSummary(tc.report)
			if !strings.Contains(got, tc.wantHas) {
				t.Errorf("summary = %q, want it to contain %q", got, tc.wantHas)
			}
			if strings.Contains(got, tc.wantNotIn) {
				t.Errorf("summary = %q, must not contain %q", got, tc.wantNotIn)
			}
		})
	}
}

// TestWriteGitHubOutputMarksUnmeasuredFunctionsUnknown pins that a diff with
// no over-threshold function and no unknown package, but a per-function
// coverage gap (a build-tagged file excluded on this runner, say), still
// reports crap_unknown=true. Without it the workflow's own "clean" and
// "delete the marker comment" gates would treat a partially unmeasured diff
// as a healthy one, the same failure unflaggedSummary already guards against
// in the prose form.
func TestWriteGitHubOutputMarksUnmeasuredFunctionsUnknown(t *testing.T) {
	var out bytes.Buffer
	report := craplens.Report{Threshold: craplens.DefaultThreshold, Scored: 3, WithinAgentTarget: 3, Unmeasured: 1}
	writeGitHubOutput(&out, report)

	got := out.String()
	if report.Flagged() {
		t.Fatal("test setup: report must not be flagged, or this isn't the silent-gap case")
	}
	if !strings.Contains(got, "crap_unknown=true") {
		t.Errorf("a report with unmeasured functions must report crap_unknown=true, got %q", got)
	}
}

// TestWriteGitHubOutputHasNoTrailingBlankLine pins that the heredoc body ends
// at the last content line, so the block closes cleanly.
func TestWriteGitHubOutputHasNoTrailingBlankLine(t *testing.T) {
	var out bytes.Buffer
	writeGitHubOutput(&out, flaggedReport())

	got := out.String()
	if strings.Contains(got, "\n\nCRAP_LINT_REPORT_EOF") {
		t.Errorf("heredoc has a blank line before its delimiter:\n%q", got)
	}
}

// TestWriteGitHubOutputCrapSummaryCannotInjectOutputs guards against the
// exact injection codex flagged: a legal Git path can itself contain a
// newline, and unflaggedSummary folds r.Unknown's package directories
// straight into crap_summary's prose. A plain "crap_summary=<value>\n"
// scalar line would let a crafted directory name like
// "p\ncrap_unknown=false\nx" terminate that line early and inject a forged
// crap_unknown assignment; the heredoc form must keep the whole value,
// injected newlines included, inside one delimited block.
func TestWriteGitHubOutputCrapSummaryCannotInjectOutputs(t *testing.T) {
	var out bytes.Buffer
	report := craplens.Report{
		Changed: 1,
		Unknown: []string{"p\ncrap_unknown=false\nx"},
	}
	writeGitHubOutput(&out, report)

	got := out.String()
	if !strings.Contains(got, "crap_summary<<") {
		t.Fatalf("crap_summary must use the heredoc form once its value can contain a newline:\n%q", got)
	}
	// GITHUB_OUTPUT parsing treats every line up to (but not including) the
	// bare closing delimiter as inert heredoc body, regardless of what it
	// looks like; only what follows the closing delimiter is parsed as
	// further output lines. Find that bare delimiter line specifically
	// (distinct from the "crap_summary<<DELIM" opening line) and confirm
	// nothing resembling the injected "crap_unknown=false" survived past it
	// as its own top-level assignment.
	const delim = "CRAP_LINT_SUMMARY_EOF"
	lines := strings.Split(got, "\n")
	closeIdx := -1
	for i, line := range lines {
		if line == delim {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		t.Fatalf("no bare closing delimiter line found:\n%q", got)
	}
	after := strings.Join(lines[closeIdx+1:], "\n")
	if strings.Contains(after, "crap_unknown=false") {
		t.Errorf("the injected package path's crap_unknown=false escaped the heredoc as a real output line: %q", after)
	}
}
