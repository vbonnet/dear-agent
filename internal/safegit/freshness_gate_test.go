package safegit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckAllCIContextForBaseRejectsRetarget(t *testing.T) {
	installRequiredCheckFakeGH(t, `
case "$*" in
  "pr view 7 --repo owner/repo --json baseRefName") printf '%s\n' '{"baseRefName":"develop"}' ;;
  *rules/branches/develop*) printf '%s\n' '[[]]' ;;
  *protection/required_status_checks*) printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  "pr checks 7 --repo owner/repo --required --json name,state") printf '%s\n' '[]' ;;
  *) printf '%s\n' "unexpected gh invocation: $*" >&2; exit 2 ;;
esac
`)
	err := checkAllCIContextForBase(t.Context(), 7, "owner/repo", "main")
	if err == nil || !strings.Contains(err.Error(), "base changed before CI policy evaluation") {
		t.Fatalf("got %v, want retarget error", err)
	}
}

func TestAttemptMergeDryRunRejectsHeadThatMissesLiveBase(t *testing.T) {
	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("FAKE_CALL_LOG", callLog)
	t.Setenv("SAFE_MERGE_AUDIT_DIR", t.TempDir())
	installRequiredCheckFakeGH(t, `
printf '%s\n' "$*" >> "$FAKE_CALL_LOG"
case "$*" in
  "pr view 7 --repo owner/repo --json number,title,url,state,isDraft,mergeable,mergeStateStatus,reviewDecision,baseRefName,headRefName,headRefOid")
    printf '%s\n' '{"number":7,"state":"OPEN","isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"UNSTABLE","baseRefName":"main","headRefName":"topic","headRefOid":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}' ;;
  "pr view 7 --repo owner/repo --json baseRefName") printf '%s\n' '{"baseRefName":"main"}' ;;
  "api --paginate --slurp repos/owner/repo/rules/branches/main?per_page=100") printf '%s\n' '[[]]' ;;
  "api repos/owner/repo/branches/main/protection/required_status_checks") printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  "pr checks 7 --repo owner/repo --required --json name,state"|\
  "pr checks 7 --repo owner/repo --json name,state") printf '%s\n' '[]' ;;
  "pr view 7 --repo owner/repo --json headRefOid,commits")
    printf '%s\n' '{"headRefOid":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","commits":[{"committedDate":"2000-01-01T00:00:00Z"}]}' ;;
  "pr view 7 --repo owner/repo --json baseRefName,headRefOid")
    printf '%s\n' '{"baseRefName":"main","headRefOid":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}' ;;
  "api repos/owner/repo/git/ref/heads%2Fmain")
    printf '%s\n' '{"ref":"refs/heads/main","object":{"sha":"cccccccccccccccccccccccccccccccccccccccc","type":"commit"}}' ;;
  "api repos/owner/repo/compare/cccccccccccccccccccccccccccccccccccccccc...aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
    printf '%s\n' '{"status":"diverged","base_commit":{"sha":"cccccccccccccccccccccccccccccccccccccccc"},"merge_base_commit":{"sha":"dddddddddddddddddddddddddddddddddddddddd"}}' ;;
  "pr view 7 --repo owner/repo --json mergeStateStatus") printf '%s\n' '{"mergeStateStatus":"UNSTABLE"}' ;;
  *update-branch*|"pr merge "*) printf '%s\n' 'unexpected mutation' >&2; exit 9 ;;
  *) printf '%s\n' "unexpected gh invocation: $*" >&2; exit 2 ;;
esac
`)

	err := attemptMerge(t.Context(), MergeConfig{
		PRNumber:        7,
		Repo:            "owner/repo",
		DryRun:          true,
		SkipReviewCheck: true,
		Now:             time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC),
	})
	if err == nil || !strings.Contains(err.Error(), "does not contain live base main@"+freshnessLiveBaseOID) {
		t.Fatalf("attemptMerge error = %v, want exact live-base rejection", err)
	}
	calls, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read provider call log: %v", readErr)
	}
	logText := string(calls)
	if !strings.Contains(logText, "compare/"+freshnessLiveBaseOID+"..."+freshnessHeadOID) {
		t.Fatalf("attempt did not compare the live base OID to the gated head:\n%s", logText)
	}
	if strings.Contains(logText, "update-branch") || strings.Contains(logText, "pr merge ") {
		t.Fatalf("dry-run stale attempt mutated provider state:\n%s", logText)
	}
}
