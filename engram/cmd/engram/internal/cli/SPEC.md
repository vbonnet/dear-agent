# Engram CLI Support Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/cmd/engram/internal/cli` provides structured errors, output helpers,
progress reporting, input validation, and filesystem security for Engram commands.

## EARS Requirements

**ECS-01** When a structured CLI error is rendered, the system shall include its symbol, message, optional details, suggestions, documentation link, and wrapped cause without exposing empty sections.

**ECS-02** When enum, range, positive, non-empty, namespace, output-format, shell, or tier validation fails, the system shall return an actionable field-specific validation error.

**ECS-03** When required filesystem input is validated, the system shall distinguish missing paths, files, and directories according to the caller's expectation.

**ECS-04** When a path is checked against allowed roots, the system shall expand it, canonicalize it, reject null bytes, and require either exact-root equality or a separator-bounded descendant.

**ECS-05** When a relative path contains traversal or resolves outside its base directory, the system shall reject it.

**ECS-06** When untrusted text exceeds configured limits or contains forbidden shell metacharacters, the system shall reject it before command construction.

**ECS-07** When namespace components are validated, the system shall enforce total length, component count, component length, and non-empty component limits.

**ECS-08** When environment-derived paths are used, the system shall validate the expanded value against the same allowed-root boundary as direct input.

**ECS-09** When output color is disabled, the system shall use stable plain-text icons and preserve message semantics.

**ECS-10** When progress state starts, updates, completes, fails, or stops, the system shall serialize state changes and stop any active renderer safely.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_cli_support_guardrails.feature`
- Package tests: `engram/cmd/engram/internal/cli/*_test.go`
