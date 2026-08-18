# Mock Claude Harness Specification

<!-- Last audited at: 2026-08-18 -->

**Version:** 1.0
**Status:** Living
**Scope:** `mock-claude`, the stand-in harness for the reaper E2E.

## Overview

`mock-claude` stands in for the Claude Code harness in the reaper end-to-end
tests. It must be built to a file literally named `claude`: AGM decides whether
a session is alive by matching the pane process `COMM` against the harness, and
Linux takes `COMM` from the executable's file name. An interpreted mock reports
`python3` regardless of the script's name, so the session computes as `zombie`,
`agm session archive --async` refuses it as not active, and the reaper spawn
path the tests exist to exercise is never reached.

Its behaviour is the part of the harness contract the reaper depends on and
nothing more: a banner, the prompt the reaper's prompt detection matches, and a
clean exit on the native shutdown command.

## EARS Requirements

**MOCK-CLAUDE-01** When started, the system shall print the U+276F prompt, so the reaper's prompt detection completes rather than timing out.

**MOCK-CLAUDE-02** When the native exit command (`/exit` or `/quit`) is received, the system shall acknowledge and exit zero, so the reaper's pane-close wait completes.

**MOCK-CLAUDE-03** When any other input is received, the system shall re-display the prompt and continue running.

**MOCK-CLAUDE-04** When standard input reaches EOF, the system shall exit without acknowledging a shutdown command.

**MOCK-CLAUDE-05** When `--stuck` is set, the system shall never display the prompt, so the reaper's prompt-detection timeout, timer fallback, and pane-close escalation are exercised.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Package tests: `agm/test/e2e/docker/cmd/mock-claude/main_test.go`
- Consumed by: `agm/test/e2e/docker/scripts/test_reaper_*.sh`
