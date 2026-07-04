# Why: Wayfinder V2 Is Canonical

## The principle
Wayfinder is the gate that PRs pass through. A gate with two definitions of its
own phases is a gate that lets the wrong things through and blocks the right
ones. V2 (9 phases) is the decided model; V1 (13 phases) is a broken window that
must be removed, not merely deprecated.

## Real failure cases (this repo)
- **Split-brain default.** `wayfinder/cmd/wayfinder-session/internal/status/types.go`
  defines V2 as canonical (`:42-44`) but `AllPhases()` still defaults to V1
  (`:74-85`). Any code path that trusts the default gets the retired model.
- **Stub validators.** The build-loop exit criteria are labelled "stub
  implementations" — a policy-critical gate standing on placeholders.
- **Phantom V1 docs.** Wayfinder docs still referenced a non-existent TypeScript
  engine, orienting agents toward the pre-V2 world.

## The model (V2, 9 phases)
CHARTER (intake) → PROBLEM → RESEARCH → DESIGN → SPEC (EARS-gated) → PLAN →
SETUP → BUILD (TDD loop) → RETRO.

## How to apply
- New Wayfinder work targets V2 only.
- Touching phase logic? Flip the default to V2 and delete V1 in the same change.
- Preserve the deterministic EARS gate at SPEC; replace stub validators with real
  gates rather than leaving placeholders.

See also: [broken-windows](broken-windows.why.md).
