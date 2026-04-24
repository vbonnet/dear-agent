# Claude Code Cost Analysis

Parses all Claude Code conversation logs and computes per-API-call costs using model pricing tables, then generates comprehensive analytics.

## What It Does

Scans all JSONL conversation log files in `~/.claude/projects/` and produces:

- **Daily cost breakdown** with bar chart visualization
- **Weekly and monthly summaries** with session counts and averages
- **Model family split** (Opus / Sonnet / Haiku) with token counts
- **Top 10 most expensive sessions** with duration and project info
- **Token usage breakdown** with cache efficiency metrics
- **Cost trend analysis** (7-day and 30-day comparisons)
- **Hourly distribution** (UTC) showing peak usage times
- **Cache savings estimate** showing how much prompt caching saved

## How to Run

```bash
python3 tools/cost-analysis/main.py
```

No external dependencies -- uses only the Python 3.6+ standard library (`json`, `glob`, `collections`, `datetime`).

Output includes ANSI color codes for terminal display. To capture a clean version:

```bash
python3 tools/cost-analysis/main.py 2>&1 | sed 's/\x1b\[[0-9;]*m//g' > report.txt
```

## Data Sources

The script reads JSONL files from:

```
~/.claude/projects/**/*.jsonl
```

Each JSONL file represents one Claude Code session. The script extracts `usage` blocks from entries with `type: "assistant"` or `type: "progress"`, which contain token counts (`input_tokens`, `output_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens`) and the model identifier.

Symlinked files (e.g., subagent sessions) are resolved and deduplicated to avoid double-counting.

## Pricing Table

Costs are computed using the following per-million-token rates:

| Model Family | Input | Output | Cache Read | Cache Write |
|---|---:|---:|---:|---:|
| Opus | $15.00 | $75.00 | $1.50 | $18.75 |
| Sonnet | $3.00 | $15.00 | $0.30 | $3.75 |
| Haiku | $0.80 | $4.00 | $0.08 | $1.00 |

Model classification is based on substring matching in the model identifier string (e.g., `claude-opus-4-20250514` matches "opus").

## Example Output

See [`docs/cost-report-2026-03-24.md`](../../docs/cost-report-2026-03-24.md) for a sample report formatted as markdown.
