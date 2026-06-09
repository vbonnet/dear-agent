package audit

import (
	"context"
	"testing"
	"time"
)

// TestFinding_VerifierRole_ReadsEvidence checks the convenience reader
// for the verifier-role Evidence key. Empty when absent; empty when
// malformed (wrong type).
func TestFinding_VerifierRole_ReadsEvidence(t *testing.T) {
	cases := []struct {
		name string
		f    Finding
		want string
	}{
		{name: "nil evidence", f: Finding{}, want: ""},
		{name: "absent key", f: Finding{Evidence: map[string]any{"other": "x"}}, want: ""},
		{name: "wrong type", f: Finding{Evidence: map[string]any{EvidenceVerifierRole: 42}}, want: ""},
		{name: "present", f: Finding{Evidence: map[string]any{EvidenceVerifierRole: "mythos.fuzz"}}, want: "mythos.fuzz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.f.VerifierRole(); got != tc.want {
				t.Fatalf("VerifierRole() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFinding_ReviewDepth_DefaultsCasual matches the documented "treat
// unknown values as casual" rule from verifier.go — a Finding without
// the key, with a wrong type, or with an empty string all read as
// casual. Only an explicit valid string is preserved.
func TestFinding_ReviewDepth_DefaultsCasual(t *testing.T) {
	cases := []struct {
		name string
		f    Finding
		want string
	}{
		{name: "nil evidence", f: Finding{}, want: ReviewDepthCasual},
		{name: "absent key", f: Finding{Evidence: map[string]any{"other": "x"}}, want: ReviewDepthCasual},
		{name: "empty string", f: Finding{Evidence: map[string]any{EvidenceReviewDepth: ""}}, want: ReviewDepthCasual},
		{name: "wrong type", f: Finding{Evidence: map[string]any{EvidenceReviewDepth: 1}}, want: ReviewDepthCasual},
		{name: "adversarial", f: Finding{Evidence: map[string]any{EvidenceReviewDepth: ReviewDepthAdversarial}}, want: ReviewDepthAdversarial},
		{name: "casual explicit", f: Finding{Evidence: map[string]any{EvidenceReviewDepth: ReviewDepthCasual}}, want: ReviewDepthCasual},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.f.ReviewDepth(); got != tc.want {
				t.Fatalf("ReviewDepth() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFinding_VerifiedAt_ParsesRFC3339 walks the three accepted shapes
// (nanoseconds, no-nanos, malformed) so the reader's tolerance matches
// what JSON consumers actually emit.
func TestFinding_VerifiedAt_ParsesRFC3339(t *testing.T) {
	ts := time.Date(2026, 5, 10, 12, 30, 0, 123456789, time.UTC)
	cases := []struct {
		name    string
		f       Finding
		wantOK  bool
		wantSec time.Time // zero if wantOK is false
	}{
		{name: "absent", f: Finding{}, wantOK: false},
		{name: "wrong type", f: Finding{Evidence: map[string]any{EvidenceVerifiedAt: 12345}}, wantOK: false},
		{name: "empty string", f: Finding{Evidence: map[string]any{EvidenceVerifiedAt: ""}}, wantOK: false},
		{name: "malformed", f: Finding{Evidence: map[string]any{EvidenceVerifiedAt: "yesterday"}}, wantOK: false},
		{
			name:    "rfc3339nano",
			f:       Finding{Evidence: map[string]any{EvidenceVerifiedAt: ts.Format(time.RFC3339Nano)}},
			wantOK:  true,
			wantSec: ts,
		},
		{
			name:    "rfc3339",
			f:       Finding{Evidence: map[string]any{EvidenceVerifiedAt: ts.Truncate(time.Second).Format(time.RFC3339)}},
			wantOK:  true,
			wantSec: ts.Truncate(time.Second),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.f.VerifiedAt()
			if ok != tc.wantOK {
				t.Fatalf("VerifiedAt ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantOK && !got.Equal(tc.wantSec) {
				t.Fatalf("VerifiedAt time = %v, want %v", got, tc.wantSec)
			}
			if got := tc.f.IsVerified(); got != tc.wantOK {
				t.Fatalf("IsVerified = %v, want %v", got, tc.wantOK)
			}
		})
	}
}

// TestRunnerVerifierAdversarialStampsVerifiedAt is the §6.5 happy path:
// an adversarial-depth verifier produces a finding, the runner stamps
// EvidenceVerifiedAt with its own clock, and IsVerified() returns true.
func TestRunnerVerifierAdversarialStampsVerifiedAt(t *testing.T) {
	clock := time.Date(2026, 5, 10, 12, 30, 0, 0, time.UTC)
	r, _, reg := newTestRunner(t)
	r.Now = func() time.Time { return clock }

	if err := reg.RegisterVerifier(fakeVerifier{
		name:  "adversarial-v",
		depth: ReviewDepthAdversarial,
		findings: []Finding{
			{Fingerprint: "vf1", Severity: SeverityP1, Title: "deep finding"},
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
	if !f.IsVerified() {
		t.Fatalf("IsVerified = false; Evidence = %v", f.Evidence)
	}
	got, ok := f.VerifiedAt()
	if !ok {
		t.Fatalf("VerifiedAt = (_, false)")
	}
	if !got.Equal(clock) {
		t.Errorf("VerifiedAt = %v, want %v (runner.Now)", got, clock)
	}
	if f.ReviewDepth() != ReviewDepthAdversarial {
		t.Errorf("ReviewDepth = %q, want adversarial", f.ReviewDepth())
	}
}

// TestRunnerVerifierCasualDoesNotStampVerifiedAt is the negative path:
// a casual-depth verifier produces findings, but the runner declines to
// promote them into the verified set. Trust inversion requires depth,
// not just a verifier signature.
func TestRunnerVerifierCasualDoesNotStampVerifiedAt(t *testing.T) {
	r, _, reg := newTestRunner(t)
	if err := reg.RegisterVerifier(fakeVerifier{
		name:  "casual-v",
		depth: ReviewDepthCasual,
		findings: []Finding{
			{Fingerprint: "vf1", Severity: SeverityP2, Title: "shallow drift"},
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
	if f.IsVerified() {
		t.Fatalf("IsVerified = true on casual-depth verifier; Evidence = %v", f.Evidence)
	}
	if f.ReviewDepth() != ReviewDepthCasual {
		t.Errorf("ReviewDepth = %q, want casual", f.ReviewDepth())
	}
}

// TestRunnerVerifierPreservesCallerSetVerifiedAt mirrors the existing
// "caller-set evidence wins" contract for verifier_role / review_depth.
// A Verifier that already populated EvidenceVerifiedAt — e.g. forwarding
// a finding from an upstream tool that ran earlier — must not have its
// timestamp clobbered.
func TestRunnerVerifierPreservesCallerSetVerifiedAt(t *testing.T) {
	upstream := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	r, _, reg := newTestRunner(t)
	if err := reg.RegisterVerifier(fakeVerifier{
		name:  "passthrough-v",
		depth: ReviewDepthAdversarial,
		findings: []Finding{{
			Fingerprint: "vf1", Severity: SeverityP1, Title: "kept",
			Evidence: map[string]any{
				EvidenceVerifiedAt: upstream,
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
	if got, _ := f.Evidence[EvidenceVerifiedAt].(string); got != upstream {
		t.Errorf("verified_at overwritten: got %q, want %q", got, upstream)
	}
}
