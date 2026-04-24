package evaluation

import "context"

// JudgeResponse represents the result of an LLM-as-judge evaluation
type JudgeResponse struct {
	Pass      bool    // Whether the output meets the criteria
	Score     float64 // Normalized score 0.0-1.0
	Reasoning string  // Explanation of the judgment
}

// EvaluationCriteria defines what to evaluate
type EvaluationCriteria struct {
	Name        string  // Name of the criteria (e.g., "correctness", "safety")
	Description string  // Detailed description of what to evaluate
	Threshold   float64 // Minimum score to pass (0.0-1.0)
}

// Judge represents a simple LLM-based evaluator (legacy interface)
type Judge interface {
	// Evaluate assesses an output and returns a score
	Evaluate(ctx context.Context, prompt string, response string) (float64, error)
}

// DetailedJudge represents an LLM-based evaluator with detailed responses
type DetailedJudge interface {
	// EvaluateDetailed assesses an output against expected output and criteria
	// Returns detailed response with pass/fail, score, and reasoning
	EvaluateDetailed(ctx context.Context, input, expectedOutput string, criteria EvaluationCriteria) (*JudgeResponse, error)
}

// GPT4Judge implements both Judge and DetailedJudge using OpenAI's GPT-4
// Implementation is in gpt4_judge.go

// ClaudeJudge implements both Judge and DetailedJudge using Anthropic's Claude
// Implementation is in claude_judge.go
