package roles

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadBytesValidRegistry(t *testing.T) {
	y := `
version: 1
defaults:
  effort: high
  max_context: 200000
roles:
  research:
    description: "long-context"
    capabilities: [long_context, citations]
    primary:
      model: claude-opus-4-7
      effort: max
      max_context: 1000000
      cost_per_mtok:
        input: 15.0
        output: 75.0
    secondary:
      model: gemini-3.1-pro
`
	reg, err := LoadBytes([]byte(y))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if got := len(reg.Roles); got != 1 {
		t.Errorf("len(Roles) = %d", got)
	}
	role, ok := reg.Lookup("research")
	if !ok {
		t.Fatalf("research role not found")
	}
	if role.Primary.Model != "claude-opus-4-7" {
		t.Errorf("primary.model = %q", role.Primary.Model)
	}
	if got := reg.RoleNames(); len(got) != 1 || got[0] != "research" {
		t.Errorf("RoleNames = %v", got)
	}
}

func TestLoadBytesRejectsRoleWithNoTiers(t *testing.T) {
	y := `
roles:
  empty:
    description: "no tiers"
`
	_, err := LoadBytes([]byte(y))
	if err == nil || !strings.Contains(err.Error(), "tier") {
		t.Errorf("expected tier error, got %v", err)
	}
}

func TestLoadBytesRejectsTierWithoutModel(t *testing.T) {
	y := `
roles:
  bad:
    primary:
      effort: high
`
	_, err := LoadBytes([]byte(y))
	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Errorf("expected model error, got %v", err)
	}
}

func TestBuiltinRegistry(t *testing.T) {
	reg := BuiltinRegistry()
	if reg == nil {
		t.Fatal("BuiltinRegistry returned nil")
	}
	if err := reg.Validate(); err != nil {
		t.Fatalf("BuiltinRegistry failed Validate: %v", err)
	}
	for _, want := range []string{"research", "implementer", "reviewer"} {
		if _, ok := reg.Lookup(want); !ok {
			t.Errorf("BuiltinRegistry missing %q", want)
		}
	}
}

func TestAutoLoadFallsBackToBuiltin(t *testing.T) {
	reg, src, err := AutoLoad("", "", "")
	if err != nil {
		t.Fatalf("AutoLoad: %v", err)
	}
	if src != "<builtin>" {
		t.Errorf("source = %q, want <builtin>", src)
	}
	if _, ok := reg.Lookup("research"); !ok {
		t.Error("expected built-in research role")
	}
}

func TestAutoLoadPrefersEnvPath(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "roles.yaml")
	if err := os.WriteFile(envFile, []byte(`
roles:
  custom:
    primary:
      model: m1
`), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, src, err := AutoLoad(envFile, "", "")
	if err != nil {
		t.Fatalf("AutoLoad: %v", err)
	}
	if src != envFile {
		t.Errorf("source = %q, want %q", src, envFile)
	}
	if _, ok := reg.Lookup("custom"); !ok {
		t.Error("custom role missing")
	}
}

func TestAutoLoadPrefersCWDOverHome(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()
	cwdFile := filepath.Join(cwd, ".dear-agent", "roles.yaml")
	homeFile := filepath.Join(home, ".config", "dear-agent", "roles.yaml")
	for path, name := range map[string]string{cwdFile: "from-cwd", homeFile: "from-home"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "roles:\n  " + name + ":\n    primary:\n      model: m\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reg, src, err := AutoLoad("", cwd, home)
	if err != nil {
		t.Fatalf("AutoLoad: %v", err)
	}
	if src != cwdFile {
		t.Errorf("source = %q, want cwd path", src)
	}
	if _, ok := reg.Lookup("from-cwd"); !ok {
		t.Error("expected from-cwd role")
	}
}

func TestResolverPrimaryWins(t *testing.T) {
	reg := &Registry{Roles: map[string]Role{
		"research": {
			Primary:   &Tier{Model: "primary-m"},
			Secondary: &Tier{Model: "secondary-m"},
		},
	}}
	res, err := (&Resolver{Registry: reg}).Resolve(Request{Role: "research"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Model != "primary-m" || res.TierName != "primary" {
		t.Errorf("got %+v, want primary-m/primary", res)
	}
}

func TestResolverFallsBackToSecondary(t *testing.T) {
	reg := &Registry{Roles: map[string]Role{
		"research": {
			Capabilities: []string{"long_context"},
			Primary:      &Tier{Model: "p", CostPerMTok: CostPerMTok{Input: 100}},
			Secondary:    &Tier{Model: "s", CostPerMTok: CostPerMTok{Input: 1}},
		},
	}}
	// Cap forces primary to be skipped.
	res, err := (&Resolver{Registry: reg}).Resolve(Request{Role: "research", MaxDollars: 5})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Model != "s" || res.TierName != "secondary" {
		t.Errorf("got %+v, want s/secondary", res)
	}
}

func TestResolverCapabilityFilter(t *testing.T) {
	reg := &Registry{Roles: map[string]Role{
		"research": {
			Capabilities: []string{"long_context"},
			Primary:      &Tier{Model: "p", Capabilities: []string{"citations"}},
		},
	}}
	res, err := (&Resolver{Registry: reg}).Resolve(Request{
		Role:                 "research",
		RequiredCapabilities: []string{"long_context", "citations"},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Model != "p" {
		t.Errorf("got %+v", res)
	}

	// Missing capability → no model.
	_, err = (&Resolver{Registry: reg}).Resolve(Request{
		Role:                 "research",
		RequiredCapabilities: []string{"web_search"},
	})
	if !errors.Is(err, ErrNoModelAvailable) {
		t.Errorf("expected ErrNoModelAvailable, got %v", err)
	}
}

type fakeCapacity struct{ unavailable map[string]bool }

func (f *fakeCapacity) HasCapacity(model string) bool {
	return !f.unavailable[model]
}

func TestResolverCapacityFilter(t *testing.T) {
	reg := &Registry{Roles: map[string]Role{
		"research": {
			Primary:   &Tier{Model: "p"},
			Secondary: &Tier{Model: "s"},
		},
	}}
	r := &Resolver{
		Registry: reg,
		Capacity: &fakeCapacity{unavailable: map[string]bool{"p": true}},
	}
	res, err := r.Resolve(Request{Role: "research"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Model != "s" {
		t.Errorf("got %+v, want s", res)
	}
}

func TestResolverOverridesShortCircuit(t *testing.T) {
	reg := &Registry{Roles: map[string]Role{
		"research": {
			Primary: &Tier{Model: "p"},
		},
	}}
	r := &Resolver{
		Registry:  reg,
		Overrides: map[string]string{"research": "manual-m"},
	}
	res, err := r.Resolve(Request{Role: "research"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Model != "manual-m" || res.TierName != "override" {
		t.Errorf("got %+v, want manual-m/override", res)
	}
}

func TestResolverModelOverrideShortCircuit(t *testing.T) {
	reg := &Registry{Roles: map[string]Role{
		"research": {Primary: &Tier{Model: "p"}},
	}}
	r := &Resolver{Registry: reg}
	res, err := r.Resolve(Request{Role: "research", ModelOverride: "node-m"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Model != "node-m" || res.TierName != "override" {
		t.Errorf("got %+v, want node-m/override", res)
	}
}

func TestResolverLegacyModelPath(t *testing.T) {
	res, err := (&Resolver{}).Resolve(Request{Model: "legacy-m"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Model != "legacy-m" || res.TierName != "model" {
		t.Errorf("got %+v", res)
	}
}

func TestResolverErrorWhenNothingSet(t *testing.T) {
	_, err := (&Resolver{}).Resolve(Request{})
	if !errors.Is(err, ErrNoModelAvailable) {
		t.Errorf("expected ErrNoModelAvailable, got %v", err)
	}
}

func TestResolverUnknownRole(t *testing.T) {
	_, err := (&Resolver{Registry: &Registry{Roles: map[string]Role{}}}).
		Resolve(Request{Role: "ghost"})
	if !errors.Is(err, ErrNoModelAvailable) || !strings.Contains(err.Error(), "ghost") {
		t.Errorf("expected unknown-role error, got %v", err)
	}
}

func TestResolverNilRegistryFallsBackToBuiltin(t *testing.T) {
	res, err := (&Resolver{}).Resolve(Request{Role: "research"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Model == "" {
		t.Error("expected built-in model")
	}
}
