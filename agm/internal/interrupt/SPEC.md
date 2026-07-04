# AGM Interrupt Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/interrupt` owns non-destructive interrupt flags, interrupt audit
logging, and repeat-interrupt friction. It exists so orchestrators can stop,
steer, or kill sessions through a harness-neutral flag protocol instead of
injecting keystrokes into tmux panes and corrupting active user or agent input.

## EARS Requirements

**INTERRUPT-01** When an interrupt type is validated, the system shall accept only `stop`, `steer`, and `kill`.

**INTERRUPT-02** When an interrupt flag is written, the system shall create the interrupt directory with private permissions and atomically rename a temporary JSON file into place.

**INTERRUPT-03** When an interrupt flag is read and no flag exists, the system shall return no flag without error.

**INTERRUPT-04** When an interrupt flag is consumed, the system shall return the parsed flag and remove the flag file.

**INTERRUPT-05** When interrupt flags are cleared, the system shall remove only matching JSON flag files and tolerate missing directories.

**INTERRUPT-06** When stale interrupt flags exceed the configured maximum age, the system shall remove them and return the number cleared.

**INTERRUPT-07** When an interrupt is logged, the system shall append a JSONL audit entry with timestamp, sender, recipient, reason, state, flag, and recipient-local interrupt number.

**INTERRUPT-08** When a session receives its first interrupt, the system shall allow it without requiring a reason.

**INTERRUPT-09** When a session receives a second or later interrupt, the system shall require a reason before allowing the interrupt.

**INTERRUPT-10** When a session receives a third or later interrupt with a reason, the system shall allow the interrupt and log a high-frequency warning.

## BDD Traceability

- `agm/test/bdd/features/harness_parity.feature`

## Package Test Traceability

- `agm/internal/interrupt/interrupt_test.go`
- `agm/internal/interrupt/audit_test.go`
- `agm/internal/interrupt/friction_test.go`
