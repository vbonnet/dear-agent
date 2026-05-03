// Package audit implements the DEAR Audit subsystem — scheduled,
// repo-scoped health checks that produce de-duplicated findings,
// drive remediation, and propose amendments back to the Define and
// Enforce layers. See ADR-011 for the architectural decisions this
// package implements.
//
// The substrate is the same SQLite database as the workflow engine
// (pkg/workflow's runs.db). Three additive tables — audit_findings,
// audit_runs, audit_proposals — are created by ApplySchema; they
// JOIN against the workflow engine's existing rows when needed but
// can be queried in isolation.
//
// The mental model is a fleet of named Checks ("build", "test",
// "lint.go", "vuln.govulncheck", ...) registered in a Registry and
// invoked by a Runner against a per-call Env. A Check finds; a
// Remediator fixes; a Refiner proposes amendments. The three stages
// are intentionally separate so checks stay pure and trivial to test.
//
// Higher-level surfaces — the workflow-audit CLI, the .dear-agent.yml
// > audits: config loader, the KindAuditCheck node kind in
// pkg/workflow — sit on top of this package. They are thin; the
// substrate is here.
package audit
