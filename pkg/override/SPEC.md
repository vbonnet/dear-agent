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

**OVR-03** When a human approval is minted, the system shall require an interactive terminal, a typed confirmation naming the override kind, and fresh OS authentication that cannot be satisfied by a cached or passwordless Unix sudo rule.

**OVR-04** When a human approval exists for one override kind, the system shall not treat it as approval for any other override kind.

**OVR-05** When an override is authorized, the system shall durably append a ledger record containing the kind, reason, actor, session, and timestamp through operator-owned storage that the agent user cannot truncate, replace, or remove.

**OVR-06** When the ledger cannot be read or appended, the system shall refuse the override.

**OVR-07** When a session that launched under an override is resumed, the system shall re-authorize the override instead of inheriting it.

**OVR-08** When the override audit runs, the system shall group recorded uses by kind and by reason and rank repeated reasons first.

**OVR-09** When any single override kind reaches the alert threshold within the audit window, the system shall exit with code 3.

**OVR-10** When override use is aggregated, the system shall evaluate the alert threshold separately for each override kind.

**OVR-11** When AGM's private Codex executor receives a hook-trust bypass, the system shall require a one-shot prepared handoff that binds the exact attested hook root and complete launch request and lives outside the workspace and every agent-writable root.

**OVR-12** When the scheduled override audit reaches a threshold, the system shall deliver the breach through Notification Center or the system log before exiting with the reserved breach status.

**OVR-13** When an override record crosses a privileged append boundary, the system shall reject oversized reasons, attribution fields, records, and ledger growth before writing.

**OVR-14** When an admission-brake override reaches the executable boundary, the system shall reserve current human authorization without recording a use, repeat every live admission gate, and commit the one-shot ledger use only when the brake remains the sole refusal immediately before the irreversible process or tmux submission boundary.

**OVR-15** When the privileged ledger helper is installed, the system shall require the operator to confirm the complete artifact SHA-256, copy those bytes to a unique root-owned staging file, verify the staged digest against the confirmed digest, and atomically activate only the verified staged file.

**OVR-16** When the macOS system audit is installed, the system shall require the operator to confirm the executable and rendered LaunchDaemon SHA-256 values, copy both artifacts into unique root-owned staging, verify both staged digests, validate the exact staged plist, and atomically activate only those verified bytes.

**OVR-17** When the Linux system audit is installed, the system shall require the operator to confirm the executable, service, and timer SHA-256 values, copy all three artifacts into unique root-owned staging, verify every staged digest, and atomically activate only those verified bytes before reloading the system manager.

## Override kinds

| Kind | Disables | Requested by |
|---|---|---|
| `codex-hook-trust` | Codex per-path hook trust review | `sandbox.bypass_codex_hook_trust_reason`, or `agm session new --dangerously-bypass-hook-trust="<reason>"` |
| `admission-brake` | The watchdog admission brake | `agm session new --brake-override="<reason>"` |

## Enforcement surfaces

Authorization is enforced **in the binary**, at every launch and resume path.
It does not depend on a harness hook firing, because a hook that silently fails
to run is exactly how a guardrail becomes decorative (see
`.codex/hooks/SPEC.md` and the `${CLAUDE_PROJECT_DIR}` path defect).

This boundary governs AGM-owned create and resume paths. A repository
PreToolUse hook cannot mediate descendant process execution (for example, a
script that launches an external Codex binary), so it must not be represented
as enforcement for raw external launches. Operators that must prohibit direct
Codex execution need an operator-owned executable policy outside this
repository; AGM does not substitute command-text scanning for that boundary.

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
Unix path invalidates any sudo timestamp, rejects passwordless validation,
requires a fresh password challenge, and invalidates the new timestamp after
installation. That OS authentication and the command's interactive typed
confirmation form the human boundary.

On Unix, the operator first runs
`make install-override-ledger-helper`. That separately authenticated setup
prints and requires typed confirmation of the helper artifact's complete
SHA-256, copies the approved bytes into a unique root-owned staging file,
recomputes the staged digest, and atomically activates it only on an exact
match. It then installs the one-purpose root-owned
`/usr/local/libexec/dear-agent-override-ledger-append` binary and an exact
per-user NOPASSWD sudoers rule for that path. Runtime authorization invokes it
with `sudo -n`: no fresh prompt or cached sudo credential is needed, and AGM,
`tee`, `chmod`, arbitrary arguments, and arbitrary paths are never privileged.
The helper accepts exactly one canonical bounded JSONL record on stdin, appends
only to `/var/log/dear-agent-overrides.jsonl`, revalidates the matching active
root-owned grant and a near-current timestamp, synchronizes before returning,
and stops at a 16 MiB ledger cap pending operator-owned rotation.

The ledger file and its parent are root-owned, so the scheduled audit process
can read the audit while the agent user cannot truncate, replace, or remove
prior records. AGM also validates the privileged result before permitting the
requested launch.

On macOS, `make install-override-audit-launchdaemon` uses fresh operator
authentication to install both
`deploy/launchd/com.dear-agent.override-audit.plist` and a reviewed AGM copy
into root-owned system locations. The system LaunchDaemon cannot be disabled
through the same-user GUI domain and runs the fixed audit command as the named
unprivileged operator. Its `--notify` mode delivers threshold breaches to the
unified system log even when Notification Center is unavailable. On Linux,
`make install-override-audit-systemd` uses the same operator boundary to
install a root-owned audit executable and system-manager templates; the service
runs as the named unprivileged operator, but an unattended same-user agent
cannot disable its timer through `systemctl --user`.

## BDD Traceability

- `agm/test/bdd/features/dangerous_override_governance.feature`

## Test Traceability

- `pkg/override/override_test.go`
- `agm/internal/ops/session_resume_test.go` (OVR-07)
- `agm/cmd/agm/new_session_prompt_test.go`
