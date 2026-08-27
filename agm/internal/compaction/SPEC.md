# AGM Compaction Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/compaction` generates identity-preserving compact prompts,
records compaction history, and prevents repeated compaction loops. It also
loads session-specific AGM state to build preserve prompts without borrowing
identity from another session. It owns strict delivery readiness and the
positive-evidence protocol that distinguishes a sent request from verified
completion.

## Requirements

**AGM-COMPACTION-01** When a session state file is loaded, the system shall read only `<session>-state.json` from the configured base directory and shall use its contents only when its embedded `session_id` exactly matches the stable session ID resolved by AGM.

**AGM-COMPACTION-02** When no matching session state file exists, the system shall return an error naming the session and attempted path instead of falling back to another state file.

**AGM-COMPACTION-03** When a preserve prompt is generated, the system shall use the target session name as identity and fall back to `worker` only when the target name is empty.

**AGM-COMPACTION-04** When state includes scan-loop interval, policy rules, managed sessions, completed count, queued items, or focus text, the system shall include those values in the preserve prompt.

**AGM-COMPACTION-05** When a generic compaction prompt is generated without any metadata, the system shall return `/compact`.

**AGM-COMPACTION-06** When a generic compaction prompt includes session metadata, the system shall format the metadata as structured preservation bullets.

**AGM-COMPACTION-07** When prompt audit files are saved, the system shall create or tighten `compaction-prompts` to owner-only directory permissions, durably publish that metadata, and write prompt files with owner-only file permissions.

**AGM-COMPACTION-08** When a compaction invocation can deliver a request and the expected harness has not been proven live-ready on the exact target pane, the system shall reject the invocation before delivery and return an explanatory error.

**AGM-COMPACTION-09** When anti-loop checks are not forced, the system shall block compaction during the cooldown window or after the maximum compactions in the rolling window.

**AGM-COMPACTION-10** When governed force is requested for a target with positive live-ready evidence, the system shall durably persist the complete owner-only override verdict before bypassing only the cooldown and maximum-count limits, deny delivery when that audit cannot be proven durable, and never weaken delivery or completion verification.

**AGM-COMPACTION-11** When a registered session passes compaction policy and is about to receive a compaction request, the system shall durably record one pending attempt under the stable session ID before attempting input delivery.

**AGM-COMPACTION-12** When compaction request delivery and its terminal accounting are both confirmed while completion verification is disabled, the system shall report the request as sent without reporting completion.

**AGM-COMPACTION-13** When compaction completion verification is requested, the system shall report completion only after the exact delivered pane and foreground harness process are positively observed to enter a post-delivery native processing state and subsequently return to stable live-ready before the verification-owned deadline.

**AGM-COMPACTION-14** When compaction completion verification observes a blocked, terminal, absent, unreadable, unrecognized, or failed session before completion is proven, the system shall return a non-successful unverified outcome without reporting completion.

**AGM-COMPACTION-15** When compaction completion verification reaches its configured timeout before completion is proven, the system shall return a non-successful unverified outcome without reporting completion.

**AGM-COMPACTION-16** When a registered session's compaction prompt audit is allocated, the system shall key the audit file by stable session ID and create it exclusively with `O_EXCL` semantics so an existing prompt is never truncated or overwritten.

**AGM-COMPACTION-17** When a pending compaction attempt reaches terminal delivery accounting, the system shall persist exactly one `confirmed`, `uncertain`, or `definite_not_sent` outcome and reject a contradictory later terminal outcome.

**AGM-COMPACTION-18** Where compaction anti-loop accounting applies, the system shall count structurally valid pending, confirmed, uncertain, and legacy attempts conservatively, reject malformed or unknown ledger outcomes without authorizing another attempt, and release budget early only for an explicitly persisted `definite_not_sent` outcome.

**AGM-COMPACTION-19** When a legacy display-name-keyed compaction ledger has no embedded stable session ID, the system shall reject the ledger as ambiguous instead of migrating it or authorizing another attempt.

**AGM-COMPACTION-20** When exact paste has mutated the composer or input submission acknowledgement is lost after the irreversible delivery boundary, the system shall return a non-successful uncertain outcome with the exact pane ID, pane-root PID, tmux session ID, stable binding, foreground harness PID, and harness process birth time, preserve `MayHaveStarted`, and prohibit automatic retry or completion wording.

**AGM-COMPACTION-21** When input delivery is confirmed but terminal compaction accounting cannot be persisted, the system shall return a non-successful accounting-incomplete outcome that prohibits automatic retry and completion wording.

**AGM-COMPACTION-22** When compaction verification evaluates whether the delivered harness became active, the system shall arm completion proof only from positively recognized current-tail native `PROCESSING`; a human draft, generic busy composer, queued AGM input, unsupported harness output, historical text, or unrecognized content shall remain non-positive evidence.

**AGM-COMPACTION-23** When a generated compaction command contains embedded newlines, the system shall deliver it as one raw bracketed composer paste followed by one verified submit action instead of allowing embedded newlines to become separate submit keys.

**AGM-COMPACTION-24** When a generated compaction command is invalid UTF-8 or contains any terminal control other than line feed, the system shall reject it before audit allocation, dry-run output, resolution, or delivery instead of emitting or submitting terminal control input.

**AGM-COMPACTION-25** When the first post-delivery observation is already live-ready, or a new active observation interrupts the one-ready stabilization phase, the system shall stop unverified with lost causal attribution instead of waiting for a later unrelated work cycle to satisfy completion proof.

**AGM-COMPACTION-26** When a display-name-keyed state file omits `session_id`, carries a different stable session ID, or AGM has no stable target ID, the system shall reject that state as ambiguous instead of using an archived same-name session's policy, queue, managed-session, or scan-loop context.

**AGM-COMPACTION-27** When strict delivery positively observes native `PROCESSING` during its exact post-submit reproof, the system shall carry that same-pane, same-process evidence into completion verification as the delivered request's initial transition; when no such seed exists, an initial verifier observation of `READY` shall remain causally unverified.

**AGM-COMPACTION-28** When a prompt audit, attempt ledger, or legacy-ledger migration reports durable success, the system shall have synced file contents and final permissions before publication and synced the containing directory after every creation, replacement, claim rename, or claim removal needed for that reported state; any failed durability step shall fail closed instead of authorizing delivery from an unproven record.

**AGM-COMPACTION-29** When strict compaction delivery is attempted, the system shall condition its uniquely buffered paste and every Enter on the same stable-bound tmux pane incarnation and zero attached clients, require a structural exact-command occurrence created after the complete pre-paste history baseline to leave its parked shape, and then accept only live-ready or native-processing continuity on the unchanged foreground harness PID and process birth time. Generic busy, a prior identical command echo, mere absence, an empty composer, or arbitrary output after an exact-command prefix is not submission proof. Prompt-like payload lines, partial or appended human drafts, attached operators, concurrent clears, bounded-tail truncation, PID reuse, pane replacement, server restart, and ambiguous captures shall never produce confirmed delivery or an additional Enter.

### Open terminal-input boundary

The zero-attached-client condition in AGM-COMPACTION-29 does not provide an
exclusive composer lease. A detached external `tmux send-keys` writer, or an
operator who types and detaches between readiness and paste, can change composer
bytes without changing the conditioned identities; the foreground harness can
also exit or restart because tmux's queued condition cannot bind its child PID
and birth marker. Until a harness-native input transaction or equivalent
mutation-time content and process authority exists, strict terminal delivery
retains a risk of appending to and submitting foreign input or targeting a
different foreground process. The `ps lstart` marker also has one-second
resolution on macOS, leaving a same-PID same-second reuse collision. These
limitations must remain explicit in release and completion claims.

## Applicability and terms

- AGM-COMPACTION-08 through AGM-COMPACTION-29 apply to both delivery-capable
  command surfaces. Both surfaces require an AGM-registered session with a
  durable ID; a raw tmux target is intentionally unsupported.
- `agm send compact` verifies only with `--verify`; `agm session compact`
  verifies by default and becomes delivery-only with `--monitor=false`.
- Governed force applies only on a surface that exposes it. It bypasses
  AGM-COMPACTION-09 policy limits but no readiness, delivery-accounting, or
  completion-proof requirement.
- Dry-run invocations cannot deliver and therefore do not establish either
  delivery or completion evidence.
- Positive live-ready means the configured harness process owns the foreground
  terminal and an empty composer on the exact pane selected for delivery.
  Stable live-ready means two consecutive such observations after a positively
  recognized native `PROCESSING` observation on that same pane and process
  identity.
- Native `PROCESSING` is a current-tail Codex, Claude, or managed Pi active-work
  signal, not a compatibility `WORKING` label. Verification applies only when
  the observer can positively establish that transition and later stable
  live-ready evidence before the owned deadline. Caller cancellation, pane or
  process replacement, unsupported harness observation, and observer failure
  all fail closed.
- Delivery is `uncertain` when input may have crossed the irreversible
  submission boundary but acknowledgement was lost. Pending and uncertain
  attempts count conservatively through the ordinary cooldown and rolling
  24-hour policy windows; only `definite_not_sent` releases budget early.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_runtime_package_guardrails.feature` (AGM-COMPACTION-08 through AGM-COMPACTION-28 and co-located SPEC coverage)
