package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// fakeQuery returns a canned queue so tests never touch the network.
func fakeQuery(prs []queuePR, err error) queryFunc {
	return func(context.Context, string, int) ([]queuePR, error) { return prs, err }
}

func redQueue(n int, check string) []queuePR {
	out := make([]queuePR, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, queuePR{Number: i, FailingChecks: []string{check}})
	}
	return out
}

func TestRunExitsHealthyOnOrdinaryChurn(t *testing.T) {
	// GHC-01: exit 0 when no check dominates.
	q := []queuePR{
		{Number: 1, FailingChecks: []string{"Shell lint"}},
		{Number: 2, FailingChecks: []string{"Bats (zsh)"}},
		{Number: 3},
		{Number: 4},
		{Number: 5},
		{Number: 6},
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"--json"}, &out, &errOut, fakeQuery(q, nil)); code != exitHealthy {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, exitHealthy, errOut.String())
	}
}

func TestRunExitsDegradedOnSystemicGateFailure(t *testing.T) {
	// GHC-02: the deadlock shape must exit 1 so the absence-alarm scheduler,
	// which escalates on any non-zero probe exit, actually pages.
	q := redQueue(19, "govulncheck")
	for i := 20; i <= 44; i++ {
		q = append(q, queuePR{Number: i})
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--json"}, &out, &errOut, fakeQuery(q, nil))
	if code != exitDegraded {
		t.Fatalf("exit = %d, want %d", code, exitDegraded)
	}

	var report Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, out.String())
	}
	if report.Status != "systemic" {
		t.Errorf("Status = %q, want systemic", report.Status)
	}
	if report.Dominant == nil || report.Dominant.Check != "govulncheck" {
		t.Fatalf("Dominant = %+v, want govulncheck", report.Dominant)
	}
	if report.Remediation == "" {
		t.Error("Remediation is empty; the report must name the likely fix")
	}
	if report.CheckedAt == "" {
		t.Error("CheckedAt is empty")
	}
}

func TestRunExitsDownWhenTheQueueCannotBeRead(t *testing.T) {
	// GHC-03: a failed query is exit 2 (down), never 0. Reporting health when
	// the probe could not look is the exact silent-monitor failure this tool
	// was built to end.
	var out, errOut bytes.Buffer
	code := run([]string{"--json"}, &out, &errOut, fakeQuery(nil, errors.New("gh: HTTP 401")))
	if code != exitDown {
		t.Fatalf("exit = %d, want %d", code, exitDown)
	}
	var report Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}
	if report.Status != "down" {
		t.Errorf("Status = %q, want down", report.Status)
	}
	if !strings.Contains(report.Error, "401") {
		t.Errorf("Error = %q, want it to carry the underlying cause", report.Error)
	}
}

func TestRunExitsDownOnAnEmptyQueue(t *testing.T) {
	// GHC-04: no evaluable PRs is exit 2, not 0. An empty queue during active
	// hours is itself suspicious, and it is never positive evidence of health.
	var out, errOut bytes.Buffer
	if code := run([]string{"--json"}, &out, &errOut, fakeQuery(nil, nil)); code != exitDown {
		t.Fatalf("exit = %d, want %d", code, exitDown)
	}
}

func TestRunRejectsBadFlagsWithUsageExit(t *testing.T) {
	// GHC-05: usage errors are exit 3, distinct from a real outage, so the
	// scheduler does not page a human for a typo in the pulse registry.
	for _, args := range [][]string{
		{"--nonsense"},
		{"--min-fraction", "0"},
		{"--min-fraction", "1.5"},
		{"--min-prs", "0"},
		{"--limit", "0"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut, fakeQuery(redQueue(19, "govulncheck"), nil)); code != exitUsage {
			t.Errorf("run(%v) = %d, want %d", args, code, exitUsage)
		}
	}
}

func TestRunHumanSummaryNamesCheckCountAndFix(t *testing.T) {
	// GHC-06: the default non-JSON output is what lands in a desktop
	// notification body, so it must carry the three things a responder needs
	// without opening anything: which check, how wide, and what to do.
	q := redQueue(19, "govulncheck")
	for i := 20; i <= 44; i++ {
		q = append(q, queuePR{Number: i})
	}
	var out, errOut bytes.Buffer
	run(nil, &out, &errOut, fakeQuery(q, nil))
	got := out.String()
	for _, want := range []string{"govulncheck", "19", "44", "go.mod"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q:\n%s", want, got)
		}
	}
}

func TestRunThresholdFlagsOverrideDefaults(t *testing.T) {
	// GHC-07: an operator can tighten the alarm without a rebuild.
	q := redQueue(19, "govulncheck")
	for i := 20; i <= 44; i++ {
		q = append(q, queuePR{Number: i})
	}
	var out, errOut bytes.Buffer
	// 43.2% is below a 0.90 threshold, so this must read healthy.
	if code := run([]string{"--min-fraction", "0.90"}, &out, &errOut, fakeQuery(q, nil)); code != exitHealthy {
		t.Fatalf("exit = %d, want %d with --min-fraction 0.90", code, exitHealthy)
	}
}

func TestRunCountsDraftsByDefault(t *testing.T) {
	// GHC-11: the live-data correction, pinned at the CLI boundary. A queue of
	// mostly-draft PRs sharing one red gate must exit degraded.
	q := redQueue(19, "govulncheck")
	for i := range q {
		q[i].Draft = true
	}
	for i := 20; i <= 42; i++ {
		q = append(q, queuePR{Number: i})
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"--json"}, &out, &errOut, fakeQuery(q, nil)); code != exitDegraded {
		t.Fatalf("exit = %d, want %d: drafts are counted by default", code, exitDegraded)
	}
	if code := run([]string{"--exclude-drafts"}, &out, &errOut, fakeQuery(q, nil)); code != exitHealthy {
		t.Errorf("exit = %d, want %d with --exclude-drafts", code, exitHealthy)
	}
}

func TestRunTreatsUnknownRollupsAsUnevaluated(t *testing.T) {
	// GHC-08: PRs whose checks have not reported are excluded rather than
	// counted as green, so a queue of pending PRs cannot mask an outage.
	q := redQueue(5, "govulncheck")
	for i := 6; i <= 30; i++ {
		q = append(q, queuePR{Number: i, ChecksUnknown: true})
	}
	var out, errOut bytes.Buffer
	code := run([]string{"--json"}, &out, &errOut, fakeQuery(q, nil))
	if code != exitDegraded {
		t.Fatalf("exit = %d, want %d: 5 of 5 evaluated is systemic", code, exitDegraded)
	}
	var report Report
	_ = json.Unmarshal(out.Bytes(), &report)
	if report.EvaluatedPRs != 5 {
		t.Errorf("EvaluatedPRs = %d, want 5", report.EvaluatedPRs)
	}
	if report.SkippedPRs != 25 {
		t.Errorf("SkippedPRs = %d, want 25", report.SkippedPRs)
	}
}

func TestParseRollupReadsRealGitHubShapes(t *testing.T) {
	// GHC-09: the parser is the only untestable-by-unit part of the live path,
	// so it is pinned against the shapes GitHub actually returns: a CheckRun
	// with name/conclusion, a legacy StatusContext with context/state, a
	// missing rollup, and a draft.
	raw := []byte(`{"data":{"repository":{"pullRequests":{"nodes":[
      {"number":1429,"isDraft":false,"commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
        {"__typename":"CheckRun","name":"govulncheck","conclusion":"FAILURE"},
        {"__typename":"CheckRun","name":"Build & Test (ubuntu-latest)","conclusion":"SUCCESS"},
        {"__typename":"StatusContext","context":"legacy/status","state":"ERROR"}
      ]}}}}]}},
      {"number":1430,"isDraft":true,"commits":{"nodes":[{"commit":{"statusCheckRollup":null}}]}},
      {"number":1431,"isDraft":false,"commits":{"nodes":[]}}
    ]}}}}`)

	prs, err := parseRollup(raw)
	if err != nil {
		t.Fatalf("parseRollup: %v", err)
	}
	if len(prs) != 3 {
		t.Fatalf("len = %d, want 3", len(prs))
	}

	if got := prs[0].FailingChecks; len(got) != 2 || got[0] != "govulncheck" || got[1] != "legacy/status" {
		t.Errorf("PR 1429 FailingChecks = %v, want [govulncheck legacy/status]", got)
	}
	if prs[0].ChecksUnknown {
		t.Error("PR 1429 must not be marked unknown")
	}
	if !prs[1].ChecksUnknown || !prs[1].Draft {
		t.Errorf("PR 1430 = %+v, want draft with unknown checks", prs[1])
	}
	if !prs[2].ChecksUnknown {
		t.Errorf("PR 1431 = %+v, want unknown checks when no commit is returned", prs[2])
	}
}

func TestParseRollupRejectsGarbage(t *testing.T) {
	// GHC-11: a malformed response must error, so run() reports down instead of
	// silently treating an unparseable queue as an empty healthy one.
	if _, err := parseRollup([]byte("not json")); err == nil {
		t.Fatal("parseRollup(garbage) = nil error, want a decode failure")
	}
}

func TestParseRollupCountsEveryTerminalBlockingConclusion(t *testing.T) {
	// GHC-10: a required check that is CANCELLED, TIMED_OUT, ACTION_REQUIRED,
	// STARTUP_FAILURE or STALE blocks the merge exactly as FAILURE does, so a
	// detector that recognises only FAILURE/ERROR reports healthy while the
	// queue is wedged. The repository already classifies this way in
	// internal/safegit (requiredCheckStatus, parseFailingChecks); this pins the
	// same classification here. SUCCESS, NEUTRAL and SKIPPED pass; a CheckRun
	// still running reports a null conclusion and must count as neither.
	raw := []byte(`{"data":{"repository":{"pullRequests":{"nodes":[
      {"number":1,"isDraft":false,"commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
        {"__typename":"CheckRun","name":"cancelled-check","conclusion":"CANCELLED"},
        {"__typename":"CheckRun","name":"timed-out-check","conclusion":"TIMED_OUT"},
        {"__typename":"CheckRun","name":"action-required-check","conclusion":"ACTION_REQUIRED"},
        {"__typename":"CheckRun","name":"startup-failure-check","conclusion":"STARTUP_FAILURE"},
        {"__typename":"CheckRun","name":"stale-check","conclusion":"STALE"},
        {"__typename":"CheckRun","name":"failure-check","conclusion":"FAILURE"},
        {"__typename":"StatusContext","context":"legacy-error","state":"ERROR"},
        {"__typename":"CheckRun","name":"passing-check","conclusion":"SUCCESS"},
        {"__typename":"CheckRun","name":"neutral-check","conclusion":"NEUTRAL"},
        {"__typename":"CheckRun","name":"skipped-check","conclusion":"SKIPPED"},
        {"__typename":"CheckRun","name":"still-running-check","conclusion":null},
        {"__typename":"StatusContext","context":"legacy-pending","state":"PENDING"}
      ]}}}}]}}
    ]}}}}`)

	prs, err := parseRollup(raw)
	if err != nil {
		t.Fatalf("parseRollup: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("len = %d, want 1", len(prs))
	}

	want := []string{
		"cancelled-check", "timed-out-check", "action-required-check",
		"startup-failure-check", "stale-check", "failure-check", "legacy-error",
	}
	got := prs[0].FailingChecks
	if len(got) != len(want) {
		t.Fatalf("FailingChecks = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("FailingChecks[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseRollupTreatsAnUnrecognisedConclusionAsFailing(t *testing.T) {
	// GHC-12: fail closed. A conclusion GitHub adds after this code was written
	// must not be silently read as a passing check, because that is the exact
	// shape of the blindness this detector exists to end. Passing and pending
	// states are enumerated; everything else counts as blocking.
	raw := []byte(`{"data":{"repository":{"pullRequests":{"nodes":[
      {"number":1,"isDraft":false,"commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
        {"__typename":"CheckRun","name":"future-conclusion","conclusion":"SOME_NEW_TERMINAL_STATE"}
      ]}}}}]}}
    ]}}}}`)

	prs, err := parseRollup(raw)
	if err != nil {
		t.Fatalf("parseRollup: %v", err)
	}
	if got := prs[0].FailingChecks; len(got) != 1 || got[0] != "future-conclusion" {
		t.Errorf("FailingChecks = %v, want [future-conclusion]", got)
	}
}

func TestParseRollupTreatsAnAllPendingRollupAsUnevaluated(t *testing.T) {
	// GHC-13: a rollup whose contexts are all still running has reported no
	// terminal result, so it is not evidence of health. Counting it as an
	// evaluated passing PR lets an in-progress queue dilute the denominator and
	// mask the outage: five failing PRs beside 25 all-pending ones would read
	// 5/30 rather than 5/5. GHC-09 already excludes a missing rollup from the
	// denominator for exactly this reason; a rollup with no terminal context is
	// the same state wearing a different shape.
	raw := []byte(`{"data":{"repository":{"pullRequests":{"nodes":[
      {"number":1,"isDraft":false,"commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
        {"__typename":"CheckRun","name":"still-running","conclusion":null},
        {"__typename":"StatusContext","context":"legacy-pending","state":"PENDING"}
      ]}}}}]}},
      {"number":2,"isDraft":false,"commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
        {"__typename":"CheckRun","name":"done","conclusion":"SUCCESS"},
        {"__typename":"CheckRun","name":"still-running","conclusion":null}
      ]}}}}]}},
      {"number":3,"isDraft":false,"commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[]}}}}]}}
    ]}}}}`)

	prs, err := parseRollup(raw)
	if err != nil {
		t.Fatalf("parseRollup: %v", err)
	}
	if !prs[0].ChecksUnknown {
		t.Errorf("PR 1 (all contexts pending) = %+v, want ChecksUnknown", prs[0])
	}
	if prs[1].ChecksUnknown {
		t.Errorf("PR 2 has a terminal SUCCESS, so it is evaluated: %+v", prs[1])
	}
	if !prs[2].ChecksUnknown {
		t.Errorf("PR 3 (empty context list) = %+v, want ChecksUnknown", prs[2])
	}
}
