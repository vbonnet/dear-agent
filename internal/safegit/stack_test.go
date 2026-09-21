package safegit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseStackMembership_NullStackIsNotStacked(t *testing.T) {
	stacked, err := parseStackMembership([]byte(`{"number":1412,"stack":null}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stacked {
		t.Fatal("a null stack must not be read as stack membership — it would " +
			"route an ordinary PR to the async transport")
	}
}

func TestParseStackMembership_AbsentStackIsNotStacked(t *testing.T) {
	stacked, err := parseStackMembership([]byte(`{"number":1510}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stacked {
		t.Fatal("an absent stack key must not be read as stack membership")
	}
}

func TestParseStackMembership_StackObjectIsStacked(t *testing.T) {
	payload := []byte(`{"number":1411,"stack":{"id":1212736,"number":1528,"position":1,"size":3}}`)
	stacked, err := parseStackMembership(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stacked {
		t.Fatal("a stack object must be read as stack membership — GraphQL merge " +
			"is refused for these PRs and the merge would fail at the transport")
	}
}

func TestParseStackMembership_RejectsInvalidPayload(t *testing.T) {
	if _, err := parseStackMembership([]byte(`not json`)); err == nil {
		t.Fatal("an unparseable PR payload must be an error, not a silent 'not stacked'")
	}
}

func TestBuildAsyncMergeArgs_AnchorsHeadSHA(t *testing.T) {
	args := BuildAsyncMergeArgs(1411, "vbonnet/dear-agent", "abc123def456")

	if !containsArg(args, "sha=abc123def456") {
		t.Fatalf("async merge must anchor the exact head SHA (TOCTOU parity with "+
			"--match-head-commit); got: %v", args)
	}
	if !containsArg(args, "merge_method=squash") {
		t.Fatalf("async merge must request a squash merge — it is the only method "+
			"the ruleset allows; got: %v", args)
	}
}

func TestBuildAsyncMergeArgs_TargetsAsyncRESTEndpoint(t *testing.T) {
	args := BuildAsyncMergeArgs(1411, "vbonnet/dear-agent", "deadbeef")

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "repos/vbonnet/dear-agent/pulls/1411/merge-async") {
		t.Fatalf("async merge must target the REST merge endpoint; got: %s", joined)
	}
	if !containsArg(args, "PUT") {
		t.Fatalf("the REST merge endpoint requires PUT; got: %s", joined)
	}
	for _, forbidden := range []string{"pr", "--match-head-commit"} {
		if containsArg(args, forbidden) {
			t.Fatalf("async merge must not reuse the GraphQL argv (%q); GitHub "+
				"refuses that mutation for stacked PRs; got: %s", forbidden, joined)
		}
	}
}

func TestBuildAsyncMergeArgs_PanicsOnEmptySHA(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("BuildAsyncMergeArgs must panic on empty headSHA — an empty " +
				"anchor would silently defeat the TOCTOU protection")
		}
	}()
	BuildAsyncMergeArgs(1, "o/r", "")
}

func TestBuildAsyncMergeArgs_EscapesNothingIntoTheSHASlot(t *testing.T) {
	// The SHA travels as a single -f value, so it can never be read as a flag.
	args := BuildAsyncMergeArgs(7, "o/r", "--delete-branch")
	if !containsArg(args, "sha=--delete-branch") {
		t.Fatalf("the head SHA must travel as one -f value; got: %v", args)
	}
}

// fakeGHStackProbe puts a gh on PATH that answers the stack-membership probe
// with the given payload, so transport selection can be tested end to end.
func fakeGHStackProbe(t *testing.T, payload string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = \"api\" ]; then printf '%s\\n' '" + payload +
		"'; exit 0; fi\nprintf 'unexpected gh invocation: %s\\n' \"$*\" >&2\nexit 2\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestResolveStackMembership_StackedPRUsesAsyncTransport(t *testing.T) {
	fakeGHStackProbe(t, `{"number":1411,"stack":{"id":1212736,"number":1528,"position":1,"size":3}}`)

	stacked, err := resolveStackMembership(context.Background(), 1411, "vbonnet/dear-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stacked {
		t.Fatal("a PR with a stack object must be routed as stacked")
	}
	args := mergeArgsForTransport(stacked, 1411, "vbonnet/dear-agent", "abc123")
	if !containsArg(args, "PUT") || containsArg(args, "pr") {
		t.Fatalf("stacked PR must merge through the async REST endpoint; got: %v", args)
	}
}

func TestResolveStackMembership_OrdinaryPRKeepsGraphQLTransport(t *testing.T) {
	fakeGHStackProbe(t, `{"number":1510,"stack":null}`)

	stacked, err := resolveStackMembership(context.Background(), 1510, "vbonnet/dear-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stacked {
		t.Fatal("a PR with a null stack must not be routed as stacked")
	}
	args := mergeArgsForTransport(stacked, 1510, "vbonnet/dear-agent", "abc123")
	if !containsArg(args, "--match-head-commit") {
		t.Fatalf("ordinary PRs must keep the GraphQL path and its TOCTOU anchor; got: %v", args)
	}
}

func TestResolveStackMembership_ProbeFailureIsReportedNotGuessed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := resolveStackMembership(context.Background(), 1, "o/r"); err == nil {
		t.Fatal("an unresolvable stack membership must fail loudly — defaulting " +
			"either way merges through a transport the provider may refuse")
	}
}

// mergeArgsForTransport must not talk to the provider. The stack probe runs
// before the base-freshness gate precisely so that no provider round trip sits
// between the freshness proof and the merge; a probe there would let the base
// advance unchecked for the probe's whole timeout.
func TestMergeArgsForTransport_MakesNoProviderCall(t *testing.T) {
	dir := t.TempDir()
	// A gh that fails loudly if anything invokes it during argv construction.
	if err := os.WriteFile(filepath.Join(dir, "gh"),
		[]byte("#!/bin/sh\necho 'gh must not run between the freshness gate and the merge' >&2\nexit 3\n"),
		0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, stacked := range []bool{true, false} {
		args := mergeArgsForTransport(stacked, 42, "o/r", "abc123")
		if len(args) == 0 {
			t.Fatalf("stacked=%v: expected merge argv", stacked)
		}
		if !containsArg(args, "sha=abc123") && !containsArg(args, "abc123") {
			t.Fatalf("stacked=%v: merge argv lost the head anchor: %v", stacked, args)
		}
	}
}

// The stack probe runs on every merge, so an unbounded wait here would stall
// every merge, not just this one. It must bound pipe draining the way the other
// mandatory provider read does.
func TestResolveStackMembership_BoundsDescendantHeldPipe(t *testing.T) {
	dir := t.TempDir()
	// gh exits immediately but leaves a descendant holding stdout.
	script := "#!/bin/sh\nsleep 30 &\nprintf '%s\\n' '{\"number\":1,\"stack\":null}'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	done := make(chan error, 1)
	go func() {
		_, err := resolveStackMembership(context.Background(), 1, "o/r")
		done <- err
	}()
	select {
	case <-done:
		// Returned promptly: the WaitDelay bound did its job.
	case <-time.After(20 * time.Second):
		t.Fatal("stack probe blocked on a descendant-held pipe; it must set a " +
			"finite WaitDelay or a hung credential helper stalls every merge")
	}
}

func TestDeleteRemoteHeadBranch_RequiresABranch(t *testing.T) {
	if err := deleteRemoteHeadBranch(context.Background(), "o/r", "", mergedOID); err == nil {
		t.Fatal("deleting an unnamed remote head must be an error, not a silent no-op")
	}
}

func TestDeleteRemoteHeadBranch_TreatsAlreadyDeletedAsSuccess(t *testing.T) {
	dir := t.TempDir()
	// delete_branch_on_merge may have removed it first; that is not a failure.
	script := "#!/bin/sh\nprintf '%s\\n' 'gh: Not Found (HTTP 404)' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := deleteRemoteHeadBranch(context.Background(), "o/r", "topic", mergedOID); err != nil {
		t.Fatalf("an already-deleted remote head must be success; got %v", err)
	}
}

func TestDeleteRemoteHeadBranch_EscapesSlashesInBranchName(t *testing.T) {
	// A slash-bearing branch must not be split into a different ref path.
	deleted := fakeGHRefServer(t, "fix/some-topic", mergedOID)

	if err := deleteRemoteHeadBranch(context.Background(), "o/r", "fix/some-topic", mergedOID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(deleted)
	if err != nil {
		t.Fatalf("read recorded delete args: %v", err)
	}
	if !strings.Contains(string(got), "refs/heads/fix%2Fsome-topic") {
		t.Fatalf("branch name must be path-escaped so it cannot address a different ref; got %s", got)
	}
}

const mergedOID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func fakeGHRefServer(t *testing.T, branch, refOID string) (deletedMarker string) {
	t.Helper()
	dir := t.TempDir()
	deletedMarker = filepath.Join(dir, "deleted")
	script := `#!/bin/sh
for a in "$@"; do
  if [ "$a" = "DELETE" ]; then
    printf '%s' "$*" > ` + deletedMarker + `
    printf '{}'
    exit 0
  fi
done
printf '{"ref":"refs/heads/` + branch + `","object":{"sha":"` + refOID + `","type":"commit"}}'
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return deletedMarker
}

func TestDeleteRemoteHeadBranch_RequiresAMergedCommit(t *testing.T) {
	if err := deleteRemoteHeadBranch(context.Background(), "o/r", "topic", ""); err == nil {
		t.Fatal("deleting without a merged commit must be an error — an unguarded " +
			"delete can discard commits pushed after the merge")
	}
}

// An async merge can complete between confirmation polls. If the branch moved
// in that gap, deleting it would destroy work the merge never contained.
func TestDeleteRemoteHeadBranch_RefusesWhenBranchAdvanced(t *testing.T) {
	deleted := fakeGHRefServer(t, "topic", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	err := deleteRemoteHeadBranch(context.Background(), "o/r", "topic", mergedOID)
	if !errors.Is(err, errRemoteHeadAdvanced) {
		t.Fatalf("error = %v, want errRemoteHeadAdvanced", err)
	}
	if _, statErr := os.Stat(deleted); statErr == nil {
		t.Fatal("an advanced branch must not be deleted — that discards post-merge commits")
	}
}

func TestDeleteRemoteHeadBranch_DeletesWhenStillAtMergedCommit(t *testing.T) {
	deleted := fakeGHRefServer(t, "topic", mergedOID)

	if err := deleteRemoteHeadBranch(context.Background(), "o/r", "topic", mergedOID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(deleted); statErr != nil {
		t.Fatal("a branch still at the merged commit must be deleted")
	}
}
