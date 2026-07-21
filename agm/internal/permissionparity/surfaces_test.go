package permissionparity

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
			t.Errorf("active harness %q missing permission surface; got %v", harness, got)
		}
	}
}

func TestValidateActiveHarnessSurfaces(t *testing.T) {
	t.Parallel()

	if err := ValidateActiveHarnessSurfaces(); err != nil {
		t.Fatal(err)
	}
}

func TestPermissionSurfacesDocumentNativeLimits(t *testing.T) {
	t.Parallel()

	codex, ok := SurfaceForHarness("codex-cli")
	if !ok {
		t.Fatal("codex-cli surface missing")
	}
	if codex.PolicySurface != "AGM manifest permission_policy" {
		t.Fatalf("codex policy surface = %q, want manifest-backed policy", codex.PolicySurface)
	}
	if codex.StartupSurface != "codex -s workspace-write" {
		t.Fatalf("codex startup surface = %q, want workspace-write sandbox", codex.StartupSurface)
	}

	agy, ok := SurfaceForHarness("agy")
	if !ok {
		t.Fatal("agy surface missing")
	}
	if agy.StartupSurface != "agy --dangerously-skip-permissions for auto mode" {
		t.Fatalf("agy startup surface = %q, want auto permission flag mapping", agy.StartupSurface)
	}

	pi, ok := SurfaceForHarness("pi")
	if !ok {
		t.Fatal("pi-cli surface missing")
	}
	if !slices.Contains([]string{"/agm-mode plan|default|auto"}, pi.RuntimeSurface) || pi.NativeEnforcement == "" {
		t.Fatalf("Pi permission surface = %+v", pi)
	}
}
