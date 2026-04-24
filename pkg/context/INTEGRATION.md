# Context Compression — Thalamus Integration Guide

## Overview

The `context` package provides two compression primitives:

1. **Token estimation** (`EstimateTokens`) — heuristic `len(text)/4 * 4/3` with no API call
2. **Tool result clearing** (`ClearOldToolResults`) — replaces old tool results with placeholders

These are designed to integrate with the thalamus sleep cycle as earlier-stage interventions
that extend session lifetime before the nuclear "full consolidation" option.

## Proposed Integration Point

In `core/thalamus/sleep_cycle.go`, the `MonitorSleepCycle` function currently triggers
full consolidation at 80% (160K tokens). The new progressive pipeline:

```
tokens > 60% (120K)  → ClearOldToolResults (this package)
tokens > 80% (160K)  → Full compaction (future: LLM-based summarization)
tokens > 90% (180K)  → Existing sleep cycle (archive + restart)
```

## How to Wire

```go
// In thalamus sleep cycle monitoring loop:

usage := getTokenUsage(sessionID)
estimated := float64(usage.UsedTokens) / float64(usage.TotalTokens)

if estimated > 0.6 {
    config := context.DefaultClearConfig()
    config.MaxContextTokens = usage.TotalTokens
    messages = context.ClearOldToolResults(messages, config)
    // Push cleared messages back to AGM session
}
```

## Key Design Decisions

- **Unclearable tools**: Bash, Read/FileRead, WebSearch results are preserved because
  they contain unique data that can't easily be re-obtained.
- **Clearable tools**: Glob, Grep, Edit, Write results are replaced because the user
  can re-run these tools if needed.
- **Threshold**: 60% is conservative — clears early to avoid hitting the expensive
  80% compaction. Configurable via `ClearConfig.Threshold`.
- **Recent messages**: The last N messages (default 10) are never cleared regardless
  of tool type, preserving immediate working context.

## Dependencies

- The `Message` and `ContentBlock` types are defined locally in this package to avoid
  cross-module dependencies with `core/conversation`. When wiring into thalamus,
  convert between the conversation types and these types, or unify the type definitions.
