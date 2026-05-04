// Package collectors implements the Phase 1 first-party Collectors for
// the signal aggregator (ADR-015 §D7).
//
// Each collector is independent and testable: it depends only on a small
// Exec indirection, so unit tests can fake out the external command
// (golangci-lint, govulncheck, …) without invoking it. The production
// constructor wires exec.CommandContext.
package collectors
