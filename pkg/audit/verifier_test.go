package audit

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeVerifier is a Verifier implementation that returns the findings
// (or error) it was constructed with. Used to drive the runner's
// verifier dispatch deterministically.
type fakeVerifier struct {
	name        string
	description string
	depth       string
	findings    []Finding
	err         error
}

func (f fakeVerifier) Name() string        { return f.name }
func (f fakeVerifier) Description() string { return f.description }
func (f fakeVerifier) ReviewDepth() string { return f.depth }
func (f fakeVerifier) Verify(_ context.Context, _ VerifyTarget) ([]Finding, error) {
	return f.findings, f.err
}

func TestRegistryRegisterVerifier(t *testing.T) {
	reg := NewRegistry()
	if err := reg.RegisterVerifier(fakeVerifier{name: "v1"}); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := reg.RegisterVerifier(fakeVerifier{name: "v1"}); err == nil {
		t.Fatal("expected duplicate-name error")
	}
	if err := reg.RegisterVerifier(fakeVerifier{name: ""}); err == nil {
		t.Fatal("expected empty-name error")
	}
	if err := reg.RegisterVerifier(nil); err == nil {
		t.Fatal("expected nil error")
	}
}

func TestRegistryVerifiersSnapshotSortedByName(t *testing.T) {
	reg := NewRegistry()
	for _, name := range []string{"bravo", "alpha", "charlie"} {
		if err := reg.RegisterVerifier(fakeVerifier{name: name}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
	}
	got := reg.Verifiers()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	want := []string{"alpha", "bravo", "charlie"}
	for i, v := range got {
		if v.Name() != want[i] {
			t.Errorf("[%d] = %s, want %s", i, v.Name(), want[i])
		}
	}
	if _, ok := reg.LookupVerifier("alpha"); !ok {
		t.Error("LookupVerifier(alpha) = false")
	}
	if _, ok := reg.LookupVerifier("missing"); ok {
		t.Error("LookupVerifier(missing) = true")
	}
}

func TestRunnerDispatchesVerifiersWithoutChecks(t *testing.T) {
	// Ship-criterion shape: a Verifier registers, dispatches end-to-end,
	// and emits findings stamped with verifier_role / review_depth even
	// when no Checks are scheduled.
	r, store, reg := newTestRunner(t)
	if err := reg.RegisterVerifier(fakeVerifier{
		name:  "demo-verifier",
		depth: ReviewDepthAdversarial,
		findings: []Finding{
			{Fingerprint: "vf1", Severity: SeverityP1, Title: "verifier finding"},
		},
	}); err != nil {
		t.Fatalf("register verifier: %v", err)
	}

	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceOnDemand,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo"}},
	}
	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.VerifierOutcomes) != 1 {
		t.Fatalf("VerifierOutcomes = %d, want 1", len(report.VerifierOutcomes))
	}
	outcome := report.VerifierOutcomes[0]
	if outcome.VerifierName != "demo-verifier" {
		t.Errorf("VerifierName = %s, want demo-verifier", outcome.VerifierName)
	}
	if len(outcome.Findings) != 1 {
		t.Fatalf("Findings = %d, want 1", len(outcome.Findings))
	}
	f := outcome.Findings[0]
	if f.CheckID != "verify.demo-verifier" {
		t.Errorf("CheckID = %s, want verify.demo-verifier", f.CheckID)
	}
	if got, _ := f.Evidence[EvidenceVerifierRole].(string); got != "demo-verifier" {
		t.Errorf("Evidence[verifier_role] = %v, want demo-verifier", f.Evidence[EvidenceVerifierRole])
	}
	if got, _ := f.Evidence[EvidenceReviewDepth].(string); got != ReviewDepthAdversarial {
		t.Errorf("Evidence[review_depth] = %v, want adversarial", f.Evidence[EvidenceReviewDepth])
	}
	c, _ := store.CountFindings(context.Background(), "demo")
	if c.Open != 1 {
		t.Errorf("CountFindings.Open = %d, want 1", c.Open)
	}
}

func TestRunnerVerifierDefaultDepthIsAdversarial(t *testing.T) {
	r, _, reg := newTestRunner(t)
	if err := reg.RegisterVerifier(fakeVerifier{
		name:  "depthless",
		depth: "", // runner stamps adversarial as the default
		findings: []Finding{
			{Fingerprint: "vf1", Severity: SeverityP2, Title: "drift"},
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceOnDemand,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo"}},
	}
	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	f := report.VerifierOutcomes[0].Findings[0]
	if got, _ := f.Evidence[EvidenceReviewDepth].(string); got != ReviewDepthAdversarial {
		t.Errorf("default depth = %v, want adversarial", got)
	}
}

func TestRunnerVerifierErrorTransitionsPartial(t *testing.T) {
	r, _, reg := newTestRunner(t)
	if err := reg.RegisterVerifier(fakeVerifier{
		name: "broken",
		err:  errors.New("boom"),
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceOnDemand,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo"}},
	}
	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.AuditRun.State != AuditRunPartial {
		t.Errorf("state = %s, want partial", report.AuditRun.State)
	}
	if len(report.VerifierOutcomes) != 1 || report.VerifierOutcomes[0].Err == nil {
		t.Fatalf("expected one outcome with non-nil Err, got %+v", report.VerifierOutcomes)
	}
}

func TestRunnerInvalidVerifierFindingTransitionsPartial(t *testing.T) {
	r, store, reg := newTestRunner(t)
	if err := reg.RegisterVerifier(fakeVerifier{
		name: "invalid-suggestion",
		findings: []Finding{{
			Fingerprint: "vf-invalid",
			Severity:    SeverityP2,
			Title:       "invalid verifier suggestion",
			Suggested:   Remediation{Strategy: Strategy("future")},
		}},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceOnDemand,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo"}},
	}

	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.AuditRun.State != AuditRunPartial {
		t.Fatalf("state = %q, want partial", report.AuditRun.State)
	}
	if len(report.VerifierOutcomes) != 1 || report.VerifierOutcomes[0].Err == nil {
		t.Fatalf("expected one verifier outcome with error, got %+v", report.VerifierOutcomes)
	}
	if len(report.VerifierOutcomes[0].Findings) != 0 {
		t.Fatalf("findings = %+v, want invalid verifier finding dropped", report.VerifierOutcomes[0].Findings)
	}
	stored, err := store.ListFindings(context.Background(), FindingFilter{Repo: "demo"})
	if err != nil {
		t.Fatalf("ListFindings: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("stored findings = %+v, want none", stored)
	}
}

func TestRunnerVerifierStoreFailureTransitionsPartial(t *testing.T) {
	r, memory, reg := newTestRunner(t)
	store := &observingFindingStore{Store: memory, upsertFindingErr: errors.New("write failed")}
	r.Store = store
	if err := reg.RegisterVerifier(fakeVerifier{
		name: "store-failure",
		findings: []Finding{{
			Fingerprint: "vf-store-failure",
			Severity:    SeverityP1,
			Title:       "cannot persist verifier finding",
		}},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceOnDemand,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo"}},
	}

	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.AuditRun.State != AuditRunPartial {
		t.Fatalf("state = %q, want partial", report.AuditRun.State)
	}
	if len(report.VerifierOutcomes) != 1 || report.VerifierOutcomes[0].Err == nil {
		t.Fatalf("expected one verifier outcome with error, got %+v", report.VerifierOutcomes)
	}
	if len(report.VerifierOutcomes[0].Findings) != 0 {
		t.Fatalf("findings = %+v, want unpersisted verifier finding omitted", report.VerifierOutcomes[0].Findings)
	}
	if store.upsertFindingCalls != 1 {
		t.Fatalf("UpsertFinding calls = %d, want one", store.upsertFindingCalls)
	}
}

func TestRunnerVerifierPreservesCallerSetEvidence(t *testing.T) {
	// A Verifier that sets verifier_role / review_depth explicitly on a
	// Finding (e.g. forwarding a Mythos finding with its native depth)
	// must not have those values clobbered by the runner stamp.
	r, _, reg := newTestRunner(t)
	if err := reg.RegisterVerifier(fakeVerifier{
		name:  "passthrough",
		depth: ReviewDepthAdversarial,
		findings: []Finding{{
			Fingerprint: "vf1", Severity: SeverityP1, Title: "kept",
			Evidence: map[string]any{
				EvidenceVerifierRole: "upstream-mythos",
				EvidenceReviewDepth:  ReviewDepthCasual,
			},
		}},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceOnDemand,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo"}},
	}
	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	f := report.VerifierOutcomes[0].Findings[0]
	if got, _ := f.Evidence[EvidenceVerifierRole].(string); got != "upstream-mythos" {
		t.Errorf("verifier_role overwritten: got %v, want upstream-mythos", got)
	}
	if got, _ := f.Evidence[EvidenceReviewDepth].(string); got != ReviewDepthCasual {
		t.Errorf("review_depth overwritten: got %v, want casual", got)
	}
}

func TestRunnerVerifierMissingSeverityDefaultsToP2(t *testing.T) {
	r, _, reg := newTestRunner(t)
	if err := reg.RegisterVerifier(fakeVerifier{
		name:  "no-sev",
		depth: ReviewDepthAdversarial,
		findings: []Finding{
			{Fingerprint: "vf1", Title: "no severity set by verifier"},
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceOnDemand,
		Trees: []TreePlan{{WorkingDir: "/tmp/demo"}},
	}
	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.VerifierOutcomes) != 1 || len(report.VerifierOutcomes[0].Findings) != 1 {
		t.Fatalf("expected 1 finding, got %+v", report.VerifierOutcomes)
	}
	if report.VerifierOutcomes[0].Findings[0].Severity != SeverityP2 {
		t.Errorf("default severity = %s, want P2", report.VerifierOutcomes[0].Findings[0].Severity)
	}
	if report.VerifierOutcomes[0].Findings[0].Suggested != (Remediation{Strategy: StrategyIssue}) {
		t.Errorf("default suggestion = %+v, want payloadless issue", report.VerifierOutcomes[0].Findings[0].Suggested)
	}
}

func TestRunnerVerifiersRunPerTree(t *testing.T) {
	// Multi-tree plans dispatch every verifier into every tree, the
	// same shape executePlanChecks uses. The fingerprint differs per
	// tree because the verifier sets it from the working dir.
	r, _, reg := newTestRunner(t)
	if err := reg.RegisterVerifier(treeAwareVerifier{name: "tw"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	plan := Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: CadenceOnDemand,
		Trees: []TreePlan{
			{WorkingDir: "/tmp/demo/app"},
			{WorkingDir: "/tmp/demo/lib"},
		},
	}
	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.VerifierOutcomes) != 2 {
		t.Fatalf("VerifierOutcomes = %d, want 2 (one per tree)", len(report.VerifierOutcomes))
	}
	gotDirs := []string{
		report.VerifierOutcomes[0].WorkingDir,
		report.VerifierOutcomes[1].WorkingDir,
	}
	if gotDirs[0] == gotDirs[1] {
		t.Errorf("tree dirs collided: %v", gotDirs)
	}
}

// treeAwareVerifier emits one finding fingerprinted by the working
// directory so multi-tree dispatch is observable in tests without
// hitting the (repo, fingerprint) UNIQUE constraint.
type treeAwareVerifier struct{ name string }

func (t treeAwareVerifier) Name() string      { return t.name }
func (treeAwareVerifier) Description() string { return "" }
func (treeAwareVerifier) ReviewDepth() string { return ReviewDepthAdversarial }
func (treeAwareVerifier) Verify(_ context.Context, target VerifyTarget) ([]Finding, error) {
	return []Finding{{
		Fingerprint: "tw-" + strings.ReplaceAll(target.WorkingDir, "/", "_"),
		Severity:    SeverityP2,
		Title:       "tree " + target.WorkingDir,
	}}, nil
}
