package delegation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// HeadlessStrategy invokes CLI binaries in non-interactive (headless) mode.
//
// Supported CLIs:
//   - gemini -p "prompt" (Gemini CLI)
//   - claude -p "prompt" (Claude CLI)
//   - codex exec "prompt" (Codex CLI)
//
// This strategy preserves OAuth credentials if the CLI is running in a harness
// environment, as the CLI will inherit the same session context.
//
// Example usage:
//
//	strategy := NewHeadlessStrategy("anthropic")
//	if strategy.Available() {
//	    output, err := strategy.Execute(ctx, &AgentInput{
//	        Prompt: "Explain quantum computing",
//	        Model: "claude-opus-4",
//	    })
//	}
type HeadlessStrategy struct {
	provider string // "gemini", "google", "anthropic", "claude", "codex"
}

// NewHeadlessStrategy creates a new headless strategy for the given provider.
//
// Supported providers:
//   - "gemini" or "google": Uses gemini CLI
//   - "anthropic" or "claude": Uses claude CLI
//   - "codex": Uses codex CLI
func NewHeadlessStrategy(provider string) *HeadlessStrategy {
	return &HeadlessStrategy{provider: provider}
}

// Name returns the strategy name for logging and debugging.
func (s *HeadlessStrategy) Name() string {
	return "Headless"
}

// Available checks if the CLI binary for this provider is available in PATH.
//
// Returns true if the corresponding CLI binary (gemini, claude, or codex) is found.
func (s *HeadlessStrategy) Available() bool {
	switch s.provider {
	case "gemini", "google":
		_, err := exec.LookPath("gemini")
		return err == nil
	case "anthropic", "claude":
		_, err := exec.LookPath("claude")
		return err == nil
	case "codex":
		_, err := exec.LookPath("codex")
		return err == nil
	default:
		return false
	}
}

// Execute runs an LLM agent request using the CLI in headless mode.
//
// The CLI is invoked with the appropriate flags for non-interactive execution:
//   - gemini: gemini -p "prompt"
//   - claude: claude -p "prompt"
//   - codex: codex exec "prompt"
//
// The CLI output is expected to be plain text (or JSON if the CLI supports it).
// Errors from the CLI (via stderr) are captured and returned as StrategyError.
func (s *HeadlessStrategy) Execute(ctx context.Context, input *AgentInput) (*AgentOutput, error) {
	if input == nil {
		return nil, NewStrategyError("Headless", "execute", fmt.Errorf("input cannot be nil"))
	}

	if input.Prompt == "" {
		return nil, NewStrategyError("Headless", "execute", fmt.Errorf("prompt cannot be empty"))
	}

	// Build command based on provider
	var cmd *exec.Cmd
	switch s.provider {
	case "gemini", "google":
		cmd = exec.CommandContext(ctx, "gemini", "-p", input.Prompt)
	case "anthropic", "claude":
		cmd = exec.CommandContext(ctx, "claude", "-p", input.Prompt)
	case "codex":
		cmd = exec.CommandContext(ctx, "codex", "exec", input.Prompt)
	default:
		return nil, NewStrategyError("Headless", "execute",
			fmt.Errorf("unsupported provider: %s", s.provider))
	}

	// Execute command and capture both stdout and stderr
	output, err := cmd.Output()
	if err != nil {
		// Check if it's an ExitError (contains stderr)
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			stderr := string(exitErr.Stderr)
			return nil, NewStrategyError("Headless", "execute",
				fmt.Errorf("CLI failed with stderr: %s", stderr))
		}
		return nil, NewStrategyError("Headless", "execute", err)
	}

	// Parse output - try JSON first, fall back to plain text
	outputText := strings.TrimSpace(string(output))
	response := s.parseOutput(outputText, input)

	return response, nil
}

// parseOutput attempts to parse the CLI output as JSON, falling back to plain text.
//
// Some CLIs may return structured JSON with metadata (model, usage, etc.),
// while others return plain text responses. This function handles both cases.
func (s *HeadlessStrategy) parseOutput(outputText string, input *AgentInput) *AgentOutput {
	// Try to parse as JSON first
	var jsonOutput struct {
		Text     string         `json:"text,omitempty"`
		Response string         `json:"response,omitempty"`
		Content  string         `json:"content,omitempty"`
		Model    string         `json:"model,omitempty"`
		Usage    *UsageStats    `json:"usage,omitempty"`
		Metadata map[string]any `json:"metadata,omitempty"`
	}

	if err := json.Unmarshal([]byte(outputText), &jsonOutput); err == nil {
		// Successfully parsed as JSON
		text := jsonOutput.Text
		if text == "" {
			text = jsonOutput.Response
		}
		if text == "" {
			text = jsonOutput.Content
		}

		model := jsonOutput.Model
		if model == "" {
			model = input.Model
		}

		metadata := jsonOutput.Metadata
		if metadata == nil {
			metadata = make(map[string]any)
		}
		metadata["cli"] = s.provider

		return &AgentOutput{
			Text:     text,
			Model:    model,
			Provider: input.Provider,
			Usage:    jsonOutput.Usage,
			Metadata: metadata,
			Strategy: "Headless",
		}
	}

	// Fall back to plain text
	model := input.Model
	if model == "" {
		// Set default models based on provider
		switch s.provider {
		case "gemini", "google":
			model = "gemini-2.0-flash-exp"
		case "anthropic", "claude":
			model = "claude-sonnet-4"
		case "codex":
			model = "codex-default"
		}
	}

	return &AgentOutput{
		Text:     outputText,
		Model:    model,
		Provider: input.Provider,
		Strategy: "Headless",
		Metadata: map[string]any{
			"cli":    s.provider,
			"format": "plaintext",
		},
	}
}
