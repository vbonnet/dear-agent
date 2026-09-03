// Package pricing is the shared source of truth for model pricing across
// ai-tools. Used by agm cost reports and (later) the Go research orchestrator.
//
// Prices are USD per 1M tokens and are approximate — list prices from
// anthropic.com, google.ai, and openai.com at the time of writing. The goal
// is truthful relative cost for budgeting, not invoice-accurate accounting.
package pricing

import "strings"

// ModelPrice is per-million-token pricing for a single model.
type ModelPrice struct {
	// Model is a canonical identifier. May be a short alias ("opus",
	// "sonnet", "haiku") or a full name ("claude-opus-4-6[1m]").
	Model string
	// InputPerMillion is USD per 1M input tokens.
	InputPerMillion float64
	// OutputPerMillion is USD per 1M output tokens.
	OutputPerMillion float64
	// Source identifies the rate-card authority for auditable non-placeholder entries.
	Source string
	// AsOf is the YYYY-MM-DD date on which the source price was verified.
	AsOf string
}

// UnknownModel is returned for models we have no pricing for. Callers should
// display the result as unknown rather than pretending the cost is zero.
var UnknownModel = ModelPrice{InputPerMillion: 0, OutputPerMillion: 0}

// table holds canonical prices. Lookup is case-insensitive and matches against
// both the short alias and a prefix of the full model ID.
//
// The map is intentionally flat — model families are small enough that a
// search over the whole table is cheaper to read than a tree of regexes.
var table = []ModelPrice{
	// Anthropic Claude Fable. Both generations bill the same base rates; they
	// differ only in cache-read multiplier (0.025x on 5.1 vs 0.1x on 5), which
	// this table does not model. The Pro/Max/Team free window on Fable 5 ended
	// 2026-06-23, so list price is the truthful rate for budgeting.
	// The 5.1 entry is listed first so an exact "claude-fable-5-1" lookup is
	// unambiguous even though the "fable" entry would also match it.
	{Model: "claude-fable-5-1", InputPerMillion: 10.00, OutputPerMillion: 50.00, Source: "https://platform.claude.com/docs/en/about-claude/pricing", AsOf: "2026-09-02"},
	{Model: "fable", InputPerMillion: 10.00, OutputPerMillion: 50.00, Source: "https://platform.claude.com/docs/en/about-claude/pricing", AsOf: "2026-09-02"},

	// Anthropic — Claude 4.x
	{Model: "opus", InputPerMillion: 15.00, OutputPerMillion: 75.00},
	{Model: "sonnet", InputPerMillion: 3.00, OutputPerMillion: 15.00},
	{Model: "haiku", InputPerMillion: 1.00, OutputPerMillion: 5.00},

	// Google — Gemini (best-effort; flash pricing dominated by input for our workload)
	// Gemini 3.8 Flash carries introductory pricing through 2026-12-31; it
	// doubles to $1.50/$7.50 on 2027-01-01. Revisit this entry then.
	{Model: "3.8-flash", InputPerMillion: 0.75, OutputPerMillion: 3.75, Source: "https://ai.google.dev/gemini-api/docs/pricing", AsOf: "2026-09-02"},
	{Model: "2.5-pro", InputPerMillion: 1.25, OutputPerMillion: 10.00},
	{Model: "3.1-pro", InputPerMillion: 1.25, OutputPerMillion: 10.00},
	{Model: "3-flash", InputPerMillion: 0.30, OutputPerMillion: 2.50},
	{Model: "3.1-flash-lite", InputPerMillion: 0.075, OutputPerMillion: 0.30},
	{Model: "2.5-flash", InputPerMillion: 0.30, OutputPerMillion: 2.50},
	{Model: "2.5-flash-lite", InputPerMillion: 0.075, OutputPerMillion: 0.30},

	// OpenAI — GPT-5.x (best-effort placeholders; adjust when Anthropic/OpenAI
	// publish authoritative rate cards).
	{Model: "5.4", InputPerMillion: 10.00, OutputPerMillion: 30.00},
	{Model: "5.4-mini", InputPerMillion: 2.00, OutputPerMillion: 8.00},
	{Model: "5.3-codex", InputPerMillion: 3.00, OutputPerMillion: 15.00},
	{Model: "5.3-codex-spark", InputPerMillion: 3.00, OutputPerMillion: 15.00},

	// OpenRouter routed model-family defaults. These are current displayed
	// input/output rates, not cache-adjusted rolling effective prices.
	{Model: "z-ai/glm-5.2", InputPerMillion: 0.93, OutputPerMillion: 3.00, Source: "https://openrouter.ai/z-ai/glm-5.2/api", AsOf: "2026-07-11"},
	{Model: "deepseek/deepseek-v4-pro", InputPerMillion: 0.435, OutputPerMillion: 0.87, Source: "https://openrouter.ai/deepseek/deepseek-v4-pro", AsOf: "2026-07-11"},
	{Model: "nvidia/nemotron-3-ultra-550b-a55b", InputPerMillion: 0.50, OutputPerMillion: 2.20, Source: "https://openrouter.ai/nvidia/nemotron-3-ultra-550b-a55b/pricing", AsOf: "2026-07-11"},
	{Model: "qwen/qwen3.6-max-preview", InputPerMillion: 1.04, OutputPerMillion: 6.24, Source: "https://openrouter.ai/qwen/qwen3.6-max-preview/pricing", AsOf: "2026-07-11"},
}

// Lookup returns a ModelPrice for model, matching on either the short alias or
// a prefix of the canonical full name. A model of "" returns UnknownModel.
//
// Matching rules (first hit wins):
//  1. Exact case-insensitive alias match.
//  2. The full model string contains the alias (handles
//     "claude-opus-4-6[1m]" → opus, "claude-sonnet-4-6" → sonnet, etc.).
func Lookup(model string) ModelPrice {
	if model == "" {
		return UnknownModel
	}
	q := strings.ToLower(model)
	for _, p := range table {
		if strings.EqualFold(p.Model, q) {
			return p
		}
	}
	// Substring match against the canonical alias — so "claude-opus-4-6[1m]"
	// resolves to the "opus" entry.
	for _, p := range table {
		if strings.Contains(q, strings.ToLower(p.Model)) {
			return p
		}
	}
	return UnknownModel
}

// Estimate computes cost in USD for the given model and token counts.
// Returns 0 when the model is unknown — callers that care about the
// unknown case should check Lookup directly.
func Estimate(model string, tokensIn, tokensOut int64) float64 {
	p := Lookup(model)
	return float64(tokensIn)/1_000_000*p.InputPerMillion +
		float64(tokensOut)/1_000_000*p.OutputPerMillion
}

// IsKnown reports whether we have pricing data for the given model.
func IsKnown(model string) bool {
	return Lookup(model) != UnknownModel
}
