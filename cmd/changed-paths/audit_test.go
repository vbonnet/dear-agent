package main

import (
	"strings"
	"testing"
)

func ok(outputs map[string]string) NeedResult {
	return NeedResult{Result: "success", Outputs: outputs}
}

func TestGatewayFailsOnUpstreamFailure(t *testing.T) {
	rep := Audit(AuditInput{EventName: "pull_request", Needs: map[string]NeedResult{
		"changes":                  ok(map[string]string{"go": "true", "agm": "false", "engram": "false"}),
		"ci":                       {Result: "failure"},
		"integration-tests":        {Result: "success"},
		"agm-codex-contracts":      {Result: "skipped"},
		"engram-storage-hardening": {Result: "skipped"},
	}})
	if !rep.Failed() {
		t.Fatal("a failed upstream job must fail the gateway")
	}
	if !strings.Contains(strings.Join(rep.Failures, ","), "ci -> failure") {
		t.Fatalf("failure not reported: %v", rep.Failures)
	}
}

func TestGatewayFailsWhenARelevantJobWasSkipped(t *testing.T) {
	rep := Audit(AuditInput{EventName: "pull_request", Needs: map[string]NeedResult{
		"changes":                  ok(map[string]string{"go": "true", "agm": "true", "engram": "false"}),
		"ci":                       {Result: "success"},
		"integration-tests":        {Result: "success"},
		"agm-codex-contracts":      {Result: "skipped"},
		"engram-storage-hardening": {Result: "skipped"},
	}})
	if !rep.Failed() {
		t.Fatal("a skipped but relevant job must fail the gateway")
	}
	if !strings.Contains(strings.Join(rep.Violations, ","), "agm-codex-contracts") {
		t.Fatalf("violation not reported: %v", rep.Violations)
	}
}

func TestGatewayPassesOnLegitimateSkips(t *testing.T) {
	rep := Audit(AuditInput{EventName: "pull_request", Needs: map[string]NeedResult{
		"changes":                  ok(map[string]string{"go": "false", "agm": "false", "engram": "false"}),
		"ci":                       {Result: "success"},
		"integration-tests":        {Result: "skipped"},
		"agm-codex-contracts":      {Result: "skipped"},
		"engram-storage-hardening": {Result: "skipped"},
	}})
	if rep.Failed() {
		t.Fatalf("docs-only PR must pass: failures=%v violations=%v", rep.Failures, rep.Violations)
	}
}

// A detector that fails publishes no outputs. Reading the resulting empty
// strings as "nothing was relevant" would let a checkout or runner failure
// disable every scoped gate at once — the exact shape where a required check
// goes absent instead of red.
func TestFailedDetectorMeansEverythingWasRelevant(t *testing.T) {
	rep := Audit(AuditInput{EventName: "pull_request", Needs: map[string]NeedResult{
		"changes":                  {Result: "failure"},
		"ci":                       {Result: "success"},
		"integration-tests":        {Result: "skipped"},
		"agm-codex-contracts":      {Result: "skipped"},
		"engram-storage-hardening": {Result: "skipped"},
	}})
	if !rep.Failed() {
		t.Fatal("a failed detector with skipped consumers must fail the gateway")
	}
	joined := strings.Join(rep.Violations, ",")
	for _, job := range []string{"integration-tests", "agm-codex-contracts", "engram-storage-hardening"} {
		if !strings.Contains(joined, job) {
			t.Fatalf("expected %s to be flagged, got %v", job, rep.Violations)
		}
	}
}

// `ci` is never allowed to be skipped: it produces the required matrix
// contexts, so a skip makes them unreportable rather than green.
func TestGatewayFlagsSkippedBuildAndTest(t *testing.T) {
	rep := Audit(AuditInput{EventName: "pull_request", Needs: map[string]NeedResult{
		"changes":                  ok(map[string]string{"go": "false", "agm": "false", "engram": "false"}),
		"ci":                       {Result: "skipped"},
		"integration-tests":        {Result: "skipped"},
		"agm-codex-contracts":      {Result: "skipped"},
		"engram-storage-hardening": {Result: "skipped"},
	}})
	if !rep.Failed() {
		t.Fatal("Build & Test must never be skipped — it owns two required contexts")
	}
}

func TestGatewayFlagsAJobMissingFromNeeds(t *testing.T) {
	rep := Audit(AuditInput{EventName: "pull_request", Needs: map[string]NeedResult{
		"changes": ok(map[string]string{"go": "true", "agm": "true", "engram": "true"}),
		"ci":      {Result: "success"},
	}})
	if !rep.Failed() {
		t.Fatal("a job dropped from the gateway's needs must be flagged")
	}
}

func TestParseNeedsEmpty(t *testing.T) {
	got, err := ParseNeeds("")
	if err != nil || len(got) != 0 {
		t.Fatalf("empty needs: got %v err %v", got, err)
	}
	if _, err := ParseNeeds("{not json"); err == nil {
		t.Fatal("malformed needs JSON must error")
	}
}
