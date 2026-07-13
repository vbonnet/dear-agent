// Package selfimprove implements the recursive self-improvement loop on top
// of pkg/benchmarks. The loop is:
//
//  1. Run a benchmark to get a baseline.
//  2. Analyze failures into FailurePatterns.
//  3. Propose improvement Hypotheses across configured model families.
//  4. Apply each Hypothesis as a Patch on the working tree.
//  5. Re-run the benchmark.
//  6. Compare. If the regression gate is on and SolveRate dropped, revert.
//  7. Otherwise, accept the cycle and emit a CycleResult.
//
// The package owns orchestration only — it delegates execution to a
// Benchmark, hypothesis generation to a Proposer, and patch application to an
// Applier. Each is an interface so that tests can drive the loop without
// touching real models, real git, or real benchmarks. VROOM events are
// emitted at each phase via a thin Notifier so a supervisor can observe and
// kill runaway loops.
//
// Cost is gated by a configurable BudgetUSD per run, drawing on
// pkg/costtrack semantics: when the cycle's accumulated cost exceeds the
// cap, the loop stops and emits a budget-exhausted reason.
package selfimprove
