package safegit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

const (
	cleanupHelperEnv            = "SAFEGIT_CLEANUP_HELPER"
	cleanupHelperBranchEnv      = "SAFEGIT_CLEANUP_BRANCH"
	cleanupHelperPrimaryEnv     = "SAFEGIT_CLEANUP_PRIMARY"
	cleanupHelperCallerEnv      = "SAFEGIT_CLEANUP_CALLER"
	attemptMergeHelperEnv       = "SAFEGIT_ATTEMPT_MERGE_HELPER"
	attemptMergeHeadMismatchEnv = "SAFEGIT_ATTEMPT_MERGE_HEAD_MISMATCH"
	attemptMergeMarkerEnv       = "SAFEGIT_ATTEMPT_MERGE_MARKER"
	attemptMergeWatchEnv        = "SAFEGIT_ATTEMPT_MERGE_WATCH"
	attemptMergeDelayConfirmEnv = "SAFEGIT_ATTEMPT_MERGE_DELAY_CONFIRMATION"
	attemptMergeTransientEnv    = "SAFEGIT_ATTEMPT_MERGE_TRANSIENT_CONFIRMATION"
	attemptMergeConfirmCountEnv = "SAFEGIT_ATTEMPT_MERGE_CONFIRM_COUNT"
	attemptMergeProviderFailEnv = "SAFEGIT_ATTEMPT_MERGE_PROVIDER_FAILURE"
)

type attemptMergeTestOutcome uint8

const (
	attemptMergeOutcomeSuccess attemptMergeTestOutcome = iota
	attemptMergeOutcomeHeadMismatch
	attemptMergeOutcomeProviderFailure
)

type cleanupFixture struct {
	primary string
	linked  string
	branch  string
}

func TestCleanupWorktreeHelper(t *testing.T) {
	if os.Getenv(cleanupHelperEnv) != "1" {
		return
	}
	branch := os.Getenv(cleanupHelperBranchEnv)
	if branch == "" {
		t.Fatal("cleanup helper requires a branch")
	}

	plan := prepareCleanupPlan(context.Background(), branch)
	plan.run(context.Background())
	// Exit without asking the test runner to inspect the helper's removed cwd.
	os.Exit(0)
}

func TestAttemptMergeCleanupHelper(t *testing.T) {
	if os.Getenv(attemptMergeHelperEnv) != "1" {
		return
	}

	cfg := MergeConfig{
		PRNumber:        42,
		Repo:            "owner/repo",
		Watch:           os.Getenv(attemptMergeWatchEnv) == "1",
		WatchTimeout:    30 * time.Second,
		WatchInterval:   time.Millisecond,
		SkipReviewCheck: true,
	}
	var err error
	if cfg.Watch && os.Getenv(attemptMergeProviderFailEnv) != "1" {
		// Exercise the outer watch loop for both successful confirmation and a
		// terminal exact-head mismatch. Provider-command failure is covered by
		// the one-shot adapter path below.
		err = watchMerge(context.Background(), cfg)
	} else {
		err = attemptMerge(context.Background(), cfg)
	}
	if os.Getenv(attemptMergeProviderFailEnv) == "1" {
		if err == nil || !strings.Contains(err.Error(), "gh pr merge failed") {
			t.Fatalf("attemptMerge error = %v, want gh pr merge failure", err)
		}
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(0)
	}
	if os.Getenv(attemptMergeHeadMismatchEnv) == "1" {
		if err == nil || !strings.Contains(err.Error(), "merge completion head changed") {
			t.Fatalf("attemptMerge error = %v, want exact-head mismatch", err)
		}
		if strings.Contains(err.Error(), "gh pr merge failed") {
			t.Fatalf("attemptMerge misclassified exact-head mismatch as provider failure: %v", err)
		}
		os.Exit(0)
	}
	if err != nil {
		t.Fatalf("attemptMerge: %v", err)
	}
	os.Exit(0)
}

func TestRunCleanupGitHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := runCleanupGit(ctx, "", "version"); !errors.Is(err, context.Canceled) {
		t.Fatalf("runCleanupGit error = %v, want context.Canceled", err)
	}
}

func TestProviderMergeDoesNotStartAfterCleanupPreparationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	missingProvider := filepath.Join(t.TempDir(), "must-not-start")
	failure := runProviderMergeTransaction(
		ctx,
		"cleanup-topic",
		[]string{missingProvider},
		func() error { return nil },
		nil,
	)
	if failure == nil || failure.stage != providerMergeCommandStage ||
		!errors.Is(failure.err, context.Canceled) {
		t.Fatalf("provider merge failure = %#v, want command-stage context.Canceled", failure)
	}
}

func TestProviderMergeCommandCancellationRequiresConfirmation(t *testing.T) {
	tests := []struct {
		name              string
		confirmationErr   error
		wantFailure       bool
		wantConfirmedCall int
	}{
		{name: "unconfirmed", confirmationErr: context.Canceled, wantFailure: true},
		{name: "confirmed", wantConfirmedCall: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeDir := t.TempDir()
			provider := filepath.Join(fakeDir, "provider")
			marker := filepath.Join(fakeDir, "started")
			if err := os.WriteFile(provider, []byte("#!/bin/sh\nprintf started > \"$1\"\nwhile :; do :; done\n"), 0o700); err != nil {
				t.Fatalf("write provider: %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			confirmCalls := 0
			confirmedCalls := 0
			result := make(chan *providerMergeFailure, 1)
			go func() {
				result <- runProviderMergeTransaction(
					ctx,
					"",
					[]string{provider, marker},
					func() error {
						confirmCalls++
						return tc.confirmationErr
					},
					func() { confirmedCalls++ },
				)
			}()

			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(marker); err == nil {
					break
				} else if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("stat provider marker: %v", err)
				}
				if time.Now().After(deadline) {
					t.Fatal("provider did not start before timeout")
				}
				time.Sleep(10 * time.Millisecond)
			}

			cancel()
			select {
			case failure := <-result:
				if tc.wantFailure {
					if failure == nil || failure.stage != providerMergeConfirmationStage ||
						!errors.Is(failure.err, context.Canceled) {
						t.Fatalf("provider merge failure = %#v, want confirmation-stage context.Canceled", failure)
					}
				} else if failure != nil {
					t.Fatalf("confirmed provider outcome failed: %#v", failure)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("provider command did not stop after cancellation")
			}
			if confirmCalls != 1 || confirmedCalls != tc.wantConfirmedCall {
				t.Fatalf("callback counts = confirm:%d confirmed:%d, want 1:%d",
					confirmCalls, confirmedCalls, tc.wantConfirmedCall)
			}
		})
	}
}

func TestProviderMergeCommandFailureStopsBeforeConfirmationAndCleanup(t *testing.T) {
	fixture := newCleanupFixture(t)
	confirmed := 0
	onConfirmed := 0
	failure := runProviderMergeTransaction(
		context.Background(),
		fixture.branch,
		[]string{filepath.Join(t.TempDir(), "missing-provider")},
		func() error {
			confirmed++
			return nil
		},
		func() { onConfirmed++ },
	)
	if failure == nil || failure.stage != providerMergeCommandStage {
		t.Fatalf("provider merge failure = %#v, want command-stage failure", failure)
	}
	if confirmed != 0 || onConfirmed != 0 {
		t.Fatalf("callbacks after provider failure = confirm:%d confirmed:%d, want 0:0", confirmed, onConfirmed)
	}
	if _, err := os.Stat(fixture.linked); err != nil {
		t.Fatalf("target worktree changed after provider failure: %v", err)
	}
	assertWorktreeRegistration(t, fixture, true)
	assertBranchRef(t, fixture, true)
}

func TestProviderMergeRetriesLocalConfirmationTimeoutWithinTransaction(t *testing.T) {
	fakeDir := t.TempDir()
	provider := filepath.Join(fakeDir, "provider")
	marker := filepath.Join(fakeDir, "provider-calls")
	if err := os.WriteFile(provider, []byte("#!/bin/sh\nprintf 'provider\\n' >> \"$1\"\n"), 0o700); err != nil {
		t.Fatalf("write provider: %v", err)
	}

	confirmCalls := 0
	confirmedCalls := 0
	failure := runProviderMergeTransaction(
		context.Background(),
		"",
		[]string{provider, marker},
		func() error {
			return waitForMergeCompletion(context.Background(), time.Second, time.Millisecond, func() error {
				confirmCalls++
				if confirmCalls == 1 {
					return context.DeadlineExceeded
				}
				return nil
			})
		},
		func() { confirmedCalls++ },
	)
	if failure != nil {
		t.Fatalf("provider merge failure = %#v, want success", failure)
	}
	providerCalls, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read provider marker: %v", err)
	}
	if got := string(providerCalls); got != "provider\n" {
		t.Fatalf("provider calls = %q, want exactly one invocation", got)
	}
	if confirmCalls != 2 || confirmedCalls != 1 {
		t.Fatalf("callback counts = confirm:%d confirmed:%d, want 2:1",
			confirmCalls, confirmedCalls)
	}
}

func TestProviderMergeCleanupHonorsCancellationAfterConfirmation(t *testing.T) {
	fixture := newCleanupFixture(t)
	t.Chdir(fixture.primary)
	provider := filepath.Join(t.TempDir(), "provider")
	if err := os.WriteFile(provider, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write provider: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	confirmed := 0
	failure := runProviderMergeTransaction(
		ctx,
		fixture.branch,
		[]string{provider},
		func() error {
			cancel()
			return nil
		},
		func() { confirmed++ },
	)
	if failure != nil {
		t.Fatalf("confirmed provider merge failed after caller cancellation: %v", failure.err)
	}
	if confirmed != 1 {
		t.Fatalf("confirmed callback count = %d, want 1", confirmed)
	}
	if _, err := os.Stat(fixture.linked); err != nil {
		t.Fatalf("canceled cleanup changed target worktree: %v", err)
	}
	assertWorktreeRegistration(t, fixture, true)
	assertBranchRef(t, fixture, true)
}

func TestProviderMergeTreatsCleanupPreparationFailureAsPostMergeWarning(t *testing.T) {
	fakeDir := t.TempDir()
	for name, script := range map[string]string{
		"git":      "#!/bin/sh\nexit 1\n",
		"provider": "#!/bin/sh\nprintf ran > \"$SAFEGIT_TEST_PROVIDER_MARKER\"\n",
	} {
		if err := os.WriteFile(filepath.Join(fakeDir, name), []byte(script), 0o700); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}
	marker := filepath.Join(t.TempDir(), "provider-ran")
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SAFEGIT_TEST_PROVIDER_MARKER", marker)

	readDiagnostics, writeDiagnostics, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	originalStderr := os.Stderr
	confirmed := 0
	var failure *providerMergeFailure
	func() {
		os.Stderr = writeDiagnostics
		defer func() {
			os.Stderr = originalStderr
			_ = writeDiagnostics.Close()
		}()
		failure = runProviderMergeTransaction(
			context.Background(),
			"cleanup-topic",
			[]string{filepath.Join(fakeDir, "provider")},
			func() error { return nil },
			func() { confirmed++ },
		)
	}()
	diagnostics, readErr := io.ReadAll(readDiagnostics)
	_ = readDiagnostics.Close()
	if readErr != nil {
		t.Fatalf("read stderr capture: %v", readErr)
	}
	if failure != nil {
		t.Fatalf("provider merge failed because cleanup preparation failed: %v", failure.err)
	}
	if confirmed != 1 {
		t.Fatalf("confirmed callback count = %d, want 1", confirmed)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "ran" {
		t.Fatalf("provider marker = %q, %v; want ran", data, err)
	}
	if !strings.Contains(string(diagnostics), "cleanup: pre-merge context: git worktree list") {
		t.Fatalf("cleanup preparation warning missing from stderr: %q", diagnostics)
	}
}

func TestRunCleanupGitBoundsDescendantHeldPipes(t *testing.T) {
	fakeGit := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\n(sleep 30) &\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(fakeGit)+string(os.PathListSeparator)+os.Getenv("PATH"))

	// ErrWaitDelay proves the configured drain bound expired and closed the
	// descendant-held pipes; command-start wall time also includes scheduler delay.
	_, err := runCleanupGit(context.Background(), "", "version")
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("runCleanupGit error = %v, want exec.ErrWaitDelay", err)
	}
}

func TestCleanupWorktreeUsesPrimaryAfterRemovingCallerDirectory(t *testing.T) {
	fixture := newCleanupFixture(t)
	output := runCleanupHelper(t, fixture)

	if _, err := os.Stat(fixture.linked); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("linked worktree path still exists after cleanup: %v", err)
	}
	assertWorktreeRegistration(t, fixture, false)
	assertBranchRef(t, fixture, false)
	assertPrimaryUsable(t, fixture.primary)

	if !strings.Contains(output, "removed local branch "+fixture.branch) {
		t.Fatalf("cleanup did not report successful branch deletion:\n%s", output)
	}
	for _, diagnostic := range []string{
		"getcwd",
		"unable to read current working directory",
		"cannot chdir",
		"no such file or directory",
	} {
		if strings.Contains(strings.ToLower(output), diagnostic) {
			t.Fatalf("cleanup emitted stale-cwd diagnostic %q:\n%s", diagnostic, output)
		}
	}
}

func TestAttemptMergeRetainsCleanupPlanAcrossProviderMutation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		watch bool
	}{
		{name: "one-shot waits for queued merge", watch: false},
		{name: "watch confirms once", watch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newCleanupFixture(t)
			caller := addCleanupCaller(t, fixture)
			output := runAttemptMergeCleanupHelper(t, fixture, caller, attemptMergeOutcomeSuccess, tc.watch)

			if _, err := os.Stat(caller); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("provider did not remove caller worktree: %v", err)
			}
			if _, err := os.Stat(fixture.linked); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("target worktree path still exists after confirmed merge cleanup: %v", err)
			}
			assertWorktreeRegistration(t, fixture, false)
			assertBranchRef(t, fixture, false)
			assertPrimaryUsable(t, fixture.primary)
			if !strings.Contains(output, "removed local branch "+fixture.branch) {
				t.Fatalf("attemptMerge did not retain the pre-provider cleanup plan:\n%s", output)
			}
			if count := strings.Count(output, "safe-merge: ✓ merge complete"); count != 1 {
				t.Fatalf("confirmed merge message count = %d, want 1:\n%s", count, output)
			}
			if confirmedAt, cleanupAt := strings.Index(output, "safe-merge: ✓ merge complete"),
				strings.Index(output, "removed local branch "+fixture.branch); confirmedAt > cleanupAt {
				t.Fatalf("cleanup completed before the confirmed-merge record:\n%s", output)
			}
		})
	}
}

func TestAttemptMergeDoesNotCleanupBeforeExactHeadConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		watch bool
	}{
		{name: "one-shot", watch: false},
		{name: "watch", watch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newCleanupFixture(t)
			caller := addCleanupCaller(t, fixture)
			output := runAttemptMergeCleanupHelper(t, fixture, caller, attemptMergeOutcomeHeadMismatch, tc.watch)

			if _, err := os.Stat(caller); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("provider did not remove caller worktree: %v", err)
			}
			if _, err := os.Stat(fixture.linked); err != nil {
				t.Fatalf("target worktree was removed before exact-head confirmation: %v", err)
			}
			assertWorktreeRegistration(t, fixture, true)
			assertBranchRef(t, fixture, true)
			assertPrimaryUsable(t, fixture.primary)
			if strings.Contains(output, "removed local branch "+fixture.branch) {
				t.Fatalf("attemptMerge cleaned local state before exact-head confirmation:\n%s", output)
			}
			if strings.Contains(output, "safe-merge: ✓ merge complete") {
				t.Fatalf("attemptMerge reported completion before exact-head confirmation:\n%s", output)
			}
		})
	}
}

func TestAttemptMergeProviderFailureDoesNotConfirmOrCleanup(t *testing.T) {
	fixture := newCleanupFixture(t)
	caller := addCleanupCaller(t, fixture)
	output := runAttemptMergeCleanupHelper(
		t,
		fixture,
		caller,
		attemptMergeOutcomeProviderFailure,
		false,
	)

	if _, err := os.Stat(caller); err != nil {
		t.Fatalf("provider failure changed caller worktree: %v", err)
	}
	if _, err := os.Stat(fixture.linked); err != nil {
		t.Fatalf("provider failure changed target worktree: %v", err)
	}
	assertWorktreeRegistration(t, fixture, true)
	assertBranchRef(t, fixture, true)
	assertPrimaryUsable(t, fixture.primary)
	if !strings.Contains(output, "gh pr merge failed") {
		t.Fatalf("attemptMerge did not preserve provider-failure classification:\n%s", output)
	}
	if strings.Contains(output, "safe-merge: ✓ merge complete") ||
		strings.Contains(output, "removed local branch "+fixture.branch) {
		t.Fatalf("attemptMerge confirmed or cleaned after provider failure:\n%s", output)
	}
}

func TestCleanupWorktreePreservesPrimaryPathWhitespace(t *testing.T) {
	fixture := newCleanupFixtureWithPrimaryName(t, "primary\ncheckout \t")
	output := runCleanupHelper(t, fixture)

	if _, err := os.Stat(fixture.linked); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("linked worktree path still exists after cleanup: %v", err)
	}
	assertWorktreeRegistration(t, fixture, false)
	assertBranchRef(t, fixture, false)
	assertPrimaryUsable(t, fixture.primary)
	if !strings.Contains(output, "removed local branch "+fixture.branch) {
		t.Fatalf("cleanup did not preserve the primary worktree path:\n%s", output)
	}
}

func TestCleanupWorktreeWarnsWhenDirtyWorktreeCannotBeRemoved(t *testing.T) {
	fixture := newCleanupFixture(t)
	readme := filepath.Join(fixture.linked, "README.md")
	if err := os.WriteFile(readme, []byte("# dirty linked worktree\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	output := runCleanupHelper(t, fixture)

	if !strings.Contains(output, "cleanup: worktree remove "+fixture.linked+":") {
		t.Fatalf("cleanup did not identify the failed worktree removal:\n%s", output)
	}
	if !strings.Contains(output, "cleanup: branch -d "+fixture.branch+":") {
		t.Fatalf("cleanup did not identify the conservative branch deletion failure:\n%s", output)
	}
	if strings.Contains(output, "removed local branch "+fixture.branch) {
		t.Fatalf("cleanup falsely reported complete branch cleanup:\n%s", output)
	}
	if _, err := os.Stat(fixture.linked); err != nil {
		t.Fatalf("dirty linked worktree path was not preserved: %v", err)
	}
	assertWorktreeRegistration(t, fixture, true)
	assertBranchRef(t, fixture, true)
	assertPrimaryUsable(t, fixture.primary)
}

func TestCleanupWorktreePreservesUnmergedBranchAfterRemovingCleanWorktree(t *testing.T) {
	fixture := newCleanupFixture(t)
	unique := filepath.Join(fixture.linked, "branch-only.txt")
	if err := os.WriteFile(unique, []byte("unmerged branch content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, fixture.linked, "add", "--", "branch-only.txt")
	gittest.Run(t, fixture.linked, "commit", "-m", "branch-only commit")

	output := runCleanupHelper(t, fixture)

	if _, err := os.Stat(fixture.linked); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clean linked worktree path still exists after cleanup: %v", err)
	}
	assertWorktreeRegistration(t, fixture, false)
	assertBranchRef(t, fixture, true)
	assertPrimaryUsable(t, fixture.primary)
	if !strings.Contains(output, "cleanup: branch -d "+fixture.branch+":") {
		t.Fatalf("cleanup did not report conservative unmerged-branch preservation:\n%s", output)
	}
	if strings.Contains(output, "removed local branch "+fixture.branch) {
		t.Fatalf("cleanup forcibly deleted an unmerged branch:\n%s", output)
	}
}

func newCleanupFixture(t *testing.T) cleanupFixture {
	return newCleanupFixtureWithPrimaryName(t, "primary")
}

func newCleanupFixtureWithPrimaryName(t *testing.T, primaryName string) cleanupFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture := cleanupFixture{
		primary: filepath.Join(root, primaryName),
		linked:  filepath.Join(root, "linked-topic"),
		branch:  "cleanup-topic",
	}
	gittest.InitRepo(t, fixture.primary)
	gittest.Run(t, fixture.primary, "branch", fixture.branch)
	gittest.Run(t, fixture.primary, "worktree", "add", "--quiet", fixture.linked, fixture.branch)
	return fixture
}

func addCleanupCaller(t *testing.T, fixture cleanupFixture) string {
	t.Helper()
	caller := filepath.Join(filepath.Dir(fixture.primary), "linked-caller")
	callerBranch := fixture.branch + "-caller"
	gittest.Run(t, fixture.primary, "branch", callerBranch)
	gittest.Run(t, fixture.primary, "worktree", "add", "--quiet", caller, callerBranch)
	return caller
}

func runCleanupHelper(t *testing.T, fixture cleanupFixture) string {
	t.Helper()
	helper := exec.Command(os.Args[0], "-test.run=^TestCleanupWorktreeHelper$")
	helper.Dir = fixture.linked
	helper.Env = append(gittest.Env(t),
		cleanupHelperEnv+"=1",
		cleanupHelperBranchEnv+"="+fixture.branch,
	)
	out, err := helper.CombinedOutput()
	if err != nil {
		t.Fatalf("cleanup helper failed: %v\n%s", err, out)
	}
	return string(out)
}

func runAttemptMergeCleanupHelper(
	t *testing.T,
	fixture cleanupFixture,
	caller string,
	outcome attemptMergeTestOutcome,
	watch bool,
) string {
	t.Helper()
	fakeDir := t.TempDir()
	fakeGH := filepath.Join(fakeDir, "gh")
	script := `#!/bin/sh
set -eu
case "$*" in
  "pr view 42 --repo owner/repo --json number,title,url,state,isDraft,mergeable,mergeStateStatus,reviewDecision,baseRefName,headRefName,headRefOid")
    printf '%s\n' '{"number":42,"title":"t","url":"u","state":"OPEN","isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","reviewDecision":"","baseRefName":"main","headRefName":"cleanup-topic","headRefOid":"abc123"}' ;;
  "pr view 42 --repo owner/repo --json baseRefName")
    printf '%s\n' '{"baseRefName":"main"}' ;;
  "api --paginate --slurp repos/owner/repo/rules/branches/main?per_page=100")
    printf '%s\n' '[[]]' ;;
  "api repos/owner/repo/branches/main/protection/required_status_checks")
    printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2
    exit 1 ;;
  "pr checks 42 --repo owner/repo --required --json name,state"|\
  "pr checks 42 --repo owner/repo --json name,state")
    printf '%s\n' '[]' ;;
  "pr view 42 --repo owner/repo --json headRefOid,commits")
    printf '%s\n' '{"headRefOid":"abc123","commits":[{"committedDate":"2000-01-01T00:00:00Z"}]}' ;;
  "pr view 42 --repo owner/repo --json headRefName,headRefOid")
    printf '%s\n' '{"headRefName":"cleanup-topic","headRefOid":"abc123"}' ;;
  "pr merge 42 --repo owner/repo --squash --auto --delete-branch --match-head-commit abc123")
	printf '%s\n' provider >> "$SAFEGIT_ATTEMPT_MERGE_MARKER"
	if [ "${SAFEGIT_ATTEMPT_MERGE_PROVIDER_FAILURE:-}" = "1" ]; then
	  printf '%s\n' 'synthetic provider failure' >&2
	  exit 9
	fi
    git -C "$SAFEGIT_CLEANUP_PRIMARY" worktree remove --force -- "$SAFEGIT_CLEANUP_CALLER" ;;
  "pr view 42 --repo owner/repo --json state,headRefOid")
	if [ -e "$SAFEGIT_CLEANUP_CALLER" ]; then
	  printf '%s\n' 'caller still exists at confirmation' >&2
	  exit 2
	fi
	printf '%s\n' confirm >> "$SAFEGIT_ATTEMPT_MERGE_MARKER"
	count=0
	if [ -f "$SAFEGIT_ATTEMPT_MERGE_CONFIRM_COUNT" ]; then
	  count=$(cat "$SAFEGIT_ATTEMPT_MERGE_CONFIRM_COUNT")
	fi
	count=$((count + 1))
	printf '%s' "$count" > "$SAFEGIT_ATTEMPT_MERGE_CONFIRM_COUNT"
    if [ "${SAFEGIT_ATTEMPT_MERGE_HEAD_MISMATCH:-}" = "1" ]; then
      printf '%s\n' '{"state":"MERGED","headRefOid":"changed-head"}'
	elif [ "${SAFEGIT_ATTEMPT_MERGE_TRANSIENT_CONFIRMATION:-}" = "1" ] && [ "$count" = "1" ]; then
	  printf '%s\n' 'synthetic confirmation transport failure' >&2
	  exit 7
	elif [ "${SAFEGIT_ATTEMPT_MERGE_DELAY_CONFIRMATION:-}" = "1" ] && [ "$count" = "1" ]; then
	  printf '%s\n' '{"state":"OPEN","headRefOid":"abc123"}'
    else
      printf '%s\n' '{"state":"MERGED","headRefOid":"abc123"}'
    fi ;;
  *)
    printf '%s\n' "unexpected gh invocation: $*" >&2
    exit 2 ;;
esac
`
	if err := os.WriteFile(fakeGH, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "provider-order")
	confirmCount := filepath.Join(t.TempDir(), "confirm-count")
	auditDir := t.TempDir()

	helper := exec.Command(os.Args[0], "-test.run=^TestAttemptMergeCleanupHelper$")
	helper.Dir = caller
	helper.Env = append(gittest.Env(t),
		"PATH="+fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		attemptMergeHelperEnv+"=1",
		cleanupHelperPrimaryEnv+"="+fixture.primary,
		cleanupHelperCallerEnv+"="+caller,
		attemptMergeMarkerEnv+"="+marker,
		attemptMergeConfirmCountEnv+"="+confirmCount,
		"SAFE_MERGE_AUDIT_DIR="+auditDir,
	)
	switch outcome {
	case attemptMergeOutcomeHeadMismatch:
		helper.Env = append(helper.Env, attemptMergeHeadMismatchEnv+"=1")
	case attemptMergeOutcomeProviderFailure:
		helper.Env = append(helper.Env, attemptMergeProviderFailEnv+"=1")
	}
	if watch {
		helper.Env = append(helper.Env, attemptMergeWatchEnv+"=1")
	}
	if outcome == attemptMergeOutcomeSuccess {
		if watch {
			helper.Env = append(helper.Env, attemptMergeTransientEnv+"=1")
		} else {
			helper.Env = append(helper.Env, attemptMergeDelayConfirmEnv+"=1")
		}
	}
	out, err := helper.CombinedOutput()
	if err != nil {
		t.Fatalf("attemptMerge helper failed: %v\n%s", err, out)
	}
	order, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read provider order marker: %v", err)
	}
	wantOrder := "provider\n"
	if outcome != attemptMergeOutcomeProviderFailure {
		wantOrder += "confirm\n"
	}
	if outcome == attemptMergeOutcomeSuccess {
		wantOrder += "confirm\n"
	}
	if got, want := string(order), wantOrder; got != want {
		t.Fatalf("provider order = %q, want %q", got, want)
	}
	auditData, err := os.ReadFile(filepath.Join(auditDir, "safe-merge-audit.jsonl"))
	if err != nil {
		t.Fatalf("read attemptMerge audit: %v", err)
	}
	events := map[string]int{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(auditData)), "\n") {
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("parse attemptMerge audit line %q: %v", line, err)
		}
		events[entry.Event]++
	}
	switch outcome {
	case attemptMergeOutcomeHeadMismatch:
		if events["merge_pending"] != 1 || events["merged"] != 0 || events["error"] != 0 {
			t.Fatalf("head-mismatch audit events = %#v, want one merge_pending and no merged/error", events)
		}
	case attemptMergeOutcomeProviderFailure:
		if events["error"] != 1 || events["merge_pending"] != 0 || events["merged"] != 0 {
			t.Fatalf("provider-failure audit events = %#v, want one error and no merged/merge_pending", events)
		}
	case attemptMergeOutcomeSuccess:
		if events["merged"] != 1 || events["merge_pending"] != 0 || events["error"] != 0 {
			t.Fatalf("confirmed audit events = %#v, want one merged and no merge_pending/error", events)
		}
	}
	return string(out)
}

func assertWorktreeRegistration(t *testing.T, fixture cleanupFixture, want bool) {
	t.Helper()
	list := gittest.Run(t, fixture.primary, "worktree", "list", "--porcelain")
	got := strings.Contains(list, "worktree "+fixture.linked+"\n")
	if got != want {
		t.Fatalf("linked worktree registration present = %t, want %t:\n%s", got, want, list)
	}
}

func assertBranchRef(t *testing.T, fixture cleanupFixture, want bool) {
	t.Helper()
	refs := strings.TrimSpace(gittest.Run(t, fixture.primary,
		"for-each-ref", "--format=%(refname)", "refs/heads/"+fixture.branch))
	got := refs == "refs/heads/"+fixture.branch
	if got != want {
		t.Fatalf("topic branch ref present = %t, want %t (refs %q)", got, want, refs)
	}
}

func assertPrimaryUsable(t *testing.T, primary string) {
	t.Helper()
	if head := strings.TrimSpace(gittest.Run(t, primary, "rev-parse", "--verify", "HEAD")); head == "" {
		t.Fatal("primary worktree has no usable HEAD after cleanup")
	}
	gittest.Run(t, primary, "status", "--porcelain")
}
