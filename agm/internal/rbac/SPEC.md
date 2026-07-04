# AGM RBAC Permission Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/rbac` resolves AGM permission profiles into concrete tool
allowlists. It combines safe defaults, explicit CLI flags, profile grants, and
optional parent Claude Code settings while writing local project permissions
without modifying tracked settings files.

## EARS Requirements

**RBAC-01** When permissions are resolved, the system shall include the safe default permission allowlist before explicit, profile, or inherited permissions.

**RBAC-02** When explicit permissions are supplied, the system shall include them in the merged allowlist.

**RBAC-03** When a permission profile is supplied, the system shall look up the profile and include that profile's allowed tools.

**RBAC-04** When parent permission inheritance is enabled, the system shall read `~/.claude/settings.json` and include string entries from `permissions.allow`.

**RBAC-05** When inherited settings are missing or do not contain `permissions.allow`, the system shall treat inherited permissions as empty.

**RBAC-06** When merged permissions contain duplicates, the system shall deduplicate them while preserving first-seen order.

**RBAC-07** When project permissions are configured, the system shall write `.claude/settings.local.json` with merged `permissions.allow` entries and shall not write tracked `.claude/settings.json`.

**RBAC-08** When a role maps to a supervisor-tier profile, the system shall include AGM, tmux, git, documentation, and `.agm` access needed for supervised session coordination.

## BDD Traceability

- Feature: `agm/test/bdd/features/permission_parity.feature`
- Package tests: `agm/internal/rbac/rbac_test.go`
