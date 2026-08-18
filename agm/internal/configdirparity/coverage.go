// Package configdirparity defines executable coverage for harness-specific
// configuration directory contracts.
package configdirparity

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

// DirectorySurface describes the repo-local configuration directory for a
// harness.
type DirectorySurface struct {
	Harness    string
	Directory  string
	Deprecated bool
	Purpose    string
}

// SurfaceForHarness returns the configuration directory surface for a harness.
func SurfaceForHarness(harness string) (DirectorySurface, bool) {
	switch agent.NormalizeHarnessName(harness) {
	case "claude-code":
		return DirectorySurface{Harness: "claude-code", Directory: ".claude", Purpose: "Claude Code settings, hooks, and skills"}, true
	case "codex-cli":
		return DirectorySurface{Harness: "codex-cli", Directory: ".codex", Purpose: "Codex CLI config and hooks"}, true
	case "agy":
		return DirectorySurface{Harness: "agy", Directory: ".agents", Purpose: "AGY/Antigravity hooks, skills, and instructions"}, true
	case "opencode-cli":
		return DirectorySurface{Harness: "opencode-cli", Directory: ".opencode", Purpose: "OpenCode hooks and fallback metadata"}, true
	case "pi-cli":
		return DirectorySurface{Harness: "pi-cli", Directory: ".pi", Purpose: "Pi skill discovery and project-local settings"}, true
	case "gemini-cli":
		return DirectorySurface{Harness: "gemini-cli", Directory: ".gemini", Deprecated: true, Purpose: "Gemini Code Assist compatibility"}, true
	default:
		return DirectorySurface{}, false
	}
}

// ActiveSurfaces returns configuration directory surfaces for active harnesses.
func ActiveSurfaces() []DirectorySurface {
	active := agent.ActiveHarnesses()
	out := make([]DirectorySurface, 0, len(active))
	for _, harness := range active {
		surface, ok := SurfaceForHarness(harness)
		if ok {
			out = append(out, surface)
		}
	}
	return out
}

// ValidateActiveDirectories verifies active harness directories exist and are
// not marked deprecated.
func ValidateActiveDirectories(root string) error {
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(harness)
		if !ok {
			return fmt.Errorf("active harness %q has no configuration directory surface", harness)
		}
		if surface.Deprecated || surface.Directory == "" || surface.Purpose == "" {
			return fmt.Errorf("active harness %q has invalid configuration directory surface: %+v", harness, surface)
		}
		if err := requireDir(filepath.Join(root, surface.Directory)); err != nil {
			return fmt.Errorf("active harness %q directory %s: %w", harness, surface.Directory, err)
		}
	}
	return nil
}

// ValidateDeprecatedCompatibility verifies deprecated harness directories
// remain available for compatibility.
func ValidateDeprecatedCompatibility(root string) error {
	for _, harness := range agent.DeprecatedHarnesses() {
		surface, ok := SurfaceForHarness(harness)
		if !ok {
			return fmt.Errorf("deprecated harness %q has no configuration directory surface", harness)
		}
		if !surface.Deprecated {
			return fmt.Errorf("deprecated harness %q surface is not marked deprecated", harness)
		}
		if err := requireDir(filepath.Join(root, surface.Directory)); err != nil {
			return fmt.Errorf("deprecated harness %q directory %s: %w", harness, surface.Directory, err)
		}
	}
	return nil
}

func requireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}
