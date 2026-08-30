package ranking

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// buildClaudeRankingPrompt owns the prompt format shared by Claude transport
// adapters. Authentication, transport, model selection, and cost accounting
// remain adapter-local.
func buildClaudeRankingPrompt(query string, candidates []Candidate) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Query: %s\n\nCandidates:\n", query)

	for i, candidate := range candidates {
		fmt.Fprintf(&builder, "%d. Name: %s\n", i, candidate.Name)
		if candidate.Description != "" {
			fmt.Fprintf(&builder, "   Description: %s\n", candidate.Description)
		}
		if len(candidate.Tags) > 0 {
			fmt.Fprintf(&builder, "   Tags: %v\n", candidate.Tags)
		}
		builder.WriteString("\n")
	}

	builder.WriteString("\nProvide a ranked list of candidate indices (0-indexed) with scores and reasoning.")
	return builder.String()
}

// buildClaudeRankingRequest owns the provider-neutral Claude request payload.
// Each adapter remains responsible for invoking its configured SDK client and
// wrapping provider-specific transport errors.
func buildClaudeRankingRequest(model, query string, candidates []Candidate) anthropic.MessageNewParams {
	return anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildClaudeRankingPrompt(query, candidates))),
		},
		System: []anthropic.TextBlockParam{
			{Text: claudeRankingSystemPrompt},
		},
	}
}

// parseClaudeRankingResponse owns the Claude response policy shared by both
// adapters. Structured results retain response order and ignore indices that
// do not name an admitted candidate. Malformed JSON preserves the established
// deterministic fallback behavior.
func parseClaudeRankingResponse(resp *anthropic.Message, candidates []Candidate) ([]RankedResult, error) {
	var responseText strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			responseText.WriteString(block.Text)
		}
	}

	if responseText.Len() == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	var structuredResults []struct {
		Index     int     `json:"index"`
		Score     float64 `json:"score"`
		Reasoning string  `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(responseText.String()), &structuredResults); err == nil {
		results := make([]RankedResult, 0, len(structuredResults))
		for _, result := range structuredResults {
			if result.Index < 0 || result.Index >= len(candidates) {
				continue
			}
			results = append(results, RankedResult{
				Candidate: candidates[result.Index],
				Score:     result.Score,
				Reasoning: result.Reasoning,
			})
		}
		return results, nil
	}

	results := make([]RankedResult, len(candidates))
	for i, candidate := range candidates {
		results[i] = RankedResult{
			Candidate: candidate,
			Score:     1.0 / float64(i+1),
			Reasoning: "Fallback ranking (structured output parsing failed)",
		}
	}
	return results, nil
}

const claudeRankingSystemPrompt = `You are an expert at semantic ranking. Given a user query and a list of candidates, your task is to:

1. Analyze the semantic similarity between the query and each candidate
2. Consider the description and tags of each candidate
3. Rank the candidates by relevance to the query
4. Provide a score (0.0-1.0) and brief reasoning for each candidate

Return your response as a JSON array of objects, each containing:
- "index": the 0-based index of the candidate
- "score": a float between 0.0 (not relevant) and 1.0 (highly relevant)
- "reasoning": a brief explanation (1-2 sentences) of why this score was assigned

Example output format:
[
  {"index": 2, "score": 0.95, "reasoning": "Exact match on key concepts and tags"},
  {"index": 0, "score": 0.7, "reasoning": "Partial match on description"},
  {"index": 1, "score": 0.3, "reasoning": "Weak semantic connection"}
]`
