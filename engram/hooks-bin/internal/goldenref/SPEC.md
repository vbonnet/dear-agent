# Golden-reference Hook Configuration Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hooks-bin/internal/goldenref` identifies protected source roots so hook
enforcement can direct agent writes into worktrees.

## EARS Requirements

**EHGR-01** When configuration is missing or malformed, the advisory loader shall return an empty configuration without crashing the hook.

**EHGR-02** When configured paths are loaded, the system shall expand only `~` and `~/` home references and shall preserve named-user paths such as `~someone`.

**EHGR-03** When a path equals a configured source root or is its descendant, the system shall classify it as protected.

**EHGR-04** When a path merely shares a textual prefix with a configured root, the system shall not classify it as protected.

**EHGR-05** When a protected path is matched, the system shall return the original configured root associated with that path.

**EHGR-06** When session-isolation settings are decoded, the system shall preserve enablement, provisioning, branch, cleanup, and maximum-age controls.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_hook_guardrails.feature`
- Package tests: `engram/hooks-bin/internal/goldenref/*_test.go`
