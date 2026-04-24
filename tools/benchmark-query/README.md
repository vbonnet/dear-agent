# benchmark-query

CLI tool for querying benchmark metrics from the ai-tools test and session
infrastructure.

## Installation

```bash
go install github.com/vbonnet/dear-agent/tools/benchmark-query@latest
```

## Usage

```bash
# List available metrics
benchmark-query -list

# Query a specific metric
benchmark-query -metric test_pass_rate_delta

# Query with time window
benchmark-query -metric session_success_rate -since 24h

# JSON output
benchmark-query -metric hook_bypass_rate -json
```

## Available Metrics

| Metric | Description |
|--------|-------------|
| `test_pass_rate_delta` | Change in test pass rate |
| `false_completion_rate` | Rate of false task completions |
| `hook_bypass_rate` | Rate of hook bypass attempts |
| `session_success_rate` | Overall session success rate |

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-metric` | | Metric name to query |
| `-since` | | Time window (e.g., `24h`, `7d`, `30d`) |
| `-dir` | | Custom metrics directory |
| `-list` | `false` | List available metrics |
| `-json` | `false` | Output in JSON format |
