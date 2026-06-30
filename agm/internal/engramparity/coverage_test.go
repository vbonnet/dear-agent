package engramparity

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

func TestValidateActiveHarnessSurfaces(t *testing.T) {
	if err := ValidateActiveHarnessSurfaces(); err != nil {
		t.Fatal(err)
	}
	got := ActiveHarnessSurfaces()
	if len(got) != len(agent.ActiveHarnesses()) {
		t.Fatalf("Engram surfaces = %d, want %d: %+v", len(got), len(agent.ActiveHarnesses()), got)
	}
}

func TestActiveHarnessesHaveEngramSurfaces(t *testing.T) {
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(harness)
		if !ok {
			t.Fatalf("SurfaceForHarness(%q) not found", harness)
		}
		if surface.Harness != harness {
			t.Errorf("surface.Harness = %q, want %q", surface.Harness, harness)
		}
		if surface.PersistenceSurface != "manifest.EngramMetadata" {
			t.Errorf("%s persistence surface = %q", harness, surface.PersistenceSurface)
		}
		if surface.StorageSurface != "Dolt engram_* columns" {
			t.Errorf("%s storage surface = %q", harness, surface.StorageSurface)
		}
	}
}

func TestValidateManifestMetadata(t *testing.T) {
	if err := ValidateManifestMetadata(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateOpsSurfaces(t *testing.T) {
	if err := ValidateOpsSurfaces(); err != nil {
		t.Fatal(err)
	}
}
