// Package verifiers ships the reference Verifier implementations that
// dear-agent uses to prove the Phase 6.6 dispatch seam end-to-end.
// Real external verifiers (Mythos, llm-judge, semgrep adapters, …)
// live outside this package and plug in via plugin.VerifierProvider;
// the implementations here exist to keep the test fleet honest and to
// give first-party docs a concrete reference.
package verifiers

import (
	"context"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// NoopVerifier is the trivial Verifier the Phase 6.6 ship criterion
// names: it compiles, registers, and is dispatched from the Audit
// runner end-to-end, but emits no findings. Operators use it as a
// smoke test ("verifier dispatch is wired") and as a template for
// building real verifiers (the production shape is identical: name,
// description, depth, Verify).
type NoopVerifier struct {
	// VerifierName overrides the default name. Empty means use the
	// package default ("noop"). Exposed so two test cases can register
	// distinct no-op verifiers without colliding on Name.
	VerifierName string
}

// Name returns the verifier's unique name. Defaults to "noop".
func (v NoopVerifier) Name() string {
	if v.VerifierName == "" {
		return "noop"
	}
	return v.VerifierName
}

// Description returns a one-line human-readable summary.
func (NoopVerifier) Description() string {
	return "Reference verifier — proves the dispatch path, emits no findings"
}

// ReviewDepth declares the depth at which the no-op verifier inspects
// targets. Casual on purpose: a verifier that emits no findings cannot
// honestly claim adversarial depth. Real verifiers should return
// audit.ReviewDepthAdversarial.
func (NoopVerifier) ReviewDepth() string {
	return audit.ReviewDepthCasual
}

// Verify is a no-op: returns an empty findings slice and a nil error.
// The runner still records a VerifierOutcome row for the call, which
// is the ship-criterion behaviour ("dispatched end-to-end").
func (NoopVerifier) Verify(_ context.Context, _ audit.VerifyTarget) ([]audit.Finding, error) {
	return nil, nil
}
