// Package engramparity defines executable coverage for harness-neutral Engram
// support.
package engramparity

import (
	"fmt"
	"reflect"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// HarnessSurface describes how a harness receives and persists Engram context.
type HarnessSurface struct {
	Harness            string
	InjectionSurface   string
	PersistenceSurface string
	StorageSurface     string
	Degradation        string
}

// SurfaceForHarness returns the Engram integration contract for a harness.
func SurfaceForHarness(harness string) (HarnessSurface, bool) {
	switch agent.NormalizeHarnessName(harness) {
	case "claude-code":
		return HarnessSurface{
			Harness:            "claude-code",
			InjectionSurface:   "startup prompt, slash command context, and SKILL guidance",
			PersistenceSurface: "manifest.EngramMetadata",
			StorageSurface:     "Dolt engram_* columns",
			Degradation:        "missing retrievals persist disabled metadata rather than empty Claude UUID state",
		}, true
	case "codex-cli":
		return HarnessSurface{
			Harness:            "codex-cli",
			InjectionSurface:   "startup prompt and AGENTS.md/SKILL fallback context",
			PersistenceSurface: "manifest.EngramMetadata",
			StorageSurface:     "Dolt engram_* columns",
			Degradation:        "retrieval metadata remains available without Codex-native memory APIs",
		}, true
	case "agy":
		return HarnessSurface{
			Harness:            "agy",
			InjectionSurface:   "startup prompt and AGENTS.md/SKILL fallback context",
			PersistenceSurface: "manifest.EngramMetadata",
			StorageSurface:     "Dolt engram_* columns",
			Degradation:        "retrieval metadata remains available without AGY-native memory APIs",
		}, true
	case "opencode-cli":
		return HarnessSurface{
			Harness:            "opencode-cli",
			InjectionSurface:   "startup prompt and AGENTS.md/SKILL fallback context",
			PersistenceSurface: "manifest.EngramMetadata",
			StorageSurface:     "Dolt engram_* columns",
			Degradation:        "retrieval metadata remains available without OpenCode-native memory APIs",
		}, true
	case "pi-cli":
		return HarnessSurface{
			Harness:            "pi-cli",
			InjectionSurface:   "startup prompt and native AGENTS.md/SKILL context",
			PersistenceSurface: "manifest.EngramMetadata",
			StorageSurface:     "Dolt engram_* columns",
			Degradation:        "retrieval metadata remains available without Pi-native memory APIs",
		}, true
	default:
		return HarnessSurface{}, false
	}
}

// ActiveHarnessSurfaces returns Engram surfaces for all active harnesses.
func ActiveHarnessSurfaces() []HarnessSurface {
	active := agent.ActiveHarnesses()
	out := make([]HarnessSurface, 0, len(active))
	for _, harness := range active {
		surface, ok := SurfaceForHarness(harness)
		if ok {
			out = append(out, surface)
		}
	}
	return out
}

// ValidateActiveHarnessSurfaces verifies all active harnesses have complete
// Engram surfaces.
func ValidateActiveHarnessSurfaces() error {
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(harness)
		if !ok {
			return fmt.Errorf("active harness %q has no Engram surface", harness)
		}
		if surface.InjectionSurface == "" || surface.PersistenceSurface == "" ||
			surface.StorageSurface == "" || surface.Degradation == "" {
			return fmt.Errorf("active harness %q has incomplete Engram surface: %+v", harness, surface)
		}
	}
	return nil
}

// ValidateManifestMetadata verifies the durable manifest has the shared Engram
// metadata fields every harness depends on.
func ValidateManifestMetadata() error {
	t := reflect.TypeFor[manifest.EngramMetadata]()
	for _, field := range []string{"Enabled", "Query", "EngramIDs", "LoadedAt", "Count"} {
		if _, ok := t.FieldByName(field); !ok {
			return fmt.Errorf("manifest.EngramMetadata missing field %s", field)
		}
	}
	return nil
}

// ValidateOpsSurfaces verifies Engram-facing ops are discoverable through
// harness-neutral operation contracts.
func ValidateOpsSurfaces() error {
	for _, op := range ops.ListOps().Operations {
		if op.Name == "get_session" && op.Surface != "" {
			return nil
		}
	}
	return fmt.Errorf("get_session op missing; Engram metadata would not be discoverable")
}
