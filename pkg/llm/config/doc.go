// Package config provides configuration management for LLM model preferences.
//
// This package enables per-tool model selection with a three-tier fallback hierarchy:
//  1. Tool-specific provider configuration (e.g., ecphory with Gemini)
//  2. Global defaults from config file
//  3. Hardcoded defaults
//
// # Configuration File
//
// The configuration is loaded from ~/.engram/llm-config.yaml with the following structure:
//
//	tools:
//	  ecphory:
//	    gemini:
//	      model: gemini-2.0-flash-exp
//	      max_tokens: 8192
//	    default_family: gemini
//	  multi-persona-review:
//	    anthropic:
//	      model: claude-opus-4-6
//	      max_tokens: 8192
//	    default_family: anthropic
//	defaults:
//	  anthropic:
//	    model: claude-3-5-sonnet-20241022
//	  gemini:
//	    model: gemini-2.0-flash-exp
//
// # Usage Example
//
//	config, err := config.LoadConfig("~/.engram/llm-config.yaml")
//	if err != nil {
//	    log.Fatalf("Failed to load config: %v", err)
//	}
//
//	// Select model for ecphory with Gemini provider
//	model := config.SelectModel(config, "ecphory", "gemini")
//	// Returns: "gemini-2.0-flash-exp" (from tool-specific config)
//
//	// Select model for review with Claude (no tool-specific config)
//	model = config.SelectModel(config, "review", "anthropic")
//	// Returns: "claude-3-5-sonnet-20241022" (from global defaults)
//
//	// Get max_tokens setting
//	maxTokens := config.GetMaxTokens(config, "ecphory", "gemini")
//	// Returns: 8192
//
// # Design Rationale
//
// Different tools have different cost/accuracy tradeoffs:
//   - ecphory: Runs frequently for semantic search → use flash models for cost
//   - multi-persona-review: High-stakes accuracy → use premium models
//   - review-spec: Balanced complexity → use mid-tier models
//
// This configuration system allows tools to declare their preferences while
// maintaining flexibility through the fallback hierarchy.
package config
