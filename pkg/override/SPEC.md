# Dangerous Override Specification

<!-- Last audited at: 2026-07-29 -->

## Purpose

`pkg/override` is the shared contract for every escape hatch that switches off
a safety control. Two exist today — the Codex hook-trust bypass and the
admission-brake override — and both reach the same failure mode if left
ungoverned: an unattended agent flips one to get past a blocker, nobody
notices, and the control is silently dead.

Rather than two bespoke implementations, both travel one pattern: **stated
reason, human approval, ledger, recurring audit**.

## EARS Requirements

**OVR-01** When a dangerous override is requested, the system shall refuse it unless the caller supplied a reason that is present, long enough to audit, and not boilerplate.

**OVR-02** When a dangerous override is requested, the system shall refuse it unless an unexpired human approval exists for that override kind in root-owned storage that the agent user cannot modify.

**OVR-03** When a human approval is minted, the system shall require an interactive terminal and a typed confirmation naming the override kind.

**OVR-04** When a human approval exists for one override kind, the system shall not treat it as approval for any other override kind.

**OVR-05** When an override is authorized, the system shall durably append a ledger record containing the kind, reason, actor, session, and timestamp through operator-owned storage that the agent user cannot truncate, replace, or remove.

**OVR-06** When the ledger cannot be read or appended, the system shall refuse the override.

**OVR-07** When a session that launched under an override is resumed, the system shall re-authorize the override instead of inheriting it.

**OVR-08** When the override audit runs, the system shall group recorded uses by kind and by reason and rank repeated reasons first.

**OVR-09** When any single override kind reaches the alert threshold within the audit window, the system shall exit with code 3.

**OVR-10** When override use is aggregated, the system shall evaluate the alert threshold separately for each override kind.

**OVR-11** When a raw Codex hook-trust bypass is requested outside AGM, the system shall route it through the canonical authorization entry point and shall record the authorized use.

**OVR-12** When AGM's private Codex executor receives a hook-trust bypass, the system shall require a one-shot prepared handoff that binds the exact attested hook root and complete launch request and lives outside the workspace and every agent-writable root.

## Override kinds

| Kind | Disables | Requested by |
|---|---|---|
| `codex-hook-trust` | Codex per-path hook trust review | `sandbox.bypass_codex_hook_trust_reason`, or `agm new --dangerously-bypass-hook-trust="<reason>"` |
| `admission-brake` | The watchdog admission brake | `agm new --brake-override="<reason>"` |

## Enforcement surfaces

Authorization is enforced **in the binary**, at every launch and resume path.
It does not depend on a harness hook firing, because a hook that silently fails
to run is exactly how a guardrail becomes decorative (see
`.codex/hooks/SPEC.md` and the `${CLAUDE_PROJECT_DIR}` path defect).

`.codex/hooks/pretool-dangerous-override-guard` is defence in depth for the
case where an agent shells out to a raw `codex --dangerously-bypass-hook-trust`
instead of going through AGM. Raw Codex's flag is boolean, so its reason comes
from `AGM_CODEX_HOOK_TRUST_REASON` in the parent Codex environment; a tool
command cannot set that environment retroactively. The hook invokes `agm
override authorize`, which validates the root-owned grant and appends the same
ledger as in-process callers. It refuses via Codex's `permissionDecision:
"deny"` wire form; a non-zero hook exit is **not** a refusal in Codex.

For the Codex hook-trust kind, authorization composes with attestation
(`agm/internal/codexhooks`). They answer different questions — whether the
hooks are still the reviewed ones, and whether anyone currently agrees to run
them unreviewed — and both fail closed.

## Operations

    agm override approve <kind> --ttl 1h
    agm override status                    # approvals and recent use
    agm override audit --window 168h --threshold 5
    agm override revoke <kind>

Approvals are stored as `/etc/dear-agent-override-<kind>.json`, owned by root
and not writable by group or others. On macOS, `agm override approve` streams
the confirmed bytes to the system `authopen` authorization service; on other
Unix systems it streams them across the system `sudo` boundary. AGM itself is
never elevated, so an agent-writable AGM binary is not executed as root. The
OS authentication and the command's interactive typed confirmation form the
human boundary. Uses append through the same fixed system authorization helper
to `/var/log/dear-agent-overrides.jsonl`. The file and its parent are
root-owned, so the scheduled user agent can read the audit while the agent user
cannot truncate, replace, or remove prior records. AGM synchronizes the
privileged append before permitting the requested launch.

The recurring audit ships as `deploy/launchd/com.dear-agent.override-audit.plist`
and is staged with `make install-override-audit-launchagent`.

## BDD Traceability

- `agm/test/bdd/features/dangerous_override_governance.feature`

## Test Traceability

- `pkg/override/override_test.go`
- `agm/internal/ops/session_resume_test.go` (OVR-07)
- `agm/cmd/agm/new_session_prompt_test.go`
