# Private Harness Executor Specification

<!-- Last audited at: 2026-07-30 -->

## Overview

`agm/internal/harnessexec` is the private process boundary between token-free
tmux commands and interactive harness processes. The protocol is intercepted
before normal AGM startup and is not a user-facing command surface.

## Security Boundary

For Codex, the executor prevents ambient cross-harness credential inheritance
with a deny-by-default child environment. For Claude, it treats authentication
and OpenTelemetry state as a complete caller snapshot, but otherwise preserves
the harness environment. For other harnesses, it binds the exact submitted
command and launch-admission effects to a one-shot executor handoff. It protects transported values
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

**HEXEC-06** When the private executor launches Claude with OAuth, the system shall resolve the current token in the invoking AGM process, remove a competing Anthropic API key, and pass the OAuth value to the executor and child only through an owner-only one-shot handoff and child environment.

**HEXEC-07** When an executor request contains an unknown flag, positional argument, unsupported permission value, or control character, the system shall reject it before resolving or starting the harness executable.

**HEXEC-08** When caller authentication, executable search path, working directory, or telemetry differs from a long-lived tmux server's environment, the system shall treat the caller's Codex allowlist, including `PATH` for fixed-name `codex` resolution, and Claude authentication and telemetry state as a complete snapshot, including removals, for fresh and resumed sessions without exposing values in command text or process arguments; the Codex executor shall replace caller terminal identity and capability values with the target pane's `TMUX`, `TMUX_PANE`, `TERM`, `COLORTERM`, `TERM_PROGRAM`, and `TERM_PROGRAM_VERSION` values, normalize the child `PWD` to the validated target working directory, and enter Claude's validated target directory before resolving project-relative `PATH` entries or replacing the executor.

**HEXEC-09** When AGM or a co-installed companion prepares a private harness command, the system shall invoke the absolute current AGM executable regardless of its filename, or the matching co-installed AGM executable for an installed or release-versioned MCP companion, so the pane cannot resolve a missing or different installation through `PATH`.

**HEXEC-10** When AGM stages a private launch handoff, the system shall store it at a canonical absolute path in a current-user-owned, owner-only `private-launch` directory and file, reject paths or names outside that staging namespace without removing them, bind an accepted handoff to one harness protocol and a bounded lifetime, unlink it immediately after securely opening the exact file so successful and rejected reads are both one-shot, and remove an undelivered handoff only when command delivery positively fails before the irreversible submission boundary.

**HEXEC-11** When Claude telemetry forwarding is disabled, the system shall remove ambient OpenTelemetry endpoint and header configuration; when forwarding is enabled, it shall use the invoking AGM process's endpoint and headers.

**HEXEC-12** When AGM stages a credential-bearing launch handoff, the system shall start an independent, credential-free expiration helper before returning the command, shall intercept that helper before application or test main execution, shall fail closed and remove the handoff if the helper cannot start, shall asynchronously reap the helper when a long-lived caller survives it, and shall remove the same unconsumed handoff no later than its bounded lifetime without relying on a later AGM launch.

**HEXEC-13** When a current-pane launch command cannot execute until its producing AGM process exits, the system shall give the expiration helper a credential-free producer-liveness pipe, keep the handoff fresh only while that pipe remains open, and begin one bounded post-exit lifetime when the producer exits; cancellation shall release the pipe and remove the handoff.

**HEXEC-14** When tmux accepts a private launch submission but its acknowledgement is lost, the system shall require every current-pane, detached CLI, MCP, shared operations, agent adapter, legacy resume, and resumability-validation surface to treat the launch as potentially delivered, preserve its handoff and producer lease, and continue the lifecycle without retrying or compensating the possibly queued command.

**HEXEC-15** When AGM resolves an executable or stages a private handoff path that will be interpolated into a pasted pane command, the system shall reject invalid UTF-8 and terminal control characters in that generated value before building or delivering the command.

**HEXEC-16** When the private Codex executor consumes a hook-trust handoff, the system shall require the handoff to bind the exact source repository, full commit, hook digest, hook-trust claim, and every other launch override claim; re-run persisted Git attestation, hook configuration, helper validation, executable resolution, and every live circuit-breaker gate before and after reauthorization whether or not the handoff carries an admission-brake claim; treat every carried proof as a non-authoritative claim; require and re-reserve a current exact admission-brake grant whenever either live check reports the brake as the sole refusal; re-reserve every other current exact grant with a fresh authorization ID; and append the complete override transaction as the final userspace action before executing Codex.

**HEXEC-17** When a non-Codex tmux launch carries launch-admission reservations or a successful-spawn recording obligation, the system shall bind its exact session and command plus those effects into a cancellable one-shot handoff before submission; the private executor shall revalidate and commit every bound override immediately before recording the spawn and replacing itself with the submitted command, refuse execution if commit fails, remove the handoff after a definite pre-delivery failure, and preserve it when delivery is uncertain.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/harnessexec/*_test.go`
