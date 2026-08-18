# AGM Command Hook Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**AGM-CMD-HOOK-01** When a command hook validates a test session operation, the system shall distinguish isolated test sessions from production sessions.

**AGM-CMD-HOOK-02** If a production session is targeted by a test-only operation, the system shall reject the operation with remediation guidance.

**AGM-CMD-HOOK-03** When Claude emits SessionStart for an AGM-launched session, the installed hook shall treat the payload's conversation UUID as authoritative over any inherited `CLAUDE_SESSION_ID`, use that environment variable only when the payload omits an ID, retry association asynchronously for at least the complete 90-second launch-readiness window when detached creation has not registered the AGM session yet, and report the session READY only after association succeeds, including when the Claude command was queued behind current-pane AGM creation.

**AGM-CMD-HOOK-04** When AGM publishes or installs the Claude SessionStart state hook, the embedded command-hook source shall be the only repository copy that advertises the `agm-state-ready` installation destination, so packaging and manual installation cannot select a retired implementation that omits UUID association.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Feature: `agm/test/bdd/features/harness_parity.feature`
