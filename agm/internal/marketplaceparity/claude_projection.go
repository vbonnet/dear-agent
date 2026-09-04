package marketplaceparity

import (
	"encoding/json"
	"fmt"
	"slices"
)

type claudeCatalog struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Owner       Owner               `json:"owner"`
	Plugins     []claudePluginEntry `json:"plugins"`
}

func (catalog *claudeCatalog) UnmarshalJSON(data []byte) error {
	fields, err := decodeJSONObject(data)
	if err != nil {
		return err
	}
	if err := validateClaudeCatalogFields(fields); err != nil {
		return err
	}
	var decoded struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		Owner       Owner               `json:"owner"`
		Plugins     []claudePluginEntry `json:"plugins"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	catalog.Name = decoded.Name
	catalog.Description = decoded.Description
	catalog.Owner = decoded.Owner
	catalog.Plugins = decoded.Plugins
	return nil
}

type claudePluginEntry struct {
	PluginEntry
	fields map[string]json.RawMessage
}

func (entry *claudePluginEntry) UnmarshalJSON(data []byte) error {
	fields, err := decodeJSONObject(data)
	if err != nil {
		return err
	}
	if err := validateExactCaseFields("claude marketplace entry", fields, claudeCanonicalFieldNames); err != nil {
		return err
	}
	var plugin PluginEntry
	if err := json.Unmarshal(data, &plugin); err != nil {
		return err
	}
	entry.PluginEntry = plugin
	entry.fields = fields
	return nil
}

type claudePluginManifest struct {
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Description string       `json:"description"`
	Author      PluginAuthor `json:"author"`
	Repository  string       `json:"repository"`
	License     string       `json:"license"`
	Skills      []string     `json:"skills"`
	fields      map[string]json.RawMessage
}

func (manifest *claudePluginManifest) UnmarshalJSON(data []byte) error {
	fields, err := decodeJSONObject(data)
	if err != nil {
		return err
	}
	if err := validateExactCaseFields("claude plugin manifest", fields, claudeCanonicalFieldNames); err != nil {
		return err
	}
	var decoded struct {
		Name        string       `json:"name"`
		Version     string       `json:"version"`
		Description string       `json:"description"`
		Author      PluginAuthor `json:"author"`
		Repository  string       `json:"repository"`
		License     string       `json:"license"`
		Skills      []string     `json:"skills"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	manifest.Name = decoded.Name
	manifest.Version = decoded.Version
	manifest.Description = decoded.Description
	manifest.Author = decoded.Author
	manifest.Repository = decoded.Repository
	manifest.License = decoded.License
	manifest.Skills = decoded.Skills
	manifest.fields = fields
	return nil
}

var claudeCatalogAllowedFields = map[string]bool{
	"description": true,
	"name":        true,
	"owner":       true,
	"plugins":     true,
}

var claudeMarketplaceEntryAllowedFields = map[string]bool{
	"author":      true,
	"description": true,
	"license":     true,
	"name":        true,
	"repository":  true,
	"source":      true,
	"strict":      true,
	"version":     true,
}

var claudePluginManifestAllowedFields = map[string]bool{
	"author":      true,
	"description": true,
	"license":     true,
	"name":        true,
	"repository":  true,
	"skills":      true,
	"version":     true,
}

var claudeCanonicalFieldNames = []string{
	"$schema", "agents", "allowedMarketplaces", "author", "category", "channels",
	"commands", "defaultEnabled", "dependencies", "description", "displayName",
	"experimental", "homepage", "hooks", "keywords", "license", "lspServers",
	"mcpServers", "metadata", "name", "outputStyles", "plugins", "relevance",
	"renames", "repository", "settings", "skills", "source", "strict", "tags",
	"userConfig", "version", "workflows",
}

// ValidateClaudeMarketplaceMirror verifies the native Claude marketplace is an
// exact source projection of the harness-neutral catalog.
func ValidateClaudeMarketplaceMirror(root string) error {
	neutral, err := LoadCatalog(root)
	if err != nil {
		return err
	}
	var claude claudeCatalog
	if err := readJSONWithin(root, ClaudeCatalogPath, &claude); err != nil {
		return err
	}
	if neutral.Name != claude.Name {
		return fmt.Errorf("claude marketplace name = %q, want %q", claude.Name, neutral.Name)
	}
	neutralByName, err := indexPlugins(neutral.Plugins, "neutral marketplace")
	if err != nil {
		return err
	}
	claudeByName := make(map[string]claudePluginEntry, len(claude.Plugins))
	for _, plugin := range claude.Plugins {
		if _, duplicate := claudeByName[plugin.Name]; duplicate {
			return fmt.Errorf("claude marketplace contains duplicate plugin %q", plugin.Name)
		}
		if plugin.Name == canonicalPluginName {
			if err := validateCanonicalClaudeEntry(plugin); err != nil {
				return err
			}
		}
		claudeByName[plugin.Name] = plugin
	}
	neutralNames := sortedPluginNames(neutralByName)
	claudeNames := sortedClaudePluginNames(claudeByName)
	if !slices.Equal(claudeNames, neutralNames) {
		return fmt.Errorf("claude marketplace plugin inventory = %v, want exactly %v", claudeNames, neutralNames)
	}
	for _, name := range neutralNames {
		plugin := neutralByName[name]
		claudePlugin := claudeByName[name]
		if claudePlugin.Source != plugin.Source {
			return fmt.Errorf("claude marketplace plugin %q source = %q, want %q", plugin.Name, claudePlugin.Source, plugin.Source)
		}
		if claudePlugin.Version != plugin.Version {
			return fmt.Errorf("claude marketplace plugin %q version = %q, want %q", plugin.Name, claudePlugin.Version, plugin.Version)
		}
	}
	return nil
}

func validateCanonicalClaudeEntry(plugin claudePluginEntry) error {
	if err := validateAllowedFields("claude marketplace entry", plugin.fields, claudeMarketplaceEntryAllowedFields); err != nil {
		return err
	}
	raw, present := plugin.fields["strict"]
	if !present {
		return fmt.Errorf("claude marketplace plugin %q must set strict=true", plugin.Name)
	}
	var strict bool
	if err := json.Unmarshal(raw, &strict); err != nil {
		return fmt.Errorf("claude marketplace plugin %q strict field: %w", plugin.Name, err)
	}
	if !strict {
		return fmt.Errorf("claude marketplace plugin %q must set strict=true", plugin.Name)
	}
	return validateCanonicalAuthority("claude marketplace plugin", plugin.PluginEntry)
}

func validateClaudeCatalogFields(fields map[string]json.RawMessage) error {
	if err := validateExactCaseFields("claude marketplace catalog", fields, claudeCanonicalFieldNames); err != nil {
		return err
	}
	for field := range fields {
		if !claudeCatalogAllowedFields[field] {
			return fmt.Errorf("claude marketplace catalog defines forbidden field %q", field)
		}
	}
	return nil
}

func validateAllowedFields(surface string, fields map[string]json.RawMessage, allowed map[string]bool) error {
	for field := range fields {
		if !allowed[field] {
			return fmt.Errorf("%s defines forbidden field %q", surface, field)
		}
	}
	return nil
}

func sortedPluginNames(plugins map[string]PluginEntry) []string {
	names := make([]string, 0, len(plugins))
	for name := range plugins {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func sortedClaudePluginNames(plugins map[string]claudePluginEntry) []string {
	names := make([]string, 0, len(plugins))
	for name := range plugins {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
