# A2A Package Audit — 2026-04-07

## Existing Code

| File/Package | Purpose |
|---|---|
| `cards.go` | Generate A2A AgentCard from AGM manifest (name, description, skills) |
| `registry.go` | File-based card CRUD at `~/.agm/a2a/cards/`, sync from manifests |
| `protocol/message.go` | Collaboration message: Context, Proposal, Questions, Blockers, NextSteps |
| `protocol/status.go` | Status types: pending, awaiting-response, consensus-reached, etc. |
| `channel/` | File-based markdown channels with creation, management, archiving |
| `jsonrpc/` | JSON-RPC 2.0 serialization of protocol messages |
| `discovery/checker.go` | Polling-based channel update detection |
| `config/` | A2A configuration: channels dir, retention days, poll interval |
| `personas/` | Persona loading from `.ai.md` files |
| `beads/` | Bead validation and linking |
| `artifacts/` | Artifact management |
| `review/` | Review aggregation |
| `token/` | Token counting |
| `tasks/` | Task claiming |
| `wayfinder/` | Wayfinder integration |

## Gaps Identified

### 1. Model Cards — Enhanced Agent Identity
Current `cards.go` generates basic AgentCards (name, description, skills) but lacks:
- **Capabilities**: what tools/skills the agent actually has
- **Status**: active/busy/idle runtime state
- **Runtime registration**: agents publishing their card on startup

### 2. Structured Message Protocol
Current `protocol/message.go` is collaboration-focused. Missing:
- **Message types**: request, response, notification, delegation
- **Routing**: direct (by name), broadcast, role-based
- **Correlation IDs**: request-response pairing
- **Sender/recipient fields**: for message routing

### 3. MessageBroker Abstraction
No broker interface exists. Need:
- Abstract interface for sending/receiving structured messages
- Integration with session lifecycle (card registration on create)
- Pluggable backends (file-based, eventbus, future: network)
