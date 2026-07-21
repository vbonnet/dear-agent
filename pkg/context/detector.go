package context

import (
	"fmt"
	"os"
	"time"
)

// Detector handles context usage detection across different CLI tools.
type Detector struct {
	registry *Registry
}

// NewDetector creates a new detector with the given registry.
func NewDetector(registry *Registry) *Detector {
	return &Detector{
		registry: registry,
	}
}

// Detect auto-detects CLI type and extracts token usage.
func (d *Detector) Detect() (*Usage, error) {
	cli := d.DetectCLI()
	if cli == CLIUnknown {
		return d.DetectFromHeuristic()
	}
	return d.DetectFromSession(sessionIDForCLI(cli), cli)
}

// DetectCLI identifies which CLI is currently running based on environment variables.
func (d *Detector) DetectCLI() CLI {
	if os.Getenv("CLAUDE_SESSION_ID") != "" {
		return CLIClaude
	}
	if os.Getenv("GEMINI_SESSION_ID") != "" {
		return CLIGemini
	}
	if os.Getenv("OPENCODE_SESSION_ID") != "" {
		return CLIOpenCode
	}
	if os.Getenv("CODEX_SESSION_ID") != "" {
		return CLICodex
	}
	if os.Getenv("PI_SESSION_ID") != "" {
		return CLIPi
	}
	if os.Getenv("AGY_CONVERSATION_ID") != "" || os.Getenv("AGY_SESSION_ID") != "" ||
		os.Getenv("ANTIGRAVITY_SESSION_ID") != "" {
		return CLIAgy
	}

	return CLIUnknown
}

// DetectWithModel detects usage for a specific model ID (overrides auto-detection).
func (d *Detector) DetectWithModel(modelID string) (*Usage, error) {
	usage, err := d.Detect()
	if err != nil {
		return nil, err
	}

	// Override model ID
	usage.ModelID = modelID

	// Update total tokens based on model config
	model := d.registry.GetModel(modelID)
	if model != nil {
		usage.TotalTokens = model.MaxContextTokens
		// Recalculate percentage
		usage.PercentageUsed = float64(usage.UsedTokens) / float64(usage.TotalTokens) * 100.0
	}

	return usage, nil
}

// DetectFromSession detects usage for a specific session ID.
func (d *Detector) DetectFromSession(sessionID string, cli CLI) (*Usage, error) {
	switch cli {
	case CLIClaude:
		usage, err := d.DetectFromClaudeSession(sessionID)
		if err == nil {
			return usage, nil
		}
		return d.detectPortableSession(sessionID, cli)
	case CLIGemini, CLIOpenCode, CLICodex, CLIPi, CLIAgy:
		return d.detectPortableSession(sessionID, cli)
	case CLIUnknown:
		return nil, fmt.Errorf("unsupported CLI type: %s", cli)
	}

	return nil, fmt.Errorf("unsupported CLI type: %s", cli)
}

// EstimateFromMessageCount estimates token usage from message count (fallback).
func EstimateFromMessageCount(messageCount int, maxTokens int) *Usage {
	// Average 150 words per message, 1.3 tokens per word
	estimatedTokens := messageCount * 150 * 13 / 10 // (150 * 1.3)

	// Cap at max tokens
	if estimatedTokens > maxTokens {
		estimatedTokens = maxTokens
	}

	percentage := float64(estimatedTokens) / float64(maxTokens) * 100.0

	return &Usage{
		TotalTokens:    maxTokens,
		UsedTokens:     estimatedTokens,
		PercentageUsed: percentage,
		LastUpdated:    time.Now(),
		Source:         "heuristic",
		ModelID:        "default", // Will be updated if model known
		Estimated:      true,
	}
}
