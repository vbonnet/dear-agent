# ADR-001: Keep evaluation policy behind judge and feedback boundaries

Status: Accepted

## Context

Evaluation needs both a simple numeric score for existing callers and detailed
pass, score, and reasoning output for policy gates. Providers and alert
destinations vary, while threshold policy belongs to the evaluator rather than
to an LLM transport.

## Decision

The package exposes `Judge` for the legacy score-only boundary and
`DetailedJudge` for criteria-based comparisons. OpenAI and Anthropic
implementations satisfy both boundaries. Evaluators apply explicit normalized
thresholds to judge responses. Feedback and alert delivery depend on narrow
interfaces so provider and channel selection remains outside evaluation logic.

Expected and actual output are distinct required inputs to detailed evaluation;
comparing one value with itself is invalid.

## Consequences

- Existing score callers can migrate without a flag day.
- Provider, threshold, and notification policy can be tested independently.
- Adding a provider or alert channel requires an interface implementation, not
  changes to evaluation semantics.

## Evidence

- `../judge.go`
- `../offline_evaluator.go`, `../online_evaluator.go`
- package tests beside those files
