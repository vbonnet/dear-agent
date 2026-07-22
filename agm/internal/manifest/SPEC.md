# AGM Manifest Specification

<!-- Last audited at: 2026-07-21 -->

## Purpose

`agm/internal/manifest` defines the persisted session record shared by AGM
commands, ops, harness adapters, permission policy, cost/statusline reporting,
sandboxing, and archive flows. It is the durable control-plane schema for
cross-harness session lifecycle state.

## EARS Requirements

**MAN-01** When a session manifest is written, the system shall use the current manifest schema version.

**MAN-02** When harness-specific saved-session metadata is present, the system shall preserve Claude, Codex, AGY, OpenCode, and Pi metadata in their dedicated manifest fields.

**MAN-03** When permission policy is resolved for a session, the system shall record the profile, sources, allowlist, and per-harness target surfaces in the manifest.

**MAN-04** When a disposable session has a valid TTL and the TTL has elapsed, the system shall report the manifest as expired.

**MAN-05** When context usage, cost, model, sandbox, resource, or workflow-phase metadata is available, the system shall preserve that metadata without changing the session lifecycle value.

**MAN-06** When a Pi manifest is persisted to or restored from Dolt, the system shall preserve the exact native session ID, private session directory, and transcript path instead of dropping harness-specific identity metadata.

**MAN-07** When a Pi session uses an explicit coding-agent directory, the manifest and storage adapter shall preserve its validated absolute path so cold resume does not depend on the invoking shell or tmux server environment.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/permission_parity.feature`
- Feature: `agm/test/bdd/features/quota_parity.feature`
