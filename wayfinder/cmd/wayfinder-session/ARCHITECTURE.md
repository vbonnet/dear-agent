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
- `internal/archive`: completed-session archiving and complete archive references.

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

Validation and admission errors are actionable and leave the prior status
intact. Forceful operations require the command's explicit justification flag.
Persistence errors follow each transition's documented ordering contract.

Rewind takes an interprocess lock at the project's owned
`.wayfinder/locks/rewind.lock` before parsing status and holds it through the
scoped commit. Keeping one fixed lock with the project makes project-root
symlink, case, home, cache, profile, and temporary-directory aliases converge on
the same underlying file; lock admission does not depend on a writable
OS-account home. Links or reparse points in `.wayfinder`, `locks`, or the lock
file itself are rejected rather than followed outside the project. A concurrent
rewind is rejected before lifecycle mutation. Within that serialized boundary,
rewind is ordered rather than transactionally atomic: it publishes a complete
archive, persists status and trace evidence, then commits the exact archive and
canonical markers. If a required later step fails, the command returns an error
without a success claim; earlier filesystem writes remain inspectable for
recovery rather than being represented as rolled back.

## Verification

```sh
go test ./wayfinder/cmd/wayfinder-session/...
go test ./agm/test/bdd -run TestFeatures
```
