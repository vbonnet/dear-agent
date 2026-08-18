# Wayfinder Task Manager Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Canonical roadmap task persistence and dependency validation.

## EARS Requirements

**WFT-01** When a task is added, the system shall require one of the nine canonical descriptive phase names and a title.

**WFT-02** When a task identifier is generated, the system shall prefix the next phase-local sequence number with the canonical phase name.

**WFT-03** When task options declare priority, status, tests status, or effort, the system shall reject values outside their documented constraints.

**WFT-04** When a task declares dependencies, the system shall require every target to exist and shall reject dependency cycles.

**WFT-05** When a task with a bead declares deliverables, the system shall validate that deliverable paths are safe and exist beneath the configured repository root.

**WFT-06** When a task is updated to in-progress or completed, the system shall assign the corresponding timestamp once.

**WFT-07** When a task is deleted, the system shall reject deletion while another task depends on it.

**WFT-08** When tasks are listed, the system shall support canonical phase and status filters without mutating persisted state.

**WFT-09** When task state changes, the system shall atomically rewrite valid schema 2.0 status and update its timestamp.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/taskmanager/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
