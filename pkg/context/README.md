# Context Window Management Library

**Version**: 1.0
**Package**: `github.com/vbonnet/engram/core/pkg/context`
**Status**: Production-ready (Claude Code), Framework for other CLIs
**Last Updated**: 2026-03-18

---

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Core API](#core-api)
4. [CLI Commands](#cli-commands)
5. [Integration Patterns](#integration-patterns)
6. [JSON Output Schema](#json-output-schema)
7. [Error Handling](#error-handling)
8. [Testing](#testing)
9. [Advanced Usage](#advanced-usage)
10. [References](#references)

---

## Overview

The context management library provides research-backed context window monitoring and smart compaction recommendations for LLM conversations. It replaces heuristic estimation with model-specific sweet spot detection based on academic benchmarks (RULER, Lost in the Middle, NeedleBench, etc.).

### Key Features

- **Model-specific thresholds**: 15+ models with benchmark-validated thresholds
- **Multi-CLI support**: Claude Code (production), Gemini CLI, OpenCode, Codex (framework)
- **Smart compaction logic**: Phase-aware recommendations (start/middle/end)
- **Zone classification**: Safe/Warning/Danger/Critical zones
- **Zero dumb-zone sessions**: Prevents exceeding 90% context usage

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                  CLI Detection Layer                        │
│  (Auto-detects: Claude, Gemini, OpenCode, Codex)           │
└────────────────────────┬────────────────────────────────────┘
                         │ Token Usage
┌────────────────────────▼────────────────────────────────────┐
│                  Core Library (Go)                          │
│  Registry → Calculator → Detector                           │
│  (models.yaml) (zones) (multi-CLI)                          │
└────────────────────────┬────────────────────────────────────┘
                         │ JSON/Text Output
┌────────────────────────▼────────────────────────────────────┐
│            Integration Layer                                │
│  Wayfinder | Swarm | AGM | Hooks                           │
└─────────────────────────────────────────────────────────────┘
```

---

## Quick Start

### Installation

```bash
# Library is part of engram core
go get github.com/vbonnet/engram/core/pkg/context
```

### Basic Usage (Go)

```go
package main

import (
    "fmt"
    "github.com/vbonnet/engram/core/pkg/context"
)

func main() {
    // Load model registry
    registry, err := context.NewRegistry("")
    if err != nil {
        panic(err)
    }

    // Create detector and calculator
    detector := context.NewDetector(registry)
    calculator := context.NewCalculator(registry)

    // Detect current context usage
    usage, err := detector.Detect()
    if err != nil {
        panic(err)
    }

    // Calculate zone
    zone, _ := calculator.CalculateZone(usage.PercentageUsed, usage.ModelID)

    // Check if compaction recommended
    shouldCompact, _ := calculator.ShouldCompact(usage, context.PhaseMiddle)

    fmt.Printf("Context: %.1f%% (%s) - Compact: %v\n",
        usage.PercentageUsed, zone, shouldCompact)
}
```

### Basic Usage (CLI)

```bash
# Check current context status
engram context status

# Output:
# 📊 Context: 72.0% (144K/200K) - Zone: ⚠️  warning
# Model: claude-sonnet-4.5 | Source: claude-cli
# Thresholds: Sweet Spot: 60% | Warning: 70% | Danger: 80% | Critical: 90%

# Get JSON output for scripting
engram context status --json

# Check specific model/tokens
engram context check --model claude-sonnet-4.5 --tokens 144000/200000 --phase-state middle

# List all supported models
engram context models --list
```

---

## Core API

### 1. Registry API

**Purpose**: Load and query model configurations from `models.yaml`.

#### `NewRegistry(path string) (*Registry, error)`

Creates a new registry from YAML file.

**Parameters**:
- `path`: Path to models.yaml (empty string = auto-detect: `~/.engram/context/models.yaml` or embedded)

**Returns**:
- `*Registry`: Loaded registry
- `error`: Parse/read errors

**Example**:
```go
// Auto-detect location
registry, err := context.NewRegistry("")

// Explicit path
registry, err := context.NewRegistry("/path/to/models.yaml")
```

#### `GetModel(modelID string) *ModelConfig`

Retrieves model configuration by ID with fuzzy matching.

**Parameters**:
- `modelID`: Model identifier (e.g., "claude-sonnet-4.5", "CLAUDE_SONNET_4_5")

**Returns**:
- `*ModelConfig`: Model config or default fallback (never nil)

**Fuzzy Matching**:
- Case-insensitive
- Hyphens/underscores normalized
- `"claude-sonnet-4.5"` matches `"claude_sonnet_4_5"`, `"CLAUDE-SONNET-4.5"`

**Example**:
```go
model := registry.GetModel("claude-sonnet-4.5")
fmt.Printf("Max tokens: %d\n", model.MaxContextTokens) // 200000

// Fuzzy match
model = registry.GetModel("CLAUDE_SONNET_4_5")
fmt.Printf("Provider: %s\n", model.Provider) // "anthropic"

// Unknown model → default
model = registry.GetModel("unknown-model-xyz")
fmt.Printf("Fallback: %s\n", model.ModelID) // "default"
```

#### `GetThresholds(modelID string) (*Thresholds, error)`

Calculates absolute token thresholds for a model.

**Parameters**:
- `modelID`: Model identifier

**Returns**:
- `*Thresholds`: Token counts and percentages for all zones
- `error`: Model not found

**Example**:
```go
thresholds, err := registry.GetThresholds("claude-sonnet-4.5")
if err != nil {
    panic(err)
}

fmt.Printf("Sweet Spot: %d tokens (%.0f%%)\n",
    thresholds.SweetSpotTokens,    // 120000
    thresholds.SweetSpotPercentage * 100) // 60%

fmt.Printf("Warning: %d tokens (%.0f%%)\n",
    thresholds.WarningTokens,      // 140000
    thresholds.WarningPercentage * 100) // 70%

fmt.Printf("Danger: %d tokens (%.0f%%)\n",
    thresholds.DangerTokens,       // 160000
    thresholds.DangerPercentage * 100) // 80%

fmt.Printf("Critical: %d tokens (%.0f%%)\n",
    thresholds.CriticalTokens,     // 180000
    thresholds.CriticalPercentage * 100) // 90%
```

#### `GetModelCapabilities(modelID string) (*ModelCapabilities, error)`

Returns the capability profile for a model describing its strengths.

**`ModelCapabilities` fields**:
- `ContextStability` (`string`): How well the model handles large contexts (`"high"`, `"medium"`, `"low"`)
- `SelfEvalQuality` (`string`): How reliably the model evaluates its own work (`"high"`, `"medium"`, `"low"`)
- `LongContext` (`bool`): Whether the model supports 1M+ tokens

**Example**:
```go
caps, err := registry.GetModelCapabilities("claude-sonnet-4.5")
if err != nil {
    panic(err)
}
fmt.Printf("Context stability: %s\n", caps.ContextStability) // "high"
fmt.Printf("Self-eval quality: %s\n", caps.SelfEvalQuality)  // "high"
fmt.Printf("Long context: %v\n", caps.LongContext)            // false
```

#### `GetScaffoldingOverrides(modelID string) (*ScaffoldingOverrides, error)`

Returns process adaptation overrides for a model.

**`ScaffoldingOverrides` fields**:
- `SkipContextResets` (`bool`): Model doesn't need context resets between phases
- `EvaluatorRequired` (`bool`): Whether an evaluator step is mandatory
- `LiteProcessEligible` (`bool`): Model can use a lighter Wayfinder process

**Example**:
```go
overrides, err := registry.GetScaffoldingOverrides("claude-sonnet-4.5")
if err != nil {
    panic(err)
}
fmt.Printf("Skip resets: %v\n", overrides.SkipContextResets)
fmt.Printf("Evaluator required: %v\n", overrides.EvaluatorRequired)
fmt.Printf("Lite eligible: %v\n", overrides.LiteProcessEligible)
```

#### `ListModels() []string`

Returns all model IDs in registry.

**Example**:
```go
models := registry.ListModels()
for _, modelID := range models {
    fmt.Println(modelID)
}
// Output:
// claude-sonnet-4.5
// claude-sonnet-4-5
// claude-sonnet-4.6
// gemini-1.5-pro
// gpt-4o
// ...
```

---

### 2. Calculator API

**Purpose**: Calculate zones and compaction recommendations.

#### `NewCalculator(registry *Registry) *Calculator`

Creates a calculator with the given registry.

**Example**:
```go
calculator := context.NewCalculator(registry)
```

#### `CalculateZone(percentage float64, modelID string) (Zone, error)`

Determines context zone based on usage percentage.

**Parameters**:
- `percentage`: Usage percentage (0.0-100.0)
- `modelID`: Model identifier

**Returns**:
- `Zone`: One of `ZoneSafe`, `ZoneWarning`, `ZoneDanger`, `ZoneCritical`
- `error`: Model not found

**Zone Definitions**:
- `ZoneSafe` (< 70%): Quality maintained
- `ZoneWarning` (70-79%): Light degradation begins
- `ZoneDanger` (80-89%): Noticeable quality loss
- `ZoneCritical` (≥ 90%): High utilization, performance degrades

**Example**:
```go
zone, err := calculator.CalculateZone(72.0, "claude-sonnet-4.5")
if err != nil {
    panic(err)
}

switch zone {
case context.ZoneSafe:
    fmt.Println("✅ Context healthy")
case context.ZoneWarning:
    fmt.Println("⚠️  Monitor closely")
case context.ZoneDanger:
    fmt.Println("🔥 Recommend compaction")
case context.ZoneCritical:
    fmt.Println("🚨 critical - Compact now!")
}
```

#### `ShouldCompact(usage *Usage, phaseState PhaseState) (bool, error)`

Determines if compaction is recommended (phase-aware logic).

**Parameters**:
- `usage`: Current usage data
- `phaseState`: One of `PhaseStart`, `PhaseMiddle`, `PhaseEnd`

**Returns**:
- `bool`: True if compaction recommended
- `error`: Model not found or invalid data

**Smart Compaction Rules**:

| Phase State | Threshold | Rationale |
|-------------|-----------|-----------|
| `PhaseStart` | ≥ 70% (warning) | New phase adds context → will hit 85% mid-phase |
| `PhaseMiddle` | ≥ 80% (danger) | Standard threshold |
| `PhaseEnd` | ≥ 85% (danger+5%) | Can push slightly, compact after completion |

**Example**:
```go
usage := &context.Usage{
    TotalTokens:    200000,
    UsedTokens:     144000,
    PercentageUsed: 72.0,
    ModelID:        "claude-sonnet-4.5",
}

// Phase start: compact at 70%+
shouldCompact, _ := calculator.ShouldCompact(usage, context.PhaseStart)
fmt.Println("Phase start:", shouldCompact) // true (72% ≥ 70%)

// Phase middle: compact at 80%+
shouldCompact, _ = calculator.ShouldCompact(usage, context.PhaseMiddle)
fmt.Println("Phase middle:", shouldCompact) // false (72% < 80%)

// Phase end: compact at 85%+
shouldCompact, _ = calculator.ShouldCompact(usage, context.PhaseEnd)
fmt.Println("Phase end:", shouldCompact) // false (72% < 85%)
```

#### `GetCompactionRecommendation(usage *Usage, phaseState PhaseState) (*CompactionRecommendation, error)`

Provides detailed compaction recommendation with reasoning.

**Returns**:
- `*CompactionRecommendation`: Detailed recommendation with urgency and reason
- `error`: Model not found or invalid data

**Example**:
```go
usage := &context.Usage{
    TotalTokens:    200000,
    UsedTokens:     170000,
    PercentageUsed: 85.0,
    ModelID:        "claude-sonnet-4.5",
}

rec, err := calculator.GetCompactionRecommendation(usage, context.PhaseMiddle)
if err != nil {
    panic(err)
}

fmt.Printf("Recommended: %v\n", rec.Recommended) // true
fmt.Printf("Urgency: %s\n", rec.Urgency)         // "high"
fmt.Printf("Reason: %s\n", rec.Reason)
// "Context at 85.0% — utilization is high. Compaction will improve performance."
fmt.Printf("Estimated reduction: %s\n", rec.EstimatedReduction) // "15-25%"
```

#### `Check(usage *Usage, phaseState PhaseState) (*CheckResult, error)`

Performs complete context check (zone + compaction + thresholds).

**Returns**:
- `*CheckResult`: Complete check result
- `error`: Model not found or invalid data

**Example**:
```go
usage := &context.Usage{
    TotalTokens:    200000,
    UsedTokens:     144000,
    PercentageUsed: 72.0,
    ModelID:        "claude-sonnet-4.5",
    Source:         "claude-cli",
}

result, err := calculator.Check(usage, context.PhaseMiddle)
if err != nil {
    panic(err)
}

fmt.Printf("Zone: %s\n", result.Zone)                    // "warning"
fmt.Printf("Percentage: %.1f%%\n", result.Percentage)    // 72.0%
fmt.Printf("Should compact: %v\n", result.ShouldCompact) // false
fmt.Printf("Model: %s\n", result.ModelID)                // "claude-sonnet-4.5"
fmt.Printf("Source: %s\n", result.Source)                // "claude-cli"

// Thresholds
fmt.Printf("Sweet spot: %dK tokens\n", result.Thresholds.SweetSpotTokens/1000) // 120K
fmt.Printf("Warning: %dK tokens\n", result.Thresholds.WarningTokens/1000)       // 140K
fmt.Printf("Danger: %dK tokens\n", result.Thresholds.DangerTokens/1000)         // 160K
fmt.Printf("Critical: %dK tokens\n", result.Thresholds.CriticalTokens/1000)     // 180K
```

---

### 3. Detector API

**Purpose**: Auto-detect CLI type and extract token usage.

#### `NewDetector(registry *Registry) *Detector`

Creates a detector with the given registry.

**Example**:
```go
detector := context.NewDetector(registry)
```

#### `Detect() (*Usage, error)`

Auto-detects CLI type and extracts token usage from current environment.

**Returns**:
- `*Usage`: Current context usage
- `error`: Detection failed

**Detection Strategy**:
1. Check environment variables (`CLAUDE_SESSION_ID`, `GEMINI_SESSION_ID`, etc.)
2. Call CLI-specific detector
3. Fallback to heuristic estimation if detection fails

**Example**:
```go
// Auto-detects from environment
usage, err := detector.Detect()
if err != nil {
    panic(err)
}

fmt.Printf("CLI: %s\n", usage.Source)           // "claude-cli"
fmt.Printf("Model: %s\n", usage.ModelID)        // "claude-sonnet-4.5"
fmt.Printf("Used: %dK/%dK\n",
    usage.UsedTokens/1000,                      // 144K
    usage.TotalTokens/1000)                     // 200K
fmt.Printf("Percentage: %.1f%%\n", usage.PercentageUsed) // 72.0%
```

#### `DetectCLI() CLI`

Identifies which CLI is running based on environment variables.

**Returns**:
- `CLI`: One of `CLIClaude`, `CLIGemini`, `CLIOpenCode`, `CLICodex`, `CLIUnknown`

**Example**:
```go
cli := detector.DetectCLI()

switch cli {
case context.CLIClaude:
    fmt.Println("Running in Claude Code")
case context.CLIGemini:
    fmt.Println("Running in Gemini CLI")
case context.CLIUnknown:
    fmt.Println("Unknown CLI - will use heuristic")
}
```

#### `DetectFromSession(sessionID string, cli CLI) (*Usage, error)`

Detects usage for a specific session ID.

**Parameters**:
- `sessionID`: Session identifier
- `cli`: CLI type

**Example**:
```go
usage, err := detector.DetectFromSession(
    "session-abc123",
    context.CLIClaude,
)
```

#### `DetectWithModel(modelID string) (*Usage, error)`

Detects usage and overrides model ID.

**Use case**: Force specific model when auto-detection fails or needs override.

**Example**:
```go
// Detect usage but force specific model
usage, err := detector.DetectWithModel("claude-opus-4.6")
```

---

### 4. Formatting Helpers

#### `FormatZone(zone Zone) string`

Returns human-readable zone description with emoji.

**Example**:
```go
fmt.Println(context.FormatZone(context.ZoneSafe))     // "✅ safe"
fmt.Println(context.FormatZone(context.ZoneWarning))  // "⚠️  warning"
fmt.Println(context.FormatZone(context.ZoneDanger))   // "🔥 danger"
fmt.Println(context.FormatZone(context.ZoneCritical)) // "🚨 critical"
```

#### `FormatPercentage(pct float64) string`

Formats percentage with one decimal place.

**Example**:
```go
fmt.Println(context.FormatPercentage(72.456)) // "72.5%"
```

#### `FormatTokens(used, total int) string`

Formats token counts (K/M suffixes).

**Example**:
```go
fmt.Println(context.FormatTokens(144000, 200000))   // "144K/200K"
fmt.Println(context.FormatTokens(600000, 1000000))  // "600K/1.0M"
fmt.Println(context.FormatTokens(1500000, 2000000)) // "1.5M/2.0M"
```

---

## CLI Commands

### `engram context status`

Show current context usage.

**Flags**:
- `--session <ID>`: Specific session ID (default: auto-detect from env)
- `--cli <type>`: CLI type: `claude`, `gemini`, `opencode`, `codex` (default: auto-detect)
- `--json`: Output JSON format

**Examples**:

```bash
# Auto-detect current session
engram context status

# Output:
# 📊 Context: 72.0% (144K/200K) - Zone: ⚠️  warning
# Model: claude-sonnet-4.5 | Source: claude-cli
# Thresholds: Sweet Spot: 60% | Warning: 70% | Danger: 80% | Critical: 90%

# JSON output
engram context status --json
# {"percentage":72.0,"zone":"warning","model_id":"claude-sonnet-4.5",...}

# Specific session
engram context status --session abc123 --cli claude
```

### `engram context check`

Check if compaction is recommended for given usage.

**Flags**:
- `--model <ID>`: Model ID (required)
- `--tokens <used/total>`: Token usage (e.g., `144000/200000`) (required)
- `--phase-state <state>`: Phase state: `start`, `middle`, `end` (default: `middle`)
- `--json`: Output JSON format

**Examples**:

```bash
# Check specific usage
engram context check --model claude-sonnet-4.5 --tokens 144000/200000

# Output:
# Zone: ⚠️  warning (72.0%)
# Should compact: false (middle phase: threshold 80%)
# Thresholds: 120K (60%) | 140K (70%) | 160K (80%) | 180K (90%)

# Phase start (lower threshold)
engram context check --model claude-sonnet-4.5 --tokens 144000/200000 --phase-state start

# Output:
# Zone: ⚠️  warning (72.0%)
# Should compact: true (start phase: threshold 70%)
# Recommendation: Phase start at 72.0% - will exceed danger zone mid-phase. Compact now.

# JSON output
engram context check --model claude-sonnet-4.5 --tokens 144000/200000 --json
```

### `engram context models`

List all models in registry.

**Flags**:
- `--list`: List all model IDs (default behavior)
- `--model <ID>`: Show details for specific model
- `--json`: Output JSON format

**Examples**:

```bash
# List all models
engram context models --list

# Output:
# Supported Models (16):
# - claude-sonnet-4.5 (200K, HIGH confidence)
# - claude-sonnet-4.6 (200K, MEDIUM confidence)
# - claude-opus-4.6 (200K, MEDIUM confidence)
# - gemini-1.5-pro (1M, HIGH confidence)
# - gpt-4o (128K, HIGH confidence)
# ...

# Show specific model details
engram context models --model claude-sonnet-4.5

# Output:
# Model: claude-sonnet-4.5
# Provider: anthropic
# Max Context: 200000 tokens
# Thresholds:
#   Sweet Spot: 60% (120K tokens)
#   Warning:    70% (140K tokens)
#   Danger:     80% (160K tokens)
#   Critical:   90% (180K tokens)
# Confidence: HIGH
# Benchmark Sources:
#   - RULER: 85% accuracy up to 60% fill (~120K tokens)
#   - Lost in the Middle: Degradation begins at 80% fill (~160K tokens)
#   - Production Swarm Data: 60/70/80/90% thresholds validated

# JSON output
engram context models --model claude-sonnet-4.5 --json
```

---

## Integration Patterns

### 1. Wayfinder Integration

Add context checking to Step 1.5 of Wayfinder phase transitions.

**Location**: `~/.claude/skills/wayfinder-next-phase.md`

**Implementation**:

```bash
#!/bin/bash
# Step 1.5: Context Check (in wayfinder-next-phase skill)

CONTEXT_JSON=$(engram context status --json 2>/dev/null)

if [ $? -eq 0 ]; then
    PERCENTAGE=$(echo "$CONTEXT_JSON" | jq -r '.percentage')
    ZONE=$(echo "$CONTEXT_JSON" | jq -r '.zone')
    SHOULD_COMPACT=$(echo "$CONTEXT_JSON" | jq -r '.should_compact')
    MODEL=$(echo "$CONTEXT_JSON" | jq -r '.model_id')

    # Display zone-specific alert
    if [ "$ZONE" = "critical" ]; then
        echo "🚨 Context at ${PERCENTAGE}% — near capacity (Model: ${MODEL})"
        echo "   Please compact before continuing."
        exit 1  # Block phase progression
    elif [ "$ZONE" = "danger" ] && [ "$SHOULD_COMPACT" = "true" ]; then
        echo "⚠️  Context at ${PERCENTAGE}% — utilization is high (Model: ${MODEL})"
        echo "   Compaction will improve performance."
    elif [ "$ZONE" = "warning" ]; then
        echo "⚠️  Context at ${PERCENTAGE}% - Monitor closely (Model: ${MODEL})"
    else
        echo "✅ Context healthy - ${PERCENTAGE}% (Model: ${MODEL})"
    fi
fi

# Continue with phase progression...
```

### 2. Swarm Integration

Same as Wayfinder, but include bead context.

**Location**: `./engram/swarm-plugin/commands/next.md`

**Implementation**:

```bash
# Step 1.5: Context Check + Bead Summary

CONTEXT_JSON=$(engram context status --json 2>/dev/null)
ACTIVE_BEADS=$(bd --db ~/.beads list --status active | wc -l)

if [ $? -eq 0 ]; then
    PERCENTAGE=$(echo "$CONTEXT_JSON" | jq -r '.percentage')
    ZONE=$(echo "$CONTEXT_JSON" | jq -r '.zone')

    if [ "$ZONE" = "warning" ] || [ "$ZONE" = "danger" ]; then
        echo "⚠️  Context at ${PERCENTAGE}% with ${ACTIVE_BEADS} active beads"
        echo "   Consider compacting bead summaries before continuing."
    fi
fi
```

### 3. AGM Integration

CLI-based integration (no code dependency).

**Location**: `agm/internal/session/context.go`

**Implementation**:

```go
package session

import (
    "encoding/json"
    "os/exec"
)

type ContextResult struct {
    Percentage    float64 `json:"percentage"`
    Zone          string  `json:"zone"`
    ModelID       string  `json:"model_id"`
    UsedTokens    int     `json:"used_tokens"`
    TotalTokens   int     `json:"total_tokens"`
    ShouldCompact bool    `json:"should_compact"`
}

func DetectContextViaLibrary(sessionID string) (*ContextResult, error) {
    // Call engram CLI
    cmd := exec.Command("engram", "context", "status", "--json")

    output, err := cmd.Output()
    if err != nil {
        // Fallback to existing detection
        return DetectContextFromConversationLog(sessionID)
    }

    var result ContextResult
    if err := json.Unmarshal(output, &result); err != nil {
        return nil, err
    }

    return &result, nil
}

// Status line color coding
func GetZoneColor(zone string) string {
    switch zone {
    case "safe":     return "green"
    case "warning":  return "yellow"
    case "danger":   return "colour208"  // orange
    case "critical": return "red"
    default:         return "white"
    }
}
```

### 4. Hook Integration

Python hook calls Go library via subprocess.

**Location**: `agm/scripts/hooks/posttool-context-monitor.py`

**Implementation**:

```python
import subprocess
import json

def update_context_via_library(self, session_name: str):
    """Use engram library instead of duplicating logic."""
    try:
        result = subprocess.run(
            ['engram', 'context', 'status', '--json'],
            capture_output=True,
            text=True,
            timeout=5
        )

        if result.returncode == 0:
            data = json.loads(result.stdout)
            percentage = data['percentage']
            zone = data['zone']

            # Update AGM manifest
            subprocess.run([
                'agm', 'session', 'set-context-usage',
                str(int(percentage)),
                '--session', session_name
            ])

            return data
    except Exception as e:
        self.log(f"Library call failed: {e}", level='WARN')
        # Fallback to existing logic
        return None
```

---

## JSON Output Schema

### Status Command Output

```json
{
  "percentage": 72.0,
  "zone": "warning",
  "model_id": "claude-sonnet-4.5",
  "source": "claude-cli",
  "used_tokens": 144000,
  "total_tokens": 200000,
  "should_compact": false,
  "phase_state": "middle",
  "thresholds": {
    "sweet_spot_tokens": 120000,
    "sweet_spot_percentage": 0.60,
    "warning_tokens": 140000,
    "warning_percentage": 0.70,
    "danger_tokens": 160000,
    "danger_percentage": 0.80,
    "critical_tokens": 180000,
    "critical_percentage": 0.90,
    "max_tokens": 200000
  },
  "timestamp": "2026-03-18T14:23:45Z"
}
```

### Check Command Output

```json
{
  "zone": "warning",
  "percentage": 72.0,
  "should_compact": true,
  "recommendation": {
    "recommended": true,
    "reason": "Phase start at 72.0% - will exceed danger zone mid-phase. Compact now.",
    "urgency": "medium",
    "estimated_reduction": "15-25%"
  },
  "thresholds": {
    "sweet_spot_tokens": 120000,
    "sweet_spot_percentage": 0.60,
    "warning_tokens": 140000,
    "warning_percentage": 0.70,
    "danger_tokens": 160000,
    "danger_percentage": 0.80,
    "critical_tokens": 180000,
    "critical_percentage": 0.90,
    "max_tokens": 200000
  },
  "model_id": "claude-sonnet-4.5",
  "source": "manual",
  "timestamp": "2026-03-18T14:23:45Z"
}
```

### Models Command Output

```json
{
  "models": [
    {
      "model_id": "claude-sonnet-4.5",
      "provider": "anthropic",
      "max_context_tokens": 200000,
      "sweet_spot_threshold": 0.60,
      "warning_threshold": 0.70,
      "danger_threshold": 0.80,
      "critical_threshold": 0.90,
      "confidence": "HIGH",
      "benchmark_sources": [
        "RULER: 85% accuracy up to 60% fill (~120K tokens)",
        "Lost in the Middle: Degradation begins at 80% fill (~160K tokens)",
        "Production Swarm Data: 60/70/80/90% thresholds validated"
      ],
      "notes": "Production validated from engram swarm data. High confidence.",
      "last_updated": "2026-03-18T00:00:00Z"
    }
  ],
  "count": 16
}
```

---

## Error Handling

### 1. Fallback Strategy

The library uses multiple fallback layers:

```
Primary Detection (CLI-specific)
    ↓ (fails)
Conversation Log Parsing
    ↓ (fails)
Message Count Heuristic
    ↓ (always succeeds with "estimated" flag)
Default Thresholds (60/70/80/90%)
```

**Example**:

```go
usage, err := detector.Detect()
if err != nil {
    // This rarely happens - detector has built-in fallbacks
    log.Printf("Detection failed: %v", err)
}

// Check confidence
if usage.Source == "heuristic" {
    log.Println("Warning: Using estimated context (30-40% confidence)")
}
```

### 2. Unknown Model Handling

Unknown models automatically use default thresholds:

```go
model := registry.GetModel("unknown-model-xyz")
// Returns default config (60/70/80/90% thresholds)

fmt.Printf("Model: %s\n", model.ModelID)        // "default"
fmt.Printf("Confidence: %s\n", model.Confidence) // "N/A"
fmt.Printf("Max tokens: %d\n", model.MaxContextTokens) // 200000
```

### 3. Validation Errors

The library validates input data:

```go
// Invalid percentage
zone, err := calculator.CalculateZone(-5.0, "claude-sonnet-4.5")
// err: "invalid percentage: must be 0.0-100.0"

// Invalid tokens
usage := &context.Usage{
    UsedTokens:  250000,  // > total
    TotalTokens: 200000,
}
_, err = calculator.Check(usage, context.PhaseMiddle)
// err: "invalid usage: used tokens exceed total"
```

### 4. File System Errors

Registry loading handles missing files gracefully:

```go
registry, err := context.NewRegistry("/nonexistent/models.yaml")
if err != nil {
    // Error with clear message
    log.Printf("Failed to load registry: %v", err)
    // Fallback: use embedded models.yaml or in-memory defaults
}
```

---

## Testing

### Unit Tests

**Location**: `./repos/worktrees/engram/context-mgmt/core/pkg/context/*_test.go`

**Coverage**: 60%+ (21+ tests)

**Run tests**:

```bash
cd ./repos/worktrees/engram/context-mgmt/core/pkg/context
go test -v ./...
```

**Example test cases**:

```go
// registry_test.go
func TestGetModel_ExactMatch(t *testing.T) {
    registry := createTestRegistry(t)
    model := registry.GetModel("claude-sonnet-4.5")
    assert.Equal(t, "claude-sonnet-4.5", model.ModelID)
}

func TestGetModel_NormalizedMatch(t *testing.T) {
    registry := createTestRegistry(t)
    model := registry.GetModel("CLAUDE_SONNET_4_5")
    assert.Equal(t, "claude-sonnet-4-5", model.ModelID)
}

func TestGetModel_UnknownFallback(t *testing.T) {
    registry := createTestRegistry(t)
    model := registry.GetModel("unknown-model")
    assert.Equal(t, "default", model.ModelID)
}

// usage_test.go
func TestCalculateZone_Safe(t *testing.T) {
    calculator := createTestCalculator(t)
    zone, _ := calculator.CalculateZone(50.0, "test-model")
    assert.Equal(t, context.ZoneSafe, zone)
}

func TestShouldCompact_PhaseStart(t *testing.T) {
    calculator := createTestCalculator(t)
    usage := &context.Usage{
        PercentageUsed: 70.0,
        ModelID:        "test-model",
    }
    shouldCompact, _ := calculator.ShouldCompact(usage, context.PhaseStart)
    assert.True(t, shouldCompact) // 70% triggers at phase start
}

func TestShouldCompact_PhaseMiddle(t *testing.T) {
    calculator := createTestCalculator(t)
    usage := &context.Usage{
        PercentageUsed: 75.0,
        ModelID:        "test-model",
    }
    shouldCompact, _ := calculator.ShouldCompact(usage, context.PhaseMiddle)
    assert.False(t, shouldCompact) // 75% < 80% danger threshold
}
```

### Integration Tests

**Run integration tests**:

```bash
cd ./repos/worktrees/engram/context-mgmt/core/cmd/engram
go test -tags=integration -v ./...
```

**Example scenarios**:

```go
func TestContextStatusCommand(t *testing.T) {
    // Set up environment
    os.Setenv("CLAUDE_SESSION_ID", "test-session")

    // Run command
    cmd := exec.Command("engram", "context", "status", "--json")
    output, err := cmd.Output()
    require.NoError(t, err)

    // Parse JSON
    var result map[string]interface{}
    json.Unmarshal(output, &result)

    // Validate structure
    assert.Contains(t, result, "percentage")
    assert.Contains(t, result, "zone")
    assert.Contains(t, result, "model_id")
}
```

### E2E Tests

**Test matrix**:

| CLI | Detection | Extraction | Compaction | Status |
|-----|-----------|------------|------------|--------|
| Claude Code | ✅ | ✅ | ✅ | Production |
| Gemini CLI | ✅ | 🚧 | ✅ | Framework |
| OpenCode | ✅ | 🚧 | ✅ | Framework |
| Codex | ✅ | 🚧 | ✅ | Framework |

---

## Advanced Usage

### Custom Model Registry

You can create a custom model registry:

```yaml
# custom-models.yaml
models:
  - model_id: "my-custom-llm"
    provider: "custom"
    max_context_tokens: 128000
    sweet_spot_threshold: 0.50
    warning_threshold: 0.65
    danger_threshold: 0.75
    critical_threshold: 0.85
    benchmark_sources:
      - "Internal benchmarks"
    notes: "Custom model for testing"
    confidence: "LOW"
    last_updated: "2026-03-18T00:00:00Z"
```

**Load custom registry**:

```go
registry, err := context.NewRegistry("/path/to/custom-models.yaml")
```

### Phase State Detection

Auto-detect phase state from Wayfinder status:

```bash
#!/bin/bash
# Detect phase state from WAYFINDER-STATUS.md

detect_phase_state() {
    local status_file="WAYFINDER-STATUS.md"

    if [ ! -f "$status_file" ]; then
        echo "middle"
        return
    fi

    # Check if deliverables exist
    local current_phase=$(grep "Current Phase:" "$status_file" | cut -d: -f2)
    local deliverable_count=$(grep "✅" "$status_file" | wc -l)

    if [ "$deliverable_count" -eq 0 ]; then
        echo "start"
    elif grep -q "Next Phase:" "$status_file"; then
        echo "end"
    else
        echo "middle"
    fi
}

PHASE_STATE=$(detect_phase_state)
engram context check --model claude-sonnet-4.5 --tokens 144000/200000 --phase-state "$PHASE_STATE"
```

### Programmatic Compaction Suggestions

Generate specific compaction recommendations:

```go
func suggestCompactionTargets(usage *context.Usage) []string {
    suggestions := []string{}

    if usage.PercentageUsed >= 80.0 {
        suggestions = append(suggestions,
            "Remove old conversation history (first 20-30 messages)",
            "Compact bead summaries (keep only recent 5)",
            "Remove verbose error logs",
            "Summarize long file contents",
        )
    } else if usage.PercentageUsed >= 70.0 {
        suggestions = append(suggestions,
            "Compact verbose tool outputs",
            "Summarize completed phase results",
        )
    }

    return suggestions
}
```

### Monitoring Dashboard Data

Export metrics for monitoring:

```go
type ContextMetrics struct {
    Timestamp     time.Time
    SessionID     string
    Percentage    float64
    Zone          string
    ModelID       string
    UsedTokens    int
    TotalTokens   int
    ShouldCompact bool
}

func collectMetrics(detector *context.Detector, calculator *context.Calculator) *ContextMetrics {
    usage, _ := detector.Detect()
    zone, _ := calculator.CalculateZone(usage.PercentageUsed, usage.ModelID)
    shouldCompact, _ := calculator.ShouldCompact(usage, context.PhaseMiddle)

    return &ContextMetrics{
        Timestamp:     time.Now(),
        SessionID:     usage.SessionID,
        Percentage:    usage.PercentageUsed,
        Zone:          string(zone),
        ModelID:       usage.ModelID,
        UsedTokens:    usage.UsedTokens,
        TotalTokens:   usage.TotalTokens,
        ShouldCompact: shouldCompact,
    }
}
```

---

## References

### Documentation

- **Specification**: `SPEC.md`
- **Architecture**: `ARCHITECTURE.md`
- **Research Report**: `phase-0/deliverables/RESEARCH-REPORT.md`
- **Hook Audit**: `phase-0/deliverables/HOOK-AUDIT.md`
- **CLI Detection Strategy**: `phase-0/deliverables/CLI-DETECTION-STRATEGY.md`

### Benchmarks

- **RULER** (Hsieh 2024): https://github.com/NVIDIA/RULER
- **Lost in the Middle** (Liu 2023): Long-context QA position effects
- **NeedleBench** (Li 2024): 1M context retrieval benchmarks
- **LMSys Arena**: https://lmsys.org
- **Artificial Analysis**: https://artificialanalysis.ai

### Source Code

- **Package**: `./repos/worktrees/engram/context-mgmt/core/pkg/context/`
- **CLI Commands**: `./repos/worktrees/engram/context-mgmt/core/cmd/engram/cmd/context*.go`
- **Models Registry**: `./repos/worktrees/engram/context-mgmt/core/pkg/context/models.yaml`

### Related Projects

- **AGM (AI Agent Manager)**: `agm/`
- **Wayfinder**: `cortex/`
- **Beads**: `./repos/beads/`

---

**Version**: 1.0
**Last Updated**: 2026-03-18
**Maintainer**: Engram Core Team
**License**: MIT (see repository LICENSE file)
