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

func testRetriggerCandidate() StuckPR {
	return StuckPR{
		Number:         7,
		BaseRefName:    "main",
		HeadRefName:    "fix/x",
		HeadSHA:        "abc123",
		requiredChecks: map[string]bool{"Build": true},
	}
}

func TestValidateRetriggerSnapshot(t *testing.T) {
	notDraft := false
	scanned := testRetriggerCandidate()
	valid := retriggerSnapshot{
		Number:           7,
		State:            "open",
		IsDraft:          &notDraft,
		BaseRefName:      "main",
		BaseRepoID:       101,
		BaseRepoFullName: "owner/repo",
		HeadRefName:      "fix/x",
		HeadSHA:          "abc123",
		HeadRepoID:       101,
		HeadRepoFullName: "owner/repo",
	}

	cases := []struct {
		name        string
		mutate      func(*retriggerSnapshot)
		wantErrPart string
	}{
		{
			name: "unchanged same-repository head remains eligible",
		},
		{
			name: "wrong pull request number",
			mutate: func(current *retriggerSnapshot) {
				current.Number = 8
			},
			wantErrPart: "#8",
		},
		{
			name: "closed pull request",
			mutate: func(current *retriggerSnapshot) {
				current.State = "closed"
			},
			wantErrPart: "closed",
		},
		{
			name: "pull request became draft",
			mutate: func(current *retriggerSnapshot) {
				draft := true
				current.IsDraft = &draft
			},
			wantErrPart: "draft",
		},
		{
			name: "draft state missing",
			mutate: func(current *retriggerSnapshot) {
				current.IsDraft = nil
			},
			wantErrPart: "draft state is missing",
		},
		{
			name: "base branch changed",
			mutate: func(current *retriggerSnapshot) {
				current.BaseRefName = "release"
			},
			wantErrPart: "base",
		},
		{
			name: "head branch changed",
			mutate: func(current *retriggerSnapshot) {
				current.HeadRefName = "fix/renamed"
			},
			wantErrPart: "head ref",
		},
		{
			name: "head commit changed",
			mutate: func(current *retriggerSnapshot) {
				current.HeadSHA = "def456"
			},
			wantErrPart: "head SHA",
		},
		{
			name: "head belongs to a fork",
			mutate: func(current *retriggerSnapshot) {
				current.HeadRepoID = 202
				current.HeadRepoFullName = "contributor/repo"
			},
			wantErrPart: "fork",
		},
		{
			name: "base repository identity missing",
			mutate: func(current *retriggerSnapshot) {
				current.BaseRepoID = 0
				current.BaseRepoFullName = ""
			},
			wantErrPart: "identity is missing",
		},
		{
			name: "head repository identity missing",
			mutate: func(current *retriggerSnapshot) {
				current.HeadRepoID = 0
				current.HeadRepoFullName = ""
			},
			wantErrPart: "identity is missing",
		},
		{
			name: "base repository name differs from target",
			mutate: func(current *retriggerSnapshot) {
				current.BaseRepoFullName = "renamed/repo"
			},
			wantErrPart: "base repository",
		},
		{
			name: "head repository name differs from target despite equal id",
			mutate: func(current *retriggerSnapshot) {
				current.HeadRepoFullName = "renamed/repo"
			},
			wantErrPart: "head repository",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := valid
			if tc.mutate != nil {
				tc.mutate(&current)
			}

			err := validateRetriggerSnapshot("owner/repo", scanned, current)
			if tc.wantErrPart == "" {
				if err != nil {
					t.Fatalf("validateRetriggerSnapshot() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErrPart) {
				t.Fatalf("validateRetriggerSnapshot() error = %v, want error containing %q", err, tc.wantErrPart)
			}
		})
	}
}

func TestRetriggerCIDriftStopsBeforeCommitOrRefCalls(t *testing.T) {
	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installSourceFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "api repos/owner/repo/git/commits/abc123"*) printf '%s\n' 'tree123' ;;
  "api repos/owner/repo/commits/abc123/check-runs"*) ;;
  "api repos/owner/repo/pulls/7"*)
    printf '%s\n' '{"number":7,"state":"open","draft":false,"base":{"ref":"main","repo":{"id":101,"full_name":"owner/repo"}},"head":{"ref":"fix/x","sha":"def456","repo":{"id":101,"full_name":"owner/repo"}}}'
    ;;
  *) printf '%s\n' 'retrigger continued after stale head snapshot' >&2; exit 9 ;;
esac
`)

	_, err := RetriggerCI(context.Background(), "owner/repo", testRetriggerCandidate(), false)
	if err == nil || !strings.Contains(err.Error(), "head SHA") {
		t.Fatalf("RetriggerCI() error = %v, want stale head-SHA rejection", err)
	}

	logged, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read fake-gh calls: %v", readErr)
	}
	calls := string(logged)
	if !strings.Contains(calls, "repos/owner/repo/pulls/7") {
		t.Fatalf("RetriggerCI() did not re-read the current PR snapshot:\n%s", calls)
	}
	for _, forbidden := range []string{"repos/owner/repo/git/commits -f", "/git/refs"} {
		if strings.Contains(calls, forbidden) {
			t.Fatalf("stale PR snapshot reached forbidden provider call %q:\n%s", forbidden, calls)
		}
	}
}

func TestRetriggerCIValidSnapshotPreservesMutationOrder(t *testing.T) {
	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installSourceFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "api repos/owner/repo/git/commits/abc123"*) printf '%s\n' 'tree123' ;;
  "api repos/owner/repo/commits/abc123/check-runs"*) ;;
  "api repos/owner/repo/pulls/7"*)
    printf '%s\n' '{"number":7,"state":"open","draft":false,"base":{"ref":"main","repo":{"id":101,"full_name":"owner/repo"}},"head":{"ref":"fix/x","sha":"abc123","repo":{"id":101,"full_name":"owner/repo"}}}'
    ;;
  "api repos/owner/repo/git/commits -f"*) printf '%s\n' 'newsha' ;;
  "api -X PATCH repos/owner/repo/git/refs/heads/fix/x"*) ;;
  *) printf '%s\n' 'unexpected gh invocation' >&2; exit 9 ;;
esac
`)

	outcome, err := RetriggerCI(context.Background(), "owner/repo", testRetriggerCandidate(), false)
	if err != nil {
		t.Fatalf("RetriggerCI() error = %v", err)
	}
	if outcome != Retriggered {
		t.Fatalf("RetriggerCI() outcome = %q, want %q", outcome, Retriggered)
	}

	logged, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read fake-gh calls: %v", readErr)
	}
	calls := string(logged)
	ordered := []string{
		"repos/owner/repo/git/commits/abc123",
		"repos/owner/repo/commits/abc123/check-runs",
		"repos/owner/repo/pulls/7",
		"repos/owner/repo/git/commits -f",
		"repos/owner/repo/git/refs/heads/fix/x",
	}
	previous := -1
	for _, call := range ordered {
		index := strings.Index(calls, call)
		if index < 0 {
			t.Fatalf("RetriggerCI() omitted %q:\n%s", call, calls)
		}
		if index <= previous {
			t.Fatalf("RetriggerCI() provider calls out of order at %q:\n%s", call, calls)
		}
		previous = index
	}
	for _, payload := range []string{"parents[]=abc123", "sha=newsha", "force=false"} {
		if !strings.Contains(calls, payload) {
			t.Fatalf("RetriggerCI() omitted existing mutation payload %q:\n%s", payload, calls)
		}
	}
}

func TestRetriggerCIDryRunRevalidatesWithoutMutation(t *testing.T) {
	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installSourceFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "api repos/owner/repo/commits/abc123/check-runs"*) ;;
  "api repos/owner/repo/pulls/7"*)
    printf '%s\n' '{"number":7,"state":"open","draft":false,"base":{"ref":"main","repo":{"id":101,"full_name":"owner/repo"}},"head":{"ref":"fix/x","sha":"abc123","repo":{"id":101,"full_name":"owner/repo"}}}'
    ;;
  *) printf '%s\n' 'dry-run continued into mutation' >&2; exit 9 ;;
esac
`)

	outcome, err := RetriggerCI(context.Background(), "owner/repo", testRetriggerCandidate(), true)
	if err != nil {
		t.Fatalf("RetriggerCI(dryRun) error = %v", err)
	}
	if outcome != RetriggerWouldRun {
		t.Fatalf("RetriggerCI(dryRun) outcome = %q, want %q", outcome, RetriggerWouldRun)
	}

	logged, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read fake-gh calls: %v", readErr)
	}
	calls := string(logged)
	if !strings.Contains(calls, "repos/owner/repo/pulls/7") {
		t.Fatalf("dry-run did not re-read current PR snapshot:\n%s", calls)
	}
	for _, forbidden := range []string{"/git/commits", "/git/refs"} {
		if strings.Contains(calls, forbidden) {
			t.Fatalf("dry-run reached forbidden provider call %q:\n%s", forbidden, calls)
		}
	}
}

func TestRetriggerCISelfHealedChecksStopBeforeMutation(t *testing.T) {
	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installSourceFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "api repos/owner/repo/git/commits/abc123"*) printf '%s\n' 'tree123' ;;
  "api repos/owner/repo/commits/abc123/check-runs"*) printf '%s\n' 'Build' ;;
  "api repos/owner/repo/pulls/7"*)
    printf '%s\n' '{"number":7,"state":"open","draft":false,"base":{"ref":"main","repo":{"id":101,"full_name":"owner/repo"}},"head":{"ref":"fix/x","sha":"abc123","repo":{"id":101,"full_name":"owner/repo"}}}'
    ;;
  *) printf '%s\n' 'self-healed retrigger reached mutation' >&2; exit 9 ;;
esac
`)

	outcome, err := RetriggerCI(context.Background(), "owner/repo", testRetriggerCandidate(), false)
	if err != nil {
		t.Fatalf("RetriggerCI() error = %v", err)
	}
	if outcome != RetriggerNoLongerNeeded {
		t.Fatalf("RetriggerCI() outcome = %q, want %q", outcome, RetriggerNoLongerNeeded)
	}

	logged, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read fake-gh calls: %v", readErr)
	}
	calls := string(logged)
	for _, forbidden := range []string{"repos/owner/repo/git/commits -f", "/git/refs"} {
		if strings.Contains(calls, forbidden) {
			t.Fatalf("self-healed candidate reached forbidden mutation %q:\n%s", forbidden, calls)
		}
	}
}

func TestRetriggerCICallerCancellationStopsProviderSequence(t *testing.T) {
	callLog := filepath.Join(t.TempDir(), "calls")
	t.Setenv("GH_CALL_LOG", callLog)
	installSourceFakeGH(t, `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "api repos/owner/repo/git/commits/abc123"*) exec sleep 5 ;;
  *) printf '%s\n' 'retrigger continued after cancellation' >&2; exit 9 ;;
esac
`)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := RetriggerCI(ctx, "owner/repo", testRetriggerCandidate(), false)
		result <- err
	}()

	started := time.Now()
	for {
		if _, err := os.Stat(callLog); err == nil {
			break
		}
		if time.Since(started) > 2*time.Second {
			cancel()
			t.Fatal("fake gh did not enter blocking provider call")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	var err error
	select {
	case err = <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("RetriggerCI() did not return after caller cancellation")
	}
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("RetriggerCI() error = %v, want caller cancellation", err)
	}

	logged, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read fake-gh calls: %v", readErr)
	}
	calls := string(logged)
	for _, forbidden := range []string{"/check-runs", "/pulls/", "git/commits -f", "/git/refs"} {
		if strings.Contains(calls, forbidden) {
			t.Fatalf("canceled retrigger reached forbidden later call %q:\n%s", forbidden, calls)
		}
	}
}

func TestRetriggerCIPreflightReadFailuresStopBeforeMutation(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		dryRun      bool
		wantErrPart string
	}{
		{
			name: "tree read failure",
			body: `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "api repos/owner/repo/git/commits/abc123"*) printf '%s\n' 'tree unavailable' >&2; exit 1 ;;
  *) printf '%s\n' 'unexpected provider call' >&2; exit 9 ;;
esac
`,
			wantErrPart: "resolving head tree",
		},
		{
			name: "check-run reread failure",
			body: `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "api repos/owner/repo/git/commits/abc123"*) printf '%s\n' 'tree123' ;;
  "api repos/owner/repo/commits/abc123/check-runs"*) printf '%s\n' 'checks unavailable' >&2; exit 1 ;;
  *) printf '%s\n' 'unexpected provider call' >&2; exit 9 ;;
esac
`,
			wantErrPart: "re-reading check-runs",
		},
		{
			name: "provider read failure",
			body: `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "api repos/owner/repo/commits/abc123/check-runs"*) ;;
  "api repos/owner/repo/pulls/7"*) printf '%s\n' 'provider unavailable' >&2; exit 1 ;;
  *) printf '%s\n' 'unexpected provider call' >&2; exit 9 ;;
esac
`,
			dryRun:      true,
			wantErrPart: "reading current pull request",
		},
		{
			name: "malformed response",
			body: `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "api repos/owner/repo/commits/abc123/check-runs"*) ;;
  "api repos/owner/repo/pulls/7"*) printf '%s\n' '{' ;;
  *) printf '%s\n' 'unexpected provider call' >&2; exit 9 ;;
esac
`,
			dryRun:      true,
			wantErrPart: "parsing current pull request",
		},
		{
			name: "incomplete response",
			body: `
printf '%s\n' "$*" >> "$GH_CALL_LOG"
case "$*" in
  "api repos/owner/repo/commits/abc123/check-runs"*) ;;
  "api repos/owner/repo/pulls/7"*) printf '%s\n' '{"number":7,"state":"open","base":{"ref":"main"},"head":{"ref":"fix/x","sha":"abc123"}}' ;;
  *) printf '%s\n' 'unexpected provider call' >&2; exit 9 ;;
esac
`,
			dryRun:      true,
			wantErrPart: "draft state is missing",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			callLog := filepath.Join(t.TempDir(), "calls")
			t.Setenv("GH_CALL_LOG", callLog)
			installSourceFakeGH(t, tc.body)

			_, err := RetriggerCI(context.Background(), "owner/repo", testRetriggerCandidate(), tc.dryRun)
			if err == nil || !strings.Contains(err.Error(), tc.wantErrPart) {
				t.Fatalf("RetriggerCI() error = %v, want error containing %q", err, tc.wantErrPart)
			}

			logged, readErr := os.ReadFile(callLog)
			if readErr != nil {
				t.Fatalf("read fake-gh calls: %v", readErr)
			}
			calls := string(logged)
			for _, forbidden := range []string{"repos/owner/repo/git/commits -f", "/git/refs"} {
				if strings.Contains(calls, forbidden) {
					t.Fatalf("failed preflight reached forbidden provider call %q:\n%s", forbidden, calls)
				}
			}
		})
	}
}
