package harnesseffort

import (
	"encoding/json"
	"fmt"
	"sort"
)

// openCodeModelMap maps concrete model names to OpenCode model strings.
var openCodeModelMap = map[string]string{
	"claude-haiku-4-5":  "google-vertex-anthropic/claude-haiku-4-5@20251001",
	"claude-sonnet-4":   "google-vertex-anthropic/claude-sonnet-4@20250514",
	"claude-opus-4":     "google-vertex-anthropic/claude-opus-4@20250514",
	"o4-mini":           "openai/o4-mini",
	"o3":                "openai/o3",
	"codex-mini-latest": "openai/codex-mini-latest",
	"gemini-2.5-flash":  "google/gemini-2.5-flash",
	"gemini-2.5-pro":    "google/gemini-2.5-pro",
}

// tierPrompts maps effort tier names to canonical system prompts.
var tierPrompts = map[string]string{
	"lookup":      "You are a fast CLI helper. Answer concisely.",
	"operational": "You are a workflow executor. Follow steps precisely.",
	"analysis":    "You are a quality reviewer. Be thorough and critical.",
	"deep":        "You are an architecture reviewer. Consider multiple perspectives.",
}

// commandTierMap maps engram skill names to their canonical effort tier.
var commandTierMap = map[string]string{
	"agm-list":            "lookup",
	"agm-status":          "lookup",
	"agm-assoc":           "lookup",
	"agm-search":          "lookup",
	"agm-exit":            "lookup",
	"agm-new":             "operational",
	"agm-send":            "operational",
	"agm-resume":          "operational",
	"batch-edit":          "operational",
	"find-fix":            "operational",
	"git-flow":            "operational",
	"test-watch":          "operational",
	"bow":                 "operational",
	"create-bead":         "operational",
	"render-diagrams":     "operational",
	"review-spec":         "analysis",
	"review-adr":          "analysis",
	"create-spec":         "analysis",
	"create-diagrams":     "analysis",
	"diagram-sync":        "analysis",
	"retrospect":          "analysis",
	"review-architecture": "deep",
}

// openCodeTierOrder defines canonical tier ordering.
var openCodeTierOrder = []string{"lookup", "operational", "analysis", "deep"}

// providerOrder defines the order providers appear in agent names.
var providerOrder = []string{"anthropic", "openai", "google"}

// openCodeAgentConfig mirrors the OpenCode AgentConfig schema.
type openCodeAgentConfig struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Mode   string `json:"mode"`
}

// openCodeCommandConfig mirrors the OpenCode CommandConfig schema.
type openCodeCommandConfig struct {
	Agent string `json:"agent"`
}

// openCodeDoc is the full opencode.json document structure.
type openCodeDoc struct {
	Schema  string                           `json:"$schema"`
	Model   string                           `json:"model"`
	Agent   map[string]openCodeAgentConfig   `json:"agent"`
	Command map[string]openCodeCommandConfig `json:"command"`
}

// resolveOpenCodeModel converts a concrete model name to OpenCode format.
// Unknown model names pass through unchanged.
func resolveOpenCodeModel(model string) string {
	if mapped, ok := openCodeModelMap[model]; ok {
		return mapped
	}
	return model
}

// agentName returns the OpenCode agent name for a tier+provider combination.
// The anthropic provider uses bare tier name; others use tier-provider.
func agentName(tierName, providerName string) string {
	if providerName == "anthropic" {
		return tierName
	}
	return fmt.Sprintf("%s-%s", tierName, providerName)
}

// GenerateOpenCode produces the content for opencode.json.
// existingJSON is the current file content (empty string if file does not exist).
// Engram-managed agents and commands WIN; user-defined ones not in the default set are preserved.
//nolint:gocyclo // reason: linear template generator covering many fields
func GenerateOpenCode(cfg HarnessEffortConfig, existingJSON string) ([]byte, error) {
	// Start from existing doc or empty
	doc := openCodeDoc{
		Schema:  "https://opencode.ai/config.json",
		Agent:   make(map[string]openCodeAgentConfig),
		Command: make(map[string]openCodeCommandConfig),
	}

	// Merge existing content first
	if existingJSON != "" {
		if err := json.Unmarshal([]byte(existingJSON), &doc); err != nil {
			return nil, fmt.Errorf("parsing existing opencode.json: %w", err)
		}
		if doc.Agent == nil {
			doc.Agent = make(map[string]openCodeAgentConfig)
		}
		if doc.Command == nil {
			doc.Command = make(map[string]openCodeCommandConfig)
		}
	}

	// Always set schema
	doc.Schema = "https://opencode.ai/config.json"

	// Set default model to anthropic operational tier
	if cfg.Tiers != nil {
		if opTier, ok := cfg.Tiers["operational"]; ok {
			if anthProv, ok := opTier.Providers["anthropic"]; ok {
				doc.Model = resolveOpenCodeModel(anthProv.Model)
			}
		}
	}

	// Generate agents for all tiers × providers
	if cfg.Tiers != nil {
		for _, tierName := range openCodeTierOrder {
			tier, ok := cfg.Tiers[tierName]
			if !ok {
				continue
			}
			prompt := tierPrompts[tierName]

			// Built-in providers in canonical order
			for _, provName := range providerOrder {
				prov, ok := tier.Providers[provName]
				if !ok {
					continue
				}
				name := agentName(tierName, provName)
				doc.Agent[name] = openCodeAgentConfig{
					Model:  resolveOpenCodeModel(prov.Model),
					Prompt: prompt,
					Mode:   "primary",
				}
			}

			// Custom providers (not in providerOrder) — also generate agents
			for provName, prov := range tier.Providers {
				isBuiltIn := false
				for _, bp := range providerOrder {
					if provName == bp {
						isBuiltIn = true
						break
					}
				}
				if isBuiltIn {
					continue
				}
				name := agentName(tierName, provName)
				doc.Agent[name] = openCodeAgentConfig{
					Model:  resolveOpenCodeModel(prov.Model),
					Prompt: prompt,
					Mode:   "primary",
				}
			}
		}
	}

	// Generate commands (engram-managed commands win)
	commandNames := make([]string, 0, len(commandTierMap))
	for cmd := range commandTierMap {
		commandNames = append(commandNames, cmd)
	}
	sort.Strings(commandNames)
	for _, cmd := range commandNames {
		tierName := commandTierMap[cmd]
		doc.Command[cmd] = openCodeCommandConfig{Agent: tierName}
	}

	return json.MarshalIndent(doc, "", "  ")
}
