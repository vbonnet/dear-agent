# Private Harness Executor Specification

<!-- Last audited at: 2026-07-19 -->

## Overview

`agm/internal/harnessexec` is the private process boundary between token-free
tmux commands and interactive Codex or Claude processes. The protocol is
intercepted before normal AGM startup and is not a user-facing command surface.

## Security Boundary

The executor protects credentials from ambient cross-harness inheritance and
from shell command, process-argument, pane-scrollback, and debug-log exposure.
It assumes the installed AGM and harness executables, the user's environment,
and the user's configuration and credential files are trusted. It does not
sandbox the harness beyond the native permission mode requested by AGM.

## Requirements

**HEXEC-01** When AGM builds a Codex or Claude tmux launch command, the system shall include only non-secret launch metadata and shall not interpolate any credential value into shell text or process arguments.

**HEXEC-02** When the private executor launches Codex, the system shall resolve only the fixed `codex` executable, validate every forwarded argument, and replace the AGM process directly without invoking a second shell.

**HEXEC-03** When the private executor launches interactive Codex, the system shall construct a deny-by-default child environment containing only documented terminal, locale, network, Codex state/authentication, built-in OpenAI authentication, and narrowly named AGM runtime variables.

**HEXEC-04** When ambient Claude, Anthropic, Google, GitHub, Engram, OpenTelemetry, SSH-agent, or arbitrary variables exist, the Codex child environment shall exclude them.

**HEXEC-05** When explicit OpenAI authentication is configured for AGM's built-in Codex provider, the system shall pass it only through the child environment and shall not expose the value in command text, argv, pane scrollback, or debug logs.

**HEXEC-06** When the private executor launches Claude with OAuth, the system shall resolve the current token inside the executor, remove a competing Anthropic API key, and pass the OAuth value only through the child environment.

**HEXEC-07** When an executor request contains an unknown flag, positional argument, unsupported permission value, or control character, the system shall reject it before resolving or starting the harness executable.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/harnessexec/*_test.go`
