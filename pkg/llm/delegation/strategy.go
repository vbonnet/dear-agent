// Package delegation provides strategies for LLM agent execution across different environments.
//
// This package implements two delegation strategies:
//   - Headless: Invokes CLI in headless mode (gemini -p, claude -p)
//   - External: Direct SDK calls with API keys or Vertex AI
//
// The factory automatically selects the best strategy based on environment detection
// and user preferences (provider override).
package delegation

import (
	"context"
	"fmt"
)

// DelegationStrategy defines the interface for executing LLM agent requests.
//
// Implementations:
//   - HeadlessStrategy: Invokes CLI binaries in non-interactive mode
//   - ExternalAPIStrategy: Direct SDK calls (API keys or Vertex AI)
type DelegationStrategy interface {
	// Execute runs an LLM agent request and returns the response.
	//
	// Parameters:
	//   - ctx: Context for cancellation and deadlines
	//   - input: Agent request (prompt, model, parameters)
	//
	// Returns:
	//   - output: Agent response (text, usage stats, metadata)
	//   - error: Any execution error
	Execute(ctx context.Context, input *AgentInput) (*AgentOutput, error)

	// Name returns the strategy name for logging and debugging.
	Name() string

	// Available checks if this strategy can be used in the current environment.
	//
	// For example:
	//   - Headless: Returns true if CLI binary (gemini, claude) is available
	//   - External: Always returns true (fallback strategy)
	Available() bool
}

// AgentInput represents a request to an LLM agent.
type AgentInput struct {
	// Prompt is the text prompt to send to the agent.
	Prompt string

	// Model is the specific model to use (e.g., "claude-opus-4-6", "gemini-2.0-flash-exp").
	// If empty, the strategy will use a default model.
	Model string

	// Provider is the provider family (e.g., "anthropic", "gemini").
	// Used for strategy selection and authentication.
	Provider string

	// MaxTokens is the maximum number of tokens to generate.
	// If 0, uses provider default.
	MaxTokens int

	// Temperature controls randomness (0.0 = deterministic, 1.0 = creative).
	// If 0, uses provider default (typically 1.0).
	Temperature float64

	// SystemPrompt is an optional system-level instruction.
	SystemPrompt string

	// ToolName is the name of the engram tool making this request.
	// Used for per-tool model selection from config.
	ToolName string
}

// AgentOutput represents the response from an LLM agent.
type AgentOutput struct {
	// Text is the generated response text.
	Text string

	// Model is the actual model that processed the request.
	// May differ from input if strategy selected a different model.
	Model string

	// Provider is the provider that processed the request.
	Provider string

	// Usage contains token usage statistics.
	Usage *UsageStats

	// Metadata contains strategy-specific metadata.
	Metadata map[string]any

	// Strategy is the name of the strategy that executed this request.
	Strategy string
}

// UsageStats contains token usage information.
type UsageStats struct {
	// InputTokens is the number of tokens in the prompt.
	InputTokens int

	// OutputTokens is the number of tokens in the response.
	OutputTokens int

	// TotalTokens is the sum of input and output tokens.
	TotalTokens int

	// CostUSD is the estimated cost in US dollars (if available).
	CostUSD float64
}

// StrategyError represents an error from a delegation strategy.
type StrategyError struct {
	// Strategy is the name of the strategy that failed.
	Strategy string

	// Operation is the operation that failed (e.g., "execute", "detect").
	Operation string

	// Err is the underlying error.
	Err error
}

// Error implements the error interface.
func (e *StrategyError) Error() string {
	return fmt.Sprintf("%s strategy %s failed: %v", e.Strategy, e.Operation, e.Err)
}

// Unwrap returns the underlying error for errors.Is and errors.As.
func (e *StrategyError) Unwrap() error {
	return e.Err
}

// NewStrategyError creates a new StrategyError.
func NewStrategyError(strategy, operation string, err error) *StrategyError {
	return &StrategyError{
		Strategy:  strategy,
		Operation: operation,
		Err:       err,
	}
}
