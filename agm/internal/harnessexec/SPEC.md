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
command and launch-admission effects to a one-shot executor handoff. It
protects transported values from shell command, process-argument,
pane-scrollback, and debug-log exposure.
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

**HEXEC-17** When a non-Codex tmux or foreground launch carries launch-admission reservations or a successful-spawn recording obligation, the system shall bind its exact session and command plus those effects into a cancellable one-shot handoff before submission; the private executor shall revalidate and commit every bound override immediately before recording the spawn and replacing itself with the submitted command, refuse execution if commit fails, remove the handoff after a definite pre-delivery failure, and preserve it when delivery is uncertain. A foreground Claude supervisor launch shall use the same one-shot boundary, bind its exact executable and argument request, re-resolve the executable before commitment, and leave every bound effect unconsumed when the executor or Claude executable cannot start.

**HEXEC-18** When a foreground Claude launcher carries launch-admission effects, the system shall expose a direct invocation of the same prepared one-shot Claude executor, bind only Claude-compatible override claims and the successful-spawn obligation before starting that executor, re-resolve the configured Claude executable inside it, and consume no bound effect when executor delivery or executable resolution is confirmed to have failed.

**HEXEC-19** When a private Codex, Claude, or generic harness handoff carries any override proof, the authenticated parent AGM process, or the separately attested co-installed AGM MCP companion that prepares MCP launches directly, shall obtain a root-owned capability for the exact protocol, path, handoff digest, proofs, and accompanying successful-spawn obligation before submission; the AGM executor shall require the matching capability and atomically consume it under the AGM-only caller policy before accepting those claims, rejecting self-generated, mutated, and replayed same-user handoffs.

**HEXEC-20** When a trusted AGM launch prepares a Codex worker with host-authorized write roots, the system shall carry the derived `AGM_WORKER_WRITE_ROOTS_JSON` value through the owner-only private handoff into the Codex child environment, independent of long-lived tmux server state; when the caller omits the value, stale pane or tmux state shall not recreate it.

**HEXEC-21** When the private executor receives the cold-remote-resume marker from AGM's Codex resume lifecycle, the system shall pass the documented `model_reasoning_effort="xhigh"` configuration override so a persisted unsupported effort value cannot prevent the resumed worker from receiving its first provider turn; fresh remote controller attachments and local Codex launches shall not receive that override.

**HEXEC-23** When a private Claude Code or Codex CLI launch supplies a per-session sandbox-workspace snapshot, the system shall carry it through the owner-only handoff, replace the inherited `FSGUARD_SANDBOX_WORKSPACE` value in the child, omit the stale inherited value from generated tmux command text, and encode neither a `FSGUARD_SANDBOX_WORKSPACE` assignment nor a disk-root or sandbox-workspace authority flag in that command text.

**HEXEC-24** When a private Claude Code or Codex CLI launch omits a sandbox workspace, the system shall remove inherited `FSGUARD_SANDBOX_WORKSPACE` from the child instead of recreating stale pane or tmux authority.

**HEXEC-25** When the private Claude Code or Codex CLI executor finishes executable preparation for a launch handoff, the system shall repeat the disk-only headroom gate using the retained configured parent sandbox root when present and the default disk reader when that root is omitted, plus the caller's explicit finite non-negative minimum-free-disk snapshot (including zero), before any optional override-proof commitment or process replacement; the proof-bearing full admission recheck shall use that same carried disk threshold, and either check shall refuse further launch processing if it fails or cannot be evaluated instead of consulting a stale pane threshold.

**HEXEC-26** When AGM prepares an ordinary Codex CLI private handoff, the system shall bind the exact requested launch arguments even when no override proof is present; when a managed Claude Code or Codex CLI executor consumes a handoff with an exact sandbox workspace, the system shall require the workspace itself to retain its exact physical spelling, physically re-resolve the effective working directory immediately before launch, and reject either a retargeted workspace authority or a working directory that no longer remains within it.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/private_sandbox_launch_authority.feature`
- Package tests: `agm/internal/harnessexec/*_test.go`
