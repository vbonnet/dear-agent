package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/stackguard"
)

func TestReportExitsCleanWithoutFindings(t *testing.T) {
	var out strings.Builder
	if code := report(&out, 1404, nil, false); code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if !strings.Contains(out.String(), "#1404") {
		t.Fatalf("clean report should name the pull request, got %q", out.String())
	}
}

func TestReportExitsViolationOnBlockingFinding(t *testing.T) {
	findings := []stackguard.Finding{{
		Code: stackguard.CodeStaleLink, Blocking: true,
		Detail: "head does not descend from base", Remedy: "restack",
	}}
	var out strings.Builder
	if code := report(&out, 1380, findings, false); code != exitViolation {
		t.Fatalf("exit = %d, want %d", code, exitViolation)
	}
	rendered := out.String()
	for _, want := range []string{"STACK-03", "blocking", "fix: restack"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("report %q missing %q", rendered, want)
		}
	}
}

func TestReportExitsCleanOnAdvisoryOnly(t *testing.T) {
	findings := []stackguard.Finding{{
		Code: stackguard.CodeUnmarkedChain, Detail: "no marker", Remedy: "add the marker",
	}}
	var out strings.Builder
	if code := report(&out, 1392, findings, false); code != exitOK {
		t.Fatalf("advisory findings must not fail the check, exit = %d", code)
	}
	if !strings.Contains(out.String(), "advisory") {
		t.Fatalf("advisory findings must be labelled, got %q", out.String())
	}
}

func TestReportJSONAlwaysCarriesAFindingsArray(t *testing.T) {
	var out strings.Builder
	report(&out, 1404, nil, true)
	var payload struct {
		PullRequest int               `json:"pull_request"`
		Blocking    bool              `json:"blocking"`
		Findings    []json.RawMessage `json:"findings"`
	}
	if err := json.Unmarshal([]byte(out.String()), &payload); err != nil {
		t.Fatalf("JSON output must decode: %v", err)
	}
	if payload.PullRequest != 1404 || payload.Blocking || payload.Findings == nil {
		t.Fatalf("unexpected payload %+v", payload)
	}
}

func TestRunRejectsAMissingPullRequestNumber(t *testing.T) {
	var out, errOut strings.Builder
	if code := run(t.Context(), nil, &out, &errOut); code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut.String(), "-pr is required") {
		t.Fatalf("usage error should name the flag, got %q", errOut.String())
	}
}
