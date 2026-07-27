# AGM architecture decisions

These records capture durable AGM trade-offs. Current source and tests outrank
the prose. Routine Go patterns, one-time migrations, bug-fix diaries, and VROOM
role descriptions do not belong in this subsystem decision set.

Numbers are scoped to this directory. Gaps preserve deleted identity. Numeric
identity is independent of zero padding, so records use the three-digit form.

## Index

| ADR | Decision | Status |
| --- | --- | --- |
| [001](ADR-001-multi-agent-architecture.md) | CLI harness adapter boundary | Accepted |
| [003](ADR-003-environment-validation-philosophy.md) | Validate harness prerequisites without owning them | Accepted |
| [004](ADR-004-tmux-integration-strategy.md) | Tmux as the local CLI session runtime | Accepted |
| [006](ADR-006-message-queue-architecture.md) | Durable local message queue | Accepted |
| [007](ADR-007-hook-based-state-detection.md) | Hooks as high-confidence state signals | Accepted |
| [009](ADR-009-eventbus-multi-agent-integration.md) | Typed in-process event bus | Accepted |
| [013](ADR-013-permission-mode-persistence.md) | Persist launch permission policy | Accepted |
| [015](ADR-015-harness-rename-and-model-selection.md) | Separate harness and model selection | Accepted |
| [016](ADR-016-shared-ops-layer.md) | Shared operations layer | Accepted |
| [017](ADR-017-pending-message-files.md) | Hook-assisted pending message delivery | Accepted |
| [018](ADR-018-advisory-file-reservations.md) | Advisory file reservations | Accepted |
| [019](ADR-019-a2a-agent-cards.md) | Derive A2A agent cards from session metadata | Accepted |
| [026](ADR-026-claude-ui-session-archival.md) | Reconcile Claude UI session archival locally | Accepted |
| [027](ADR-027-bdd-enforcement-policy.md) | Go-native BDD enforcement | Accepted |
| [028](ADR-028-multi-bot-discord-portal.md) | Multi-bot Discord portal | Accepted |
| [029](ADR-029-skill-allowed-tools-syntax.md) | Skill permission pattern syntax | Accepted |
| [031](ADR-031-consumer-owned-harness-capabilities.md) | Consumer-owned harness capabilities | Accepted |

VROOM decisions live in the root decision set; daemon and MCP decisions live
with their command packages.
