# Wayfinder requirements specification

<!-- Last audited at: 2026-07-22 -->

**Status:** Active
**Scope:** The canonical Wayfinder session lifecycle and its AI-facing contract.

## EARS requirements

**WAY-01** When a session is created, the system shall persist schema version 2.0 and one of the nine named phases.

**WAY-02** When session state is read, the system shall reject a missing or unsupported schema version.

**WAY-03** When a phase is started, the system shall require the nearest preceding non-skipped phase to be complete.

**WAY-04** When a phase is completed, the system shall require that phase to be in progress and its deliverable to be meaningful.

**WAY-05** When DESIGN is started, the system shall require bounded RESEARCH evidence with an overlap assessment and the required search methodology.

**WAY-06** When SPEC is completed, the system shall validate strict EARS requirements deterministically.

**WAY-07** When a pre-BUILD phase contains modified implementation files, the system shall reject completion as a phase-boundary violation.

**WAY-08** When BUILD is completed, the system shall require implementation evidence and all applicable build, test, review, and child-project gates to pass.

**WAY-09** When unresolved clarification markers or required unchecked assumptions exist, the system shall reject phase completion with remediation guidance.

**WAY-10** When a rewind is requested, the system shall require a prior completed phase and record the supplied reason.

**WAY-11** When lifecycle state is changed, the system shall validate the state and its required diagnostic fields.

**WAY-12** When task commands modify the roadmap, the system shall validate named phases, dependencies, task identifiers, and allowed task states.

**WAY-13** When status is written, the system shall preserve canonical phase history and use an atomic filesystem update.

**WAY-14** When the stop hook encounters malformed active state, the system shall block rather than assume completion.

**WAY-15** When no Wayfinder status exists, the stop hook shall allow the unrelated session to stop.

**WAY-16** When a documented Wayfinder command or skill is changed, the system shall validate it against the registered Cobra surface and reject retired phase vocabulary.

**WAY-17** When active command guidance names the Wayfinder executable, the system shall use the canonical `wayfinder session` entrypoint and reject the retired standalone binary.

**WAY-18** When repository validation surfaces are inventoried, the system shall reject validators built for retired Wayfinder artifact or retrospective schemas.

**WAY-19** When a phase document contains canonical leading YAML frontmatter, the document quality gate shall review the Markdown body after the closing delimiter.

## Traceability

- Commands and state: `wayfinder/cmd/wayfinder-session`
- Gates: `wayfinder/cmd/wayfinder-session/internal/validator`
- Stop behavior: `wayfinder/hooks/cmd/stop-wayfinder-guard`
- Cross-surface contracts: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
- Strict-spec linkage: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`
