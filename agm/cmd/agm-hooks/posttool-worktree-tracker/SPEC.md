# PostTool Worktree Tracker Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`posttool-worktree-tracker` observes successful Bash tool calls and detects git
worktree add/remove operations for AGM session traceability.

## EARS Requirements

**PWT-01** When hook stdin is not valid JSON, the system shall fail open without recording a worktree event.

**PWT-02** When the tool call is not Bash, the system shall ignore the hook event.

**PWT-03** When the Bash command exits nonzero, the system shall ignore the hook event.

**PWT-04** When a successful Bash command adds a git worktree, the system shall extract the worktree path, branch, and optional repository path.

**PWT-05** When a successful Bash command removes a git worktree, the system shall extract the worktree path and optional repository path.

**PWT-06** When no AGM session can be found for the Claude session, the system shall skip worktree tracking.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/cmd/agm-hooks/posttool-worktree-tracker/main_test.go`

