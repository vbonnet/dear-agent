# Wayfinder Phases — V1 ↔ V2 Truth Table

**Status:** Authoritative. Use this page when in doubt about phase counts,
names, or how V1 IDs map to V2 phases.

The Wayfinder repository currently contains **two parallel phase models**:

1. **V2 (canonical):** 9 phases — `CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC,
   PLAN, SETUP, BUILD, RETRO`. Implemented by `wayfinder/cmd/wayfinder-session/`
   and described in [`wayfinder/SPEC.md`](SPEC.md),
   [`wayfinder/README.md`](README.md), [`wayfinder/SKILL.md`](SKILL.md),
   and [`wayfinder/ARCHITECTURE.md`](ARCHITECTURE.md). This is the canonical
   Wayfinder model — when a doc, command, or skill says "Wayfinder is a
   9-phase workflow," it means V2.
2. **V1 (legacy orchestrator):** 12 detailed phase IDs — `D1-D4` discovery
   plus `S4-S11` SDLC (a `W0` charter sits outside the 12-ID array,
   making the conceptual V1 set 13). Implemented by
   `wayfinder/internal/phaseisolation/` and the V1 `wayfinder` CLI in
   `wayfinder/cmd/wayfinder/`. V1 still ships and runs.

V1 is **not deprecated for removal** — both models coexist intentionally,
bridged by `wayfinder/cmd/wayfinder-session/internal/migrate/`. New work
and new docs should target V2.

## The truth table

| V2 (canonical, 9) | V1 IDs that map here    | Notes |
|-------------------|-------------------------|-------|
| `CHARTER`         | `W0`                    | W0 is a charter-detector phase that sits outside the V1 D1-D4/S4-S11 array; it has no V1 ID inside the 12-ID `AllPhaseIDs()` slice. |
| `PROBLEM`         | `D1`                    | 1:1. |
| `RESEARCH`        | `D2`                    | 1:1. |
| `DESIGN`          | `D3`                    | 1:1. |
| `SPEC`            | `D4`, `S4`              | `S4` (stakeholder alignment) **merges into** `D4`/SPEC in V2. |
| `PLAN`            | `S5`, `S6`              | `S5` (research notes) **merges into** the V2 `PLAN` phase together with V1 `S6` (design). Naming gotcha: V1 `S6` is "Design," V2 `PLAN` is "plan" — they are intentionally the same V2 phase because V2 collapsed late-design and planning. |
| `SETUP`           | `S7`                    | V1 `S7` (plan/setup) → V2 `SETUP`. |
| `BUILD`           | `S8`, `S9`, `S10`       | `S8` implementation + `S9` validation + `S10` deploy all **merge into** V2 `BUILD` (V2 expects TDD + validation + deploy inside one loop). |
| `RETRO`           | `S11`                   | 1:1. |

Source of truth in code:

- V1 IDs: [`wayfinder/internal/phaseisolation/types.go`](internal/phaseisolation/types.go) (`PhaseD1`..`PhaseS11`, `AllPhaseIDs`)
- V2 names: [`wayfinder/internal/phaseisolation/types.go`](internal/phaseisolation/types.go) (`V2Charter`..`V2Retro`)
- Mapping: [`wayfinder/internal/phaseisolation/definitions.go`](internal/phaseisolation/definitions.go) (`V1ToV2PhaseMap`)
- Migration converter: [`wayfinder/cmd/wayfinder-session/internal/migrate/converter.go`](cmd/wayfinder-session/internal/migrate/converter.go)
  and [`wayfinder/cmd/wayfinder-session/internal/converter/converter.go`](cmd/wayfinder-session/internal/converter/converter.go)

## Where each surface lives

| Surface                              | Phase model | Notes |
|--------------------------------------|-------------|-------|
| `wayfinder/SPEC.md`                  | V2 (9)      | Canonical narrative. |
| `wayfinder/README.md`                | V2 (9)      | Front door. |
| `wayfinder/SKILL.md`                 | V2 (9)      | Skill description used by Claude Code. |
| `wayfinder/ARCHITECTURE.md`          | V2 (9)      | Notes the historical V1→V2 consolidation. |
| `wayfinder` CLI (`cmd/wayfinder/`)   | V1 (12)     | Walks D1→…→S11. Help text now frames itself as the V1 legacy orchestrator. |
| `wayfinder-session` CLI              | V2 (9)      | The current/recommended CLI. |
| `wayfinder/internal/phaseisolation/` | V1 (12)     | The V1 orchestrator engine. |
| Migration bridge                     | V1 → V2     | One-way: in-flight V1 sessions can be migrated to V2 status files. |

## Status field naming gotcha

V2 status files use `CurrentWaypoint` (not `current_phase`) in the Go
struct, while the V2 YAML schema documented in
[`wayfinder/cmd/wayfinder-session/SPEC.md`](cmd/wayfinder-session/SPEC.md)
still uses `current_phase`. This is a known doc/code drift captured by
the archived 2026-06-30 temporal cleanup notes; it is **not** related to
the 9-vs-12 axis and is unchanged by this document.

## History

The V1 12-ID model predates the V2 consolidation. The V2 collapse was
motivated by two findings during V1 use: (a) `S4` stakeholder alignment
duplicated content readers expected to find in `D4` requirements, and
(b) `S9` validation and `S10` deploy were never genuinely independent of
the `S8` implementation loop in practice. V2 collapses both pairs and
adds an explicit `CHARTER` phase up front (previously implicit in the
`W0` artifact). See [`wayfinder/ARCHITECTURE.md`](ARCHITECTURE.md) §
"Design Decisions" for the longer narrative.
