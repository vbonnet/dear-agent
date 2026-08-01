// Package marketplaceparity defines executable coverage for neutral plugin
// marketplace publication across harnesses.
package marketplaceparity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/spec-governance/skillset"
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
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Owner       Owner               `json:"owner"`
	Plugins     []claudePluginEntry `json:"plugins"`
}

type claudePluginEntry struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Strict      *bool    `json:"strict,omitempty"`
	Skills      []string `json:"skills,omitempty"`
}

type nativeSpecGovernanceManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      struct {
		Name string `json:"name"`
	} `json:"author"`
	Skills []string `json:"skills"`
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
		if harness == "claude-code" && surface.Mode != "native-claude-plugin-marketplace" {
			return fmt.Errorf("claude-code marketplace mode = %q, want native-claude-plugin-marketplace", surface.Mode)
		}
		if harness != "claude-code" && surface.Mode != "agents-md-skill-fallback" {
			return fmt.Errorf("%s marketplace mode = %q, want agents-md-skill-fallback", harness, surface.Mode)
		}
		if _, err := os.Stat(filepath.Join(root, surface.Catalog)); err != nil {
			return fmt.Errorf("%s marketplace catalog %q: %w", harness, surface.Catalog, err)
		}
	}
	return nil
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
	byName := make(map[string]claudePluginEntry, len(claude.Plugins))
	for _, plugin := range claude.Plugins {
		if _, exists := byName[plugin.Name]; exists {
			return fmt.Errorf("claude marketplace has duplicate plugin %q", plugin.Name)
		}
		byName[plugin.Name] = plugin
	}
	for _, plugin := range neutral.Plugins {
		claudePlugin, ok := byName[plugin.Name]
		if !ok {
			return fmt.Errorf("claude marketplace missing plugin %q", plugin.Name)
		}
		if claudePlugin.Source != plugin.Source {
			if err := validateClaudeSkillBundleAdapter(root, plugin, claudePlugin); err != nil {
				return fmt.Errorf("claude marketplace plugin %q source = %q, want %q or a valid native skill-bundle adapter: %w", plugin.Name, claudePlugin.Source, plugin.Source, err)
			}
		}
		if claudePlugin.Version != plugin.Version {
			return fmt.Errorf("claude marketplace plugin %q version = %q, want %q", plugin.Name, claudePlugin.Version, plugin.Version)
		}
		if plugin.Name == "spec-governance" {
			if err := validateNativeSpecGovernanceSurface(root, plugin, claudePlugin); err != nil {
				return fmt.Errorf("claude marketplace SPEC governance plugin: %w", err)
			}
		}
	}
	return nil
}

func validateNativeSpecGovernanceSurface(root string, neutral PluginEntry, claude claudePluginEntry) error {
	if claude.Source != "./spec-governance" || neutral.Source != "./spec-governance" {
		return errors.New("must use the isolated spec-governance plugin root")
	}
	pluginRoot := filepath.Join(root, "spec-governance")
	if err := requireRealDirectoryTree(root, "spec-governance"); err != nil {
		return fmt.Errorf("isolated plugin root: %w", err)
	}
	want, err := nativeSkillExports(pluginRoot)
	if err != nil {
		return err
	}
	if claude.Strict != nil || len(claude.Skills) != 0 {
		return errors.New("must delegate its native surface to the isolated plugin manifest")
	}
	var manifest nativeSpecGovernanceManifest
	if err := readStrictJSON(filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"), &manifest); err != nil {
		return err
	}
	if manifest.Name != "spec-governance" || manifest.Version != neutral.Version || manifest.Description == "" || manifest.Author.Name == "" {
		return errors.New("isolated plugin manifest has incomplete identity")
	}
	if !slices.Equal(manifest.Skills, want) {
		return fmt.Errorf("isolated plugin skills = %v, want exact canonical set %v", manifest.Skills, want)
	}
	return rejectForbiddenSpecGovernanceSurfaces(pluginRoot)
}

func rejectForbiddenSpecGovernanceSurfaces(pluginRoot string) error {
	for _, forbidden := range []string{".mcp.json", "mcp.json", ".lsp.json", "lsp.json", "hooks", "agents", "commands"} {
		if _, err := os.Lstat(filepath.Join(pluginRoot, forbidden)); err == nil {
			return fmt.Errorf("isolated plugin must not expose %s", forbidden)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect isolated plugin %s: %w", forbidden, err)
		}
	}
	return nil
}

func nativeSkillExports(pluginRoot string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(pluginRoot, "skills"))
	if err != nil {
		return nil, fmt.Errorf("read isolated plugin skills: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	wantNames := skillset.Names()
	if !slices.Equal(names, wantNames) {
		return nil, fmt.Errorf("isolated plugin canonical skill directories = %v, want fixed set %v", names, wantNames)
	}
	for _, name := range wantNames {
		path := filepath.Join(pluginRoot, "skills", name, "SKILL.md")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			if err != nil {
				return nil, fmt.Errorf("isolated plugin skill %q: %w", name, err)
			}
			return nil, fmt.Errorf("isolated plugin skill %q has no regular SKILL.md", name)
		}
	}
	return skillset.NativeExports(), nil
}

func validateClaudeSkillBundleAdapter(root string, neutral PluginEntry, claude claudePluginEntry) error {
	if claude.Source != "." {
		return fmt.Errorf("expanded source must be the authenticated marketplace root")
	}
	if claude.Strict == nil || *claude.Strict {
		return fmt.Errorf("expanded source must explicitly set strict to false")
	}
	if len(neutral.Capabilities) != 1 || neutral.Capabilities[0] != "skills" {
		return fmt.Errorf("expanded source is permitted only for an exact skills capability")
	}
	want, err := canonicalSkillExports(root, neutral.Source)
	if err != nil {
		return err
	}
	got := append([]string(nil), claude.Skills...)
	sort.Strings(got)
	if !slices.Equal(got, want) {
		return fmt.Errorf("skill exports = %v, want exact canonical set %v", claude.Skills, want)
	}
	return nil
}

func canonicalSkillExports(root, source string) ([]string, error) {
	canonicalSource, err := catalogSubtree(source)
	if err != nil {
		return nil, err
	}
	skillsRoot := filepath.Join(root, canonicalSource, "skills")
	if err := requireRealDirectoryTree(root, filepath.Join(canonicalSource, "skills")); err != nil {
		return nil, fmt.Errorf("canonical skill directory: %w", err)
	}
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return nil, fmt.Errorf("read canonical skill directory: %w", err)
	}
	want := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		skillFile := filepath.Join(skillsRoot, entry.Name(), "SKILL.md")
		info, statErr := os.Lstat(skillFile)
		if statErr != nil || !info.Mode().IsRegular() {
			if statErr != nil {
				return nil, fmt.Errorf("canonical skill %q: %w", entry.Name(), statErr)
			}
			return nil, fmt.Errorf("canonical skill %q has no regular SKILL.md", entry.Name())
		}
		want = append(want, "./"+canonicalSource+"/skills/"+entry.Name())
	}
	sort.Strings(want)
	return want, nil
}

func catalogSubtree(source string) (string, error) {
	if filepath.IsAbs(source) {
		return "", fmt.Errorf("neutral plugin source must be repository-relative")
	}
	cleaned := filepath.Clean(source)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("neutral plugin source must name a repository subtree")
	}
	return filepath.ToSlash(cleaned), nil
}

func requireRealDirectoryTree(root, relative string) error {
	current := root
	for part := range strings.SplitSeq(filepath.Clean(relative), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%s must be a real directory", filepath.ToSlash(relative))
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

func readStrictJSON(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("parse %s: contains multiple JSON values", path)
		}
		return fmt.Errorf("parse trailing %s: %w", path, err)
	}
	return nil
}
