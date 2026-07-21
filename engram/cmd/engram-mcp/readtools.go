package main

// Ports of the legacy Python server's read-only tools (engram_retrieve,
// engram_plugins_list, wayfinder_phase_status) so this Go server is a
// drop-in replacement for ai-tools engram/mcp/engram_mcp_server.py.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/statusread"
)

// --- engram_retrieve ---

// RetrieveResult is the engram_retrieve payload (legacy-compatible shape).
type RetrieveResult struct {
	Query      string           `json:"query"`
	TypeFilter string           `json:"type_filter"`
	Results    []map[string]any `json:"results"`
	Count      int              `json:"count"`
	Error      string           `json:"error,omitempty"`
}

// normalizeRetrieveInput validates and applies defaults to the tool input.
func normalizeRetrieveInput(input EngramRetrieveInput) (typeFilter string, topK int, err error) {
	if strings.TrimSpace(input.Query) == "" {
		return "", 0, errors.New("query must be a non-empty string")
	}
	typeFilter = input.TypeFilter
	if typeFilter == "" {
		typeFilter = "all"
	}
	if typeFilter != "all" && typeFilter != "ai" && typeFilter != "why" {
		return "", 0, fmt.Errorf("type_filter must be ai, why, or all; got %q", typeFilter)
	}
	topK = input.TopK
	if topK <= 0 {
		topK = 5
	}
	if topK > 20 {
		topK = 20
	}
	return typeFilter, topK, nil
}

func engramRetrieve(ctx context.Context, cfg *Config, input EngramRetrieveInput) (*RetrieveResult, error) {
	typeFilter, topK, err := normalizeRetrieveInput(input)
	if err != nil {
		return nil, err
	}

	bin := cfg.EngramCLI
	if bin == "" {
		bin = "engram"
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin,
		"retrieve", "--query", input.Query, "--limit", strconv.Itoa(topK), "--format", "json")
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exitErr *exec.ExitError
		msg := err.Error()
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			msg = strings.TrimSpace(string(exitErr.Stderr))
		}
		return &RetrieveResult{Query: input.Query, TypeFilter: typeFilter,
			Results: []map[string]any{}, Error: msg}, nil
	}

	items := parseRetrieveOutput(strings.TrimSpace(string(out)))
	if typeFilter == "ai" || typeFilter == "why" {
		suffix := "." + typeFilter + ".md"
		filtered := items[:0]
		for _, item := range items {
			if retrieveItemMatchesSuffix(item, suffix) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if len(items) > topK {
		items = items[:topK]
	}
	return &RetrieveResult{Query: input.Query, TypeFilter: typeFilter, Results: items, Count: len(items)}, nil
}

// parseRetrieveOutput normalises engram CLI output: a JSON list, an object
// with a "results" key, or plain text wrapped as a single result.
func parseRetrieveOutput(raw string) []map[string]any {
	if raw == "" {
		return []map[string]any{}
	}
	var asList []map[string]any
	if json.Unmarshal([]byte(raw), &asList) == nil {
		return asList
	}
	var asObj map[string]any
	if json.Unmarshal([]byte(raw), &asObj) == nil {
		if results, ok := asObj["results"].([]any); ok {
			items := make([]map[string]any, 0, len(results))
			for _, r := range results {
				if m, ok := r.(map[string]any); ok {
					items = append(items, m)
				}
			}
			return items
		}
		return []map[string]any{asObj}
	}
	return []map[string]any{{"content": raw}}
}

func retrieveItemMatchesSuffix(item map[string]any, suffix string) bool {
	for _, key := range []string{"file", "path", "source", "name"} {
		if val, ok := item[key].(string); ok && strings.HasSuffix(val, suffix) {
			return true
		}
	}
	return false
}

// --- engram_plugins_list ---

// PluginInfo describes one installed plugin.
type PluginInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Location    string `json:"location"`
	Path        string `json:"path"`
}

// PluginsListResult is the engram_plugins_list payload.
type PluginsListResult struct {
	Plugins       []PluginInfo `json:"plugins"`
	Count         int          `json:"count"`
	SearchedPaths []string     `json:"searched_paths"`
}

func pluginsList(engramRoot string) *PluginsListResult {
	res := &PluginsListResult{Plugins: []PluginInfo{}}
	for _, location := range []string{"core", "user"} {
		base := filepath.Join(engramRoot, location, "plugins")
		res.SearchedPaths = append(res.SearchedPaths, base)

		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(base, entry.Name())
			data, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
			if err != nil {
				continue
			}
			meta := parseFlatYAML(string(data))
			pluginType := meta["type"]
			if pluginType == "" {
				pluginType = meta["pattern"]
			}
			if pluginType == "" {
				pluginType = "unknown"
			}
			name := meta["name"]
			if name == "" {
				name = entry.Name()
			}
			res.Plugins = append(res.Plugins, PluginInfo{
				Name:        name,
				Type:        pluginType,
				Description: meta["description"],
				Version:     meta["version"],
				Location:    location,
				Path:        dir,
			})
		}
	}
	res.Count = len(res.Plugins)
	return res
}

// parseFlatYAML extracts top-level "key: value" pairs — plugin.yaml has no
// nesting the legacy tool cared about, and this keeps us dependency-free.
func parseFlatYAML(text string) map[string]string {
	result := map[string]string{}
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, val, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(key) == "" || key != strings.TrimLeft(key, " \t") {
			continue // skip nested/indented keys
		}
		v := strings.TrimSpace(val)
		v = strings.Trim(v, `"'`)
		result[strings.TrimSpace(key)] = v
	}
	return result
}

// --- wayfinder_phase_status ---

// WayfinderStatusResult is the wayfinder_phase_status payload.
type WayfinderStatusResult struct {
	Project    string `json:"project"`
	Phase      string `json:"phase"`
	Progress   string `json:"progress"`
	Status     string `json:"status"`
	SourceFile string `json:"source_file"`
}

func wayfinderStatus(projectPath string) (*WayfinderStatusResult, error) {
	if strings.TrimSpace(projectPath) == "" {
		return nil, errors.New("project_path must be a non-empty string")
	}
	if strings.HasPrefix(projectPath, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			projectPath = filepath.Join(home, strings.TrimPrefix(projectPath, "~"))
		}
	}
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("resolve project path: %w", err)
	}

	statusFile := filepath.Join(abs, "WAYFINDER-STATUS.md")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return nil, fmt.Errorf("WAYFINDER-STATUS.md not found in %s", abs)
	}

	summary, err := statusread.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", statusFile, err)
	}
	return &WayfinderStatusResult{
		Project:    abs,
		Phase:      summary.CurrentWaypoint,
		Progress:   fmt.Sprintf("%d%%", summary.Progress),
		Status:     summary.Status,
		SourceFile: statusFile,
	}, nil
}
