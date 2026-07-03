# Architecture Decision Records

Decisions about dear-agent's architecture live here. The format is light:
one short ADR per decision (target: ~200 words), descriptive H1, single
`Status:` line, no fixed Context/Decision/Alternatives/Consequences scaffold.
The 2026-05-17 ADR inventory prune (`vbonnet/engram-research` `audits/2026-05-17-adr-inventory-prune.md`)
captures the audit that produced the current set; this directory was rewritten
in Pocock-tight form on 2026-05-26.

**Test for keeping an ADR** ([Matt Pocock's three-question test][pocock]):
(1) hard to reverse, (2) surprising without context, (3) the result of a real
trade-off. Vocabulary, conventions, and standard patterns go to
[/CONTEXT.md](../../CONTEXT.md) instead — that is the domain-language source
of truth; an ADR loses to it for definitions.

[pocock]: https://github.com/mattpocock/skills/blob/main/skills/engineering/improve-codebase-architecture/SKILL.md

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [001](ADR-001-monorepo-consolidation.md) | Monorepo Consolidation | Accepted |
| [002](ADR-002-vroom-execution-architecture.md) | VROOM Execution Architecture | Accepted |
| [009](ADR-009-work-item-as-first-class-substrate.md) | WorkItem as First-Class Substrate | Proposed |
| [010](ADR-010-workflow-engine-architecture.md) | Workflow Engine as Substrate-Quality Work-Item Layer | Proposed |
| [011](ADR-011-dear-audit-subsystem.md) | Scheduled Repository Audit Subsystem | Proposed |
| [012](ADR-012-provider-transport-layer.md) | Provider Transport — Roles → Providers Routing | Proposed |
| [013](ADR-013-tailscale-api.md) | Tailscale-Integrated HTTP API | Proposed |
| [014](ADR-014-plugin-system.md) | Plugin System for Composable Extensibility | Proposed |
| [015](ADR-015-signal-aggregator.md) | Signal Aggregator + Recommendation MCP | Proposed |
| [016](ADR-016-recommendation-mcp-server.md) | _Recommendation MCP Server_ | **Superseded by [015](ADR-015-signal-aggregator.md)** |
| [017](ADR-017-gateway-platform-adapters.md) | Gateway and Platform Adapters | Proposed |
| [018](ADR-018-graceful-exit-framework-default.md) | Graceful Exit as a Framework Default | Accepted |
| [022](ADR-022-backlog-suggestion-system.md) | Backlog Suggestion System | Accepted |
| [023](ADR-023-friction-reporting-and-session-handoff.md) | Friction Reporting & Session Handoff | Proposed (design) |
| [029](ADR-029-ralph-wiggum-merge-loop.md) | Ralph Wiggum — host-tick persistent merge loop | Accepted |
| [030](ADR-030-dependabot-auto-merge.md) | Dependabot Auto-Merge via GitHub Actions | Accepted |
| [033](ADR-033-commit-anchored-progress-ledger.md) | Commit-Anchored Progress Ledger for Long-Running Workers | Accepted |
| [034](ADR-034-squash-only-merge-contract.md) | Squash-only merge contract + auto-merge arming | Accepted |
| [035](ADR-035-dear-terminology-disambiguation.md) | DEAR Terminology Disambiguation | Accepted |

Number gaps (003–008, 019–021, 024+) are intentional — earlier numbers were
withdrawn, renumbered into `agm/docs/adr/`, or superseded by the 2026-05-17
prune. Renumbering would break inbound refs and ADR identity.

Numbered ADRs deeper in the tree (`agm/docs/adr/`, `engram/`, `pkg/*/docs/`,
etc.) follow their own per-subsystem numbering and are not indexed here.
Follow-up pruning of those clusters is tracked in the audit's FU-1…FU-4,6
sections.
