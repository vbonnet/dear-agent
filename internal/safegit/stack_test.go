package safegit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestSelectMergeArgs_StackedPRUsesAsyncTransport(t *testing.T) {
	fakeGHStackProbe(t, `{"number":1411,"stack":{"id":1212736,"number":1528,"position":1,"size":3}}`)

	args, stacked, err := selectMergeArgs(context.Background(), 1411, "vbonnet/dear-agent", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stacked {
		t.Fatal("a PR with a stack object must be routed as stacked")
	}
	if !containsArg(args, "PUT") || containsArg(args, "merge") && containsArg(args, "pr") {
		t.Fatalf("stacked PR must merge through the async REST endpoint; got: %v", args)
	}
}

func TestSelectMergeArgs_OrdinaryPRKeepsGraphQLTransport(t *testing.T) {
	fakeGHStackProbe(t, `{"number":1510,"stack":null}`)

	args, stacked, err := selectMergeArgs(context.Background(), 1510, "vbonnet/dear-agent", "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stacked {
		t.Fatal("a PR with a null stack must not be routed as stacked")
	}
	if !containsArg(args, "--match-head-commit") {
		t.Fatalf("ordinary PRs must keep the GraphQL path and its TOCTOU anchor; got: %v", args)
	}
}

func TestSelectMergeArgs_ProbeFailureIsReportedNotGuessed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, _, err := selectMergeArgs(context.Background(), 1, "o/r", "abc123"); err == nil {
		t.Fatal("an unresolvable stack membership must fail loudly — defaulting " +
			"either way merges through a transport the provider may refuse")
	}
}
