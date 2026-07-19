# Sample Backlog (test fixture)

Intro prose that is not a table.

## Phase 0 — Foundations

| # | Title | Files | Acceptance criteria | Dep | Size | Status |
|---|---|---|---|---|---|---|
| 0.1 | Bootstrap schema | `pkg/a` | round-trips | — | M | `done (#1)` |
| 0.2 | CLI status | `cmd/a` | flags work | 0.1 | S | `done` |

## Phase 1 — Build on Foundations

| # | Title | Files | Acceptance criteria | Dep | Size | Status |
|---|---|---|---|---|---|---|
| 1.1 | Role registry | `pkg/roles` | resolves tiers | 0.* | M | `pending` |
| 1.2 | Budget enforcer | `pkg/budget` | ceiling fails node | 1.1 | S | `pending` |
| 1.3 | Permission gate | `pkg/perm` | denial audited | 1.1 | L | `in-flight (claude/x)` |
| 1.4 | Quick lint fix | `pkg/lint` | clean | — | S | `pending` |
| 1.5 | Blocked work | `pkg/z` | n/a | 9.9 | M | `pending` |

## Phase 6 — Priority-tagged tickets

| id | Priority | Title | Slot |
|----|----------|-------|------|
| 6.1 | HIGH | Adversarial review | `pkg/audit` |
| 6.2 | MED | Trust inversion | `pkg/audit` |
| 6.3 | LOW | A/B model testing — `done` (abc123) | `pkg/roles` |

## Cross-phase

| # | Title | Notes |
|---|---|---|
| X.1 | Schema evolution policy | open question, needs a decision |
| DEAR-X.5 | ~~Flaky concurrent saves~~ DONE | fixed by retry loop |
