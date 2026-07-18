# Architecture decisions

This directory records current, durable Dear Agent trade-offs. Keep a record
only when the choice is hard to reverse, surprising without context, and the
result of a real alternative. `CONTEXT.md` owns vocabulary and conventions;
Engram Research owns active plans and retrospectives; git history owns deleted
implementation diaries.

Each record has one scoped numeric identity, one `Status:` line, and a concise
Context/Decision/Alternatives/Consequences shape. A superseded record must name
its live successor. Number gaps are intentional; do not renumber old identities
to close them.

## Index

| ADR | Decision | Status |
| --- | --- | --- |
| [001](ADR-001-monorepo-consolidation.md) | Monorepo consolidation | Accepted |
| [002](ADR-002-vroom-execution-architecture.md) | VROOM execution architecture | Accepted |
| [010](ADR-010-workflow-engine-architecture.md) | Durable workflow execution substrate | Accepted |
| [011](ADR-011-dear-audit-subsystem.md) | Scheduled repository audit subsystem | Accepted |
| [012](ADR-012-provider-transport-layer.md) | Role-based model and provider routing | Accepted |
| [013](ADR-013-tailscale-api.md) | Tailscale-integrated HTTP API | Accepted |
| [014](ADR-014-plugin-system.md) | Plugin system for composable extensibility | Accepted |
| [015](ADR-015-signal-aggregator.md) | Signal aggregator and recommendation MCP | Accepted |
| [017](ADR-017-gateway-platform-adapters.md) | Transport-neutral gateway handlers | Accepted |
| [018](ADR-018-graceful-exit-framework-default.md) | Graceful exit as a framework default | Accepted |
| [022](ADR-022-backlog-suggestion-system.md) | Backlog suggestion system | Accepted |
| [023](ADR-023-friction-reporting-and-session-handoff.md) | Friction reporting and session handoff | Proposed |
| [024](ADR-024-a2a-protocol-adoption.md) | A2A supervisor protocol | Accepted |
| [027](ADR-027-bumblebee-endpoint-scanner.md) | Pinned endpoint inventory scanner | Accepted |
| [028](ADR-028-smart-integration-test-selection.md) | Dependency-aware integration test selection | Accepted |
| [029](ADR-029-ralph-wiggum-merge-loop.md) | Host-tick merge loop | Accepted |
| [030](ADR-030-dependabot-auto-merge.md) | Dependabot auto-merge | Accepted |
| [031](ADR-031-agent-escalation-path.md) | Audited exceptions and escalation | Accepted |
| [032](ADR-032-escalate-to-supervisor.md) | Supervisor escalation chain | Accepted |
| [033](ADR-033-commit-anchored-progress-ledger.md) | Commit-anchored progress ledger | Accepted |
| [034](ADR-034-squash-only-merge-contract.md) | Squash-only merge contract | Accepted |
| [035](ADR-035-dear-terminology-disambiguation.md) | DEAR terminology disambiguation | Accepted |
| [036](ADR-036-wayfinder-enforcement.md) | Wayfinder trace at the delivery boundary | Accepted |

Subsystem ADR directories have independent scoped numbering and indexes.
