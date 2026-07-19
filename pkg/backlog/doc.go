// Package backlog turns operator-supplied markdown ticket tables into ranked,
// machine-readable work items and suggests the next item to pick up.
//
// It is the task-driven counterpart to the metric-driven recommendation
// surface (ADR-015 aggregator / ADR-016 recommendation MCP): that surface
// ranks project-health signals, this one ranks declared tickets. The two
// are intentionally independent — see ADR-022.
//
// The pipeline is:
//
//  1. A Source yields []Item. MarkdownSource parses GitHub-flavored table
//     rows, resolving column meaning by header name so it reads both the
//     common 7-column and 4-column layouts.
//  2. A Ranker computes eligibility (status==Pending and every dependency
//     Done) and a blended Score from priority, dependency leverage, and
//     effort — reproducing the VROOM Orchestrator dispatch rules (agm
//     ADR-023 § Task Dispatch) as code.
//  3. A Suggester applies the current Context (phase, capacity, effort cap)
//     and returns the next items plus an explanation of what is blocked.
//  4. An OrchestratorNotifier records the pickup as a VROOM
//     decision.dispatched event so the choice is on the audit trail.
//
// The package has no SQLite, no network, and no dependency on
// pkg/aggregator; its only non-stdlib dependency is pkg/vroom for the
// Orchestrator-integration seam.
package backlog
