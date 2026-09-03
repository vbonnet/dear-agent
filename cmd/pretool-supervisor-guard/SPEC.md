# Supervisor Guard Hook Adapter Specification

<!-- Last audited at: 2026-09-02 -->

## Overview

`cmd/pretool-supervisor-guard` adapts Claude's PreToolUse envelope to the
supervisor role policy in `internal/fsguard`. A VROOM supervisor coordinates
workers and never performs implementation work itself. The hook makes that rule
deterministic instead of leaving it to the role prompts, which stated it and did
not achieve it.

The rule exists for two operational reasons, both recorded in the retrospective:
a detached supervisor cannot answer a permission prompt, so a modal raised by a
supervisor's own tool call wedges it and stalls dispatch for the whole mesh; and
implementation detail consumes the context window the coordination role depends
on.

## EARS Requirements

**PSG-01** When the session is not a VROOM supervisor, the command shall exit 0 without evaluating any policy, so worker capability is unchanged.

**PSG-02** When the session is a supervisor and the tool is `Edit`, `Write`, `MultiEdit`, or `NotebookEdit`, the command shall evaluate the target path through `fsguard.CheckSupervisorWrite`.

**PSG-03** When the session is a supervisor and the tool is `Bash`, the command shall evaluate the command string through `fsguard.CheckSupervisorCommand`.

**PSG-04** When a supervisor tool call is refused, the command shall exit 2 and write guidance to stderr that names the attempted action, the delegation path to take instead, and why the call is blocked rather than confirmed.

**PSG-05** The command shall never emit a permission decision of `ask`, and shall not consult `FSGUARD_ENFORCEMENT`, because an `ask` raises the very modal the policy exists to prevent. Every refusal is a hard block.

**PSG-06** When the hook envelope is malformed, names another tool, or carries an empty path or command, the compatibility adapter shall exit 0 and fail open, so a guard defect can never wedge a supervisor's read and dispatch path.

**PSG-07** When a supervisor session is spawned by `agm supervisor run` (which sets `AGM_SUPERVISOR_ID`) or by `vroom-dispatch` through `agm session new` (which sets only `AGM_SESSION_NAME`), the command shall detect it identically, because guarding one spawn path would leave the production mesh unguarded.

## Installation

Build and install with the other PreToolUse guards:

```sh
make install-write-guards   # -> ~/.config/claude-code/hooks/pretool-supervisor-guard
```

Registration is chezmoi-managed, like the two write guards. The settings.json
entry is a `PreToolUse` hook with no matcher, so it sees every tool call:

```json
{ "type": "command", "command": "~/.config/claude-code/hooks/pretool-supervisor-guard", "timeout": 5 }
```

## BDD Traceability

- Package tests: `cmd/pretool-supervisor-guard/*_test.go`
- Policy tests: `internal/fsguard/supervisor_test.go`
