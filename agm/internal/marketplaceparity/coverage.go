// Package marketplaceparity defines executable coverage for neutral plugin
// marketplace publication across harnesses.
package marketplaceparity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

const (
	// NeutralCatalogPath is the harness-neutral marketplace catalog path.
	NeutralCatalogPath = ".dear-agent/marketplace.json"
	// ClaudeCatalogPath is the native Claude marketplace mirror path.
	ClaudeCatalogPath = ".claude-plugin/marketplace.json"
)

// Owner describes the marketplace owner metadata.
type Owner struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// PluginEntry describes one published plugin in a marketplace catalog.
type PluginEntry struct {
	Name         string   `json:"name"`
	Source       string   `json:"source"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// HarnessSurface describes a harness marketplace discovery surface.
type HarnessSurface struct {
	Name    string `json:"name"`
	Mode    string `json:"mode"`
	Catalog string `json:"catalog"`
}

// Catalog describes the harness-neutral marketplace catalog.
type Catalog struct {
	SchemaVersion string           `json:"schema_version"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Owner         Owner            `json:"owner"`
	Plugins       []PluginEntry    `json:"plugins"`
	Harnesses     []HarnessSurface `json:"harnesses"`
}

type claudeCatalog struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Owner       Owner         `json:"owner"`
	Plugins     []PluginEntry `json:"plugins"`
}

// LoadCatalog reads the harness-neutral marketplace catalog from root.
func LoadCatalog(root string) (Catalog, error) {
	var catalog Catalog
	if err := readJSON(filepath.Join(root, NeutralCatalogPath), &catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

// ValidateCatalog verifies the harness-neutral marketplace catalog is complete
// enough for active harness parity.
func ValidateCatalog(root string) error {
	catalog, err := LoadCatalog(root)
	if err != nil {
		return err
	}
	if catalog.SchemaVersion != "dear-agent.marketplace/v1" {
		return fmt.Errorf("marketplace schema_version = %q, want dear-agent.marketplace/v1", catalog.SchemaVersion)
	}
	if catalog.Name == "" || catalog.Owner.Name == "" {
		return fmt.Errorf("marketplace catalog missing name or owner")
	}
	if len(catalog.Plugins) == 0 {
		return fmt.Errorf("marketplace catalog has no plugins")
	}
	for _, plugin := range catalog.Plugins {
		if err := validatePlugin(root, plugin); err != nil {
			return err
		}
	}
	return ValidateHarnessSurfaces(root)
}

// ValidateHarnessSurfaces verifies every active harness has a declared
// marketplace discovery surface.
func ValidateHarnessSurfaces(root string) error {
	catalog, err := LoadCatalog(root)
	if err != nil {
		return err
	}
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(catalog, harness)
		if !ok {
			return fmt.Errorf("active harness %q has no marketplace surface", harness)
		}
		if surface.Mode == "" || surface.Catalog == "" {
			return fmt.Errorf("active harness %q has incomplete marketplace surface: %+v", harness, surface)
		}
		wantMode := ExpectedMarketplaceMode(harness)
		if surface.Mode != wantMode {
			return fmt.Errorf("%s marketplace mode = %q, want %s", harness, surface.Mode, wantMode)
		}
		if _, err := os.Stat(filepath.Join(root, surface.Catalog)); err != nil {
			return fmt.Errorf("%s marketplace catalog %q: %w", harness, surface.Catalog, err)
		}
	}
	return nil
}

// ExpectedMarketplaceMode returns the executable skill-discovery mode owned by
// each active harness.
func ExpectedMarketplaceMode(harness string) string {
	switch agent.NormalizeHarnessName(harness) {
	case "claude-code":
		return "native-claude-plugin-marketplace"
	case "codex-cli":
		return "native-codex-skill"
	case "opencode-cli":
		return "native-opencode-skill"
	case "pi-cli":
		return "native-pi-skill-path"
	default:
		return "agents-md-skill-fallback"
	}
}

// SurfaceForHarness returns the marketplace surface for a harness.
func SurfaceForHarness(catalog Catalog, harness string) (HarnessSurface, bool) {
	normalized := agent.NormalizeHarnessName(harness)
	for _, surface := range catalog.Harnesses {
		if agent.NormalizeHarnessName(surface.Name) == normalized {
			return surface, true
		}
	}
	return HarnessSurface{}, false
}

// ValidateClaudeMarketplaceMirror verifies the native Claude marketplace is a
// projection of the harness-neutral catalog.
func ValidateClaudeMarketplaceMirror(root string) error {
	neutral, err := LoadCatalog(root)
	if err != nil {
		return err
	}
	var claude claudeCatalog
	if err := readJSON(filepath.Join(root, ClaudeCatalogPath), &claude); err != nil {
		return err
	}
	if neutral.Name != claude.Name {
		return fmt.Errorf("claude marketplace name = %q, want %q", claude.Name, neutral.Name)
	}
	byName := make(map[string]PluginEntry, len(claude.Plugins))
	for _, plugin := range claude.Plugins {
		byName[plugin.Name] = plugin
	}
	for _, plugin := range neutral.Plugins {
		claudePlugin, ok := byName[plugin.Name]
		if !ok {
			return fmt.Errorf("claude marketplace missing plugin %q", plugin.Name)
		}
		if claudePlugin.Source != plugin.Source {
			return fmt.Errorf("claude marketplace plugin %q source = %q, want %q", plugin.Name, claudePlugin.Source, plugin.Source)
		}
		if claudePlugin.Version != plugin.Version {
			return fmt.Errorf("claude marketplace plugin %q version = %q, want %q", plugin.Name, claudePlugin.Version, plugin.Version)
		}
	}
	return nil
}

func validatePlugin(root string, plugin PluginEntry) error {
	if plugin.Name == "" || plugin.Source == "" || plugin.Version == "" {
		return fmt.Errorf("marketplace plugin has empty required field: %+v", plugin)
	}
	source := filepath.Join(root, plugin.Source)
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("marketplace plugin %q source %q: %w", plugin.Name, plugin.Source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("marketplace plugin %q source %q is not a directory", plugin.Name, plugin.Source)
	}
	if slices.Contains(plugin.Capabilities, "commands") {
		if err := requireDir(filepath.Join(source, "commands")); err != nil {
			return fmt.Errorf("marketplace plugin %q commands capability: %w", plugin.Name, err)
		}
	}
	if slices.Contains(plugin.Capabilities, "skills") {
		if err := requireSkillSurface(source); err != nil {
			return fmt.Errorf("marketplace plugin %q skills capability: %w", plugin.Name, err)
		}
	}
	return nil
}

func requireSkillSurface(source string) error {
	if info, err := os.Stat(filepath.Join(source, "SKILL.md")); err == nil && !info.IsDir() {
		return nil
	}
	return requireDir(filepath.Join(source, "skills"))
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

func readJSON(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}
