// Package aggregator implements the project-health signal store that feeds
// dear-agent's recommendation engine (ADR-015).
//
// A Signal is one observation about a project at a point in time. Collectors
// produce signals; an Aggregator persists them via a Store; a Scorer ranks
// the most recent observations into a per-kind weighted priority that the
// recommendation engine consumes.
//
// The package name is "aggregator" rather than "signals" because pkg/signals
// is already taken by the Hybrid Progressive Rigor detector — see
// ADR-015 §D1.
//
// Subject conventions for first-party collectors:
//
//   - dear-agent.git:      repo root path (e.g. "/repo")
//   - dear-agent.lint:     file path relative to repo root
//   - dear-agent.coverage: Go import path (e.g. "github.com/x/y/pkg/foo")
//   - dear-agent.deps:     module path (e.g. "github.com/foo/bar")
//   - dear-agent.security: vuln ID (e.g. "GO-2024-1234")
package aggregator
