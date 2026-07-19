// Package collectors implements the Phase 1 first-party Collectors for
// the signal aggregator described by ADR-015.
//
// Each collector is independent and testable: it depends only on a small
// Exec indirection, so unit tests can fake out the external command
// (golangci-lint, govulncheck, …) without invoking it. The production
// constructor wires exec.CommandContext.
package collectors
