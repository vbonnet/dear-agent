package main

import (
	"testing"

	"github.com/vbonnet/dear-agent/pkg/workflow/roles"
)

func TestRegistryToRouterConfig_BridgesTiers(t *testing.T) {
	reg := &roles.Registry{
		Version: 1,
		Roles: map[string]roles.Role{
			"reviewer": {
				Primary:   &roles.Tier{Model: "gpt-5.5-pro"},
				Secondary: &roles.Tier{Model: "claude-opus-4-8"},
				Tertiary:  &roles.Tier{Model: "gemini-2.5-pro"},
			},
			"implementer": {
				Primary: &roles.Tier{Model: "claude-opus-4-8"},
			},
		},
	}

	cfg := registryToRouterConfig(reg)

	rev, ok := cfg.Roles["reviewer"]
	if !ok {
		t.Fatal("reviewer role missing from bridged config")
	}
	if rev.Primary != "gpt-5.5-pro" {
		t.Errorf("reviewer.primary = %q, want gpt-5.5-pro", rev.Primary)
	}
	if rev.Secondary != "claude-opus-4-8" {
		t.Errorf("reviewer.secondary = %q, want claude-opus-4-8", rev.Secondary)
	}
	if rev.Tertiary != "gemini-2.5-pro" {
		t.Errorf("reviewer.tertiary = %q, want gemini-2.5-pro", rev.Tertiary)
	}

	if got := cfg.Roles["implementer"]; got.Primary != "claude-opus-4-8" {
		t.Errorf("implementer.primary = %q, want claude-opus-4-8", got.Primary)
	}
	// Unset tiers must collapse to empty so Candidates() skips them.
	if got := cfg.Roles["implementer"]; got.Secondary != "" || got.Tertiary != "" {
		t.Errorf("implementer empty tiers = %q/%q, want empty", got.Secondary, got.Tertiary)
	}
}

func TestRegistryToRouterConfig_NilSafe(t *testing.T) {
	cfg := registryToRouterConfig(nil)
	if cfg == nil || cfg.Roles == nil {
		t.Fatal("expected non-nil config with initialised roles map")
	}
	if len(cfg.Roles) != 0 {
		t.Errorf("expected empty roles, got %d", len(cfg.Roles))
	}
}

func TestLoadReviewRouter_BuiltinHasReviewer(t *testing.T) {
	// With no roles file discoverable, AutoLoad returns the builtin
	// registry, which defines the reviewer role. Point discovery at
	// empty dirs so we deterministically hit the builtin.
	t.Setenv("DEAR_AGENT_ROLES", "")
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Chdir(dir)

	r, source, err := loadReviewRouter("")
	if err != nil {
		t.Fatalf("loadReviewRouter: %v", err)
	}
	if source != "<builtin>" {
		t.Errorf("source = %q, want <builtin>", source)
	}
	if !r.HasRole("reviewer") {
		t.Error("builtin router missing reviewer role")
	}
}

func TestReviewCmd_DefaultRole(t *testing.T) {
	f := reviewCmd.Flags().Lookup("role")
	if f == nil {
		t.Fatal("review command missing --role flag")
	}
	if f.DefValue != "reviewer" {
		t.Errorf("--role default = %q, want reviewer", f.DefValue)
	}
}
