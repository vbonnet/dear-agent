package verifiers

import (
	"context"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

func TestNoopVerifierImplementsAuditVerifier(t *testing.T) {
	// Compile-time-ish assertion: the no-op verifier must satisfy the
	// audit.Verifier interface. If this breaks the interface evolved
	// and the example must be updated alongside it.
	var _ audit.Verifier = NoopVerifier{}
}

func TestNoopVerifierDefaultName(t *testing.T) {
	if got := (NoopVerifier{}).Name(); got != "noop" {
		t.Errorf("default Name = %q, want %q", got, "noop")
	}
	if got := (NoopVerifier{VerifierName: "custom"}).Name(); got != "custom" {
		t.Errorf("override Name = %q, want %q", got, "custom")
	}
}

func TestNoopVerifierReturnsNoFindings(t *testing.T) {
	findings, err := NoopVerifier{}.Verify(context.Background(), audit.VerifyTarget{
		RepoRoot:   "/tmp/demo",
		WorkingDir: "/tmp/demo",
	})
	if err != nil {
		t.Fatalf("Verify err = %v, want nil", err)
	}
	if len(findings) != 0 {
		t.Errorf("findings = %d, want 0", len(findings))
	}
}

func TestNoopVerifierDepthIsCasual(t *testing.T) {
	if got := (NoopVerifier{}).ReviewDepth(); got != audit.ReviewDepthCasual {
		t.Errorf("ReviewDepth = %q, want %q", got, audit.ReviewDepthCasual)
	}
}

func TestNoopVerifierDispatchedEndToEnd(t *testing.T) {
	// Ship criterion: the no-op verifier compiles, registers, and is
	// dispatched from the Audit runner end-to-end. We exercise that
	// path here to keep the criterion guarded.
	reg := audit.NewRegistry()
	if err := reg.RegisterVerifier(NoopVerifier{}); err != nil {
		t.Fatalf("RegisterVerifier: %v", err)
	}
	store := audit.NewMemoryStore()
	r := audit.NewRunner()
	r.Registry = reg
	r.Store = store

	plan := audit.Plan{
		Repo: "demo", RepoRoot: "/tmp/demo", Cadence: audit.CadenceOnDemand,
		Trees: []audit.TreePlan{{WorkingDir: "/tmp/demo"}},
	}
	report, err := r.Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.VerifierOutcomes) != 1 {
		t.Fatalf("VerifierOutcomes = %d, want 1", len(report.VerifierOutcomes))
	}
	if report.VerifierOutcomes[0].VerifierName != "noop" {
		t.Errorf("VerifierName = %q, want %q", report.VerifierOutcomes[0].VerifierName, "noop")
	}
	if len(report.VerifierOutcomes[0].Findings) != 0 {
		t.Errorf("findings = %d, want 0", len(report.VerifierOutcomes[0].Findings))
	}
	if report.AuditRun.State != audit.AuditRunSucceeded {
		t.Errorf("state = %s, want succeeded", report.AuditRun.State)
	}
}
