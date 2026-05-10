// Package benchmarks provides a unified framework for running coding-task
// benchmarks (SWE-Bench Lite, SWE-Bench Verified, SWE Atlas, Vibe Bench)
// against dear-agent and against raw model+harness baselines.
//
// The package defines a single Benchmark interface — Run, Analyze, Compare —
// with one implementation per suite. Each implementation accepts a TaskExecutor
// so the same suite can be run in either dear-agent mode (full DEAR protocol)
// or raw mode (model+harness only) for A/B comparison.
//
// Cost metrics (input tokens, output tokens, USD, wall time, per-task) are
// first-class. Cost-per-task-solved is reported alongside solve rate so that
// "cost per intelligence" tradeoffs are explicit.
//
// The package is the foundation for pkg/selfimprove, which loops on top of it:
// run → analyze → propose → apply → re-run → compare → commit-or-revert.
package benchmarks
