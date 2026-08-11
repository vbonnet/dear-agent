package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// initRepo creates a throwaway git repo with two commits and returns the repo
// dir plus the base and head SHAs, so run() can compute a real diff without
// touching the network.
func initRepo(t *testing.T, secondFileContent string) (dir, base, head string) {
	t.Helper()
	dir = t.TempDir()
	sandbox := gittest.Default(t)
	run := func(args ...string) string {
		return sandbox.Run(t, dir, args...)
	}
	run("init", "-q")
	sandbox.HardenRepo(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReviewFile(t, dir, specAuthoringPolicyPath, testSpecAuthoringPolicy)
	writeReviewFile(t, dir, activeHarnessRegistryPath, testActiveHarnessRegistry)
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	base = trim(run("rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(secondFileContent), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "head")
	head = trim(run("rev-parse", "HEAD"))
	return dir, base, head
}

func trim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}

// baseConfig returns a config with no PR/repo (so comments are skipped) and no
// API key, then applies overrides.
func baseConfig() config {
	return config{
		model:   "claude-opus-4-8",
		effort:  "high",
		maxDiff: defaultMaxDiffBytes,
	}
}

func TestRun_MissingKeyFailsClosed(t *testing.T) {
	c := noSpecConfig(t)
	// Still blocking (nonzero), but with the distinct cannot-run code so the
	// trusted workflow can tell the credential-starved disposition apart from
	// genuine failures. Any caller treating nonzero as failure still blocks.
	if got := run(c); got != exitKeylessCannotRun {
		t.Fatalf("missing key: run() = %d, want %d (blocking cannot-run code)", got, exitKeylessCannotRun)
	}
}

func TestRun_KeylessDeterministicHumanVerdictKeepsOrdinaryBlockingExit(t *testing.T) {
	// A SPEC.owner edge change is a conclusive SPEC-governance human-review
	// verdict that needs no model, so the absent credential is not its
	// blocker: it must keep exit 1, never the AIREV-26 cannot-run code that
	// the workflow would translate to neutral.
	dir := t.TempDir()
	sandbox := gittest.Default(t)
	git := func(args ...string) string { return sandbox.Run(t, dir, args...) }
	git("init", "-q")
	sandbox.HardenRepo(t, dir)
	writeReviewFile(t, dir, specAuthoringPolicyPath, testSpecAuthoringPolicy)
	writeReviewFile(t, dir, activeHarnessRegistryPath, testActiveHarnessRegistry)
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	base := trim(git("rev-parse", "HEAD"))
	writeReviewFile(t, dir, "domains/example/SPEC.owner", "domains/example/SPEC.md\n")
	git("add", "-A")
	git("commit", "-q", "-m", "owner edge")
	head := trim(git("rev-parse", "HEAD"))
	chdir(t, dir)
	c := baseConfig()
	c.baseSHA, c.headSHA = base, head
	if got := run(c); got != 1 {
		t.Fatalf("keyless deterministic human verdict: run() = %d, want 1 (never %d)", got, exitKeylessCannotRun)
	}
}

func TestRun_InvalidTrustedCIDeadlineFailsBeforeReviewEvenWithOverride(t *testing.T) {
	c := baseConfig()
	c.githubActions = true
	c.absoluteDeadline = "malformed"
	c.override = true
	if got := run(c); got != 1 {
		t.Fatalf("invalid trusted CI deadline + override: run() = %d, want 1 (fail closed)", got)
	}
}

func TestRun_MissingKeyWithOverridePasses(t *testing.T) {
	c := noSpecConfig(t)
	c.override = true
	if got := run(c); got != 0 {
		t.Fatalf("missing key + override: run() = %d, want 0", got)
	}
}

func TestRun_ForkFailsClosed(t *testing.T) {
	c := noSpecConfig(t)
	c.isFork = true
	c.apiKey = "sk-does-not-matter" // fork check precedes key check
	if got := run(c); got != 1 {
		t.Fatalf("fork: run() = %d, want 1 (fail closed)", got)
	}
}

func TestRun_ForkWithOverridePassesKeyCheck(t *testing.T) {
	// A fork with the override label bypasses the fork gate; with a real key
	// it would proceed to the (network) review, so we only assert it does NOT
	// short-circuit to the fork failure. Give it no key so it stops at the
	// key gate, which the override also passes — net result 0 without network.
	c := noSpecConfig(t)
	c.isFork = true
	c.override = true
	if got := run(c); got != 0 {
		t.Fatalf("fork + override (no key): run() = %d, want 0", got)
	}
}

func noSpecConfig(t *testing.T) config {
	t.Helper()
	dir, base, head := initRepo(t, "changed\n")
	chdir(t, dir)
	c := baseConfig()
	c.baseSHA, c.headSHA = base, head
	return c
}

func TestRun_EmptyDiffApproves(t *testing.T) {
	dir, _, head := initRepo(t, "changed\n")
	chdir(t, dir)
	c := baseConfig()
	c.apiKey = "sk-test"
	c.baseSHA = head
	c.headSHA = head // diff head..head is empty
	if got := run(c); got != 0 {
		t.Fatalf("empty diff: run() = %d, want 0", got)
	}
}

func TestRun_AuthenticatedEmptyDiffWithEscalationRequiresHumanReview(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, dir, head string) (base, newHead, prBody string)
	}{
		{
			name: "PR body marker",
			prepare: func(_ *testing.T, _ string, head string) (string, string, string) {
				return head, head, "HUMAN REVIEW REQUIRED"
			},
		},
		{
			name: "allow-empty commit marker",
			prepare: func(t *testing.T, dir, head string) (string, string, string) {
				gittest.Run(t, dir, "commit", "--allow-empty", "-m", "HUMAN REVIEW REQUIRED")
				return head, trim(gittest.Run(t, dir, "rev-parse", "HEAD")), ""
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, _, head := initRepo(t, "changed\n")
			base, markedHead, prBody := tt.prepare(t, dir, head)
			chdir(t, dir)

			plan, err := buildReviewPlanWithPRBody(context.Background(), base, markedHead, prBody)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.EscalationTriggers) == 0 {
				t.Fatalf("empty diff plan has no escalation trigger: %#v", plan)
			}

			c := baseConfig()
			c.apiKey = "sk-test"
			c.baseSHA, c.headSHA, c.prBody = base, markedHead, prBody
			if got := run(c); got != 1 {
				t.Fatalf("marked empty diff: run() = %d, want 1 (needs human review)", got)
			}
		})
	}
}

func TestRun_OversizeDiffFailsClosed(t *testing.T) {
	big := make([]byte, 2000)
	for i := range big {
		big[i] = 'x'
	}
	dir, base, head := initRepo(t, string(big)+"\n")
	chdir(t, dir)
	c := baseConfig()
	c.apiKey = "sk-test"
	c.baseSHA = base
	c.headSHA = head
	c.maxDiff = 500 // force oversize
	if got := run(c); got != 1 {
		t.Fatalf("oversize diff: run() = %d, want 1 (fail closed, no truncation)", got)
	}
}

func TestGitDiff_OversizeReturnsNoTruncatedPrefix(t *testing.T) {
	big := strings.Repeat("x", 2000) + "\n"
	dir, base, head := initRepo(t, big)
	chdir(t, dir)

	diff, tooLarge, err := gitDiff(base, head, 500)
	if err != nil {
		t.Fatal(err)
	}
	if !tooLarge {
		t.Fatal("gitDiff oversized diff was not reported as too large")
	}
	if diff != "" {
		t.Fatalf("gitDiff returned %d bytes of a truncated, non-reviewable diff", len(diff))
	}
}

func TestOversizeCommentReportsOnlyVerifiedLimit(t *testing.T) {
	comment := oversizeComment(500)
	if !strings.Contains(comment, "exceeds the 500-byte auto-review limit") {
		t.Fatalf("oversize comment does not report the verified limit: %q", comment)
	}
	if strings.Contains(comment, "501 bytes") {
		t.Fatalf("oversize comment reports the bounded sentinel as the actual diff size: %q", comment)
	}
}

func TestRun_OversizeDiffWithOverridePasses(t *testing.T) {
	big := make([]byte, 2000)
	for i := range big {
		big[i] = 'x'
	}
	dir, base, head := initRepo(t, string(big)+"\n")
	chdir(t, dir)
	c := baseConfig()
	c.apiKey = "sk-test"
	c.baseSHA = base
	c.headSHA = head
	c.maxDiff = 500
	c.override = true
	if got := run(c); got != 0 {
		t.Fatalf("oversize diff + override: run() = %d, want 0", got)
	}
}

func TestRun_AuthenticatedMandatoryEscalationStillRunsFullReview(t *testing.T) {
	dir, _, base := initRepo(t, "changed\n")
	writeReviewFile(t, dir, ".github/workflows/review.yml", "name: review\n")
	gittest.Run(t, dir, "add", ".github/workflows/review.yml")
	gittest.Run(t, dir, "commit", "-m", "change review workflow")
	head := strings.TrimSpace(gittest.Run(t, dir, "rev-parse", "HEAD"))
	chdir(t, dir)
	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.needsHuman() || len(plan.EscalationTriggers) == 0 {
		t.Fatalf("mandatory-escalation fixture produced plan %#v", plan)
	}

	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, string(body))
		mu.Unlock()
		response := "No blocking findings."
		if strings.Contains(string(body), "You are a synthesis agent") {
			response = "approved\nThe dimension reports contain no blocking findings."
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, modelResponse("end_turn", response))
	}))
	defer server.Close()

	c := baseConfig()
	c.baseSHA, c.headSHA = base, head
	c.apiKey = "test-key"
	c.apiBaseURL = server.URL
	if got := run(c); got != 1 {
		t.Fatalf("authenticated mandatory escalation: run() = %d, want 1", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != len(dimensions())+1 {
		t.Fatalf("authenticated mandatory escalation made %d model calls, want %d dimensions plus synthesis", len(requests), len(dimensions())+1)
	}
	if !slicesContainText(requests, "You are a synthesis agent") {
		t.Fatal("authenticated mandatory escalation skipped synthesis")
	}
}

func TestRun_MandatoryEscalationShortCircuitsForFork(t *testing.T) {
	dir, _, base := initRepo(t, "changed\n")
	writeReviewFile(t, dir, ".github/workflows/review.yml", "name: review\n")
	gittest.Run(t, dir, "add", ".github/workflows/review.yml")
	gittest.Run(t, dir, "commit", "-m", "change review workflow")
	head := strings.TrimSpace(gittest.Run(t, dir, "rev-parse", "HEAD"))
	chdir(t, dir)

	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, modelResponse("end_turn", "approved"))
	}))
	defer server.Close()

	c := baseConfig()
	c.baseSHA, c.headSHA = base, head
	c.apiKey = "test-key"
	c.apiBaseURL = server.URL
	c.isFork = true
	if got := run(c); got != 1 {
		t.Fatalf("fork mandatory escalation: run() = %d, want 1", got)
	}
	if calls.Load() != 0 {
		t.Fatalf("fork mandatory escalation made %d model calls, want 0", calls.Load())
	}
}

func TestRun_SPECReportParticipatesInSynthesisAndRemainsBlocking(t *testing.T) {
	repo := newReviewRepo(t)
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	writeReviewFile(t, repo, "module/SPEC.md", specDocument("MOD-01", "When checked, the system shall report it.", "features/module.feature"))
	writeReviewFile(t, repo, "features/module.feature", featureDocument("# SPEC: module/SPEC.md\n", "contract"))
	gittest.Run(t, repo, "add", "module/SPEC.md", "features/module.feature")
	gittest.Run(t, repo, "commit", "-m", "add contract")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	applicability := make([]specApplicabilityReview, 0, len(plan.Applicability))
	for _, evidence := range plan.Applicability {
		applicability = append(applicability, specApplicabilityReview{
			Path:          evidence.Path,
			RequirementID: evidence.RequirementID,
			Harness:       evidence.Harness,
			Disposition:   "supported",
			Rationale:     "The stable promise applies.",
		})
	}
	contractForms := make([]specContractFormReview, 0, len(plan.ContractForms))
	for _, evidence := range plan.ContractForms {
		contractForms = append(contractForms, specContractFormReview{
			Path:                  evidence.Path,
			VisibleContractDigest: evidence.VisibleContractDigest,
			Disposition:           "complete",
			Rationale:             "All promises map to stable IDs.",
		})
	}
	wireVerdict := specContractVerdictDocument{
		Version:              specContractVersion,
		BaseSHA:              plan.BaseSHA,
		MergeBaseSHA:         plan.MergeBaseSHA,
		HeadSHA:              plan.HeadSHA,
		Changes:              plan.Changes,
		Status:               "needs-work",
		Summary:              "The contract needs a single fix before approval.",
		DeletionReviews:      []specDeletionReview{},
		ContractFormReviews:  contractForms,
		ApplicabilityReviews: applicability,
		Findings: []specFinding{{
			Path:       "module/SPEC.md",
			Severity:   "blocking",
			Message:    "The observable outcome is ambiguous.",
			Suggestion: "State one observable outcome.",
		}},
	}
	verdictJSON, err := json.Marshal(wireVerdict)
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadRequest)
			return
		}
		request := string(body)
		mu.Lock()
		requests = append(requests, request)
		mu.Unlock()
		response := "No blocking findings."
		switch {
		case strings.Contains(request, "You are a strict SPEC contract reviewer"):
			response = string(verdictJSON)
		case strings.Contains(request, "You are a synthesis agent"):
			response = "approved\nThe code dimensions contain no blocking findings."
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, modelResponse("end_turn", response))
	}))
	defer server.Close()

	c := baseConfig()
	c.baseSHA, c.headSHA = base, head
	c.apiKey = "test-key"
	c.apiBaseURL = server.URL
	if got := run(c); got != 1 {
		t.Fatalf("blocking SPEC verdict with approved synthesis: run() = %d, want 1", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != len(dimensions())+2 {
		t.Fatalf("SPEC review run made %d model calls, want SPEC plus %d dimensions plus synthesis", len(requests), len(dimensions()))
	}
	var synthesisRequest string
	for _, request := range requests {
		if strings.Contains(request, "You are a synthesis agent") {
			synthesisRequest = request
			break
		}
	}
	if !strings.Contains(synthesisRequest, "SPEC-CONTRACT") || !strings.Contains(synthesisRequest, "Authoritative outcome: needs-work") {
		t.Fatalf("synthesis request omitted authoritative SPEC report: %s", synthesisRequest)
	}
}

func TestFinalReviewOutcomeAndSynthesisDisplayPreserveAuthoritativeBlocks(t *testing.T) {
	tests := []struct {
		name        string
		specVerdict Outcome
		triggers    []string
		want        Outcome
	}{
		{name: "SPEC needs work", specVerdict: NeedsWork, want: NeedsWork},
		{name: "SPEC human review", specVerdict: NeedsHumanReview, want: NeedsHumanReview},
		{name: "mandatory escalation wins over rejected SPEC", specVerdict: Rejected, triggers: []string{"CI/CD pipeline edit"}, want: NeedsHumanReview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := finalReviewOutcome(Approved, tt.specVerdict, tt.triggers)
			if got != tt.want {
				t.Fatalf("finalReviewOutcome() = %s, want %s", got, tt.want)
			}
			displayed := reconcileSynthesisDisplay(Approved, got, "approved\nNo blocking findings.")
			if first, _, _ := strings.Cut(displayed, "\n"); first != got.String() {
				t.Fatalf("displayed synthesis starts %q, want %q: %s", first, got, displayed)
			}
			if strings.Contains(strings.ToLower(displayed), "approved") {
				t.Fatalf("displayed blocking synthesis still claims approval: %s", displayed)
			}
		})
	}
}

func slicesContainText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func TestBuildReviewPlan_SelectsOnlyExactSpecBasenamesAndRenameSides(t *testing.T) {
	dir := t.TempDir()
	sandbox := gittest.Default(t)
	git := func(args ...string) string { return sandbox.Run(t, dir, args...) }
	git("init", "-q")
	sandbox.HardenRepo(t, dir)
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	valid := "# Contract\n\n**OWN-01** When changed, the system shall prove it.\n\n## BDD Traceability\n\n- Feature: `test.feature`\n"
	write("old/SPEC.md", valid)
	write("docs/NOT-SPEC.md", valid)
	write(specAuthoringPolicyPath, testSpecAuthoringPolicy)
	write(activeHarnessRegistryPath, testActiveHarnessRegistry)
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	base := trim(git("rev-parse", "HEAD"))
	if err := os.MkdirAll(filepath.Join(dir, "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	git("mv", "old/SPEC.md", "new/SPEC.md")
	write("notes/NOT-SPEC.md", "ordinary\n")
	git("add", "-A")
	git("commit", "-q", "-m", "rename")
	head := trim(git("rev-parse", "HEAD"))
	chdir(t, dir)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	want := []specChange{{Path: "new/SPEC.md", Status: "added"}, {Path: "old/SPEC.md", Status: "deleted"}}
	if !sameSpecChanges(plan.Changes, want) {
		t.Fatalf("plan changes = %#v, want %#v", plan.Changes, want)
	}
	if plan.Diff != "" {
		t.Fatalf("renamed deleted SPEC must not produce reviewable diff: %q", plan.Diff)
	}
	if !plan.needsHuman() {
		t.Fatal("renamed SPEC deletion must require a human ownership decision")
	}
}

func TestBuildReviewPlan_SPECOwnerChangesRequireMaintainerReview(t *testing.T) {
	repo := newReviewRepo(t)
	writeReviewFile(t, repo, "old/SPEC.owner", "domains/old/SPEC.md\n")
	writeReviewFile(t, repo, "modified/SPEC.owner", "domains/old/SPEC.md\n")
	writeReviewFile(t, repo, "notes/NOT-SPEC.owner", "ordinary\n")
	gittest.Run(t, repo, "add", "old/SPEC.owner", "modified/SPEC.owner", "notes/NOT-SPEC.owner")
	gittest.Run(t, repo, "commit", "-m", "add ownership fixtures")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))

	if err := os.MkdirAll(filepath.Join(repo, "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, repo, "mv", "old/SPEC.owner", "new/SPEC.owner")
	writeReviewFile(t, repo, "modified/SPEC.owner", "domains/new/SPEC.md\n")
	writeReviewFile(t, repo, "added/SPEC.owner", "domains/new/SPEC.md\n")
	writeReviewFile(t, repo, "notes/NOT-SPEC.owner", "still ordinary\n")
	gittest.Run(t, repo, "add", "-A")
	gittest.Run(t, repo, "commit", "-m", "rewire ownership")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.ReviewNeeded || !plan.ReviewRelevant || len(plan.Changes) != 0 {
		t.Fatalf("SPEC.owner plan classification = %#v", plan)
	}
	reasons := strings.Join(plan.HumanReasons, "\n")
	for _, expected := range []string{
		"SPEC ownership edge addition requires maintainer review (added/SPEC.owner)",
		"SPEC ownership edge modification requires maintainer review (modified/SPEC.owner)",
		"SPEC ownership edge addition requires maintainer review (new/SPEC.owner)",
		"SPEC ownership edge deletion requires maintainer review (old/SPEC.owner)",
	} {
		if !strings.Contains(reasons, expected) {
			t.Errorf("ownership-edge reasons %q lack %q", reasons, expected)
		}
	}
	if strings.Contains(reasons, "NOT-SPEC.owner") {
		t.Fatalf("non-contract suffix entered ownership review: %q", reasons)
	}
}

func TestBuildReviewPlan_BoundsSemanticInputToChangedSpecDiff(t *testing.T) {
	dir, base, _ := initRepo(t, "ordinary\n")
	sandbox := gittest.Default(t)
	git := func(args ...string) string { return sandbox.Run(t, dir, args...) }
	if err := os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte("# Contract\n\n**OWN-01** When changed, the system shall prove it.\n\n## BDD Traceability\n\n- Feature: `test.feature`\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test.feature"), []byte(featureDocument("# SPEC: SPEC.md\n", "proof")), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "SPEC.md", "test.feature")
	git("commit", "-q", "-m", "spec")
	head := trim(git("rev-parse", "HEAD"))
	chdir(t, dir)
	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.ReviewNeeded || plan.needsHuman() || !strings.Contains(plan.Diff, "SPEC.md") || strings.Contains(plan.Diff, "ordinary") {
		t.Fatalf("unexpected bounded SPEC plan: %#v", plan)
	}
}

func TestRun_RelevantSpecWithoutCredentialRequiresHumanReview(t *testing.T) {
	dir := t.TempDir()
	sandbox := gittest.Default(t)
	git := func(args ...string) string { return sandbox.Run(t, dir, args...) }
	git("init", "-q")
	sandbox.HardenRepo(t, dir)
	writeReviewFile(t, dir, specAuthoringPolicyPath, testSpecAuthoringPolicy)
	writeReviewFile(t, dir, activeHarnessRegistryPath, testActiveHarnessRegistry)
	git("add", specAuthoringPolicyPath, activeHarnessRegistryPath)
	git("commit", "--allow-empty", "-q", "-m", "base")
	base := trim(git("rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte("# Contract\n\n**OWN-01** When changed, the system shall prove it.\n\n## BDD Traceability\n\n- Feature: `test.feature`\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "test.feature"), []byte(featureDocument("# SPEC: SPEC.md\n", "proof")), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "SPEC.md", "test.feature")
	git("commit", "-q", "-m", "spec")
	head := trim(git("rev-parse", "HEAD"))
	chdir(t, dir)
	c := baseConfig()
	c.baseSHA, c.headSHA = base, head
	if got := run(c); got != exitKeylessCannotRun {
		t.Fatalf("relevant SPEC without credential: run() = %d, want blocking cannot-run code %d", got, exitKeylessCannotRun)
	}
}

func TestGitMergeBase_ExcludesBaseOnlyChanges(t *testing.T) {
	dir, base, _ := initRepo(t, "feature change\n")
	chdir(t, dir)
	sandbox := gittest.Default(t)
	git := func(args ...string) string {
		return sandbox.Run(t, dir, args...)
	}
	feature := trim(git("rev-parse", "HEAD"))
	git("checkout", "-q", "-b", "base-advanced", base)
	if err := os.WriteFile(filepath.Join(dir, "base-only.txt"), []byte("base advance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "base-only.txt")
	git("commit", "-q", "-m", "base advance")
	advancedBase := trim(git("rev-parse", "HEAD"))

	mergeBase, err := gitMergeBase(advancedBase, feature)
	if err != nil {
		t.Fatal(err)
	}
	if mergeBase != base {
		t.Fatalf("merge base = %s, want %s", mergeBase, base)
	}
	paths, err := gitChangedPaths(mergeBase, feature)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "a.txt" {
		t.Fatalf("PR paths = %v, want only a.txt", paths)
	}
}

// TestGitChangedPaths_IncludesRenameSource is the regression guard for the
// rename bypass: moving a protected file to an ordinary path must still expose
// the protected SOURCE path to escalation scanning.
func TestGitChangedPaths_IncludesRenameSource(t *testing.T) {
	dir := t.TempDir()
	sandbox := gittest.Default(t)
	git := func(args ...string) string {
		return sandbox.Run(t, dir, args...)
	}
	git("init", "-q")
	sandbox.HardenRepo(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Content long enough that git would otherwise detect a 100% rename.
	body := "{\n  \"permissions\": { \"allow\": [\"a\",\"b\",\"c\"], \"deny\": [\"d\"] }\n}\n"
	if err := os.WriteFile(filepath.Join(dir, ".claude/settings.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	base := trim(git("rev-parse", "HEAD"))
	git("mv", ".claude/settings.json", "safe-config.json")
	git("commit", "-q", "-m", "move")
	head := trim(git("rev-parse", "HEAD"))

	chdir(t, dir)
	paths, err := gitChangedPaths(base, head)
	if err != nil {
		t.Fatal(err)
	}
	var sawSource bool
	for _, p := range paths {
		if p == ".claude/settings.json" {
			sawSource = true
		}
	}
	if !sawSource {
		t.Fatalf("rename source .claude/settings.json missing from changed paths %v", paths)
	}
	if got := EscalationTriggers(paths, "", ""); len(got) == 0 {
		t.Fatal("renaming a protected settings file away must still escalate")
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
}
