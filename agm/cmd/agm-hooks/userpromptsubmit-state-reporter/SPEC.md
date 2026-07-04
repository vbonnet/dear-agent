# UserPromptSubmit State Reporter Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`userpromptsubmit-state-reporter` marks an AGM session thinking when a user
prompt is submitted.

## EARS Requirements

**UPS-01** When `CLAUDE_SESSION_NAME` is present, the system shall use it as the AGM session name.

**UPS-02** When `CLAUDE_SESSION_NAME` is absent, the system shall attempt to detect the current tmux session name.

**UPS-03** When no session name can be determined, the system shall skip state reporting.

**UPS-04** When a session name is available, the system shall invoke `agm session state set <session> THINKING --source userpromptsubmit-hook`.

**UPS-05** When the AGM state update command fails, the system shall ignore the error because the hook is advisory.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/cmd/agm-hooks/userpromptsubmit-state-reporter/main_test.go`

