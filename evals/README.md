# evals — regression eval dataset

This directory is the **eval dataset** that grows from real agent behaviour. It
is the "close the loop" output of the telemetry/eval flywheel described in the
deep-research telemetry report: production traces that fail online scoring or end
in an error/stall are converted into eval cases here, so future regressions can
be caught automatically and CI can block merges that drop quality.

## Layout

```
evals/
  README.md          ← this file
  cases/             ← one <trace-id>.json per eval case (generated)
```

Each file in `cases/` is a single [`EvalCase`](../pkg/evalcase/evalcase.go)
(schema version 1). It is self-contained: the task, the expected vs. actual
behaviour, the failure classification, and span excerpts from the four trace
pillars (tool-call, reasoning, state-transition, memory).

## How cases get here

Cases are produced by the `pkg/evalcase` pipeline from completed traces (the kind
the DEAR Audit phase reads). Generate them from a trace dump with the
`eval-extract` CLI:

```sh
# Dry run — classify and report, write nothing:
go run ./cmd/eval-extract -in path/to/traces.jsonl -dry-run

# Extract problematic traces into this dataset:
go run ./cmd/eval-extract -in path/to/traces.jsonl -out evals
```

Live OTel four-pillar spans (from `pkg/agenttrace`) can be turned into the
pipeline's `Trace` input in-process with `evalcase.FromReadOnlySpans`.

## Properties

- **Idempotent.** A case is keyed by its source trace ID. Re-running the pipeline
  over the same trace never overwrites an existing case, so a human-curated edit
  to a generated case survives re-runs.
- **Version-controlled.** Cases are committed JSON. The dataset *is* the
  regression suite; review changes to it like any other code.
- **Discoverable.** Everything lives under `evals/`; no external store required.

A classification taxonomy (`tool_error`, `reasoning_error`, `state_loss`,
`memory_error`, `stall`, `low_eval_score`, `error_outcome`) is defined in
[`pkg/evalcase/classify.go`](../pkg/evalcase/classify.go).
