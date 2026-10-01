package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/nochecks"
)

func TestRunPRScanNoChecksPolicyErrorStopsBeforeTrigger(t *testing.T) {
	oldRepo, oldBranch, oldLimit := noCheckRepo, noCheckBranch, noCheckLimit
	oldTrigger, oldDryRun := noCheckTrigger, noCheckDryRun
	t.Cleanup(func() {
		noCheckRepo, noCheckBranch, noCheckLimit = oldRepo, oldBranch, oldLimit
		noCheckTrigger, noCheckDryRun = oldTrigger, oldDryRun
	})

	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installNoChecksFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":7,"title":"candidate","baseRefName":"main","headRefName":"fix/x","headRefOid":"abc123","isDraft":false}]' ;;
  *rules/branches*) printf '%s\n' 'gh: provider unavailable (HTTP 500)' >&2; exit 1 ;;
  *protection/required_status_checks*) printf '%s\n' '{"contexts":["Build"]}' ;;
  *) printf '%s\n' 'unexpected mutation or check read' >&2; exit 9 ;;
esac
`)

	noCheckRepo = "owner/repo"
	noCheckBranch = "main"
	noCheckLimit = 10
	noCheckTrigger = true
	noCheckDryRun = false
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runPRScanNoChecks(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "required checks") {
		t.Fatalf("runPRScanNoChecks() error = %v, want required-policy failure", err)
	}
	if !cmd.SilenceUsage {
		t.Fatal("policy failure should silence command usage")
	}
	logged, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read fake-gh calls: %v", readErr)
	}
	calls := string(logged)
	for _, requiredCall := range []string{"rules/branches", "protection/required_status_checks"} {
		if !strings.Contains(calls, requiredCall) {
			t.Fatalf("policy discovery omitted %q:\n%s", requiredCall, calls)
		}
	}
	for _, forbidden := range []string{"/check-runs", "/git/commits", "/git/refs"} {
		if strings.Contains(calls, forbidden) {
			t.Fatalf("policy failure reached forbidden provider call %q:\n%s", forbidden, calls)
		}
	}
}

func TestRunPRScanNoChecksSecondBasePolicyErrorStopsBeforeTrigger(t *testing.T) {
	oldRepo, oldBranch, oldLimit := noCheckRepo, noCheckBranch, noCheckLimit
	oldTrigger, oldDryRun := noCheckTrigger, noCheckDryRun
	t.Cleanup(func() {
		noCheckRepo, noCheckBranch, noCheckLimit = oldRepo, oldBranch, oldLimit
		noCheckTrigger, noCheckDryRun = oldTrigger, oldDryRun
	})

	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installNoChecksFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":1,"title":"alpha","baseRefName":"alpha","headRefName":"fix/a","headRefOid":"aaa","isDraft":false},{"number":2,"title":"zeta","baseRefName":"zeta","headRefName":"fix/z","headRefOid":"zzz","isDraft":false}]' ;;
  *rules/branches/alpha*) printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Alpha"}]}}]]' ;;
  *branches/alpha/protection/required_status_checks*) printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  *rules/branches/zeta*) printf '%s\n' 'gh: provider unavailable (HTTP 500)' >&2; exit 1 ;;
  *branches/zeta/protection/required_status_checks*) printf '%s\n' '{"contexts":["Zeta"]}' ;;
  *) printf '%s\n' 'unexpected mutation or check read' >&2; exit 9 ;;
esac
`)

	noCheckRepo = "owner/repo"
	noCheckBranch = ""
	noCheckLimit = 10
	noCheckTrigger = true
	noCheckDryRun = false
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runPRScanNoChecks(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "zeta") {
		t.Fatalf("runPRScanNoChecks() error = %v, want second-base policy failure", err)
	}
	logged, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read fake-gh calls: %v", readErr)
	}
	calls := string(logged)
	alphaIndex := strings.Index(calls, "rules/branches/alpha")
	zetaIndex := strings.Index(calls, "rules/branches/zeta")
	if alphaIndex < 0 || zetaIndex < 0 {
		t.Fatalf("base policy calls missing from fake-gh log:\n%s", calls)
	}
	if alphaIndex > zetaIndex {
		t.Fatalf("base policies were not read in sorted order:\n%s", calls)
	}
	for _, forbidden := range []string{"/check-runs", "/git/commits", "/git/refs"} {
		if strings.Contains(calls, forbidden) {
			t.Fatalf("policy failure reached forbidden provider call %q:\n%s", forbidden, calls)
		}
	}
}

func TestPRScanNoChecksBranchFlagIsOptionalFilter(t *testing.T) {
	flag := prScanNoChecksCmd.Flags().Lookup("branch")
	if flag == nil {
		t.Fatal("branch flag is not registered")
	}
	if flag.DefValue != "" {
		t.Fatalf("branch default = %q, want empty all-base scan", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "filter") {
		t.Fatalf("branch help = %q, want filter semantics", flag.Usage)
	}
}

func TestPRScanNoChecksHelpDescribesSnapshotBoundary(t *testing.T) {
	for _, want := range []string{
		"provider-returned open PR up to --limit",
		"not an atomic GitHub transaction",
		"CI can appear after those observations",
	} {
		if !strings.Contains(prScanNoChecksCmd.Long, want) {
			t.Fatalf("scan-no-checks help omits %q", want)
		}
	}
}

func TestRunPRScanNoChecksRejectsNonPositiveLimitBeforeProviderCalls(t *testing.T) {
	oldRepo, oldBranch, oldLimit := noCheckRepo, noCheckBranch, noCheckLimit
	oldTrigger, oldDryRun := noCheckTrigger, noCheckDryRun
	t.Cleanup(func() {
		noCheckRepo, noCheckBranch, noCheckLimit = oldRepo, oldBranch, oldLimit
		noCheckTrigger, noCheckDryRun = oldTrigger, oldDryRun
	})

	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installNoChecksFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
printf '%s\n' 'provider must not be called' >&2
exit 9
`)

	for _, limit := range []int{0, -1} {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			noCheckRepo = "owner/repo"
			noCheckBranch = ""
			noCheckLimit = limit
			noCheckTrigger = false
			noCheckDryRun = false
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())

			err := runPRScanNoChecks(cmd, nil)
			if err == nil || !strings.Contains(err.Error(), "--limit must be a positive integer") {
				t.Fatalf("runPRScanNoChecks() error = %v, want non-positive limit rejection", err)
			}
			if _, statErr := os.Stat(callLog); !os.IsNotExist(statErr) {
				t.Fatalf("invalid limit reached provider; stat error = %v", statErr)
			}
		})
	}
}

func TestNoChecksScanResultJSONCarriesExplicitBaseEvidence(t *testing.T) {
	result := &NoChecksScanResult{
		Repo:        "owner/repo",
		BaseFilter:  "",
		OpenPRs:     1,
		EligiblePRs: 1,
		ReadErrors:  []noCheckReadError{},
		Stuck: []noCheckItem{{StuckPR: nochecks.StuckPR{
			Number:      7,
			Title:       "stacked",
			BaseRefName: "stack-base",
			HeadRefName: "feature",
			HeadSHA:     "abcdef012345",
		}}},
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var got struct {
		BaseFilter  *string            `json:"base_filter"`
		EligiblePRs int                `json:"eligible_prs"`
		ReadErrors  []noCheckReadError `json:"read_errors"`
		Stuck       []struct {
			BaseRefName string `json:"base_ref_name"`
		} `json:"stuck"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.BaseFilter == nil || *got.BaseFilter != "" {
		t.Fatalf("base_filter = %v in %s; want explicit empty all-base filter", got.BaseFilter, encoded)
	}
	if got.EligiblePRs != 1 {
		t.Fatalf("eligible_prs = %d in %s; want 1", got.EligiblePRs, encoded)
	}
	if got.ReadErrors == nil || len(got.ReadErrors) != 0 {
		t.Fatalf("read_errors = %#v in %s; want explicit empty list", got.ReadErrors, encoded)
	}
	if len(got.Stuck) != 1 || got.Stuck[0].BaseRefName != "stack-base" {
		t.Fatalf("stuck base evidence = %#v in %s; want stack-base", got.Stuck, encoded)
	}
}

func TestPrintNoChecksScanTextCarriesBaseEvidence(t *testing.T) {
	result := &NoChecksScanResult{
		Repo:        "owner/repo",
		OpenPRs:     1,
		EligiblePRs: 1,
		Stuck: []noCheckItem{{StuckPR: nochecks.StuckPR{
			Number:      7,
			Title:       "stacked",
			BaseRefName: "stack-base",
			HeadRefName: "feature",
			HeadSHA:     "abcdef012345",
		}}},
	}

	output := captureStdout(t, func() { printNoChecksScanText(result) })
	for _, want := range []string{
		"Base filter: (all observed bases)",
		"base stack-base; head abcdef0 (feature)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("text output missing %q:\n%s", want, output)
		}
	}
}

func TestRunPRScanNoChecksDraftOnlyReportsListedNotScanned(t *testing.T) {
	oldRepo, oldBranch, oldLimit := noCheckRepo, noCheckBranch, noCheckLimit
	oldTrigger, oldDryRun, oldOutput := noCheckTrigger, noCheckDryRun, outputFormat
	t.Cleanup(func() {
		noCheckRepo, noCheckBranch, noCheckLimit = oldRepo, oldBranch, oldLimit
		noCheckTrigger, noCheckDryRun, outputFormat = oldTrigger, oldDryRun, oldOutput
	})

	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installNoChecksFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":7,"title":"draft","baseRefName":"stack-base","headRefName":"feature","headRefOid":"abc123","isDraft":true}]' ;;
  *) printf '%s\n' 'draft-only scan reached a forbidden provider call' >&2; exit 9 ;;
esac
`)

	noCheckRepo = "owner/repo"
	noCheckBranch = ""
	noCheckLimit = 10
	noCheckTrigger = false
	noCheckDryRun = false
	outputFormat = "text"
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	var runErr error
	output := captureStdout(t, func() { runErr = runPRScanNoChecks(cmd, nil) })
	if runErr != nil {
		t.Fatalf("runPRScanNoChecks() error = %v", runErr)
	}
	for _, want := range []string{
		"Open PRs listed: 1",
		"Eligible non-draft PRs scanned: 0",
		"no eligible non-draft PRs to scan",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("draft-only output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "every open PR") {
		t.Fatalf("draft-only output claims unread drafts are healthy:\n%s", output)
	}
	logged, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read fake-gh calls: %v", err)
	}
	if calls := strings.TrimSpace(string(logged)); !strings.HasPrefix(calls, "pr list") || strings.Contains(calls, "\n") {
		t.Fatalf("draft-only scan made calls beyond listing:\n%s", calls)
	}
}

func TestRunPRScanNoChecksDryRunRevalidatesAndContinuesAfterDrift(t *testing.T) {
	oldRepo, oldBranch, oldLimit := noCheckRepo, noCheckBranch, noCheckLimit
	oldTrigger, oldDryRun, oldOutput := noCheckTrigger, noCheckDryRun, outputFormat
	t.Cleanup(func() {
		noCheckRepo, noCheckBranch, noCheckLimit = oldRepo, oldBranch, oldLimit
		noCheckTrigger, noCheckDryRun, outputFormat = oldTrigger, oldDryRun, oldOutput
	})

	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installNoChecksFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":7,"title":"drifted","baseRefName":"main","headRefName":"fix/x","headRefOid":"abc123","isDraft":false},{"number":8,"title":"current","baseRefName":"main","headRefName":"fix/y","headRefOid":"xyz789","isDraft":false}]' ;;
  *rules/branches/main*) printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Build"}]}}]]' ;;
  *branches/main/protection/required_status_checks*) printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  *commits/abc123/check-runs*) ;;
  *commits/xyz789/check-runs*) ;;
  "api repos/owner/repo/pulls/7"*) printf '%s\n' '{"number":7,"state":"open","draft":false,"base":{"ref":"main","repo":{"id":101,"full_name":"owner/repo"}},"head":{"ref":"fix/x","sha":"new456","repo":{"id":101,"full_name":"owner/repo"}}}' ;;
  "api repos/owner/repo/pulls/8"*) printf '%s\n' '{"number":8,"state":"open","draft":false,"base":{"ref":"main","repo":{"id":101,"full_name":"owner/repo"}},"head":{"ref":"fix/y","sha":"xyz789","repo":{"id":101,"full_name":"owner/repo"}}}' ;;
  *) printf '%s\n' 'unexpected provider call' >&2; exit 9 ;;
esac
`)

	noCheckRepo = "owner/repo"
	noCheckBranch = ""
	noCheckLimit = 10
	noCheckTrigger = true
	noCheckDryRun = true
	outputFormat = "text"
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	var runErr error
	output := captureStdout(t, func() { runErr = runPRScanNoChecks(cmd, nil) })
	if runErr == nil || !strings.Contains(runErr.Error(), "validation or mutation failed for 1 of 2") {
		t.Fatalf("runPRScanNoChecks() error = %v, want one per-PR revalidation failure", runErr)
	}
	for _, want := range []string{
		"#7 drifted",
		"re-trigger FAILED: retrigger preflight for PR #7: head SHA changed",
		"#8 current",
		"eligible for re-trigger at current snapshot",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, output)
		}
	}

	logged, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read fake-gh calls: %v", readErr)
	}
	calls := string(logged)
	for _, required := range []string{"repos/owner/repo/pulls/7", "repos/owner/repo/pulls/8"} {
		if !strings.Contains(calls, required) {
			t.Fatalf("dry-run did not continue to %q:\n%s", required, calls)
		}
	}
	for _, forbidden := range []string{"/git/commits", "/git/refs"} {
		if strings.Contains(calls, forbidden) {
			t.Fatalf("dry-run reached forbidden provider call %q:\n%s", forbidden, calls)
		}
	}
}

func TestRunPRScanNoChecksSelfHealedCandidateSucceedsWithoutMutation(t *testing.T) {
	oldRepo, oldBranch, oldLimit := noCheckRepo, noCheckBranch, noCheckLimit
	oldTrigger, oldDryRun, oldOutput := noCheckTrigger, noCheckDryRun, outputFormat
	t.Cleanup(func() {
		noCheckRepo, noCheckBranch, noCheckLimit = oldRepo, oldBranch, oldLimit
		noCheckTrigger, noCheckDryRun, outputFormat = oldTrigger, oldDryRun, oldOutput
	})

	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installNoChecksFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":7,"title":"self-healed","baseRefName":"main","headRefName":"fix/x","headRefOid":"abc123","isDraft":false}]' ;;
  *rules/branches/main*) printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Build"}]}}]]' ;;
  *branches/main/protection/required_status_checks*) printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  *commits/abc123/check-runs*)
    if [ "$(grep -c 'commits/abc123/check-runs' "$GH_CALL_LOG")" -gt 1 ]; then
      printf '%s\n' 'Build'
    fi
    ;;
  "api repos/owner/repo/git/commits/abc123"*) printf '%s\n' 'tree123' ;;
  "api repos/owner/repo/pulls/7"*)
    printf '%s\n' '{"number":7,"state":"open","draft":false,"base":{"ref":"main","repo":{"id":101,"full_name":"owner/repo"}},"head":{"ref":"fix/x","sha":"abc123","repo":{"id":101,"full_name":"owner/repo"}}}'
    ;;
  *) printf '%s\n' 'self-healed command reached an unexpected provider call' >&2; exit 9 ;;
esac
`)

	noCheckRepo = "owner/repo"
	noCheckBranch = ""
	noCheckLimit = 10
	noCheckTrigger = true
	noCheckDryRun = false
	outputFormat = "text"
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	var runErr error
	output := captureStdout(t, func() { runErr = runPRScanNoChecks(cmd, nil) })
	if runErr != nil {
		t.Fatalf("runPRScanNoChecks() error = %v, want self-healed success", runErr)
	}
	for _, want := range []string{
		"#7 self-healed",
		"no longer stuck (CI appeared after scan)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("self-healed output missing %q:\n%s", want, output)
		}
	}

	logged, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read fake-gh calls: %v", readErr)
	}
	calls := string(logged)
	if count := strings.Count(calls, "commits/abc123/check-runs"); count != 2 {
		t.Fatalf("check-runs read count = %d, want initial scan plus trigger preflight:\n%s", count, calls)
	}
	for _, forbidden := range []string{"repos/owner/repo/git/commits -f", "/git/refs"} {
		if strings.Contains(calls, forbidden) {
			t.Fatalf("self-healed command reached forbidden mutation %q:\n%s", forbidden, calls)
		}
	}
}

func TestRunPRScanNoChecksRejectsDryRunWithoutTriggerBeforeProviderCalls(t *testing.T) {
	oldRepo, oldBranch, oldLimit := noCheckRepo, noCheckBranch, noCheckLimit
	oldTrigger, oldDryRun := noCheckTrigger, noCheckDryRun
	t.Cleanup(func() {
		noCheckRepo, noCheckBranch, noCheckLimit = oldRepo, oldBranch, oldLimit
		noCheckTrigger, noCheckDryRun = oldTrigger, oldDryRun
	})

	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installNoChecksFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
printf '%s\n' 'provider must not be called' >&2
exit 9
`)

	noCheckRepo = "owner/repo"
	noCheckBranch = ""
	noCheckLimit = 10
	noCheckTrigger = false
	noCheckDryRun = true
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := runPRScanNoChecks(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--dry-run requires --trigger") {
		t.Fatalf("runPRScanNoChecks() error = %v, want invalid flag combination", err)
	}
	if _, statErr := os.Stat(callLog); !os.IsNotExist(statErr) {
		t.Fatalf("invalid flags reached provider; stat error = %v", statErr)
	}
}

func TestRunPRScanNoChecksReadErrorRemainsIndeterminate(t *testing.T) {
	oldRepo, oldBranch, oldLimit := noCheckRepo, noCheckBranch, noCheckLimit
	oldTrigger, oldDryRun, oldOutput := noCheckTrigger, noCheckDryRun, outputFormat
	t.Cleanup(func() {
		noCheckRepo, noCheckBranch, noCheckLimit = oldRepo, oldBranch, oldLimit
		noCheckTrigger, noCheckDryRun, outputFormat = oldTrigger, oldDryRun, oldOutput
	})

	installNoChecksFakeGH(t, `
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":7,"title":"unreadable","baseRefName":"main","headRefName":"fix/x","headRefOid":"abc123","isDraft":false}]' ;;
  *rules/branches/main*) printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Build"}]}}]]' ;;
  *branches/main/protection/required_status_checks*) printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  *commits/abc123/check-runs*) printf '%s\n' 'provider unavailable' >&2; exit 1 ;;
  *) printf '%s\n' 'unexpected provider call' >&2; exit 9 ;;
esac
`)

	noCheckRepo = "owner/repo"
	noCheckBranch = ""
	noCheckLimit = 10
	noCheckTrigger = false
	noCheckDryRun = false
	outputFormat = "text"
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	var runErr error
	output := captureStdout(t, func() { runErr = runPRScanNoChecks(cmd, nil) })
	if runErr == nil || !strings.Contains(runErr.Error(), "remain indeterminate") {
		t.Fatalf("runPRScanNoChecks() error = %v, want indeterminate alarm", runErr)
	}
	for _, want := range []string{
		"Indeterminate (check-runs unreadable): 1",
		"? #7",
		"no stuck PRs proven; unreadable PRs remain indeterminate",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("indeterminate output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "no eligible non-draft PR needs") {
		t.Fatalf("indeterminate output made a healthy-CI claim:\n%s", output)
	}
}

func TestRunPRScanNoChecksJSONReadErrorRemainsStructuredAndIndeterminate(t *testing.T) {
	oldRepo, oldBranch, oldLimit := noCheckRepo, noCheckBranch, noCheckLimit
	oldTrigger, oldDryRun, oldOutput := noCheckTrigger, noCheckDryRun, outputFormat
	t.Cleanup(func() {
		noCheckRepo, noCheckBranch, noCheckLimit = oldRepo, oldBranch, oldLimit
		noCheckTrigger, noCheckDryRun, outputFormat = oldTrigger, oldDryRun, oldOutput
	})

	installNoChecksFakeGH(t, `
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":7,"title":"unreadable","baseRefName":"main","headRefName":"fix/x","headRefOid":"abc123","isDraft":false}]' ;;
  *rules/branches/main*) printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Build"}]}}]]' ;;
  *branches/main/protection/required_status_checks*) printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  *commits/abc123/check-runs*) printf '%s\n' 'provider unavailable' >&2; exit 1 ;;
  *) printf '%s\n' 'unexpected provider call' >&2; exit 9 ;;
esac
`)

	noCheckRepo = "owner/repo"
	noCheckBranch = ""
	noCheckLimit = 10
	noCheckTrigger = false
	noCheckDryRun = false
	outputFormat = "json"
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	var runErr error
	output := captureStdout(t, func() { runErr = runPRScanNoChecks(cmd, nil) })
	if runErr == nil || !strings.Contains(runErr.Error(), "remain indeterminate") {
		t.Fatalf("runPRScanNoChecks() error = %v, want indeterminate alarm", runErr)
	}
	var got NoChecksScanResult
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("decode JSON output %q: %v", output, err)
	}
	if len(got.Stuck) != 0 {
		t.Fatalf("JSON stuck = %#v, want no classification from unreadable checks", got.Stuck)
	}
	if len(got.ReadErrors) != 1 || got.ReadErrors[0].Number != 7 || got.ReadErrors[0].Error == "" {
		t.Fatalf("JSON read_errors = %#v, want structured PR #7 error", got.ReadErrors)
	}
}

func TestRunPRScanNoChecksCancellationStopsLaterRetriggers(t *testing.T) {
	oldRepo, oldBranch, oldLimit := noCheckRepo, noCheckBranch, noCheckLimit
	oldTrigger, oldDryRun := noCheckTrigger, noCheckDryRun
	t.Cleanup(func() {
		noCheckRepo, noCheckBranch, noCheckLimit = oldRepo, oldBranch, oldLimit
		noCheckTrigger, noCheckDryRun = oldTrigger, oldDryRun
	})

	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installNoChecksFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":7,"title":"first","baseRefName":"main","headRefName":"fix/x","headRefOid":"aaa","isDraft":false},{"number":8,"title":"second","baseRefName":"main","headRefName":"fix/y","headRefOid":"bbb","isDraft":false}]' ;;
  *rules/branches/main*) printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Build"}]}}]]' ;;
  *branches/main/protection/required_status_checks*) printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  *commits/aaa/check-runs*) ;;
  *commits/bbb/check-runs*) ;;
  "api repos/owner/repo/git/commits/aaa"*) exec sleep 5 ;;
  *) printf '%s\n' 'unexpected provider call' >&2; exit 9 ;;
esac
`)

	noCheckRepo = "owner/repo"
	noCheckBranch = ""
	noCheckLimit = 10
	noCheckTrigger = true
	noCheckDryRun = false
	ctx, cancel := context.WithCancel(context.Background())
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	result := make(chan error, 1)
	go func() { result <- runPRScanNoChecks(cmd, nil) }()

	started := time.Now()
	for {
		logged, _ := os.ReadFile(callLog)
		if strings.Contains(string(logged), "repos/owner/repo/git/commits/aaa") {
			break
		}
		if time.Since(started) > 3*time.Second {
			cancel()
			t.Fatal("first retrigger did not reach the blocking tree read")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	var err error
	select {
	case err = <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("runPRScanNoChecks() did not stop after caller cancellation")
	}
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("runPRScanNoChecks() error = %v, want caller cancellation", err)
	}

	logged, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read fake-gh calls: %v", readErr)
	}
	calls := string(logged)
	for _, forbidden := range []string{
		"repos/owner/repo/git/commits/bbb",
		"repos/owner/repo/git/commits -f",
		"/git/refs",
	} {
		if strings.Contains(calls, forbidden) {
			t.Fatalf("canceled command reached later mutation call %q:\n%s", forbidden, calls)
		}
	}
}

func installNoChecksFakeGH(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	script := "#!/bin/sh\nset -eu\n" + body
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
