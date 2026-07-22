# Wayfinder session architecture

<!-- Last audited at: 2026-07-17 -->

`wayfinder session` is the single lifecycle command tree.

## Modules

- `commands`: Cobra input validation and orchestration.
- `internal/status`: schema 2.0 types, parsing, serialization, and phase order.
- `internal/validator`: start, completion, documentation, code, and child gates.
- `internal/taskmanager`: roadmap task mutations and dependency checks.
- `internal/history`: append-only transition evidence.
- `internal/tracker`: event publication.
- `internal/review`: optional provider-backed review behind deterministic gates.
- `internal/archive`: completed-session archiving.

Commands parse canonical status directly. There is no migration command,
version detector fallback, database-backed state path, or synthetic build
executor.

## Transition flow

```text
CLI flags
  -> locate WAYFINDER-STATUS.md
  -> ParseV2
  -> validate command preconditions
  -> validate phase-specific gates
  -> write status atomically
  -> append history / publish event
```

Errors are actionable and leave the prior status intact. Forceful operations
require the command's explicit justification flag.

## Verification

```sh
go test ./wayfinder/cmd/wayfinder-session/...
go test ./agm/test/bdd -run TestFeatures
```
