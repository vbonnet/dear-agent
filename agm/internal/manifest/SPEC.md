# AGM Manifest Specification

<!-- Last audited at: 2026-07-21 -->

## Purpose

`agm/internal/manifest` defines the persisted session record shared by AGM
commands, ops, harness adapters, permission policy, cost/statusline reporting,
sandboxing, and archive flows. It is the durable control-plane schema for
cross-harness session lifecycle state.

## EARS Requirements

**MAN-01** When a session manifest is written, the system shall use the current manifest schema version.

**MAN-02** When harness-specific saved-session metadata is present, the system shall preserve Claude, Codex, OpenAI API, AGY, OpenCode, and Pi metadata in their dedicated manifest fields.

**MAN-03** When permission policy is resolved for a session, the system shall record the profile, sources, allowlist, and per-harness target surfaces in the manifest.

**MAN-04** When a disposable session has a valid TTL and the TTL has elapsed, the system shall report the manifest as expired.

**MAN-05** When context usage, cost, model, sandbox, resource, or workflow-phase metadata is available, the system shall preserve that metadata without changing the session lifecycle value.

**MAN-06** When a Pi manifest is persisted to or restored from Dolt, the system shall preserve the exact native session ID, private session directory, and transcript path instead of dropping harness-specific identity metadata.

**MAN-07** When a pure OpenAI API session is registered, the manifest shall preserve its session-store locator and non-secret backward-compatible client fallback settings without storing an API credential.

**MAN-08** When AGM creates or imports a Pi session, the manifest and storage adapter shall preserve both the validated coding-agent directory and an explicit presence marker, including when the directory is intentionally empty for Pi's native default, so cold resume can distinguish new session state from legacy metadata and does not depend on the invoking shell or tmux server environment.

**MAN-09** When sandbox metadata is used as cleanup ownership evidence, the manifest shall require an enabled record whose sandbox ID matches the stable session ID, whose provider and creation time are present, whose clean absolute merged boundary is the identified sandbox's `merged` child, and whose clean absolute working directory is contained by that boundary.

**MAN-10** When AGM validates a session manifest, it shall accept lifecycle wire values only as empty legacy active/stopped, `reaping`, or `archived`, and archive outcome wire values only as empty legacy unknown, `completed`, `crashed`, `killed`, or `gc-stale`; every other value shall return an error without coercion.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/permission_parity.feature`
- Feature: `agm/test/bdd/features/quota_parity.feature`
