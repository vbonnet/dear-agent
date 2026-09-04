package marketplaceparity

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

const (
	canonicalPluginName        = "spec-governance"
	canonicalPluginSource      = "./spec-governance"
	canonicalPluginVersion     = "0.1.0"
	canonicalPluginDescription = "Canonical SPEC.md authoring and read-only consolidation-audit skills"
	canonicalPluginRepository  = "https://github.com/vbonnet/dear-agent"
	canonicalPluginLicense     = "Apache-2.0"
	canonicalPluginAuthor      = "dear-agent"
	canonicalPluginOwner       = "agm/internal/marketplaceparity/SPEC.md\n"
	maxCanonicalTreeEntries    = 64
	maxCanonicalFileBytes      = 1 << 20
)

var requiredPluginSkills = []string{"audit-specs", "write-spec"}
var requiredPluginSkillExports = []string{"./skills/audit-specs", "./skills/write-spec"}

var allowedCanonicalDirectories = map[string]bool{
	".claude-plugin":                true,
	"skills":                        true,
	"skills/audit-specs":            true,
	"skills/audit-specs/references": true,
	"skills/write-spec":             true,
	"skills/write-spec/references":  true,
}

var forbiddenClaudeDefaultComponentPaths = map[string]bool{
	".lsp.json":     true,
	".mcp.json":     true,
	"agents":        true,
	"bin":           true,
	"channels":      true,
	"commands":      true,
	"hooks":         true,
	"monitors":      true,
	"output-styles": true,
	"settings.json": true,
	"themes":        true,
	"workflows":     true,
}

func validateCanonicalCatalogEntry(plugin PluginEntry) error {
	if plugin.Source != canonicalPluginSource {
		return fmt.Errorf("marketplace catalog required plugin %q source = %q, want exactly %q", plugin.Name, plugin.Source, canonicalPluginSource)
	}
	capabilities := slices.Clone(plugin.Capabilities)
	slices.Sort(capabilities)
	if !slices.Equal(capabilities, []string{"skills"}) {
		return fmt.Errorf("marketplace catalog required plugin %q capabilities = %v, want exactly [skills]", plugin.Name, capabilities)
	}
	return validateCanonicalAuthority("neutral marketplace plugin", plugin)
}

func validateCanonicalAuthority(surface string, plugin PluginEntry) error {
	if plugin.Name != canonicalPluginName {
		return fmt.Errorf("%s name = %q, want %q", surface, plugin.Name, canonicalPluginName)
	}
	if plugin.Version != canonicalPluginVersion {
		return fmt.Errorf("%s %q version = %q, want %q", surface, plugin.Name, plugin.Version, canonicalPluginVersion)
	}
	if plugin.Description != canonicalPluginDescription {
		return fmt.Errorf("%s %q description does not match the canonical package description", surface, plugin.Name)
	}
	if plugin.Repository != canonicalPluginRepository {
		return fmt.Errorf("%s %q repository = %q, want %q", surface, plugin.Name, plugin.Repository, canonicalPluginRepository)
	}
	if plugin.License != canonicalPluginLicense {
		return fmt.Errorf("%s %q license = %q, want %q", surface, plugin.Name, plugin.License, canonicalPluginLicense)
	}
	if plugin.Author.Name != canonicalPluginAuthor || plugin.Author.URL != canonicalPluginRepository || plugin.Author.Email != "" {
		return fmt.Errorf("%s %q author = %+v, want name %q and URL %q", surface, plugin.Name, plugin.Author, canonicalPluginAuthor, canonicalPluginRepository)
	}
	return nil
}

func validateCanonicalManifest(plugin PluginEntry, manifest claudePluginManifest) error {
	if err := validateAllowedFields("claude plugin manifest", manifest.fields, claudePluginManifestAllowedFields); err != nil {
		return err
	}
	manifestAuthority := PluginEntry{
		Name:        manifest.Name,
		Description: manifest.Description,
		Version:     manifest.Version,
		Author:      manifest.Author,
		Repository:  manifest.Repository,
		License:     manifest.License,
	}
	if err := validateCanonicalAuthority("claude plugin manifest", manifestAuthority); err != nil {
		return err
	}
	if manifest.Version != plugin.Version {
		return fmt.Errorf("skill-capable plugin %q manifest version %q does not match catalog version %q", plugin.Name, manifest.Version, plugin.Version)
	}
	got := slices.Clone(manifest.Skills)
	want := slices.Clone(requiredPluginSkillExports)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return fmt.Errorf("skill-capable plugin %q manifest exports %v, want exactly %v", plugin.Name, got, want)
	}
	return nil
}

func loadCanonicalExportedSkills(root string, plugin PluginEntry) ([]exportedSkill, error) {
	return canonicalPluginSnapshot(root, plugin)
}

func canonicalPluginSnapshot(root string, plugin PluginEntry) ([]exportedSkill, error) {
	if err := validateCanonicalCatalogEntry(plugin); err != nil {
		return nil, err
	}
	entries, err := readAnchoredTree(root, strings.TrimPrefix(canonicalPluginSource, "./"), maxCanonicalTreeEntries, maxCanonicalFileBytes)
	if err != nil {
		return nil, err
	}
	byPath := make(map[string]anchoredTreeEntry, len(entries))
	for _, entry := range entries {
		byPath[entry.Path] = entry
		if !strings.Contains(entry.Path, "/") && forbiddenClaudeDefaultComponentPaths[entry.Path] {
			return nil, fmt.Errorf("provider-default component surface %q is forbidden", entry.Path)
		}
	}
	if err := validateCanonicalMetadataTree(byPath, plugin); err != nil {
		return nil, err
	}
	return validateCanonicalSkillTree(byPath)
}

func validateCanonicalMetadataTree(entries map[string]anchoredTreeEntry, plugin PluginEntry) error {
	manifest, ok := entries[".claude-plugin/plugin.json"]
	if !ok || manifest.Directory {
		return fmt.Errorf("plugin manifest must be a regular file")
	}
	var decoded claudePluginManifest
	if err := decodeJSONData(".claude-plugin/plugin.json", manifest.Data, &decoded); err != nil {
		return fmt.Errorf("skill-capable plugin %q manifest: %w", plugin.Name, err)
	}
	if decoded.Name != plugin.Name {
		return fmt.Errorf("skill-capable plugin %q manifest has mismatched name %q", plugin.Name, decoded.Name)
	}
	if err := validateCanonicalManifest(plugin, decoded); err != nil {
		return err
	}
	owner, ok := entries[".claude-plugin/SPEC.owner"]
	if !ok || owner.Directory {
		return fmt.Errorf("plugin SPEC.owner must be a regular file")
	}
	if string(owner.Data) != canonicalPluginOwner {
		return fmt.Errorf("plugin SPEC.owner = %q, want %q", string(owner.Data), canonicalPluginOwner)
	}
	for entryPath := range entries {
		if strings.HasPrefix(entryPath, ".claude-plugin/") && entryPath != ".claude-plugin/plugin.json" && entryPath != ".claude-plugin/SPEC.owner" {
			return fmt.Errorf("plugin metadata directory contains unexpected entry %q", entryPath)
		}
	}
	return nil
}

func validateCanonicalSkillTree(entries map[string]anchoredTreeEntry) ([]exportedSkill, error) {
	var skillDirs []string
	var skillFiles []string
	for entryPath, entry := range entries {
		if entry.Directory && !allowedCanonicalDirectories[entryPath] {
			return nil, fmt.Errorf("canonical plugin contains unexpected directory %q", entryPath)
		}
		if entry.Directory && strings.HasPrefix(entryPath, "skills/") && !strings.Contains(strings.TrimPrefix(entryPath, "skills/"), "/") {
			skillDirs = append(skillDirs, strings.TrimPrefix(entryPath, "skills/"))
		}
		if !entry.Directory && path.Base(entryPath) == "SKILL.md" {
			skillFiles = append(skillFiles, entryPath)
		}
	}
	slices.Sort(skillDirs)
	wantDirs := slices.Clone(requiredPluginSkills)
	slices.Sort(wantDirs)
	if !slices.Equal(skillDirs, wantDirs) {
		return nil, fmt.Errorf("provider-default skills inventory = %v, want exactly %v", skillDirs, wantDirs)
	}
	wantSkillFiles := []string{"skills/audit-specs/SKILL.md", "skills/write-spec/SKILL.md"}
	slices.Sort(skillFiles)
	if !slices.Equal(skillFiles, wantSkillFiles) {
		return nil, fmt.Errorf("provider-effective SKILL.md inventory = %v, want exactly %v", skillFiles, wantSkillFiles)
	}
	exported := make([]exportedSkill, 0, len(wantSkillFiles))
	for _, entryPath := range wantSkillFiles {
		entry := entries[entryPath]
		name, err := readSkillNameFromData(entryPath, entry.Data)
		if err != nil {
			return nil, fmt.Errorf("canonical exported skill %q: %w", entryPath, err)
		}
		exported = append(exported, exportedSkill{
			Name:          name,
			CanonicalPath: path.Join("spec-governance", entryPath),
		})
	}
	return exported, nil
}

func validateRequiredSkillInventory(pluginName string, exported []exportedSkill) error {
	if pluginName != canonicalPluginName {
		return nil
	}
	got := make([]string, 0, len(exported))
	for _, skill := range exported {
		got = append(got, skill.Name)
	}
	slices.Sort(got)
	want := slices.Clone(requiredPluginSkills)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return fmt.Errorf("marketplace plugin %q exports skills %v, want exactly %v", pluginName, got, want)
	}
	return nil
}
