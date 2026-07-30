# Dangerous Override Specification

<!-- Last audited at: 2026-07-29 -->

## Purpose

`pkg/override` is the shared contract for every unattended launch escape hatch
that switches off a launch-time safety control. Three exist today — the Codex
hook-trust bypass, the admission-brake override, and the supervisor OAuth-check
bypass — and all reach the same failure mode if left ungoverned: an unattended
agent flips one to get past a blocker, nobody notices, and the control is
silently dead.

Rather than three bespoke implementations, all three travel one pattern:
**stated reason, human approval, ledger, recurring audit**.

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

**OVR-14** When an admission-brake override reaches the executable boundary, the system shall reserve current human authorization without recording a use, repeat every live admission gate, and commit the one-shot ledger use only when the brake remains the sole refusal at the final harness-specific launch boundary: immediately before parent-side process or tmux submission, or in the private Codex executor after all fallible preparation and immediately before exec.

**OVR-15** When the privileged ledger helper is installed, the system shall require the operator to confirm the complete artifact SHA-256, copy those bytes to a unique root-owned staging file, verify the staged digest against the confirmed digest, and atomically activate only the verified staged file.

**OVR-16** When the macOS system audit is installed, the system shall require the operator to confirm the executable and rendered LaunchDaemon SHA-256 values, copy both artifacts into unique root-owned staging, verify both staged digests, validate the exact staged plist, and atomically activate only those verified bytes.

**OVR-17** When the Linux system audit is installed, the system shall require the operator to confirm the executable, service, and timer SHA-256 values, copy all three artifacts into unique root-owned staging, verify every staged digest, and atomically activate only those verified bytes before reloading the system manager.

**OVR-18** When a system-scheduled override audit runs, the system shall load default-only AGM configuration from the fixed OS null device and use a root-owned non-writable home instead of consulting mutable user configuration.

**OVR-19** When `agm supervisor run --skip-oauth-check` reaches the executable boundary, the system shall require the shared supervisor OAuth-check override kind, reserve current human authorization without recording a use, repeat final live admission, and commit the privileged ledger use immediately before launching Claude.

**OVR-20** When AGM exposes override-management commands, the system shall not expose a standalone authorization command that records or consumes a use independently of the launch boundary governed by that override.

**OVR-21** When AGM resolves the operator grant store on macOS, the system shall use the canonical `/private/etc` directory so the validator can reject untrusted symlinks without rejecting the operating system's `/etc` symlink.

**OVR-22** When one launch crosses more than one dangerous override, the system shall revalidate every reservation and every per-kind rate limit before recording any use, then append the complete set under one ledger lock and one write so a failed combined authorization records none of its uses.

**OVR-23** When a human approves `codex-hook-trust`, the system shall bind the root-owned grant to the canonical repository, full source commit, and committed hook-byte digest displayed during approval, and shall refuse a generic or mismatched hook-trust grant.

**OVR-24** When a Codex launch crosses hook trust together with another dangerous override, the system shall seal every prepared exact authorization claim into the same private handoff, require the executor to revalidate the persisted repository, commit, digest, materialized hook root, sandbox assets, current exact grants, and per-kind limits, re-reserve every claim with a fresh authorization ID, and append the complete ledger transaction only after every other fallible launch check and immediately before executing Codex.

**OVR-25** When AGM installs or invokes the Unix privileged ledger helper, the system shall bind its digest-qualified NOPASSWD rule and root-owned caller policy to the exact installed AGM code identity, carry the live launcher PID in the canonical append request, require that PID to be the helper's first non-sudo ancestor, and authenticate its running identity before appending.

**OVR-26** When final launch admission succeeds, the system shall record the circuit-breaker spawn timestamp only after every launch-bound override reservation has been committed or sealed successfully and immediately before process or tmux submission.

**OVR-27** When the private Codex executor receives a deferred admission-brake claim, the system shall treat the claim and its authorization ID as non-authoritative, repeat every live circuit-breaker gate before and after reserving current human authorization, commit a fresh admission-brake reservation only when the brake remains the sole refusal, and reject the launch when any other gate refuses.

## Override kinds

| Kind | Disables | Requested by |
|---|---|---|
| `codex-hook-trust` | Codex per-path hook trust review | `sandbox.bypass_codex_hook_trust_reason`, or `agm session new --dangerously-bypass-hook-trust="<reason>"` |
| `admission-brake` | The watchdog admission brake | `agm session new --brake-override="<reason>"` |
| `supervisor-oauth-check` | The supervisor Claude OAuth-token presence check | `agm supervisor run --skip-oauth-check --reason "<reason>"` |

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

This privileged contract is deliberately narrower than command-scoped
confirmation for interactive lifecycle operations. Existing-session
`--force` actions that do not authorize a new unattended harness process remain
under `internal/override`'s reason, confirmation, and user audit contract; they
are not launch override kinds and cannot satisfy this package's grant gate.

For the Codex hook-trust kind, authorization composes with attestation
(`agm/internal/codexhooks`). They answer different questions — whether the
hooks are still the reviewed ones, and whether anyone currently agrees to run
them unreviewed — and both fail closed.

## Operations

    agm override approve codex-hook-trust --codex-hook-source /reviewed/repo --ttl 1h
    agm override approve <other-kind> --ttl 1h
    agm override status                    # approvals and recent use
    agm override audit --window 168h --threshold 5
    agm override revoke <kind>

Approvals are stored as `/etc/dear-agent-override-<kind>.json` on other Unix
platforms and
`/private/etc/dear-agent-override-<kind>.json` on macOS (the canonical target
of the system `/etc` symlink), owned by root and not writable by group or
others. On macOS, `agm override approve` streams
the confirmed bytes to the system `authopen` authorization service; on other
Unix systems it streams them across the system `sudo` boundary. AGM itself is
never elevated, so an agent-writable AGM binary is not executed as root. The
Unix path invalidates any sudo timestamp, rejects passwordless validation,
requires a fresh password challenge, and invalidates the new timestamp after
installation. That OS authentication and the command's interactive typed
confirmation form the human boundary.

Hook-trust approval additionally resolves the source repository's canonical
path and current full commit, reads the manifest and referenced hook files from
immutable Git objects, displays that identity and committed-byte digest, and
stores all three in the root-owned grant. Each launch derives a compact subject
from the same fields. Both the unprivileged reservation gate and privileged
append helper require an exact subject match, so a later commit or a different
repository cannot reuse the approval.

On Unix, the operator first runs
`make install-override-ledger-helper`. That separately authenticated setup
installs the matching AGM build, prints and requires typed confirmation of the
helper artifact's complete SHA-256 and installed AGM code identity, copies the
approved helper bytes and caller policy into unique root-owned staging files,
and atomically activates each only on an exact match. It then installs the
one-purpose root-owned
`/usr/local/libexec/dear-agent-override-ledger-append` binary and an exact
per-user, helper-digest-qualified NOPASSWD sudoers rule for that path. Runtime
authorization invokes it with `sudo -n`: no fresh prompt or cached sudo
credential is needed, and AGM, `tee`, `chmod`, arbitrary arguments, and
arbitrary paths are never privileged.
The helper accepts exactly one canonical bounded launcher request on stdin.
Its embedded transaction is either one historical-compatible use record or one
envelope containing at most one use per override kind. The helper appends only to
`/var/log/dear-agent-overrides.jsonl`, revalidates every matching active
root-owned grant, exact subject, unique random authorization ID, and near-current
timestamp, synchronizes before returning, and stops at a 16 MiB ledger cap
pending operator-owned rotation. The one-shot private Codex handoff carries
prepared authorization claims and immutable source fields, but those claim IDs
are audit correlation rather than secret capabilities. After re-running the
persisted Git attestation and all other fallible launch checks, its executor
repeats the live admission gates, re-reserves each current grant with a fresh
random ID, and appends the complete transaction immediately before replacing
AGM with Codex.

The installer also installs the matching AGM build, displays its
kernel-verifiable code identity for separate typed confirmation, and records
that identity in a root-owned policy next to the helper. The canonical
privileged request binds the transaction to the live AGM PID that reached the
launch boundary. The helper permits only root-owned `/usr/bin/sudo` processes
between itself and that exact PID and requires the running macOS code-directory
hash or Linux executable SHA-256 to match the policy. Direct shell calls,
descendant agent shells under a persistent supervisor, unrelated PIDs, and
changed AGM bytes therefore fail closed. Updating AGM requires reinstalling the
helper policy before another privileged append can succeed.

The ledger file and its parent are root-owned, so the scheduled audit process
can read the audit while the agent user cannot truncate, replace, or remove
prior records. AGM also validates the privileged result before permitting the
requested launch.

On macOS, `make install-override-audit-launchdaemon` uses fresh operator
authentication to install both
`deploy/launchd/com.dear-agent.override-audit.plist` and a reviewed AGM copy
into root-owned system locations. The system LaunchDaemon cannot be disabled
through the same-user GUI domain and runs the fixed audit command as the named
unprivileged operator. It pins `--config /dev/null` and a root-owned empty home,
so malformed or blocking mutable user configuration cannot blind the audit
before it reads the operator-owned ledger. Its `--notify` mode delivers
threshold breaches to the unified system log even when Notification Center is
unavailable. On Linux,
`make install-override-audit-systemd` uses the same operator boundary to
install a root-owned audit executable and system-manager templates; the service
runs as the named unprivileged operator with `/` as its fixed home and working
directory, so an unattended same-user agent cannot disable its timer through
`systemctl --user` or redirect it through mutable user configuration.

## BDD Traceability

- `agm/test/bdd/features/dangerous_override_governance.feature`

## Test Traceability

- `pkg/override/override_test.go`
- `agm/internal/ops/session_resume_test.go` (OVR-07)
- `agm/cmd/agm/new_session_prompt_test.go`
