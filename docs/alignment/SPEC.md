# AI Alignment Document Contract

<!-- Last audited at: 2026-07-18 -->

**Status:** Active
**Scope:** `MISSION.md`, `VALUES.md`, `GOALS.md`, and their VROOM references.

## EARS Requirements

**AIC-01** When an agent needs the project's purpose or the VROOM/AGM ownership boundary, the system shall designate `MISSION.md` as the canonical source.

**AIC-02** When an alignment document assigns operational ownership, the system shall assign prioritization, dispatch decisions, supervision, and output verification to VROOM.

**AIC-03** When an alignment document describes AGM, the system shall limit AGM ownership to session lifecycle mechanics.

**AIC-04** While no executable alignment evaluator exists, the system shall not present values as a lexicographic runtime model or goals as a weighted runtime model.

**AIC-05** When `VALUES.md` or `GOALS.md` is read, the system shall identify it as qualitative guidance subordinate to `MISSION.md`.

**AIC-06** When alignment documents change, the system shall keep ADR-002 and `CONTEXT.md` consistent with the canonical mission contract.

## Verification

`go test ./internal/instructions -run Alignment -count=1`
