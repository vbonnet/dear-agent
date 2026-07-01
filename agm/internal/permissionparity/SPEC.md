# Agent Permission Harness Parity Specification

<!-- Last audited at: 2026-07-01 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** AGM permission policy resolution, persistence, and active harness startup/runtime surfaces.

## Overview

Agent permission parity means AGM resolves one role/profile permission policy
for every session and carries that policy through all active harnesses. Claude
Code has a native allowlist file, while Codex CLI, AGY, and OpenCode expose
different permission controls. The manifest `permission_policy` field is the
shared control-plane record; native harness surfaces enforce the subset each
harness can represent.

## EARS Requirements

**APP-01** When AGM resolves session permissions, the system shall start from the shared default permission set.

**APP-02** When AGM receives explicit `--permissions-allow` entries, the system shall merge them into the resolved permission policy.

**APP-03** When AGM receives a valid `--permission-profile`, the system shall merge that role profile into the resolved permission policy.

**APP-04** When AGM receives `--role` and no explicit `--permission-profile`, the system shall derive the permission profile from the role if the role is a valid RBAC profile.

**APP-05** When AGM creates a session manifest, the system shall persist the resolved permission policy, source metadata, and active harness permission targets.

**APP-06** When AGM configures Claude Code permissions, the system shall write the resolved allowlist to `.claude/settings.local.json` rather than tracked settings.

**APP-07** When AGM starts Codex CLI, the system shall preserve the resolved policy in the manifest and launch Codex with `workspace-write` sandboxing.

**APP-08** When AGM starts AGY with `permission_mode=auto`, the system shall launch AGY with `--dangerously-skip-permissions` and preserve the resolved policy in the manifest.

**APP-09** When AGM starts OpenCode, the system shall preserve the resolved policy in the manifest and record OpenCode's permission surface as server-policy plus manifest coordination.

**APP-10** When an active harness is added, the system shall require a permission parity surface with non-empty policy, startup, runtime, and native-enforcement descriptions.

## BDD Traceability

- Feature: `agm/test/bdd/features/permission_parity.feature`
