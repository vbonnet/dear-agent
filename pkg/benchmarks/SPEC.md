# Coding Benchmark Suite Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**BENCHMARKS-01** When a suite is registered, the system shall reject duplicate suite identity and provide deterministic lookup and listing.

**BENCHMARKS-02** When the default registry initializes, the system shall expose SWE-bench Lite, SWE-bench Verified, SWE-Atlas, and VibeBench suites.

**BENCHMARKS-03** When benchmark tasks are loaded from JSON arrays or NDJSON, the system shall validate shape, suite filtering, limits, and line-addressable malformed input.

**BENCHMARKS-04** When a run executes, the system shall preserve the supplied model identifier and execution mode without restricting the model family.

**BENCHMARKS-05** When cost reaches the configured budget, the system shall stop scheduling additional tasks.

**BENCHMARKS-06** When no real executor is configured, the system shall mark tasks errored instead of reporting false solved results.

**BENCHMARKS-07** When results are summarized or compared, the system shall compute solve, cost, token, and regression deltas and reject mismatched suites.

**BENCHMARKS-08** When results are persisted, the system shall round-trip their schema and encode non-finite cost-per-solved safely.

**BENCHMARKS-09** While any supported harness runs Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, or Qwen models, the system shall preserve identical suite and result semantics.

## BDD Traceability

- Feature: `agm/test/bdd/features/evaluation_control_parity.feature`

## Test Traceability

- Unit package: `pkg/benchmarks`
