package roles

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestShippedConfigValidates is a sanity check on the file shipped at
// config/roles.yaml — operators rely on it as the documented baseline,
// so any structural regression here should be caught at CI time, not at
// `dear-agent run` time.
func TestShippedConfigValidates(t *testing.T) {
	path := filepath.Join("..", "..", "..", "config", "roles.yaml")
	reg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile(%s): %v", path, err)
	}
	for _, want := range []string{"research", "implementer", "reviewer", "orchestrator"} {
		if _, ok := reg.Lookup(want); !ok {
			t.Errorf("shipped config missing role %q", want)
		}
	}
}

// TestShippedConfigSpreadsPrimariesAcrossVendors enforces the
// load-spreading invariant documented in docs/workflow-engine.md and in
// the YAML's header: research/implementer/reviewer must each have a
// primary on a different vendor so a single-provider outage degrades one
// role at a time. orchestrator is exempt because it shares the Anthropic
// vendor with implementer (cheap/fast Sonnet by design).
func TestShippedConfigSpreadsPrimariesAcrossVendors(t *testing.T) {
	path := filepath.Join("..", "..", "..", "config", "roles.yaml")
	reg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	primaryVendor := func(name string) string {
		role, ok := reg.Lookup(name)
		if !ok || role.Primary == nil {
			t.Fatalf("role %q missing primary tier", name)
		}
		return vendorOf(role.Primary.Model)
	}
	got := map[string]string{
		"research":    primaryVendor("research"),
		"implementer": primaryVendor("implementer"),
		"reviewer":    primaryVendor("reviewer"),
	}
	seen := map[string]string{}
	for role, vendor := range got {
		if vendor == "unknown" {
			t.Errorf("role %q: primary model has unknown vendor", role)
			continue
		}
		if other, dup := seen[vendor]; dup {
			t.Errorf("vendor %q is the primary for both %q and %q — load spreading invariant broken",
				vendor, other, role)
		}
		seen[vendor] = role
	}
}

// TestShippedConfigEveryRoleHasFallback asserts every role has at least
// secondary and tertiary tiers on different vendors than the primary,
// so any single-vendor outage still leaves a working tier.
func TestShippedConfigEveryRoleHasFallback(t *testing.T) {
	path := filepath.Join("..", "..", "..", "config", "roles.yaml")
	reg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	for _, name := range reg.RoleNames() {
		role, _ := reg.Lookup(name)
		if role.Primary == nil || role.Secondary == nil || role.Tertiary == nil {
			t.Errorf("role %q: expected primary/secondary/tertiary all set", name)
			continue
		}
		primary := vendorOf(role.Primary.Model)
		vendors := map[string]struct{}{
			vendorOf(role.Secondary.Model): {},
			vendorOf(role.Tertiary.Model):  {},
		}
		delete(vendors, primary)
		if len(vendors) == 0 {
			t.Errorf("role %q: secondary+tertiary share vendor with primary (%q) — single-vendor outage kills role",
				name, primary)
		}
	}
}

// vendorOf maps a canonical model id to its vendor. Kept here (in test
// code) on purpose — the registry intentionally doesn't know about
// vendors so operators can plug in arbitrary new ones in roles.yaml.
func vendorOf(model string) string {
	switch {
	case strings.HasPrefix(model, "claude-"):
		return "anthropic"
	case strings.HasPrefix(model, "gpt-"):
		return "openai"
	case strings.HasPrefix(model, "gemini-"):
		return "google"
	default:
		return "unknown"
	}
}
