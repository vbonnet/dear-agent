# Wayfinder validator requirements specification

<!-- Last audited at: 2026-07-17 -->

**Status:** Active
**Scope:** Deterministic start and completion gates.

## EARS requirements

**WFVALID-01** When a phase start is requested, the system shall accept only a named phase and require the nearest preceding non-skipped phase to be complete.

**WFVALID-02** When DESIGN is started, the system shall require bounded `RESEARCH-existing-solutions.md` content with overlap analysis, search methodology when reuse is incomplete, and at least 200 words.

**WFVALID-03** When phase completion is requested, the system shall require that phase to be in progress and a substantive canonical deliverable to exist.

**WFVALID-04** When SPEC is completed, the system shall validate strict EARS requirements without a model provider.

**WFVALID-05** When required design or plan review is configured, the system shall apply it to the canonical artifact and preserve deterministic gates.

**WFVALID-06** When a pre-BUILD phase contains modified source code, the system shall reject completion as a phase-boundary violation.

**WFVALID-07** When BUILD is completed, the system shall require implementation evidence and reject placeholder-only claims.

**WFVALID-08** When code verification runs, the system shall contain paths, bound file sizes and execution time, and require successful applicable commands.

**WFVALID-09** When unresolved clarification markers, required unchecked assumptions, or pending questions exist, the system shall reject completion.

**WFVALID-10** When a skipped phase precedes the requested phase, the system shall gate on the nearest preceding non-skipped phase.

**WFVALID-11** When a provider review is unavailable, the system shall report that boundary without manufacturing a passing result.

**WFVALID-12** When a project without a completed lifecycle state is evaluated for parent completion, the system shall require every canonical non-skipped waypoint to be present and completed.

**WFVALID-13** When BUILD or RETRO completion validates Git state, the system shall allow the current phase artifact to reach the scoped completion commit while rejecting untracked artifacts from earlier phases and untracked BUILD source code.

**WFVALID-14** When `phase_engram_path` is relative, the system shall resolve it against the Wayfinder project directory before hashing while continuing to accept absolute and home-relative paths.

Only exact `~` and `~` followed by a platform path separator are home-relative. Other leading-tilde path components are project-relative.

## Traceability

- Tests: `wayfinder/cmd/wayfinder-session/internal/validator/*_test.go`
- Cross-surface BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
