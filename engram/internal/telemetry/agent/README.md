# Agent Telemetry

Sub-agent telemetry logging for Claude Code to enable learning from execution patterns.

## Overview

This package provides telemetry logging for sub-agent launches, tracking:
- Prompt features (specificity, examples, constraints, context)
- Execution outcomes (success, failure, partial)
- Resource consumption (tokens used, duration)
- Prompt characteristics for analysis

## Installation

```bash
cd engram/
go build ./internal/telemetry/agent/...
go build ./cmd/engram-telemetry/...
```

## Usage

### Logging Agent Launches

```go
import (
    "context"
    "github.com/vbonnet/engram/core/internal/telemetry/agent"
    "github.com/vbonnet/engram/core/pkg/eventbus"
)

// Initialize telemetry
storage, err := agent.NewStorage()
if err != nil {
    log.Fatal(err)
}
defer storage.Close()

bus := eventbus.NewBus()
telemetry := agent.NewTelemetry(bus, storage)
defer telemetry.Close()

// Log an agent launch
ctx := context.Background()
prompt := "Create a function calculateTotal() that handles up to 100 items"
model := "claude-sonnet-4.5"

launchID, err := telemetry.LogAgentLaunch(ctx, prompt, model)
if err != nil {
    log.Printf("Failed to log launch: %v", err)
}

// ... agent executes ...

// Log completion
err = telemetry.LogAgentCompletion(ctx, launchID, "success", 1500)
if err != nil {
    log.Printf("Failed to log completion: %v", err)
}
```

### Querying Telemetry Data

```bash
# Query successful launches
engram-telemetry query --outcome=success --limit=50

# Query by model
engram-telemetry query --model=claude-sonnet-4.5 --limit=100

# Query since date
engram-telemetry query --since=2026-01-01 --outcome=failure

# Export to CSV
engram-telemetry query --output=csv > launches.csv
```

### View Statistics

```bash
# Overall statistics
engram-telemetry stats

# Statistics for specific model
engram-telemetry stats --model=claude-sonnet-4.5
```

## Feature Extraction

The system automatically extracts 6 features from each prompt:

1. **Word Count**: Number of words in prompt
2. **Token Count**: Approximate token count (V1: same as word count)
3. **Specificity Score** (0.0-1.0): Ratio of concrete terms (file paths, numbers, camelCase) to total words
4. **Has Examples** (boolean): Presence of code blocks (```) or structured data ({, [)
5. **Has Constraints** (boolean): Presence of numbers or limit keywords (max, limit, must, etc.)
6. **Context Embedding Score** (0.0-1.0): Self-containedness (1.0 = fully embedded, 0.0 = many references to external context)

### Example

```go
prompt := "Read features.go and extract up to 100 functions"
features := agent.ExtractFeatures(prompt)

// features.WordCount = 8
// features.SpecificityScore ≈ 0.25 (features.go + 100)
// features.HasExamples = false
// features.HasConstraints = true (100, "up to")
// features.ContextEmbeddingScore ≈ 1.0 (self-contained)
```

## Database Schema

SQLite database: `~/.engram/telemetry.db`

```sql
CREATE TABLE agent_launches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL,
    prompt_text TEXT NOT NULL,
    model TEXT NOT NULL,
    task_description TEXT,
    session_id TEXT,
    parent_agent_id TEXT,

    -- Prompt features
    word_count INTEGER NOT NULL,
    token_count INTEGER NOT NULL,
    specificity_score REAL NOT NULL,
    has_examples INTEGER NOT NULL,
    has_constraints INTEGER NOT NULL,
    context_embedding_score REAL NOT NULL,

    -- Outcome data
    outcome TEXT CHECK(outcome IN ('success', 'failure', 'partial', NULL)),
    retry_count INTEGER DEFAULT 0,
    tokens_used INTEGER,
    error_message TEXT,
    duration_ms INTEGER,

    created_at TEXT DEFAULT (datetime('now'))
);
```

## Testing

```bash
cd engram/
go test ./internal/telemetry/agent/... -v -cover
```

Expected coverage: >80%

## Architecture

```
Agent Launch
     ↓
telemetry.LogAgentLaunch(prompt, model)
     ↓
Feature Extraction
  - ExtractFeatures(prompt) → Features
     ↓
Storage Layer (SQLite)
  - storage.LogLaunch(prompt, model, features) → ID
     ↓
EventBus (optional)
  - bus.Publish(agent_launch event)
     ↓
Query via CLI
  - engram-telemetry query/stats
```

## Design Decisions

### Simple Feature Extraction (V1)

V1 uses simple regex and counting algorithms for feature extraction. This provides:
- Fast implementation (no NLP dependencies)
- Easy to understand and debug
- Sufficient accuracy for directional signals (80% accuracy)
- Clear upgrade path to NLP in V2

### EventBus Integration

Events are published to EventBus for consistency with engram architecture:
- Decouples telemetry from storage
- Enables future exporters (Datadog, Prometheus) without code changes
- Non-blocking (async publish)

### SQLite Storage

Local SQLite database for privacy and simplicity:
- No external dependencies
- Full data control (no third-party services)
- Sufficient performance (1000s writes/sec, agents launch at ~10/min)
- WAL mode enabled for concurrent writes

## Future Enhancements (V2+)

- NLP-based feature extraction (tiktoken, POS tagging)
- Machine learning prompt scoring
- Real-time dashboard (engram-mvg bead)
- Export to external systems (Datadog, Prometheus)
- Automatic prompt improvement suggestions

## License

Part of the engram project.
