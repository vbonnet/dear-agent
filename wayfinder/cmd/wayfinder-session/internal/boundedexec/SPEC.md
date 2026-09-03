# Wayfinder Bounded Execution Requirements Specification (EARS)

<!-- Last audited at: 2026-09-03 -->

**Version**: 1.0
**Status**: Active
**Scope**: External commands run by Wayfinder gates.

## Context

A context deadline is not by itself a bound. `exec.CommandContext` signals only
the direct child, so a descendant that inherited the output pipes keeps the
write end open and `Cmd.Wait` blocks until that descendant exits on its own.
`go test ./...` leaves reparented test binaries behind, which is how a
documented ten-minute gate timeout became an indefinite hang in
`wayfinder session complete-phase`.

## EARS Requirements

**WAYFINDER-BOUNDEDEXEC-01** When a command is run, the system shall detach standard input so the command never blocks waiting on a terminal.

**WAYFINDER-BOUNDEDEXEC-02** When a wall-clock timeout is configured and expires, the system shall return within a bounded wait delay even if a descendant process still holds the output pipes.

**WAYFINDER-BOUNDEDEXEC-03** When a command is still running, the system shall emit a progress line at a fixed interval naming the command and the elapsed time.

**WAYFINDER-BOUNDEDEXEC-04** When a command exits non-zero, the system shall report the exit code and the captured combined output.

**WAYFINDER-BOUNDEDEXEC-05** When a command's output exceeds the configured limit, the system shall truncate it and record that truncation.

**WAYFINDER-BOUNDEDEXEC-06** When a run ends, the system shall distinguish a timeout from an ordinary command failure.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/boundedexec/boundedexec_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
- Callers: `wayfinder/cmd/wayfinder-session/internal/validator/`, `wayfinder/cmd/wayfinder-session/internal/review/`
