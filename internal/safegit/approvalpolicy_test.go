package safegit

import (
	"strings"
	"testing"
)

func TestParseConfig_ApprovalPolicy(t *testing.T) {
	data := []byte(`
approval_policy:
  auto_approve_default: true
  human_required:
    - name: security-boundary
      reason: "touches a guard"
      patterns:
        - '(?i)\bwrite.?guard\b'
`)
	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.ApprovalPolicy == nil {
		t.Fatal("approval_policy not parsed")
	}
	if !cfg.ApprovalPolicy.AutoApproveDefault {
		t.Error("auto_approve_default should be true")
	}
	if len(cfg.ApprovalPolicy.HumanRequired) != 1 {
		t.Fatalf("want 1 category, got %d", len(cfg.ApprovalPolicy.HumanRequired))
	}

	// EscalationPolicy converts and the result compiles + gates as expected.
	cp, err := cfg.EscalationPolicy().Compile()
	if err != nil {
		t.Fatalf("compile converted policy: %v", err)
	}
	if _, _, required := cp.RequiresHuman("bypass the write-guard"); !required {
		t.Error("converted policy should require human for write-guard")
	}
	if _, _, required := cp.RequiresHuman("a product decision"); required {
		t.Error("converted custom policy should NOT carry the default product marker")
	}
}

func TestEscalationPolicy_FallsBackToDefault(t *testing.T) {
	// A config with no approval_policy yields the built-in default taxonomy.
	cfg, err := ParseConfig([]byte("flaky_checks: []\n"))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	cp, err := cfg.EscalationPolicy().Compile()
	if err != nil {
		t.Fatalf("compile default: %v", err)
	}
	if _, _, required := cp.RequiresHuman("Should we make this product decision?"); !required {
		t.Error("default policy should flag product decisions")
	}

	// A nil *Config also falls back without panicking.
	var nilCfg *Config
	if _, err := nilCfg.EscalationPolicy().Compile(); err != nil {
		t.Fatalf("nil-config default compile: %v", err)
	}
}

func TestParseConfig_ApprovalPolicy_Invalid(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"no name", "approval_policy:\n  human_required:\n    - patterns: ['x']\n"},
		{"no patterns", "approval_policy:\n  human_required:\n    - name: c\n"},
		{"bad regexp", "approval_policy:\n  human_required:\n    - name: c\n      patterns: ['(']\n"},
		{"unknown field", "approval_policy:\n  no_such_key: true\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseConfig([]byte(tc.yaml)); err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

// TestRepoSafeMergeYAML guards the checked-in repo-root .safe-merge.yml: it must
// parse, compile, and carry the four DEAR-retro buckets. If it drifts from the
// schema this fails loudly rather than silently disabling the gate at runtime.
func TestRepoSafeMergeYAML(t *testing.T) {
	cfg, err := LoadRepoConfig()
	if err != nil {
		t.Fatalf("LoadRepoConfig: %v", err)
	}
	if cfg == nil || cfg.ApprovalPolicy == nil {
		t.Skip("no repo-root .safe-merge.yml with approval_policy (not in a checkout)")
	}
	cp, err := cfg.EscalationPolicy().Compile()
	if err != nil {
		t.Fatalf("repo .safe-merge.yml approval_policy did not compile: %v", err)
	}
	for _, probe := range []struct{ text, wantCat string }{
		{"bypass the write-guard", "security-boundary"},
		{"pay the $40 invoice", "spend"},
		{"a product decision", "product"},
		{"delete the production database", "irreversible-infra"},
	} {
		cat, _, required := cp.RequiresHuman(probe.text)
		if !required || cat != probe.wantCat {
			t.Errorf("%q -> (cat=%q required=%v), want category %q", probe.text, cat, required, probe.wantCat)
		}
	}
}

func TestParseConfig_ApprovalPolicyError_NamesField(t *testing.T) {
	_, err := ParseConfig([]byte("approval_policy:\n  human_required:\n    - name: c\n      patterns: ['(']\n"))
	if err == nil || !strings.Contains(err.Error(), "patterns[0]") {
		t.Fatalf("error should name the offending pattern, got: %v", err)
	}
}
