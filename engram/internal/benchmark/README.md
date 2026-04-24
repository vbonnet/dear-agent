# Benchmark Storage Package

Provides SQLite-based persistent storage for Engram/Wayfinder benchmark results with query and export capabilities.

## Overview

This package implements the structured output format and data storage system for benchmark results as specified in bead engram-240.2.3.

**Key Features:**
- SQLite storage at `~/.engram/benchmarks.db`
- Type-safe Go API for insert and query operations
- JSON and CSV export
- Markdown table import for historical data
- Extensible metadata via JSON column

## Installation

```bash
cd engram/
go build -o ~/bin/engram-benchmark ./cmd/engram-benchmark/
```

## Go API Usage

### Basic Insert and Query

```go
package main

import (
    "github.com/google/uuid"
    "github.com/vbonnet/engram/core/internal/benchmark"
    "time"
)

func main() {
    // Open storage
    storage, err := benchmark.NewStorage()
    if err != nil {
        panic(err)
    }
    defer storage.Close()

    // Create a benchmark run
    qualityScore := 9.5
    costUSD := 4.80
    fileCount := 13

    run := benchmark.BenchmarkRun{
        RunID:        uuid.New().String(),
        Timestamp:    time.Now(),
        Variant:      "wayfinder",
        ProjectSize:  "small",
        ProjectName:  "test-project",
        QualityScore: &qualityScore,
        CostUSD:      &costUSD,
        FileCount:    &fileCount,
        Successful:   true,
        Metadata: map[string]interface{}{
            "efficiency_metrics": map[string]interface{}{
                "total_queries": 15,
                "context_window_peak": 12000,
            },
        },
    }

    // Insert
    if err := storage.InsertRun(run); err != nil {
        panic(err)
    }

    // Query
    results, err := storage.Query(benchmark.QueryParams{
        Variants: []string{"wayfinder"},
        QualityMin: &qualityScore,
    })
    if err != nil {
        panic(err)
    }

    fmt.Printf("Found %d results\n", len(results))
}
```

### Query with Filters

```go
// Query wayfinder runs with quality >= 9.0 and cost <= $5.00
minQuality := 9.0
maxCost := 5.0

results, err := storage.Query(benchmark.QueryParams{
    Variants:    []string{"wayfinder"},
    ProjectSizes: []string{"small", "medium"},
    QualityMin:  &minQuality,
    CostMax:     &maxCost,
    Limit:       50,
    OrderBy:     "quality_score",
    OrderDirection: "DESC",
})
```

### Export to JSON/CSV

```go
// Export to JSON
file, _ := os.Create("results.json")
defer file.Close()
storage.ExportJSON(benchmark.QueryParams{Variants: []string{"wayfinder"}}, file)

// Export to CSV
csvFile, _ := os.Create("results.csv")
defer csvFile.Close()
storage.ExportCSV(benchmark.QueryParams{}, csvFile)
```

## CLI Usage

### Query Benchmark Results

```bash
# Query all wayfinder runs
engram-benchmark query --variant=wayfinder

# Query with multiple filters
engram-benchmark query \
  --variant=wayfinder \
  --project-size=small,medium \
  --quality-min=9.0 \
  --cost-max=5.00 \
  --format=table

# Export to JSON
engram-benchmark query --variant=wayfinder --format=json > results.json

# Query with time range
engram-benchmark query \
  --after=2026-01-01 \
  --before=2026-01-31 \
  --format=csv
```

### Insert Benchmark Run

```bash
engram-benchmark insert \
  --variant=wayfinder \
  --project-size=small \
  --project-name=test-project \
  --quality-score=9.5 \
  --cost-usd=4.80 \
  --duration-ms=14400000 \
  --file-count=13 \
  --docs-kb=176 \
  --tokens-input=120000 \
  --tokens-output=35000 \
  --phases=11
```

### Export Results

```bash
# Export all results to JSON
engram-benchmark export --format=json --output=all-results.json

# Export filtered results to CSV
engram-benchmark export \
  --variant=wayfinder \
  --project-size=small \
  --format=csv \
  --output=wayfinder-small.csv
```

### Import Historical Data

```bash
# Import three-way comparison data
engram-benchmark import \
  --from the git history-comparisons/three-way-2025-12-11/

# Dry run (validate without importing)
engram-benchmark import \
  --from the git history-comparisons/three-way-2025-12-11/ \
  --dry-run
```

## Schema

### Core Fields

| Field | Type | Description |
|-------|------|-------------|
| run_id | TEXT | Unique identifier (UUID) |
| timestamp | TEXT | Run timestamp (ISO8601) |
| variant | TEXT | raw, engram, or wayfinder |
| project_size | TEXT | small, medium, large, or xl |
| project_name | TEXT | Project identifier (optional) |
| quality_score | REAL | 0.0-10.0 scale (optional) |
| cost_usd | REAL | Total API cost in USD (optional) |
| tokens_input | INTEGER | Input tokens (optional) |
| tokens_output | INTEGER | Output tokens (optional) |
| duration_ms | INTEGER | Wall clock time (optional) |
| file_count | INTEGER | Deliverable files created (optional) |
| documentation_kb | REAL | Documentation size in KB (optional) |
| phases_completed | INTEGER | Wayfinder phases (0-11) (optional) |
| successful | INTEGER | 1=success, 0=failure |
| metadata | TEXT | JSON for extensible metrics |

### Metadata JSON

```json
{
  "efficiency_metrics": {
    "total_queries": 15,
    "context_window_peak": 12000
  },
  "error_metrics": {
    "permission_prompts": 2,
    "tool_rejections": 0,
    "false_starts": 1
  },
  "knowledge_metrics": {
    "engrams_loaded_at_start": 5,
    "engrams_retrieved_during_execution": 3
  },
  "quality_dimensions": {
    "correctness": 10,
    "completeness": 10,
    "clarity": 10,
    "actionability": 10,
    "efficiency": 10,
    "validation": 10,
    "maintainability": 10
  }
}
```

## Testing

```bash
cd engram/
go test ./internal/benchmark/ -v
```

## Dependencies

- `github.com/mattn/go-sqlite3` - SQLite driver (existing)
- `github.com/google/uuid` - UUID generation (new)

## References

- **Bead**: engram-240.2.3 (Structured Output Format and Data Storage)
- **Metrics Inventory**: the git history/metrics-inventory.md
- **Design**: the git history/S6-design.md
