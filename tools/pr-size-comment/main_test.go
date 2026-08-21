package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDecide pins the two conditions the workflow previously encoded as
// separate `if:` expressions on separate steps, including the new trigger:
// crapUnknown now updates the comment where it previously did neither.
func TestDecide(t *testing.T) {
	tests := []struct {
		name string
		in   inputs
		want action
	}{
		{name: "nothing tripped, crap succeeded clean", in: inputs{crapOutcome: "success"}, want: actionDelete},
		{name: "should comment", in: inputs{shouldComment: true, crapOutcome: "success"}, want: actionUpsert},
		{name: "mixed concern", in: inputs{mixedConcern: true, crapOutcome: "success"}, want: actionUpsert},
		{name: "crap flagged", in: inputs{crapFlagged: true, crapOutcome: "success"}, want: actionUpsert},
		{
			name: "crap unknown alone now updates instead of staying silent",
			in:   inputs{crapUnknown: true, crapOutcome: "success"},
			want: actionUpsert,
		},
		{
			name: "crap step failed outright: neither delete (not a clean success) nor update",
			in:   inputs{crapOutcome: "failure"},
			want: actionNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := decide(tc.in); got != tc.want {
				t.Errorf("decide(%+v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestNeedsCrapRecovery pins when a prior comment's code-health section must
// be recovered rather than trusting a blank CRAP_REPORT as "clean".
func TestNeedsCrapRecovery(t *testing.T) {
	tests := []struct {
		name string
		in   inputs
		want bool
	}{
		{name: "genuinely clean", in: inputs{crapOutcome: "success"}, want: false},
		{name: "flagged: report is fresh, no recovery needed", in: inputs{crapOutcome: "success", crapReport: "stuff"}, want: false},
		{name: "operational failure", in: inputs{crapOutcome: "failure"}, want: true},
		{name: "measured nothing this run", in: inputs{crapOutcome: "success", crapUnknown: true}, want: true},
		{
			name: "unknown but a report is somehow present: trust the fresh one",
			in:   inputs{crapOutcome: "success", crapUnknown: true, crapReport: "stuff"},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsCrapRecovery(tc.in); got != tc.want {
				t.Errorf("needsCrapRecovery(%+v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestExtractCrapSection pins recovering the exact prior section, and that an
// older comment predating the marker recovers nothing rather than the whole
// body.
func TestExtractCrapSection(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "marker present",
			body: "<!-- pr-size-scope -->\n## heading\n\n<!-- crap-section -->\nover-threshold stuff\n",
			want: "over-threshold stuff\n",
		},
		{name: "no marker at all", body: "<!-- pr-size-scope -->\n## heading\n\nno crap section here\n", want: ""},
		{name: "marker with nothing after it", body: "stuff\n<!-- crap-section -->\n", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractCrapSection(tc.body); got != tc.want {
				t.Errorf("extractCrapSection(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

// TestComposeBodyRecoversPriorSectionOnlyWhenNothingFresher pins the
// precedence a stale CRAP_REPORT must not violate: a fresh report always
// wins, a recovered prior section is next, and the unknown-diagnostic
// summary is the last resort when there is truly nothing else to say.
func TestComposeBodyRecoversPriorSectionOnlyWhenNothingFresher(t *testing.T) {
	fresh := inputs{crapReport: "fresh finding\n"}
	if got := composeBody(fresh, "stale finding\n"); !strings.Contains(got, "fresh finding") || strings.Contains(got, "stale finding") {
		t.Errorf("a fresh crap_report must win over a recovered section:\n%s", got)
	}

	// An operational failure (crapOutcome != success, crapUnknown false)
	// recovering a prior section must be labeled exactly like the
	// crapUnknown case below: without a label, a stale finding republished
	// after an unrelated tool failure is indistinguishable from a fresh one.
	recovered := inputs{}
	if got := composeBody(recovered, "stale finding\n"); !strings.Contains(got, "stale finding") {
		t.Errorf("with no fresh report, the recovered section must appear:\n%s", got)
	} else if !strings.Contains(got, "recovered") {
		t.Errorf("a recovered section must be labeled as not from this revision even after a plain operational failure:\n%s", got)
	}

	diagnosticOnly := inputs{crapUnknown: true, crapSummary: "not measured: nothing could be collected"}
	if got := composeBody(diagnosticOnly, ""); !strings.Contains(got, "not measured") {
		t.Errorf("with nothing fresh and nothing to recover, the unknown summary must appear:\n%s", got)
	}

	// A run that measured nothing (crapUnknown) but has a prior finding to
	// recover must show BOTH: the recovered section alone (the old
	// behavior) gives a reviewer no indication the current revision is
	// unmeasured, letting a stale result pass as current.
	unknownWithRecovery := inputs{crapUnknown: true, crapSummary: "not measured: nothing could be collected"}
	gotBoth := composeBody(unknownWithRecovery, "stale finding\n")
	if !strings.Contains(gotBoth, "not measured") {
		t.Errorf("crapUnknown with a recovered section must still show the current unknown status:\n%s", gotBoth)
	}
	if !strings.Contains(gotBoth, "stale finding") {
		t.Errorf("crapUnknown with a recovered section must still show the recovered finding:\n%s", gotBoth)
	}
	if !strings.Contains(gotBoth, "recovered") {
		t.Errorf("the recovered finding must be clearly labeled as not from this revision:\n%s", gotBoth)
	}

	nothingAtAll := inputs{}
	got := composeBody(nothingAtAll, "")
	if !strings.Contains(got, crapSectionMarker) {
		t.Errorf("the crap-section marker must always be present so a later run can find it:\n%s", got)
	}
}

// TestComposeBodyDoesNotNestRecoveredSectionsAcrossConsecutiveRecoveries
// guards against unbounded growth: two or more consecutive runs that each
// need to recover the prior section (e.g. back-to-back crapUnknown syncs)
// must keep showing the same single recovered finding, not wrap the
// previous recovery inside another one each time.
func TestComposeBodyDoesNotNestRecoveredSectionsAcrossConsecutiveRecoveries(t *testing.T) {
	unknown := inputs{crapUnknown: true, crapSummary: "not measured: nothing could be collected"}

	round1 := composeBody(unknown, "original finding\n")
	priorFromRound1 := extractCrapSection(round1)

	round2 := composeBody(unknown, priorFromRound1)
	priorFromRound2 := extractCrapSection(round2)

	round3 := composeBody(unknown, priorFromRound2)

	if n := strings.Count(round3, "<details>"); n != 1 {
		t.Errorf("round 3 has %d <details> wrappers, want exactly 1 (no nesting):\n%s", n, round3)
	}
	if n := strings.Count(round3, "original finding"); n != 1 {
		t.Errorf("round 3 shows the original finding %d times, want exactly 1:\n%s", n, round3)
	}
	if !strings.Contains(round3, "not measured") {
		t.Errorf("round 3 must still show this run's own unknown status:\n%s", round3)
	}
	if len(round3) > 2*len(round2) {
		t.Errorf("comment grew from %d to %d bytes across one more recovery round; nesting is back", len(round2), len(round3))
	}
}

// TestRunTreatsUnsetBooleanFlagsAsFalse covers a GitHub Actions step output
// that never got set — a skipped or crashed upstream step renders as an
// empty string in the expression that becomes this flag's value. A real
// fs.BoolVar would reject "-crap-flagged=" outright ("invalid boolean
// value"); this exercises the whole flag set at once and confirms parsing
// succeeds and lands on the no-op path (crapOutcome isn't "success", so this
// never reaches the network) rather than crashing the step.
func TestRunTreatsUnsetBooleanFlagsAsFalse(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run(t.Context(), []string{
		"-repo", "vbonnet/dear-agent", "-pr", "1",
		"-should-comment=", "-mixed-concern=", "-crap-flagged=", "-crap-unknown=",
		"-crap-outcome=",
	}, &stdout, &stderr)
	if got != 0 {
		t.Errorf("exit = %d, want 0 (unset flags must not crash the command); stderr=%q", got, stderr.String())
	}
}

// installFakeGh puts a `gh` script on PATH that always fails, so a test can
// exercise this command's real gh-calling paths without touching a live
// repo. Returns the exit status the fake script reports.
func installFakeGh(t *testing.T, exitCode int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-gh PATH shim is a POSIX shell script; this test only runs on non-Windows CI")
	}
	dir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nexit %d\n", exitCode)
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestUpsertCommentReturnsDistinctFailureWhenGhFails guards the exact gap
// codex flagged: every gh error here used to be logged and then this
// returned 0 regardless, so the workflow step reported success even though
// the comment was never actually updated. A fake gh that always fails must
// now surface as a non-zero, non-2 (flag/usage) exit code.
func TestUpsertCommentReturnsDistinctFailureWhenGhFails(t *testing.T) {
	installFakeGh(t, 1)
	var stderr bytes.Buffer
	got := upsertComment(t.Context(), "vbonnet/dear-agent", "1", inputs{shouldComment: true, crapOutcome: "success"}, &stderr)
	if got != exitFailedOperation {
		t.Errorf("upsertComment() = %d, want %d (exitFailedOperation); stderr=%q", got, exitFailedOperation, stderr.String())
	}
}

// TestDeleteStaleCommentsReturnsDistinctFailureWhenGhFails is
// TestUpsertCommentReturnsDistinctFailureWhenGhFails's counterpart for the
// delete path.
func TestDeleteStaleCommentsReturnsDistinctFailureWhenGhFails(t *testing.T) {
	installFakeGh(t, 1)
	var stderr bytes.Buffer
	got := deleteStaleComments(t.Context(), "vbonnet/dear-agent", "1", &stderr)
	if got != exitFailedOperation {
		t.Errorf("deleteStaleComments() = %d, want %d (exitFailedOperation); stderr=%q", got, exitFailedOperation, stderr.String())
	}
}
