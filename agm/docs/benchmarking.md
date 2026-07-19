# Benchmarking Operator Guide

This is the maintained operator guide for AGM benchmark commands. Generated
result files are runtime artifacts and belong under `.dear-agent/benchmarks/`
or an external research workspace; they are not checked into `docs/`.

## Run a suite

```sh
agm benchmark run \
  --suite swe-bench-lite \
  --mode dear-agent \
  --model claude-opus-4-7 \
  --results-dir .dear-agent/benchmarks/
```

Use `--mode raw` to run the same model behind the comparison harness. The
available suite and task-file flags are described by `agm benchmark run --help`.

## Compare results

```sh
agm benchmark compare \
  --baseline .dear-agent/benchmarks/<raw-run>.json \
  --test .dear-agent/benchmarks/<dear-agent-run>.json
```

The comparison reports solve-rate and cost deltas. A positive solve-rate delta
means the test run solved more tasks; a negative cost-per-solved delta means it
used less cost per solved task.

## Self-improvement

```sh
agm selfimprove --suite swe-bench-lite --budget 50 --max-cycles 3
```

The loop runs, analyzes, proposes, applies, re-runs, compares, and accepts or
reverts a cycle. A regression in solve rate stops the loop by default.

Historical benchmark snapshots and dated methodology belong in
[engram-research](https://github.com/vbonnet/engram-research).
