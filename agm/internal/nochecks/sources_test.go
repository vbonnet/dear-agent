package nochecks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGHJSONContextReturnsPreCanceledCallerBeforeExecutableLookup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	t.Setenv("PATH", t.TempDir())

	got, err := ghJSONContext(ctx, time.Second, []string{"version"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ghJSONContext() = %q, %v; want caller cancellation", got, err)
	}
	if got != nil {
		t.Fatalf("ghJSONContext() returned output %q after caller cancellation", got)
	}
}

func TestListOpenPRsCarriesBaseAndOmitsEmptyFilter(t *testing.T) {
	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installSourceFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
printf '%s\n' '[{"number":7,"title":"stacked","baseRefName":"stack-base","headRefName":"feature","headRefOid":"abc123","isDraft":false}]'
`)

	prs, err := ListOpenPRs("owner/repo", 10, "")
	if err != nil {
		t.Fatalf("ListOpenPRs() error = %v", err)
	}
	if len(prs) != 1 || prs[0].BaseRefName != "stack-base" {
		t.Fatalf("PRs = %#v, want observed stack base", prs)
	}
	logged, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read fake-gh calls: %v", err)
	}
	call := string(logged)
	if !strings.Contains(call, "baseRefName") {
		t.Fatalf("PR list omitted baseRefName: %q", call)
	}
	if !strings.Contains(call, "--limit 10") {
		t.Fatalf("PR list omitted the requested provider limit: %q", call)
	}
	if strings.Contains(call, "--base") {
		t.Fatalf("empty filter emitted --base: %q", call)
	}
}

func TestListOpenPRsAppliesAndVerifiesExplicitBaseFilter(t *testing.T) {
	installSourceFakeGH(t, `
case "$*" in
  *"--base release"*) printf '%s\n' '[{"number":7,"title":"release","baseRefName":"release","headRefName":"feature","headRefOid":"abc123","isDraft":false}]' ;;
  *) printf '%s\n' 'missing explicit base filter' >&2; exit 2 ;;
esac
`)

	prs, err := ListOpenPRs("owner/repo", 10, "release")
	if err != nil {
		t.Fatalf("ListOpenPRs() error = %v", err)
	}
	if len(prs) != 1 || prs[0].BaseRefName != "release" {
		t.Fatalf("PRs = %#v, want release base", prs)
	}
}

func TestListOpenPRsRejectsRowOutsideExplicitBaseFilter(t *testing.T) {
	installSourceFakeGH(t, `
printf '%s\n' '[{"number":7,"title":"wrong","baseRefName":"main","headRefName":"feature","headRefOid":"abc123","isDraft":false}]'
`)

	prs, err := ListOpenPRs("owner/repo", 10, "release")
	if err == nil || !strings.Contains(err.Error(), "outside requested base") {
		t.Fatalf("ListOpenPRs() = %#v, %v; want filter mismatch error", prs, err)
	}
	if prs != nil {
		t.Fatalf("filter mismatch returned partial PRs %#v", prs)
	}
}

func TestListOpenPRsRejectsUnknownDraftState(t *testing.T) {
	cases := []struct {
		name  string
		field string
	}{
		{name: "omitted", field: ""},
		{name: "null", field: `,"isDraft":null`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installSourceFakeGH(t, `
printf '%s\n' '[{"number":7,"title":"unknown","baseRefName":"main","headRefName":"feature","headRefOid":"abc123"`+tc.field+`}]'
`)

			prs, err := ListOpenPRs("owner/repo", 10, "")
			if err == nil || !strings.Contains(err.Error(), "draft state") {
				t.Fatalf("ListOpenPRs() = %#v, %v; want unknown-draft error", prs, err)
			}
			if prs != nil {
				t.Fatalf("unknown-draft response returned partial PRs %#v", prs)
			}
		})
	}
}

func TestFetchRequiredChecksUsesLayeredPolicy(t *testing.T) {
	installSourceFakeGH(t, `
case "$*" in
  *rules/branches*) printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Ruleset"}]}}]]' ;;
  *protection/required_status_checks*) printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  *) printf '%s\n' 'unexpected gh invocation' >&2; exit 2 ;;
esac
`)

	required, err := fetchRequiredChecks(context.Background(), "owner/repo", "main")
	if err != nil {
		t.Fatalf("FetchRequiredChecks() error = %v", err)
	}
	if !required["Ruleset"] || len(required) != 1 {
		t.Fatalf("required checks = %#v, want ruleset context", required)
	}
}

func TestFetchRequiredChecksReturnsDiscoveryError(t *testing.T) {
	installSourceFakeGH(t, `
case "$*" in
  *rules/branches*) printf '%s\n' 'gh: provider unavailable (HTTP 500)' >&2; exit 1 ;;
  *protection/required_status_checks*) printf '%s\n' '{"contexts":["Classic"]}' ;;
  *) printf '%s\n' 'unexpected gh invocation' >&2; exit 2 ;;
esac
`)

	required, err := fetchRequiredChecks(context.Background(), "owner/repo", "main")
	if err == nil {
		t.Fatal("FetchRequiredChecks() succeeded with incomplete policy")
	}
	if required != nil {
		t.Fatalf("incomplete policy returned %#v, want nil", required)
	}
}

func TestCheckRunNamesForRefRequestsEveryPage(t *testing.T) {
	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installSourceFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
printf '%s\n' 'Optional' 'Required'
`)

	runs, err := CheckRunNamesForRef("owner/repo", "abc123")
	if err != nil {
		t.Fatalf("CheckRunNamesForRef() error = %v", err)
	}
	if len(runs) != 2 || runs[0].Name != "Optional" || runs[1].Name != "Required" {
		t.Fatalf("check runs = %#v", runs)
	}
	logged, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read fake-gh calls: %v", err)
	}
	call := string(logged)
	if !strings.Contains(call, "--paginate") ||
		!strings.Contains(call, "commits/abc123/check-runs?per_page=100") {
		t.Fatalf("check-run call is not fully paginated: %q", call)
	}
}

func TestCheckRunNamesForRefDiscardsPartialOutputOnFailure(t *testing.T) {
	installSourceFakeGH(t, `
printf '%s\n' 'first-page-result'
printf '%s\n' 'later page failed' >&2
exit 1
`)

	runs, err := CheckRunNamesForRef("owner/repo", "abc123")
	if err == nil {
		t.Fatal("CheckRunNamesForRef() succeeded with a failed page")
	}
	if !strings.Contains(err.Error(), "repos/owner/repo/commits/abc123/check-runs?per_page=100") {
		t.Fatalf("failed complete read omitted endpoint identity: %v", err)
	}
	if runs != nil {
		t.Fatalf("failed complete read returned partial runs %#v", runs)
	}
}

func installSourceFakeGH(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	script := "#!/bin/sh\nset -eu\n" + body
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
