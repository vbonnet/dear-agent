// Package audit implements the DEAR Audit subsystem — scheduled,
// repo-scoped health checks that produce de-duplicated findings,
// record remediation proposals, and propose amendments back to the Define and
// Enforce layers. See ADR-011 for the architectural decisions this
// package implements.
//
// The substrate is SQLite. Three additive tables — audit_findings,
// audit_runs, audit_proposals — are created by ApplySchema. Operators
// may apply them to the workflow engine's runs.db, but the shipped
// workflow-audit binary defaults to .dear-agent/audit.db and the tables
// can be queried in isolation.
//
// The mental model is a fleet of named Checks ("build", "test",
// "lint.go", "vuln.govulncheck", ...) registered in a Registry and
// invoked by a Runner against a per-call Env. A Check finds and a Refiner
// proposes amendments. The exported Remediator is a dormant compatibility
// seam whose only production implementation is a side-effect-free no-op; its
// outcome is not durable evidence. These stages remain separate so checks stay
// pure and trivial to test.
//
// Higher-level surfaces — the workflow-audit CLI, the .dear-agent.yml
// > audits: config loader, and workflow YAML wrappers that invoke the
// CLI from bash nodes — sit on top of this package. They are thin; the
// substrate is here.
package audit
