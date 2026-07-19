// Package backlog turns the repo's declared backlog (the markdown ticket
// tables in docs/workflow-engine/BACKLOG.md and ROADMAP.md) into ranked,
// machine-readable work items and suggests the next item to pick up.
//
// It is the task-driven counterpart to the metric-driven recommendation
// surface (ADR-015 aggregator and recommendation MCP): that surface
// ranks project-health signals, this one ranks declared tickets. The two
// are intentionally independent — see ADR-022.
//
// The pipeline is:
//
//  1. A Source yields []Item. MarkdownSource parses GitHub-flavored table
//     rows, resolving column meaning by header name so it reads both the
//     7-column BACKLOG.md layout and the 4-column ROADMAP.md layout.
//  2. A Ranker computes eligibility (status==Pending and every dependency
//     Done) and a blended Score from priority, dependency leverage, and
//     effort — reproducing the VROOM Orchestrator dispatch rules (agm
//     ADR-022) as code.
//  3. A Suggester applies the current Context (phase, capacity, effort cap)
//     and returns the next items plus an explanation of what is blocked.
//  4. An OrchestratorNotifier records the pickup as a VROOM
//     decision.dispatched event so the choice is on the audit trail.
//
// The package has no SQLite, no network, and no dependency on
// pkg/aggregator; its only non-stdlib dependency is pkg/vroom for the
// Orchestrator-integration seam.
package backlog
