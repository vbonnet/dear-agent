package main

import (
	"bytes"
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

	recovered := inputs{}
	if got := composeBody(recovered, "stale finding\n"); !strings.Contains(got, "stale finding") {
		t.Errorf("with no fresh report, the recovered section must appear:\n%s", got)
	}

	diagnosticOnly := inputs{crapUnknown: true, crapSummary: "not measured: nothing could be collected"}
	if got := composeBody(diagnosticOnly, ""); !strings.Contains(got, "not measured") {
		t.Errorf("with nothing fresh and nothing to recover, the unknown summary must appear:\n%s", got)
	}

	nothingAtAll := inputs{}
	got := composeBody(nothingAtAll, "")
	if !strings.Contains(got, crapSectionMarker) {
		t.Errorf("the crap-section marker must always be present so a later run can find it:\n%s", got)
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
