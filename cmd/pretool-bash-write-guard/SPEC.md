# Bash Write Hook Adapter Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/pretool-bash-write-guard` adapts Claude's Bash PreToolUse envelope to the
shared filesystem-write policy. Equivalent non-Claude manifests invoke the
same neutral policy through their native hook adapters.

## EARS Requirements

**PBW-01** When a valid Bash tool envelope is received, the command shall evaluate filesystem-mutating command patterns through `internal/fsguard`.

**PBW-02** When a command is a pure read, the command shall allow it without emitting a decision.

**PBW-03** When a protected golden-source write is denied, the command shall return exit code 2 and positive worktree guidance.

**PBW-04** When warn, ask, or defer enforcement is selected, the command shall return exit code 0 with the corresponding structured permission decision.

**PBW-05** When the hook envelope is malformed or targets another tool, the compatibility adapter shall fail open while shared native deny rules remain the backstop.

**PBW-06** When filesystem policy configuration is malformed, the command shall retain safe default enforcement rather than disabling the policy.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_safety_command_guardrails.feature`
- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `cmd/pretool-bash-write-guard/*_test.go`
