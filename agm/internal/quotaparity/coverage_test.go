package quotaparity

import (
	"slices"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

func TestActiveHarnessSurfacesCoverParitySet(t *testing.T) {
	t.Parallel()

	surfaces := ActiveHarnessSurfaces()
	if len(surfaces) != len(agent.ActiveHarnesses()) {
		t.Fatalf("ActiveHarnessSurfaces length = %d, want %d: %v", len(surfaces), len(agent.ActiveHarnesses()), surfaces)
	}

	got := make([]string, 0, len(surfaces))
	for _, surface := range surfaces {
		got = append(got, surface.Harness)
	}
	for _, harness := range agent.ActiveHarnesses() {
		if !slices.Contains(got, harness) {
			t.Errorf("active harness %q missing quota surface; got %v", harness, got)
		}
	}
}

func TestValidateActiveHarnessSurfaces(t *testing.T) {
	t.Parallel()

	if err := ValidateActiveHarnessSurfaces(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateModelFamilyCoverage(t *testing.T) {
	t.Parallel()

	if err := ValidateModelFamilyCoverage(); err != nil {
		t.Fatal(err)
	}
}

func TestModelFamilyCoverageDoesNotPretendUnknownFamiliesArePriced(t *testing.T) {
	t.Parallel()

	for _, family := range []string{"glm", "deepseek", "nemotron", "qwen"} {
		coverage, ok := ModelFamilyCoverageFor(family)
		if !ok {
			t.Fatalf("ModelFamilyCoverageFor(%q) returned no coverage", family)
		}
		if coverage.PricePolicy != "explicitly-unpriced" {
			t.Fatalf("family %q price policy = %q, want explicitly-unpriced", family, coverage.PricePolicy)
		}
	}
}
