# AGM Hook Utility Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/hooks` contains shared hook helpers used by AGM guardrails. It
tracks explicit session-exit completion markers and warns on aspirational
docs-only commits that mention features without matching implementation code.

## EARS Requirements

**HOOK-UTIL-01** When a session exit gate is checked, the system shall require a session-specific marker under `~/.agm/exit-markers`.

**HOOK-UTIL-02** When a session exit marker is missing, the system shall return an error that tells the user to run `/agm:agm-exit` first.

**HOOK-UTIL-03** When an exit marker is written, the system shall create the marker directory if needed and write a `0600` marker file for the session.

**HOOK-UTIL-04** When a commit message is not a docs aspirational commit, the system shall return no warning and shall not block the commit.

**HOOK-UTIL-05** When a docs aspirational commit names a feature that has matching non-test Go code in the repository, the system shall return no warning.

**HOOK-UTIL-06** When a docs aspirational commit names a feature without matching non-test Go code, the system shall return a warning while keeping the hook non-blocking.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/internal/hooks/exit_gate_test.go`
- Package tests: `agm/internal/hooks/living_docs_check_test.go`
