package marketplaceparity

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestValidateCatalog(t *testing.T) {
	if err := ValidateCatalog(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestActiveHarnessesHaveMarketplaceSurfaces(t *testing.T) {
	root := repoRoot(t)
	catalog, err := LoadCatalog(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(catalog, harness)
		if !ok {
			t.Fatalf("SurfaceForHarness(%q) not found", harness)
		}
		if surface.Catalog == "" {
			t.Errorf("%s marketplace surface has empty catalog", harness)
		}
		if harness == "claude-code" && surface.Mode != "native-claude-plugin-marketplace" {
			t.Errorf("claude-code mode = %q, want native-claude-plugin-marketplace", surface.Mode)
		}
		if harness != "claude-code" && surface.Mode != "agents-md-skill-fallback" {
			t.Errorf("%s mode = %q, want agents-md-skill-fallback", harness, surface.Mode)
		}
	}
}

func TestClaudeMarketplaceMirrorsNeutralCatalog(t *testing.T) {
	if err := ValidateClaudeMarketplaceMirror(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
}
