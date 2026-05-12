package gracefulexit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileIsEnabledByDefault(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if !cfg.Enabled() {
		t.Errorf("missing file should leave guardrail enabled, got disabled")
	}
}

func TestLoadAbsentSectionIsEnabled(t *testing.T) {
	dir := t.TempDir()
	yml := "version: 1\nrepo: demo\n"
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Enabled() {
		t.Errorf("absent section should leave guardrail enabled, got disabled")
	}
}

func TestLoadHonoursExplicitDisable(t *testing.T) {
	dir := t.TempDir()
	yml := `version: 1
framework-defaults:
  graceful-exit:
    disabled: true
    why: "synthetic data generator that must always emit a row"
`
	if err := os.WriteFile(filepath.Join(dir, ".dear-agent.yml"), []byte(yml), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Enabled() {
		t.Errorf("explicit disable should turn the guardrail off")
	}
}

func TestDisableWithoutWhyIsRejected(t *testing.T) {
	yml := []byte(`framework-defaults:
  graceful-exit:
    disabled: true
`)
	if _, err := ParseBytes(yml); err == nil {
		t.Errorf("disabling without why must error")
	}
}

func TestBannerEmptyWhenDisabled(t *testing.T) {
	cfg := Config{Disabled: true, Why: "n/a"}
	if Banner(cfg) != "" {
		t.Errorf("banner must be empty when guardrail is disabled")
	}
}

func TestBannerNonEmptyByDefault(t *testing.T) {
	b := Banner(Config{})
	if b == "" {
		t.Fatalf("banner must be non-empty by default")
	}
	if !strings.Contains(b, "Nothing found") {
		t.Errorf("banner missing canonical phrase: %q", b)
	}
	if !strings.Contains(b, "graceful exit") {
		t.Errorf("banner missing label: %q", b)
	}
	for _, kind := range Applies {
		if !strings.Contains(b, kind) {
			t.Errorf("banner missing applies-kind %q", kind)
		}
	}
}

func TestGuardrailIsStable(t *testing.T) {
	// The Guardrail constant is wire-readable by downstream tools. A
	// change is fine but must be deliberate; this test exists so a
	// drive-by edit shows up in review.
	const expectedPrefix = "If nothing fits the criteria, report that."
	if !strings.HasPrefix(Guardrail, expectedPrefix) {
		t.Errorf("Guardrail prefix changed: %q", Guardrail)
	}
}
