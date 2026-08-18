# VROOM Prompt Generation Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/vroom-prompt-gen` materializes idempotent worker prompts from ready Beads
and explicit harness, model, permission-mode, and workspace routes.

## EARS Requirements

**VPG-01** When ready Beads are queried, the command shall use the explicitly selected Beads database and decode structured JSON.

**VPG-02** When open pull requests cannot be listed, the command shall fail closed rather than risking duplicate dispatch.

**VPG-03** When a Bead is human-gated, already has a prompt, or is referenced by an open pull request, the command shall not generate another prompt.

**VPG-04** When Bead identifiers are matched, the command shall use token boundaries that distinguish parent and child identifiers.

**VPG-05** When candidates are selected, the command shall return them in deterministic identifier order.

**VPG-06** When a worker prompt is rendered, the command shall include source-read-only, worktree, safe-push, Beads, stop-condition, and no-force rules.

**VPG-07** When worker routing is configured, the command shall preserve the selected active harness, supported model-family identifier, permission mode, and workspace.

**VPG-08** When dry-run mode is selected, the command shall report candidate paths without creating directories or files.

**VPG-09** When prompts are written, the command shall use private temporary files and atomic rename so dispatch never sees truncated content.

**VPG-10** When no new candidates exist, the command shall report a normal steady state and exit successfully.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_operations_command_guardrails.feature`
- Package tests: `cmd/vroom-prompt-gen/*_test.go`
