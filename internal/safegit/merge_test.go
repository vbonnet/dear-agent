package safegit

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
)

// --- parseCheckRuns ---

func TestParseCheckRuns_AllChecksPass(t *testing.T) {
	data := marshalJSON([]checkRun{
		{Name: "Build", State: "success"},
		{Name: "Lint", State: "pass"},
		{Name: "Optional", State: "success"},
	})
	if err := parseCheckRuns(data); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestParseCheckRuns_NonRequiredFails(t *testing.T) {
	// parseCheckRuns validates every check in its input.
	// The caller (checkAllCI) pre-filters to required checks in production, so
	// non-required fleet-wide failures never reach this function in practice.
	data := marshalJSON([]checkRun{
		{Name: "Build", State: "success"},
		{Name: "Optional", State: "failure"},
	})
	err := parseCheckRuns(data)
	if err == nil {
		t.Fatal("expected error: non-required failing check should also block merge")
	}
	if !strings.Contains(err.Error(), "Optional") {
		t.Errorf("error should mention failing check name, got: %v", err)
	}
}

func TestParseCheckRuns_RequiredFails(t *testing.T) {
	data := marshalJSON([]checkRun{
		{Name: "Build", State: "failure"},
		{Name: "Lint", State: "success"},
	})
	err := parseCheckRuns(data)
	if err == nil {
		t.Fatal("expected error for failing required check, got nil")
	}
	if !strings.Contains(err.Error(), "Build") {
		t.Errorf("error should mention failing check name, got: %v", err)
	}
}

func TestParseCheckRuns_RequiredPending(t *testing.T) {
	data := marshalJSON([]checkRun{
		{Name: "Build", State: "pending"},
	})
	err := parseCheckRuns(data)
	if err == nil {
		t.Fatal("expected error for pending required check, got nil")
	}
	if !strings.Contains(err.Error(), "pending") {
		t.Errorf("error should mention 'pending', got: %v", err)
	}
}

func TestParseCheckRuns_SkippingIsOK(t *testing.T) {
	data := marshalJSON([]checkRun{
		{Name: "Optional", State: "skipping"},
	})
	if err := parseCheckRuns(data); err != nil {
		t.Fatalf("skipping check should be acceptable, got: %v", err)
	}
}

func TestParseCheckRuns_SkippedIsOK(t *testing.T) {
	// gh pr checks --json returns "SKIPPED" (uppercase) for skipped checks.
	data := marshalJSON([]checkRun{
		{Name: "Generate SBOM", State: "SKIPPED"},
	})
	if err := parseCheckRuns(data); err != nil {
		t.Fatalf("SKIPPED check should be acceptable, got: %v", err)
	}
}

func TestParseCheckRuns_NeutralIsOK(t *testing.T) {
	data := marshalJSON([]checkRun{
		{Name: "Benchmark", State: "neutral"},
	})
	if err := parseCheckRuns(data); err != nil {
		t.Fatalf("neutral check should be acceptable, got: %v", err)
	}
}

func TestParseCheckRuns_Empty(t *testing.T) {
	data := marshalJSON([]checkRun{})
	if err := parseCheckRuns(data); err != nil {
		t.Fatalf("empty check list should pass, got: %v", err)
	}
}

func TestParseAppliedRulesRequiredChecks(t *testing.T) {
	data := []byte(`[[
		{"type":"pull_request","parameters":{"required_approving_review_count":0}},
		{"type":"required_status_checks","parameters":{"required_status_checks":[
			{"context":"Build"},{"context":"Lint","integration_id":42},{"context":""}
		]}}
	],[
		{"type":"required_status_checks","parameters":{"required_status_checks":[
			{"context":"Build"},{"context":"Security"}
		]}}
	]]`)
	policy, err := parseAppliedRulesRequiredChecks(data)
	if err != nil {
		t.Fatalf("parseAppliedRulesRequiredChecks() error = %v", err)
	}
	contexts := policy.contexts()
	for _, name := range []string{"Build", "Lint", "Security"} {
		if !contexts[name] {
			t.Errorf("required check %q missing from %#v", name, policy.Identities)
		}
	}
	if len(contexts) != 3 {
		t.Fatalf("required check count = %d, want 3: %#v", len(contexts), policy.Identities)
	}
	if !policy.Identities[requiredCheckIdentity{Context: "Lint", IntegrationID: 42, Scoped: true}] {
		t.Fatal("integration-scoped required check identity was discarded")
	}
}

func TestParseAppliedRulesRequiredChecksRejectsMalformedJSON(t *testing.T) {
	if _, err := parseAppliedRulesRequiredChecks([]byte(`{"type":`)); err == nil {
		t.Fatal("expected malformed applied-rules JSON to fail closed")
	}
}

func TestParseAppliedRulesRequiredChecksKnownEmpty(t *testing.T) {
	policy, err := parseAppliedRulesRequiredChecks([]byte(`[[]]`))
	if err != nil {
		t.Fatalf("known-empty applied rules error = %v", err)
	}
	if len(policy.Identities) != 0 || policy.HasRequiredWorkflows {
		t.Fatalf("known-empty policy = %#v", policy)
	}
}

func TestParseAppliedRulesRequiredChecksFlagsRequiredWorkflows(t *testing.T) {
	policy, err := parseAppliedRulesRequiredChecks([]byte(`[[{"type":"workflows","parameters":{}}]]`))
	if err != nil {
		t.Fatalf("required workflow parse error = %v", err)
	}
	if !policy.HasRequiredWorkflows {
		t.Fatal("required workflow rule must block until missing runs can be proven")
	}
}

func TestParseClassicRequiredChecksPreservesIntegrationScope(t *testing.T) {
	policy, err := parseClassicRequiredChecks([]byte(`{
		"contexts":["Legacy"],
		"checks":[{"context":"Build","app_id":99}]
	}`))
	if err != nil {
		t.Fatalf("classic required check parse error = %v", err)
	}
	contexts := policy.contexts()
	if !contexts["Legacy"] || !contexts["Build"] {
		t.Fatalf("classic contexts = %#v", policy.Identities)
	}
	if !policy.Identities[requiredCheckIdentity{Context: "Build", IntegrationID: 99, Scoped: true}] {
		t.Fatal("classic app-scoped identity was discarded")
	}
}

func TestMergeRequiredCheckPoliciesUnionsLayeredSources(t *testing.T) {
	ruleset := newRequiredCheckPolicy()
	ruleset.add("Ruleset", nil)
	classic := newRequiredCheckPolicy()
	integrationID := int64(7)
	classic.add("Classic", &integrationID)
	merged := mergeRequiredCheckPolicies(ruleset, classic)
	if !merged.contexts()["Ruleset"] || !merged.Identities[requiredCheckIdentity{Context: "Classic", IntegrationID: 7, Scoped: true}] {
		t.Fatalf("layered policy was not unioned: %#v", merged)
	}
}

func TestDiscoverRequiredChecksAcceptsAuthoritativeEmpty(t *testing.T) {
	installRequiredCheckFakeGH(t, `
case "$*" in
  *rules/branches*) printf '%s\n' '[[]]' ;;
  *protection/required_status_checks*) printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  *) printf '%s\n' 'unexpected gh invocation' >&2; exit 2 ;;
esac
`)
	policy, err := discoverRequiredChecks("owner/repo", "feature/base")
	if err != nil {
		t.Fatalf("authoritative empty policy error = %v", err)
	}
	if len(policy.Identities) != 0 || policy.HasRequiredWorkflows {
		t.Fatalf("authoritative empty policy = %#v", policy)
	}
}

func TestDiscoverRequiredChecksRejectsPartialPolicyOnSourceError(t *testing.T) {
	installRequiredCheckFakeGH(t, `
case "$*" in
  *rules/branches*) printf '%s\n' 'gh: provider unavailable (HTTP 500)' >&2; exit 1 ;;
  *protection/required_status_checks*) printf '%s\n' '{"contexts":["Classic"]}' ;;
  *) printf '%s\n' 'unexpected gh invocation' >&2; exit 2 ;;
esac
`)
	if _, err := discoverRequiredChecks("owner/repo", "main"); err == nil {
		t.Fatal("partial classic policy must not be accepted when ruleset discovery fails")
	}
}

func installRequiredCheckFakeGH(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	script := "#!/bin/sh\nset -eu\n" + body
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRulesBranchEndpointEscapesSlashBase(t *testing.T) {
	got := rulesBranchEndpoint("owner/repo", "release/v1")
	want := "repos/owner/repo/rules/branches/release%2Fv1?per_page=100"
	if got != want {
		t.Fatalf("rules endpoint = %q, want %q", got, want)
	}
}

func TestDiscoverRequiredChecksUsesPaginatedSlurp(t *testing.T) {
	installRequiredCheckFakeGH(t, `
if [ "$1" = api ] && [ "$2" = --paginate ] && [ "$3" = --slurp ]; then
  printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Build"}]}}]]'
elif [ "$1" = api ] && printf '%s' "$2" | grep -q 'protection/required_status_checks'; then
  printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2
  exit 1
else
  printf '%s\n' "unexpected gh invocation: $*" >&2
  exit 2
fi
`)
	policy, err := discoverRequiredChecks("owner/repo", "main")
	if err != nil {
		t.Fatalf("paginated discovery error = %v", err)
	}
	if !policy.contexts()["Build"] {
		t.Fatalf("paginated discovery policy = %#v", policy)
	}
}

func TestProviderRequiredClassificationIgnoresAdvisoryFailure(t *testing.T) {
	policy := newRequiredCheckPolicy()
	policy.add("Build", nil)
	provider := []checkRun{{Name: "Build", State: "success"}}
	classified, err := classifyProviderRequiredChecks(provider, policy)
	if err != nil {
		t.Fatalf("classify provider-required checks: %v", err)
	}
	if err := parseCheckRuns(marshalJSON(classified)); err != nil {
		t.Fatalf("green provider-required check should pass despite an advisory failure outside the provider projection: %v", err)
	}
}

func TestProviderRequiredClassificationBlocksRequiredFailurePendingAndMissing(t *testing.T) {
	tests := []struct {
		name     string
		provider []checkRun
		expected map[string]bool
		want     string
	}{
		{name: "failed", provider: []checkRun{{Name: "Build", State: "failure"}}, expected: map[string]bool{"Build": true}, want: "Build"},
		{name: "pending", provider: []checkRun{{Name: "Build", State: "pending"}}, expected: map[string]bool{"Build": true}, want: "pending"},
		{name: "missing", provider: nil, expected: map[string]bool{"Build": true}, want: "pending"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			policy := newRequiredCheckPolicy()
			for context := range tc.expected {
				policy.add(context, nil)
			}
			classified, classifyErr := classifyProviderRequiredChecks(tc.provider, policy)
			if classifyErr != nil {
				t.Fatalf("classification error = %v", classifyErr)
			}
			err := parseCheckRuns(marshalJSON(classified))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestProviderRequiredClassificationRejectsAmbiguousIntegrationIdentity(t *testing.T) {
	policy := newRequiredCheckPolicy()
	first, second := int64(1), int64(2)
	policy.add("Build", &first)
	policy.add("Build", &second)
	_, err := classifyProviderRequiredChecks([]checkRun{{Name: "Build", State: "success"}}, policy)
	if err == nil || !strings.Contains(err.Error(), "multiple required integration identities") {
		t.Fatalf("one present and one missing integration identity error = %v", err)
	}
}

func TestProviderRequiredClassificationRejectsDiscoveryDisagreement(t *testing.T) {
	tests := []struct {
		name   string
		policy requiredCheckPolicy
	}{
		{name: "configured policy", policy: func() requiredCheckPolicy {
			policy := newRequiredCheckPolicy()
			policy.add("Build", nil)
			return policy
		}()},
		{name: "empty policy", policy: newRequiredCheckPolicy()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := classifyProviderRequiredChecks([]checkRun{{Name: "Undiscovered", State: "success"}}, tc.policy)
			if err == nil || !strings.Contains(err.Error(), "absent from discovered branch policy") {
				t.Fatalf("discovery disagreement error = %v", err)
			}
		})
	}
}

func TestCheckAllCIIgnoresFailedAdvisoryCheck(t *testing.T) {
	installRequiredCheckFakeGH(t, `
case "$*" in
  "pr checks 7 --repo owner/repo --json name,state") printf '%s\n' '[{"name":"Build","state":"success"},{"name":"Advisory","state":"failure"}]' ;;
  "pr view 7 --repo owner/repo --json baseRefName") printf '%s\n' '{"baseRefName":"main"}' ;;
  "api --paginate --slurp repos/owner/repo/rules/branches/main?per_page=100") printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Build"}]}}]]' ;;
  "api repos/owner/repo/branches/main/protection/required_status_checks") printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  "pr checks 7 --repo owner/repo --required --json name,state") printf '%s\n' '[{"name":"Build","state":"success"}]' ;;
  *) printf '%s\n' "unexpected gh invocation: $*" >&2; exit 2 ;;
esac
`)
	if err := checkAllCI(7, "owner/repo"); err != nil {
		t.Fatalf("green required check with red advisory check should pass: %v", err)
	}
}

// --- parseReviewThreads ---

type reviewThreadNode struct {
	IsResolved bool `json:"isResolved"`
	IsOutdated bool `json:"isOutdated"`
	Comments   struct {
		Nodes []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body string `json:"body"`
		} `json:"nodes"`
	} `json:"comments"`
}

type reviewThreadsDoc struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []reviewThreadNode `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

func makeReviewJSON(threads []struct {
	resolved bool
	outdated bool
	author   string
	body     string
}) []byte {
	var doc reviewThreadsDoc
	for _, t := range threads {
		node := reviewThreadNode{
			IsResolved: t.resolved,
			IsOutdated: t.outdated,
		}
		if t.body != "" || t.author != "" {
			var c struct {
				Author struct {
					Login string `json:"login"`
				} `json:"author"`
				Body string `json:"body"`
			}
			c.Author.Login = t.author
			c.Body = t.body
			node.Comments.Nodes = append(node.Comments.Nodes, c)
		}
		doc.Data.Repository.PullRequest.ReviewThreads.Nodes = append(
			doc.Data.Repository.PullRequest.ReviewThreads.Nodes, node)
	}
	b, _ := json.Marshal(doc)
	return b
}

func TestParseReviewThreads_AllResolved(t *testing.T) {
	data := makeReviewJSON([]struct {
		resolved bool
		outdated bool
		author   string
		body     string
	}{
		{resolved: true},
		{resolved: false, outdated: true},
	})
	if err := parseReviewThreads(data); err != nil {
		t.Fatalf("all threads resolved/outdated should pass, got: %v", err)
	}
}

func TestParseReviewThreads_UnresolvedThread(t *testing.T) {
	data := makeReviewJSON([]struct {
		resolved bool
		outdated bool
		author   string
		body     string
	}{
		{resolved: false, outdated: false, author: "gemini-code-assist", body: "Please fix the nil check"},
	})
	err := parseReviewThreads(data)
	if err == nil {
		t.Fatal("expected error for unresolved thread, got nil")
	}
	if !strings.Contains(err.Error(), "unresolved") {
		t.Errorf("error should mention 'unresolved', got: %v", err)
	}
}

func TestParseReviewThreads_ShowsAuthorAndBody(t *testing.T) {
	data := makeReviewJSON([]struct {
		resolved bool
		outdated bool
		author   string
		body     string
	}{
		{resolved: false, author: "gemini-code-assist", body: "Consider extracting to a helper"},
		{resolved: false, author: "vbonnet", body: "Will address in follow-up"},
	})
	err := parseReviewThreads(data)
	if err == nil {
		t.Fatal("expected error for unresolved threads, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "@gemini-code-assist") {
		t.Errorf("error should show author @gemini-code-assist, got: %v", msg)
	}
	if !strings.Contains(msg, "@vbonnet") {
		t.Errorf("error should show author @vbonnet, got: %v", msg)
	}
	if !strings.Contains(msg, "Consider extracting") {
		t.Errorf("error should show comment body, got: %v", msg)
	}
}

func TestParseReviewThreads_UnknownAuthorFallback(t *testing.T) {
	data := makeReviewJSON([]struct {
		resolved bool
		outdated bool
		author   string
		body     string
	}{
		{resolved: false, author: "", body: "No author login"},
	})
	err := parseReviewThreads(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "@unknown") {
		t.Errorf("error should show @unknown when author is absent, got: %v", err.Error())
	}
}

func TestParseReviewThreads_BodyTruncatedAt80Runes(t *testing.T) {
	longBody := strings.Repeat("x", 100)
	data := makeReviewJSON([]struct {
		resolved bool
		outdated bool
		author   string
		body     string
	}{
		{resolved: false, author: "bot", body: longBody},
	})
	err := parseReviewThreads(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "…") {
		t.Errorf("long body should be truncated with ellipsis, got: %v", err.Error())
	}
}

func TestParseReviewThreads_Empty(t *testing.T) {
	data := makeReviewJSON(nil)
	if err := parseReviewThreads(data); err != nil {
		t.Fatalf("no threads should pass, got: %v", err)
	}
}

func TestReviewThreadsQueryPaginates(t *testing.T) {
	for _, required := range []string{"$cursor: String", "after: $cursor", "hasNextPage", "endCursor"} {
		if !strings.Contains(reviewThreadsQuery, required) {
			t.Fatalf("reviewThreadsQuery must contain %q", required)
		}
	}
}

// --- parseSoak ---

func makeSoakJSON(committedAt time.Time) []byte {
	type commitEntry struct {
		CommittedDate time.Time `json:"committedDate"`
	}
	type doc struct {
		HeadRefOid string        `json:"headRefOid"`
		Commits    []commitEntry `json:"commits"`
	}
	d := doc{HeadRefOid: "abc123"}
	if !committedAt.IsZero() {
		d.Commits = []commitEntry{{CommittedDate: committedAt}}
	}
	b, _ := json.Marshal(d)
	return b
}

func TestParseSoak_OldCommit(t *testing.T) {
	now := time.Now()
	past := now.Add(-10 * time.Minute)
	data := makeSoakJSON(past)
	if err := parseSoak(data, now); err != nil {
		t.Fatalf("old commit should pass soak gate, got: %v", err)
	}
}

func TestParseSoak_TooRecent(t *testing.T) {
	now := time.Now()
	recent := now.Add(-1 * time.Minute)
	data := makeSoakJSON(recent)
	err := parseSoak(data, now)
	if err == nil {
		t.Fatal("expected error for too-recent commit, got nil")
	}
	if !strings.Contains(err.Error(), "retry in") {
		t.Errorf("error should mention retry time, got: %v", err)
	}
}

// --- SafeMerge config validation ---

func TestSafeMerge_InvalidConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  MergeConfig
		want string
	}{
		{"zero pr", MergeConfig{PRNumber: 0, Repo: "a/b"}, "--pr must be"},
		{"missing repo", MergeConfig{PRNumber: 1, Repo: ""}, "--repo is required"},
		{"bad repo format", MergeConfig{PRNumber: 1, Repo: "nodash"}, "owner/repo format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := SafeMerge(tc.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

// --- appendAuditEntry / auditLogDir ---

func TestAuditEntry_WrittenToFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SAFE_MERGE_AUDIT_DIR", dir)

	appendAuditEntry("owner/repo", 42, "merged", "squash merge complete")

	data, err := os.ReadFile(filepath.Join(dir, "safe-merge-audit.jsonl"))
	if err != nil {
		t.Fatalf("audit log not created: %v", err)
	}
	if !strings.Contains(string(data), `"merged"`) {
		t.Errorf("audit log should contain event type, got: %s", data)
	}
	if !strings.Contains(string(data), `"pr":42`) {
		t.Errorf("audit log should contain PR number, got: %s", data)
	}
}

func TestAuditEntry_MultipleEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SAFE_MERGE_AUDIT_DIR", dir)

	appendAuditEntry("owner/repo", 1, "gate_check", "CI: checks failed")
	appendAuditEntry("owner/repo", 1, "merged", "ok")

	data, err := os.ReadFile(filepath.Join(dir, "safe-merge-audit.jsonl"))
	if err != nil {
		t.Fatalf("audit log not created: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 audit entries, got %d: %s", len(lines), data)
	}
}

// --- BuildMergeArgs (TOCTOU anchor) ---

func TestBuildMergeArgs_ContainsMatchHeadCommit(t *testing.T) {
	args := BuildMergeArgs(42, "owner/repo", "abc123def456")

	foundFlag := false
	foundSHA := false
	for i, a := range args {
		if a == "--match-head-commit" {
			foundFlag = true
			if i+1 < len(args) && args[i+1] == "abc123def456" {
				foundSHA = true
			}
		}
	}
	if !foundFlag {
		t.Fatal("BuildMergeArgs must include --match-head-commit to prevent TOCTOU " +
			"(PR #460 regression); got: " + fmt.Sprint(args))
	}
	if !foundSHA {
		t.Fatal("--match-head-commit must be followed by the head SHA; got: " + fmt.Sprint(args))
	}
}

func TestBuildMergeArgs_PanicsOnEmptySHA(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("BuildMergeArgs must panic on empty headSHA — an empty anchor " +
				"would silently defeat the TOCTOU protection")
		}
	}()
	BuildMergeArgs(1, "o/r", "")
}

func TestBuildMergeArgs_RequiredFlags(t *testing.T) {
	args := BuildMergeArgs(99, "vbonnet/dear-agent", "deadbeef")

	required := map[string]bool{
		"gh":                  false,
		"pr":                  false,
		"merge":               false,
		"--squash":            false,
		"--auto":              false,
		"--delete-branch":     false,
		"--match-head-commit": false,
	}
	for _, a := range args {
		if _, ok := required[a]; ok {
			required[a] = true
		}
	}
	for flag, found := range required {
		if !found {
			t.Errorf("BuildMergeArgs missing required element %q", flag)
		}
	}
}

// --- merge completion confirmation ---

func TestMergeResultRequiresExactMergedHead(t *testing.T) {
	cases := []struct {
		name      string
		result    mergeResult
		expected  string
		wantError bool
	}{
		{name: "merged exact head", result: mergeResult{State: "MERGED", HeadRefOid: "abc123"}, expected: "abc123"},
		{name: "auto merge only enabled", result: mergeResult{State: "OPEN", HeadRefOid: "abc123"}, expected: "abc123", wantError: true},
		{name: "head changed", result: mergeResult{State: "MERGED", HeadRefOid: "def456"}, expected: "abc123", wantError: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateMergeResult(tc.result, tc.expected); (got != nil) != tc.wantError {
				t.Fatalf("validateMergeResult() error = %v, wantError %v", got, tc.wantError)
			}
		})
	}
}

func TestMergeResultPendingUsesSentinel(t *testing.T) {
	err := validateMergeResult(mergeResult{State: "OPEN", HeadRefOid: "abc123"}, "abc123")
	if !errors.Is(err, errMergePending) {
		t.Fatalf("validateMergeResult() error = %v, want errMergePending", err)
	}
}

func TestWaitForMergeCompletionPollsUntilMerged(t *testing.T) {
	attempts := 0
	err := waitForMergeCompletion(context.Background(), time.Second, time.Millisecond, func() error {
		attempts++
		if attempts < 3 {
			return errMergePending
		}
		return nil
	})
	if err != nil {
		t.Fatalf("waitForMergeCompletion() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("waitForMergeCompletion() attempts = %d, want 3", attempts)
	}
}

func TestWaitForMergeCompletionTimesOut(t *testing.T) {
	err := waitForMergeCompletion(context.Background(), time.Millisecond, time.Millisecond, func() error {
		return errMergePending
	})
	if err == nil {
		t.Fatal("waitForMergeCompletion() error = nil, want timeout")
	}
	if !errors.Is(err, errMergePending) {
		t.Fatalf("waitForMergeCompletion() error = %v, want wrapped errMergePending", err)
	}
}

// --- helpers ---

func marshalJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshalJSON: %v", err))
	}
	return b
}
