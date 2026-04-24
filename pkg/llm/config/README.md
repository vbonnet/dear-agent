# LLM Config Package

Per-tool model preferences system for the Engram LLM agent architecture.

## Overview

This package provides configuration management for LLM model selection with a three-tier fallback hierarchy:

1. **Tool-specific provider configuration** - Fine-grained control per tool
2. **Global defaults** - Fallback for unconfigured tools
3. **Hardcoded defaults** - Built-in fallback when no config file exists

## Files

- `schema.go` - Configuration structures with YAML support
- `loader.go` - Config loading and model selection logic
- `loader_test.go` - Comprehensive test suite
- `doc.go` - Package documentation
- `example-config.yaml` - Example configuration file

## Configuration Format

Place config at `~/.engram/llm-config.yaml`:

```yaml
tools:
  ecphory:
    anthropic:
      model: claude-3-5-sonnet-20241022
      max_tokens: 4096
    gemini:
      model: gemini-2.0-flash-exp
      max_tokens: 8192
    default_family: gemini

  multi-persona-review:
    anthropic:
      model: claude-opus-4-6
      max_tokens: 8192
    gemini:
      model: gemini-2.5-pro-exp
      max_tokens: 8192
    default_family: anthropic

defaults:
  anthropic:
    model: claude-3-5-sonnet-20241022
  gemini:
    model: gemini-2.0-flash-exp
```

## Usage

```go
import "github.com/vbonnet/engram/core/pkg/llm/config"

// Load configuration
cfg, err := config.LoadConfig("~/.engram/llm-config.yaml")
if err != nil {
    log.Fatalf("Failed to load config: %v", err)
}

// Select model for a tool
model := config.SelectModel(cfg, "ecphory", "gemini")
// Returns: "gemini-2.0-flash-exp"

// Get max_tokens setting
maxTokens := config.GetMaxTokens(cfg, "ecphory", "gemini")
// Returns: 8192
```

## Design Rationale

Different tools have different cost/accuracy tradeoffs:

- **ecphory**: Runs frequently for semantic search → use flash models (cost-optimized)
- **multi-persona-review**: High-stakes accuracy → use premium models
- **review-spec**: Balanced complexity → use mid-tier models

## Supported Providers

- **anthropic** (alias: claude) - Claude models
- **gemini** (alias: google) - Google Gemini models

## Defaults

When no configuration file exists:

- Anthropic: `claude-3-5-sonnet-20241022`
- Gemini: `gemini-2.0-flash-exp`

## Testing

Run tests with:

```bash
go test -v ./pkg/llm/config
```

All tests pass with 100% coverage of core functionality.

## Integration

This package is part of the LLM agent architecture refactoring (Task 1.3-1.4).
See `PLAN.md` for complete architecture details.
