# SWE-bench-lite Baseline

Baseline measurement for `agm benchmark swe-lite` using Claude Code as the agent.

## Dataset

- **Source**: [princeton-nlp/SWE-bench_Lite](https://huggingface.co/datasets/princeton-nlp/SWE-bench_Lite)
- **Size**: 300 test instances across 12 Python repositories
- **Repos**: django (114), sympy (77), scikit-learn (23), matplotlib (23),
  pytest (17), sphinx (16), astropy (6), pylint (6), requests (6),
  xarray (5), seaborn (4), flask (3)

## Setup

### Prerequisites

- Go 1.22+
- Claude Code CLI (`claude` in PATH)
- Python 3.10+ (for dataset export only)

### Export dataset from HuggingFace

```bash
python3 -m venv /tmp/swe-venv
/tmp/swe-venv/bin/pip install datasets
/tmp/swe-venv/bin/python3 agm/scripts/export_swe_bench.py -o swe-bench-lite.json
```

### Build and run

```bash
go build -o agm-bench ./agm/cmd/agm/

# Dry run — list tasks without executing
./agm-bench benchmark swe-lite --dry-run

# Run built-in 3-task sample
./agm-bench benchmark swe-lite --results-dir ./results

# Run from exported dataset, limited to 5 tasks
./agm-bench benchmark swe-lite \
  --dataset swe-bench-lite.json \
  --limit 5 \
  --results-dir ./results

# Full dataset (300 tasks, ~hours, ~$$$)
./agm-bench benchmark swe-lite \
  --dataset swe-bench-lite.json \
  --results-dir ./results
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--agent` | `claude` | Agent binary to invoke per task |
| `--dataset` | (built-in) | Path to JSON dataset file |
| `--limit` | 0 (all) | Max tasks to run |
| `--results-dir` | (none) | Directory for JSON result files |
| `--dry-run` | false | List tasks without executing |
| `--clone` | false | Clone actual repos at base commit (slow) |

## How It Works

1. **Task loading**: Loads tasks from built-in samples or `--dataset` JSON file
2. **Workdir setup**: Creates a temp directory per task (scaffold mode by default;
   `--clone` does a real `git clone` at the issue's base commit)
3. **Agent invocation**: Runs `claude --print --dangerously-skip-permissions
   --output-format json -p <prompt>` in the workdir
4. **Evaluation**: Heuristic check — looks for unified diff markers (`---`/`+++`,
   `diff --git`, `@@`) plus a reference to the target repo or fix keywords
5. **Cost estimation**: Parses Claude's JSON output for `cost_usd` or token usage;
   falls back to word-count heuristic
6. **Reporting**: Text table to stdout (or `--output json`), plus per-run JSON
   files in `--results-dir`

## Evaluation Caveats

The current `evaluatePatch` is a **heuristic**, not a ground-truth verifier.
It checks for structural markers of a plausible diff — it does NOT:

- Apply the patch to the codebase
- Run the repo's test suite
- Compare against the gold patch from SWE-bench

This means the reported resolve rate is an **upper bound** on actual correctness.
A task marked PASS means the agent produced diff-like output referencing the
target repo — it may still be wrong.

## Scaffold vs Clone Mode

| Mode | `--clone` | Speed | Accuracy |
|------|-----------|-------|----------|
| Scaffold | off (default) | Fast (~seconds/task for workdir) | Low — agent has no real codebase |
| Clone | on | Slow (~minutes/task for git clone) | Higher — agent sees actual code |

**Scaffold mode** gives the agent only a README with the problem statement.
The agent cannot browse the actual codebase, so it generates patches based
solely on its training knowledge. This is useful for measuring the agent's
"knowledge-only" baseline.

**Clone mode** checks out the real repo at the base commit. The agent can
read files, understand the codebase, and produce targeted patches. This is
closer to the real SWE-bench evaluation but much slower due to git clones.

## Initial Baseline Results

**Date**: 2026-04-10
**Agent**: Claude Code 2.1.100 (default model)
**Mode**: Scaffold (no repo clone)

### Single-task pilot (astropy)

```
Run ID: swe-lite-1775832547
Instance: astropy__astropy-12907 — separability_matrix for nested CompoundModels

  astropy__astropy-12907                    PASS  6m58s  $0.0026  0 lines

Summary: 1/1 resolved (100%)
  Cost:     $0.0026
  Avg time: 6m58s
```

**Observations**:
- The agent produced diff-like output (heuristic PASS) in scaffold mode
- Cost was ~$0.003 per task using the fallback word-count estimator
- Duration of ~7 minutes per task implies ~35 hours for the full 300-task dataset
- `patch_length: 0` indicates the `countPatchLines` heuristic didn't find
  lines starting with `+`/`-` — the agent likely embedded the diff in prose
  rather than outputting a raw unified diff

**Note**: These results use heuristic evaluation only. The PASS verdict means
the agent output contained diff-like markers, not that the patch is correct.

## Known Limitations

1. **No test-based verification**: Patches are not applied or tested against
   the repo's test suite. The gold `test_patch` and `FAIL_TO_PASS` fields
   from SWE-bench are not yet used.
2. **Scaffold mode is unrealistic**: Without the actual codebase, agents
   produce patches from memory rather than code understanding.
3. **No parallelism**: Tasks run sequentially. At ~7 min/task in scaffold mode,
   a full 300-task run would take ~35 hours.
4. **Cost estimation is approximate**: Depends on Claude's `--output-format json`
   including usage data; falls back to word-count heuristic otherwise.
5. **No SWE-bench Docker harness integration**: The official SWE-bench
   evaluation uses Docker containers with pre-built environments. This
   harness does not replicate that infrastructure.

## Next Steps

1. **Test-based evaluation**: Apply generated patches and run `FAIL_TO_PASS`
   tests from the dataset to get ground-truth resolve rates.
2. **Docker integration**: Use SWE-bench's Docker evaluation harness for
   reproducible, isolated test execution.
3. **Parallel execution**: Run multiple tasks concurrently with a worker pool.
4. **Agent comparison**: Add `--agent` presets for different models/configs
   (e.g., `claude --model sonnet` vs `claude --model opus`).
5. **Cost tracking**: Integrate with AGM's cost tracking to get precise
   per-task API costs from session metadata.
6. **Regression tracking**: Store results in Dolt for cross-run comparisons
   and trend analysis.
