# Filesystem Write Hook Adapter Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/pretool-fs-write-guard` adapts Claude edit-tool envelopes to the shared
worktree-only filesystem policy used by active harness hook manifests.

## EARS Requirements

**PFW-01** When Edit, Write, MultiEdit, or NotebookEdit is received, the command shall resolve file, path, or notebook target fields in documented precedence.

**PFW-02** When another tool or an empty target is received, the compatibility adapter shall perform no write-policy decision.

**PFW-03** When a target is allowed by the shared policy, the command shall return exit code 0 without altering tool output.

**PFW-04** When a protected golden-source target is denied, the command shall return exit code 2 with positive worktree guidance.

**PFW-05** When warn, ask, or defer enforcement is selected, the command shall emit the corresponding structured permission decision without hard blocking.

**PFW-06** When hook input is malformed, the adapter shall fail open while native harness permission and deny surfaces remain active.

**PFW-07** When filesystem policy configuration is malformed, the command shall continue enforcing safe defaults.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_safety_command_guardrails.feature`
- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `cmd/pretool-fs-write-guard/*_test.go`
