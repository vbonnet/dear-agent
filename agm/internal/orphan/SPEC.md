# Orphan Detection And Reaping Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/orphan` detects orphaned saved conversations and safely reaps
detached long-lived child processes associated with AGM sessions.

## Requirements

**ORP-01** When scanning saved conversations, the system shall distinguish tracked, orphaned, and unreadable records and apply the requested workspace filter.

**ORP-02** When identifying orphan processes, the system shall match only configured executable targets, exclude the current process, and require an absent live parent chain.

**ORP-03** When reaping in dry-run mode, the system shall report eligible processes without sending signals.

**ORP-04** When reaping live processes, the system shall attempt configured termination, retain per-process failures, and continue processing other eligible targets.

**ORP-05** When session-scoped reaping is requested, the system shall find the configured agent ancestor and limit targets to descendants of that root.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_supervision_recovery_guardrails.feature`
- Package tests: `agm/internal/orphan/*_test.go`
