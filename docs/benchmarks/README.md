# Benchmarks — historical results and methodology

This directory holds the historical record of dear-agent benchmark runs.
Each `*.json` file is a `pkg/benchmarks.Results` snapshot written by
`agm benchmark run --results-dir docs/benchmarks/`.

## Why this directory exists

dear-agent's value proposition is that it beats raw model+harness on
coding benchmarks while keeping cost-per-task-solved low. Claims like
"state-of-the-art on SWE-Bench Verified" decay fast — models change,
prompts change, harness internals change — so the only honest version of
the claim is "here's the result file, here's the model, here's the date,
here's the dear-agent commit."

Every recursive self-improvement cycle ends by writing the post-cycle
results here so the trajectory is auditable. A reviewer can `git log
docs/benchmarks/` to see the full optimization history.

## Suites tracked

| Suite                | Tasks | Tier        | When to run                                        |
|----------------------|-------|-------------|----------------------------------------------------|
| `swe-bench-lite`     | ~300  | shift-left  | every cycle of the self-improvement loop           |
| `swe-bench-verified` | ~500  | validation  | gate before claiming a regression-free improvement |
| `swe-atlas`          | varies| diagnostic  | per-pillar (QnA / Test Writing / Refactoring)      |
| `vibe-bench`         | varies| end-to-end  | generative app-quality sanity check                |

The Lite tier is run first and most often because it's the cheapest place
to detect regressions; Verified is the headline number for external
claims; Atlas helps target which dear-agent phase needs work; Vibe Bench
catches regressions that purely-functional benchmarks miss.

## Running a benchmark

```sh
agm benchmark run \
  --suite swe-bench-lite \
  --mode dear-agent \
  --model claude-opus-4-7 \
  --results-dir docs/benchmarks/
```

`--mode dear-agent` runs each task with the full DEAR protocol enforced.
`--mode raw` runs the same model behind a thin harness without DEAR
enforcement — that's the baseline dear-agent must beat.

## Comparing two runs

```sh
agm benchmark compare \
  --baseline docs/benchmarks/<raw-run>.json \
  --test     docs/benchmarks/<dear-agent-run>.json
```

A positive `solve_rate_delta` means dear-agent improved over baseline. A
negative `cost_per_solved_delta` means dear-agent is cheaper per solved
task — that's the cost-per-intelligence axis we optimize alongside
solve rate.

## Self-improvement loop

```sh
agm selfimprove \
  --suite swe-bench-lite \
  --budget 50 \
  --max-cycles 3
```

Cycle: run → analyze → propose → apply → re-run → compare → accept-or-revert.
The regression gate is on by default: a cycle that drops solve rate is
reverted and the loop stops.

## Result file shape

Each `*.json` in this directory is a `pkg/benchmarks.Results`:

```json
{
  "suite": "swe-bench-lite",
  "mode": "dear-agent",
  "model": "claude-opus-4-7",
  "run_id": "swe-bench-lite-dear-agent-1778440000",
  "started_at": "2026-05-10T17:00:00Z",
  "finished_at": "2026-05-10T17:42:00Z",
  "tasks": [{"task_id": "...", "solved": true, "cost_usd": 0.42, ...}],
  "summary": {
    "total": 300, "solved": 213, "solve_rate": 0.71,
    "total_cost_usd": 124.50, "cost_per_solved_usd": 0.585, ...
  }
}
```

`cost_per_solved_usd` is `null` when no tasks solved — the on-the-wire
encoding of the in-memory +Inf sentinel.
