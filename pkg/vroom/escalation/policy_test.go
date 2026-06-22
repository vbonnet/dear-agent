package escalation

import (
	"context"
	"testing"
)

func TestDefaultApprovalPolicyCompiles(t *testing.T) {
	if _, err := DefaultApprovalPolicy().Compile(); err != nil {
		t.Fatalf("DefaultApprovalPolicy did not compile: %v", err)
	}
}

func TestCompiledPolicyRequiresHuman(t *testing.T) {
	cp, err := DefaultApprovalPolicy().Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	cases := []struct {
		name     string
		text     string
		wantHigh bool
		wantCat  string // checked only when wantHigh
	}{
		{"hooks edit", "Can I disable the pretool hook for this run?", true, "security-boundary"},
		{"permission widen", "Should I add an allow rule to permissions?", true, "security-boundary"},
		{"write-guard", "Let me bypass the write-guard on ~/src.", true, "security-boundary"},
		{"spend", "Should I pay the $40 invoice?", true, "spend"},
		{"product", "Should we drop the free tier? product decision", true, "product"},
		{"irreversible", "I need to delete the production database", true, "irreversible-infra"},
		{"publishing", "Should I announce this on the blog?", true, "publishing"},
		{"legal", "Does this need a GDPR review?", true, "legal"},
		{"people", "Should we hire another contractor?", true, "people"},
		{"benign refactor", "Should I split this 400-line function?", false, ""},
		{"benign howto", "Which logging library does this repo prefer?", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat, reason, required := cp.RequiresHuman(tc.text)
			if required != tc.wantHigh {
				t.Fatalf("RequiresHuman(%q) = %v, want %v", tc.text, required, tc.wantHigh)
			}
			if required {
				if cat != tc.wantCat {
					t.Errorf("category = %q, want %q", cat, tc.wantCat)
				}
				if reason == "" {
					t.Errorf("expected a non-empty reason for category %q", cat)
				}
			}
			// AutoApproves is the inverse for the default (auto_approve_default=true).
			if got := cp.AutoApproves(tc.text); got == required {
				t.Errorf("AutoApproves(%q) = %v; want inverse of RequiresHuman (%v)", tc.text, got, required)
			}
		})
	}
}

func TestAutoApproveDefaultFalse(t *testing.T) {
	// With auto_approve_default=false, a non-matching decision does NOT auto-approve.
	p := ApprovalPolicy{AutoApproveDefault: false}
	cp, err := p.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if cp.AutoApproves("a perfectly benign question") {
		t.Errorf("AutoApproves should be false when auto_approve_default is false")
	}
}

func TestCompileErrors(t *testing.T) {
	cases := []struct {
		name string
		p    ApprovalPolicy
	}{
		{"no name", ApprovalPolicy{HumanRequired: []HumanRequiredCategory{{Patterns: []string{"x"}}}}},
		{"no patterns", ApprovalPolicy{HumanRequired: []HumanRequiredCategory{{Name: "c"}}}},
		{"bad regexp", ApprovalPolicy{HumanRequired: []HumanRequiredCategory{{Name: "c", Patterns: []string{"("}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.p.Compile(); err == nil {
				t.Errorf("expected compile error for %s, got nil", tc.name)
			}
		})
	}
}

func TestPolicyClassifierUsesCustomTaxonomy(t *testing.T) {
	// A custom policy that flags an otherwise-benign word forces must-reach-human,
	// proving the classifier gates on the supplied policy, not the built-in default.
	custom := ApprovalPolicy{
		AutoApproveDefault: true,
		HumanRequired: []HumanRequiredCategory{
			{Name: "frobnicate", Reason: "frobnication is human-only", Patterns: []string{`(?i)\bfrobnicate\b`}},
		},
	}
	pc, err := NewPolicyClassifier(custom)
	if err != nil {
		t.Fatalf("NewPolicyClassifier: %v", err)
	}
	ctx := context.Background()

	v, err := pc.Classify(ctx, &Escalation{Kind: KindDecision, Question: "Should I frobnicate the widget?"})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if v.Disposition != DispRouteToHuman || !v.MustReachHuman {
		t.Errorf("custom category not honoured: disp=%q human=%v", v.Disposition, v.MustReachHuman)
	}
	if v.Topic != "frobnicate" {
		t.Errorf("topic = %q, want frobnicate", v.Topic)
	}

	// A product decision is NOT in the custom taxonomy, so it falls through to the
	// supervisor — confirming the default markers are gone when a policy is set.
	v2, err := pc.Classify(ctx, &Escalation{Kind: KindDecision, Question: "Should we make this product decision?"})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if v2.MustReachHuman {
		t.Errorf("custom policy should not flag product decisions, got must-reach-human")
	}

	// PolicyClassifier and DefaultClassifier agree on routing when given the
	// default policy — the refactor is behaviour-preserving.
	def, err := NewPolicyClassifier(DefaultApprovalPolicy())
	if err != nil {
		t.Fatalf("NewPolicyClassifier(default): %v", err)
	}
	q := &Escalation{Kind: KindDecision, Question: "Should we make this product decision to drop the free tier?"}
	dv, _ := DefaultClassifier{}.Classify(ctx, q)
	pv, _ := def.Classify(ctx, q)
	if dv.Disposition != pv.Disposition || dv.MustReachHuman != pv.MustReachHuman {
		t.Errorf("default vs policy(default) diverge: %+v vs %+v", dv, pv)
	}
}
