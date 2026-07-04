# SessionEnd State Reporter Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`sessionend-state-reporter` marks an AGM session ready when a Claude Code
SessionEnd hook fires.

## EARS Requirements

**SSR-01** When `CLAUDE_SESSION_NAME` is present, the system shall use it as the AGM session name.

**SSR-02** When `CLAUDE_SESSION_NAME` is absent, the system shall attempt to detect the current tmux session name.

**SSR-03** When no session name can be determined, the system shall skip state reporting.

**SSR-04** When a session name is available, the system shall invoke `agm session state set <session> READY --source sessionend-hook`.

**SSR-05** When the AGM state update command fails, the system shall ignore the error because the hook is advisory.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/cmd/agm-hooks/sessionend-state-reporter/main_test.go`

