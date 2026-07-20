// Package backlog turns explicitly supplied Markdown ticket tables into
// ranked, machine-readable work items.
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
//     supported seven-column and four-column layouts.
//  2. A Ranker computes eligibility (status==Pending and every dependency
//     Done) and a blended Score from priority, dependency leverage, and
//     effort.
//  3. A Suggester applies the current Context (phase, capacity, effort cap)
//     and returns the next items plus an explanation of what is blocked.
//
// The package has no SQLite or network dependency. Beads owns Dear Agent's
// live work and VROOM dispatch; this package performs no task-state write.
package backlog
