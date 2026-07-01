// Package modelrouter implements task-complexity-based model routing for AGM.
//
// The router assigns each session a cost tier (cheap/mid/expensive) based on
// the task type heuristic. Default split target: 70% cheap, 20% mid, 10%
// expensive. Harness-specific tier → model mappings live in TierModels.
package modelrouter

// Tier classifies a task's model cost tier.
type Tier string

// Cost tier constants that drive model selection.
const (
	TierCheap     Tier = "cheap"     // health checks, formatting, data extraction, simple code
	TierMid       Tier = "mid"       // code gen, PR review, report analysis
	TierExpensive Tier = "expensive" // complex architecture, multi-step supervisor decisions
)

// ParseTier converts a string to a Tier, returning ("", false) for unknown values.
func ParseTier(s string) (Tier, bool) {
	switch Tier(s) {
	case TierCheap, TierMid, TierExpensive:
		return Tier(s), true
	default:
		return "", false
	}
}

// TierModels maps a harness name to (tier → model alias). When the router
// selects a tier it picks the model alias from this table, then hands it to
// agent.ResolveModelFullName for shell-safe expansion.
//
// cheap models (70%): used for health checks, formatting, extraction, simple code
// mid   models (20%): code gen, PR review, report analysis
// expensive models (10%): complex architecture, multi-step supervisor decisions
var TierModels = map[string]map[Tier]string{
	"claude-code": {
		TierCheap:     "haiku",  // claude-haiku-4-5
		TierMid:       "sonnet", // claude-sonnet-4-6[1m]
		TierExpensive: "opus",   // claude-opus-4-8[1m]
	},
	"gemini-cli": {
		TierCheap:     "2.5-flash-lite", // budget multimodal
		TierMid:       "3.5-flash",      // best price-performance
		TierExpensive: "2.5-pro",        // stable, complex tasks
	},
	"codex-cli": {
		TierCheap:     "5.4-mini", // fast, efficient
		TierMid:       "5.5",      // ChatGPT-account supported flagship default
		TierExpensive: "5.5",      // same cap - no separate expensive codex default
	},
	"openrouter": {
		TierCheap:     "deepseek/deepseek-chat-v3-0324:free", // DeepSeek V4 Flash ~$0.28/M
		TierMid:       "google/gemini-flash-1.5",             // Gemini Flash
		TierExpensive: "anthropic/claude-opus-4",             // Opus via OpenRouter
	},
}

// ModelForTier returns the model alias for a harness+tier pair. When no
// explicit mapping exists it falls through to the harness default (mid tier).
func ModelForTier(harness string, tier Tier) (string, bool) {
	tiers, ok := TierModels[harness]
	if !ok {
		return "", false
	}
	m, ok := tiers[tier]
	return m, ok
}
