package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractPRNumbers_Basic(t *testing.T) {
	t.Parallel()
	nums := extractPRNumbers("PR #449 merged, also #317")
	want := []int{449, 317}
	if len(nums) != len(want) {
		t.Fatalf("got %v, want %v", nums, want)
	}
	for i, n := range want {
		if nums[i] != n {
			t.Errorf("nums[%d] = %d, want %d", i, nums[i], n)
		}
	}
}

func TestExtractPRNumbers_Deduplicates(t *testing.T) {
	t.Parallel()
	nums := extractPRNumbers("#449 and again #449")
	if len(nums) != 1 || nums[0] != 449 {
		t.Errorf("expected [449], got %v", nums)
	}
}

func TestExtractPRNumbers_None(t *testing.T) {
	t.Parallel()
	nums := extractPRNumbers("no PR references here")
	if len(nums) != 0 {
		t.Errorf("expected empty, got %v", nums)
	}
}

func TestExtractPRNumbers_SkipsCrossRepo(t *testing.T) {
	t.Parallel()
	nums := extractPRNumbers("dotfiles#15 engram#27 but #352 is local")
	if len(nums) != 1 || nums[0] != 352 {
		t.Errorf("expected [352], got %v", nums)
	}
}

func TestExtractPRNumbers_InBeadOutput(t *testing.T) {
	t.Parallel()
	text := `✓ ce-gilj · Enforce safe-pr wrapper for PR lifecycle   [● P1 · CLOSED]
Owner: vbonnet
Description: Build safe-pr CLI wrapper. PR #449 implements the enforcement.
Comments:
  2026-06-12: PR #449 created, ready for review
  2026-06-13: Marking as done — PR #449 merged`
	nums := extractPRNumbers(text)
	if len(nums) != 1 || nums[0] != 449 {
		t.Errorf("expected [449], got %v", nums)
	}
}

func TestExtractPRNumbers_NoSpacePrefixes(t *testing.T) {
	t.Parallel()
	// Common local references without a space must NOT be skipped.
	nums := extractPRNumbers("see PR#449, pr#317, Issue#10, ISSUE#11")
	want := []int{449, 317, 10, 11}
	if len(nums) != len(want) {
		t.Fatalf("got %v, want %v", nums, want)
	}
	for i, n := range want {
		if nums[i] != n {
			t.Errorf("nums[%d] = %d, want %d", i, nums[i], n)
		}
	}
}

func TestExtractTitle_Typical(t *testing.T) {
	t.Parallel()
	text := "✓ ce-gilj · Enforce safe-pr wrapper   [● P1 · CLOSED]"
	want := "Enforce safe-pr wrapper"
	got := extractTitle(text)
	if got != want {
		t.Errorf("extractTitle(%q) = %q, want %q", text, got, want)
	}
}

func TestExtractTitle_Empty(t *testing.T) {
	t.Parallel()
	got := extractTitle("")
	if got != "(unknown)" {
		t.Errorf("got %q, want (unknown)", got)
	}
}

func TestExtractRepoFromURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/owner/repo.git", "owner/repo"},
		{"https://github.com/owner/repo", "owner/repo"},
		{"git@github.com:owner/repo.git", "owner/repo"},
		{"https://gitlab.com/owner/repo", ""},
	}
	for _, tc := range cases {
		got := extractRepoFromURL(tc.url)
		if got != tc.want {
			t.Errorf("extractRepoFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestFormatResult_NoPRs(t *testing.T) {
	t.Parallel()
	r := GuardResult{BeadID: "ce-test", PRs: nil, Passed: true}
	var buf strings.Builder
	FormatResult(r, &buf)
	if !strings.Contains(buf.String(), "no PR references") {
		t.Errorf("expected 'no PR references', got %q", buf.String())
	}
}

func TestFormatResult_AllMerged(t *testing.T) {
	t.Parallel()
	r := GuardResult{BeadID: "ce-test", PRs: []int{449}, Passed: true}
	var buf strings.Builder
	FormatResult(r, &buf)
	if !strings.Contains(buf.String(), "all") && !strings.Contains(buf.String(), "merged") {
		t.Errorf("expected merged message, got %q", buf.String())
	}
}

func TestFormatResult_Blocked(t *testing.T) {
	t.Parallel()
	r := GuardResult{
		BeadID:     "ce-test",
		PRs:        []int{449},
		UnmergedPR: []UnmergedPR{{Number: 449, State: "OPEN"}},
		Passed:     false,
	}
	var buf strings.Builder
	FormatResult(r, &buf)
	out := buf.String()
	if !strings.Contains(out, "BLOCKED") {
		t.Errorf("expected BLOCKED, got %q", out)
	}
	if !strings.Contains(out, "#449") {
		t.Errorf("expected PR #449, got %q", out)
	}
}

func TestAppendDoDViolationTrail(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := AppendDoDViolationTrail(home, "ce-test", "Some title", []int{449, 551}); err != nil {
		t.Fatalf("AppendDoDViolationTrail: %v", err)
	}

	path := filepath.Join(home, ".agm", "vroom", "trail.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("trail.jsonl not written: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("trail.jsonl is empty")
	}
	var rec struct {
		EventID   string         `json:"event_id"`
		Timestamp string         `json:"timestamp"`
		Role      string         `json:"role"`
		Kind      string         `json:"kind"`
		Payload   map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
		t.Fatalf("unmarshal trail record: %v", err)
	}
	if rec.Kind != "dod.violation.blocked" {
		t.Errorf("kind = %q, want dod.violation.blocked", rec.Kind)
	}
	if rec.Role != "bead-close-guard" {
		t.Errorf("role = %q, want bead-close-guard", rec.Role)
	}
	if rec.EventID == "" || rec.Timestamp == "" {
		t.Errorf("expected event_id and timestamp to be populated, got %+v", rec)
	}
	if got := rec.Payload["bead_id"]; got != "ce-test" {
		t.Errorf("payload.bead_id = %v, want ce-test", got)
	}
	prs, ok := rec.Payload["unmerged_prs"].([]any)
	if !ok || len(prs) != 2 {
		t.Errorf("payload.unmerged_prs = %v, want 2 entries", rec.Payload["unmerged_prs"])
	}
}

func TestAppendDoDViolationTrail_NilPRs(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := AppendDoDViolationTrail(home, "ce-test", "", nil); err != nil {
		t.Fatalf("AppendDoDViolationTrail with nil PRs: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".agm", "vroom", "trail.jsonl"))
	if err != nil {
		t.Fatalf("trail.jsonl not written: %v", err)
	}
	// nil PRs must serialise as [] so readers always see an array.
	if !strings.Contains(string(data), `"unmerged_prs":[]`) {
		t.Errorf("expected empty array for nil PRs, got %s", data)
	}
}

func TestFormatResult_Forced(t *testing.T) {
	t.Parallel()
	r := GuardResult{
		BeadID:        "ce-test",
		PRs:           []int{449},
		UnmergedPR:    []UnmergedPR{{Number: 449, State: "OPEN"}},
		Passed:        true,
		AbandonReason: "superseded by ce-xyz",
	}
	var buf strings.Builder
	FormatResult(r, &buf)
	out := buf.String()
	if !strings.Contains(out, "OVERRIDE") {
		t.Errorf("expected OVERRIDE, got %q", out)
	}
	if !strings.Contains(out, "superseded by ce-xyz") {
		t.Errorf("expected abandon reason in output, got %q", out)
	}
}

func TestFormatResult_BlockedByDeploy(t *testing.T) {
	t.Parallel()
	r := GuardResult{
		BeadID: "ce-test",
		PRs:    []int{449},
		DriftFindings: []DeployFinding{
			{Target: "stop-hook", Deployed: "~/.claude/hooks/stop", Status: "drift", Remediation: "agm admin install-hooks"},
		},
		Passed: false,
	}
	var buf strings.Builder
	FormatResult(r, &buf)
	out := buf.String()
	if !strings.Contains(out, "BLOCKED") {
		t.Errorf("expected BLOCKED, got %q", out)
	}
	if !strings.Contains(out, "Merged != deployed") {
		t.Errorf("expected merged-vs-deployed explanation, got %q", out)
	}
	if !strings.Contains(out, "stop-hook") || !strings.Contains(out, "agm admin install-hooks") {
		t.Errorf("expected target and remediation, got %q", out)
	}
}

func TestFormatResult_BlockedByBothGates(t *testing.T) {
	t.Parallel()
	r := GuardResult{
		BeadID:     "ce-test",
		PRs:        []int{449, 551},
		UnmergedPR: []UnmergedPR{{Number: 551, State: "OPEN"}},
		DriftFindings: []DeployFinding{
			{Target: "stop-hook", Deployed: "~/.claude/hooks/stop", Status: "missing", Remediation: "agm admin install-hooks"},
		},
		Passed: false,
	}
	var buf strings.Builder
	FormatResult(r, &buf)
	out := buf.String()
	if !strings.Contains(out, "#551") {
		t.Errorf("expected unmerged PR #551, got %q", out)
	}
	if !strings.Contains(out, "stop-hook") {
		t.Errorf("expected deploy finding, got %q", out)
	}
}

func TestFormatResult_OverrideDeploy(t *testing.T) {
	t.Parallel()
	r := GuardResult{
		BeadID:        "ce-test",
		PRs:           []int{449},
		DriftFindings: []DeployFinding{{Target: "stop-hook", Status: "drift", Remediation: "agm admin install-hooks"}},
		Passed:        true,
		AbandonReason: "superseded by ce-xyz",
	}
	var buf strings.Builder
	FormatResult(r, &buf)
	out := buf.String()
	if !strings.Contains(out, "OVERRIDE") {
		t.Errorf("expected OVERRIDE, got %q", out)
	}
	if !strings.Contains(out, "undeployed artifact") {
		t.Errorf("expected undeployed-artifact reason, got %q", out)
	}
}

func TestFormatResult_OKWithDeployChecked(t *testing.T) {
	t.Parallel()
	r := GuardResult{BeadID: "ce-test", PRs: []int{449}, DeployTargetsChecked: 2, Passed: true}
	var buf strings.Builder
	FormatResult(r, &buf)
	out := buf.String()
	if !strings.Contains(out, "merged") {
		t.Errorf("expected merged confirmation, got %q", out)
	}
	if !strings.Contains(out, "deploy target(s) current") {
		t.Errorf("expected deploy-gate confirmation, got %q", out)
	}
}

func TestFormatResult_OKWithSkipNote(t *testing.T) {
	t.Parallel()
	r := GuardResult{BeadID: "ce-test", PRs: []int{449}, DeploySkipNote: "no repo root resolved", Passed: true}
	var buf strings.Builder
	FormatResult(r, &buf)
	out := buf.String()
	if !strings.Contains(out, "deployed gate skipped") {
		t.Errorf("expected skip note, got %q", out)
	}
}
