package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/agenticreview"
)

// repoConfig is the checked-in repository policy: three families, quorum two.
// The fixtures below are evaluated against the real policy, not a test-local
// one, so a change to the shipped quorum shows up here as a failing receipt.
func repoConfig(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", agenticreview.DefaultConfigPath)
}

func runGate(t *testing.T, fixture string, extra ...string) (int, string) {
	t.Helper()
	args := append([]string{
		"--config", repoConfig(t),
		"--input-file", filepath.Join("testdata", fixture),
		"--json",
	}, extra...)
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	if errOut.Len() > 0 {
		t.Logf("stderr: %s", errOut.String())
	}
	return code, out.String()
}

func decodeVerdict(t *testing.T, stdout string) agenticreview.Verdict {
	t.Helper()
	var v agenticreview.Verdict
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("decode verdict %q: %v", stdout, err)
	}
	return v
}

// The four scenarios the gate is specified against, evaluated end to end
// through the command against the repository's own policy file.
func TestGateFixtures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fixture  string
		wantCode int
		wantDec  agenticreview.Decision
	}{
		{"all three approve is mergeable", "all-three-approved.json", exitPass, agenticreview.DecisionPass},
		{"codex requests changes while gemini approves is blocked", "codex-changes-requested.json", exitBlocked, agenticreview.DecisionBlock},
		{"one reviewer down plus two approvals is mergeable", "one-down-two-approved.json", exitPass, agenticreview.DecisionPass},
		{"ready with no started label is blocked", "ready-no-started.json", exitPending, agenticreview.DecisionPending},
		{"started without posted is blocked", "started-not-posted.json", exitPending, agenticreview.DecisionPending},
		{"silent reviewer past its timeout degrades to a quorum pass", "codex-silent-past-timeout.json", exitPass, agenticreview.DecisionPass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout := runGate(t, tc.fixture)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d (stdout: %s)", code, tc.wantCode, stdout)
			}
			v := decodeVerdict(t, stdout)
			if v.Decision != tc.wantDec {
				t.Fatalf("decision = %s, want %s (%s)", v.Decision, tc.wantDec, v.Reason)
			}
			if v.Mergeable() != (tc.wantCode == exitPass) {
				t.Fatalf("Mergeable() = %v but exit code was %d", v.Mergeable(), code)
			}
		})
	}
}

// The blocked-by-changes fixture must name the family that blocked, or an
// operator reading a red required check learns nothing actionable from it.
func TestGateNamesTheBlockingFamily(t *testing.T) {
	_, stdout := runGate(t, "codex-changes-requested.json")
	v := decodeVerdict(t, stdout)
	if !strings.Contains(v.Reason, "codex") {
		t.Fatalf("reason %q does not name codex", v.Reason)
	}
	for _, fv := range v.Families {
		if fv.Family == agenticreview.FamilyClaude && fv.State != agenticreview.StateApproved {
			t.Fatalf("claude state = %s; a blocking sibling must not erase another family's approval", fv.State)
		}
	}
}

// Raising the quorum to unanimity withdraws the degradation allowance, so the
// scenario that passed on two-of-three now blocks.
func TestGateQuorumOverrideIsHonoured(t *testing.T) {
	code, stdout := runGate(t, "one-down-two-approved.json", "--quorum", "3")
	if code == exitPass {
		t.Fatalf("exit = %d, want a refusal at quorum 3 (stdout: %s)", code, stdout)
	}
	if v := decodeVerdict(t, stdout); v.Quorum != 3 {
		t.Fatalf("verdict quorum = %d, want 3", v.Quorum)
	}
}

// A human-readable summary is what lands in the check output, so it has to
// carry the decision and every family's state.
func TestGateTextSummaryListsEveryFamily(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{
		"--config", repoConfig(t),
		"--input-file", filepath.Join("testdata", "one-down-two-approved.json"),
	}, &out, &errOut)
	if code != exitPass {
		t.Fatalf("exit = %d, want %d", code, exitPass)
	}
	summary := out.String()
	for _, want := range []string{"PASS", "claude", "codex", "gemini", "APPROVED", "DOWN"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary %q missing %q", summary, want)
		}
	}
}

func TestGateRejectsUsageErrors(t *testing.T) {
	for name, args := range map[string][]string{
		"no source":       {"--config", "x"},
		"both sources":    {"--input-file", "a", "--repo", "o/r", "--pr", "1"},
		"pr without repo": {"--pr", "1"},
		"bad quorum":      {"--input-file", "a", "--quorum", "nope"},
	} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != exitUsage {
			t.Errorf("%s: exit = %d, want %d", name, code, exitUsage)
		}
	}
}

// A missing or unparseable policy file fails closed with a usage error rather
// than falling back to a built-in default nobody chose.
func TestGateFailsClosedOnUnreadableConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{
		"--config", filepath.Join(t.TempDir(), "absent.yml"),
		"--input-file", filepath.Join("testdata", "all-three-approved.json"),
	}, &out, &errOut)
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
}

// fakeGH replays recorded gh output so the fetch path is tested without a
// network or a token.
type fakeGH struct {
	responses map[string]string
	calls     []string
}

func (f *fakeGH) run(args []string) (string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	for prefix, body := range f.responses {
		if strings.Contains(key, prefix) {
			return body, nil
		}
	}
	return "", errUnexpectedCall{key}
}

type errUnexpectedCall struct{ key string }

func (e errUnexpectedCall) Error() string { return "unexpected gh call: " + e.key }

func TestFetchInputBuildsLabelsTimesAndReadiness(t *testing.T) {
	gh := &fakeGH{responses: map[string]string{
		"pr view": `{"number":7,"headRefOid":"abc123","createdAt":"2026-09-03T09:00:00Z","isDraft":false,
			"labels":[{"name":"agentic-review:claude:started"},{"name":"agentic-review:claude:approved"},{"name":"unrelated"}]}`,
		"issues/7/timeline": `[
			{"event":"ready_for_review","created_at":"2026-09-03T10:00:00Z"},
			{"event":"labeled","created_at":"2026-09-03T10:01:00Z","label":{"name":"agentic-review:claude:started"}},
			{"event":"labeled","created_at":"2026-09-03T10:02:00Z","label":{"name":"agentic-review:gemini:started"}},
			{"event":"unlabeled","created_at":"2026-09-03T10:03:00Z","label":{"name":"agentic-review:gemini:started"}},
			{"event":"labeled","created_at":"2026-09-03T10:04:00Z","label":{"name":"agentic-review:claude:approved"}}
		]`,
		"commits/abc123": `{"commit":{"committer":{"date":"2026-09-03T09:30:00Z"}}}`,
	}}

	in, err := fetchInput(gh.run, "o/r", 7)
	if err != nil {
		t.Fatalf("fetchInput: %v", err)
	}

	if len(in.Labels) != 3 {
		t.Fatalf("labels = %v, want the three live labels", in.Labels)
	}
	// An unlabeled event must remove its timestamp: a stale started time would
	// let a family that was reset look like it had been running all along.
	if _, ok := in.AppliedAt["agentic-review:gemini:started"]; ok {
		t.Error("an unlabeled event left its timestamp behind")
	}
	want := time.Date(2026, 9, 3, 10, 1, 0, 0, time.UTC)
	if got := in.AppliedAt["agentic-review:claude:started"]; !got.Equal(want) {
		t.Errorf("claude started at %s, want %s", got, want)
	}
	// Readiness is the latest of the ready event and the head commit, so a push
	// onto an already-ready pull request restarts the dispatch window.
	if wantReady := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC); !in.ReadyAt.Equal(wantReady) {
		t.Errorf("ReadyAt = %s, want %s", in.ReadyAt, wantReady)
	}
	if in.Now.IsZero() {
		t.Error("fetchInput left Now unset")
	}
}

// A head pushed after the pull request went ready moves the readiness clock
// forward: the labels were cleared by that push, so the dispatch window must
// restart rather than being already half spent.
func TestFetchInputReadinessFollowsANewerHead(t *testing.T) {
	gh := &fakeGH{responses: map[string]string{
		"pr view":           `{"number":7,"headRefOid":"def456","createdAt":"2026-09-03T09:00:00Z","isDraft":false,"labels":[]}`,
		"issues/7/timeline": `[{"event":"ready_for_review","created_at":"2026-09-03T10:00:00Z"}]`,
		"commits/def456":    `{"commit":{"committer":{"date":"2026-09-03T11:30:00Z"}}}`,
	}}

	in, err := fetchInput(gh.run, "o/r", 7)
	if err != nil {
		t.Fatalf("fetchInput: %v", err)
	}
	if want := time.Date(2026, 9, 3, 11, 30, 0, 0, time.UTC); !in.ReadyAt.Equal(want) {
		t.Fatalf("ReadyAt = %s, want the newer head time %s", in.ReadyAt, want)
	}
}

// A draft pull request has not gone ready, so it has no dispatch clock and its
// families stay missing rather than degrading to down.
func TestFetchInputLeavesDraftReadinessUnset(t *testing.T) {
	gh := &fakeGH{responses: map[string]string{
		"pr view":           `{"number":7,"headRefOid":"abc123","createdAt":"2026-09-03T09:00:00Z","isDraft":true,"labels":[]}`,
		"issues/7/timeline": `[]`,
		"commits/abc123":    `{"commit":{"committer":{"date":"2026-09-03T09:30:00Z"}}}`,
	}}

	in, err := fetchInput(gh.run, "o/r", 7)
	if err != nil {
		t.Fatalf("fetchInput: %v", err)
	}
	if !in.ReadyAt.IsZero() {
		t.Fatalf("ReadyAt = %s on a draft, want the zero time", in.ReadyAt)
	}
}

// The gate must never invoke a model. Its whole reason for existing is that a
// quota incident degrades the review and not the ability to decide whether a
// review happened, so every call it makes is asserted here.
func TestFetchInputCallsOnlyCheapGitHubReads(t *testing.T) {
	gh := &fakeGH{responses: map[string]string{
		"pr view":           `{"number":7,"headRefOid":"abc123","createdAt":"2026-09-03T09:00:00Z","isDraft":false,"labels":[]}`,
		"issues/7/timeline": `[]`,
		"commits/abc123":    `{"commit":{"committer":{"date":"2026-09-03T09:30:00Z"}}}`,
	}}

	if _, err := fetchInput(gh.run, "o/r", 7); err != nil {
		t.Fatalf("fetchInput: %v", err)
	}
	if len(gh.calls) != 3 {
		t.Fatalf("gh calls = %v, want exactly three reads", gh.calls)
	}
	for _, call := range gh.calls {
		for _, forbidden := range []string{"claude", "gemini", "codex", "anthropic", "openai", "models"} {
			if strings.Contains(strings.ToLower(call), forbidden) {
				t.Errorf("gate reached a model surface: %q", call)
			}
		}
	}
}

func TestFetchInputSurfacesGitHubFailures(t *testing.T) {
	gh := &fakeGH{responses: map[string]string{}}
	if _, err := fetchInput(gh.run, "o/r", 7); err == nil {
		t.Fatal("fetchInput swallowed a gh failure")
	}
}

// Pending and blocked must not collapse into one code. The gate job waits on a
// lifecycle that has not resolved and stops immediately on one that decided
// against the merge; a single code would mean either waiting out a deadline on
// a decided rejection or abandoning a review still in flight.
func TestGateSeparatesPendingFromBlocked(t *testing.T) {
	pending, _ := runGate(t, "ready-no-started.json")
	blocked, _ := runGate(t, "codex-changes-requested.json")
	if pending == blocked {
		t.Fatalf("pending and blocked both exited %d", pending)
	}
	if pending == exitPass || blocked == exitPass {
		t.Fatal("a refusal exited zero")
	}
}
