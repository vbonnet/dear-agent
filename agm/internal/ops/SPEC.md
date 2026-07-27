# agm/internal/ops — Requirements Specification (EARS)

<!-- Last audited at: 2026-07-27 -->

**Version**: 1.0
**Last Updated**: 2026-07-27
**Status**: Baseline (derived from tests + code, not design-forward)
**Scope**: Shared business-logic layer for AGM CLI, MCP, and Skills surfaces

---

## Overview

`agm/internal/ops` is the shared implementation layer behind all three AGM API
surfaces (CLI, MCP server, Skills plugin). Every surface constructs an
`OpContext` and delegates business lifecycle ordering here. Surface adapters
may still construct storage dependencies and implement interactive tmux
readiness or completion through the cohesive `CreateSessionRuntime` seam.

---

## EARS Requirements

### Session Listing

**OPS-01** When `ListSessions` is called with no filter, the system shall return up to 100 sessions sorted by last-updated descending, each annotated with a live tmux status ("active" or "stopped").

**OPS-02** When `ListSessions` is called with `limit > 1000`, the system shall return an `OpError` with code `invalid_input` and reject the request without querying storage.

**OPS-03** When `ListSessions` is called with `status = "active"`, the system shall exclude sessions that have no live tmux pane from the result set.

**OPS-04** When `ListSessions` is called with `status = "archived"`, the system shall return only sessions whose `Lifecycle` field equals `archived`.

**OPS-05** When `ListSessions` is called with an unrecognised `status` value, the system shall return an `OpError` with code `invalid_input`.

**OPS-06** When a tmux session exists that has no corresponding AGM manifest, the system shall surface it as an orphan entry in the list response.

**OPS-07** When a session's `LastKnownCost` is zero, the system shall compute an estimated cost from token counts using the Opus pricing schedule (input: $15/MTok, output: $75/MTok) before returning the summary.

### Session Resolution

**OPS-08** When `GetSession` is called with an exact session ID, the system shall return the matching session detail without performing a name scan.

**OPS-09** When `GetSession` is called with a string that does not exactly match any session ID, the system shall perform a full-table name scan comparing `manifest.Name` and `manifest.Tmux.SessionName`.

**OPS-10** When a session has no tmux interface, the system shall report its status as "unknown" rather than "active" or "stopped".

**OPS-11** When a session's `Lifecycle` is `archived`, the system shall report its status as "archived" regardless of tmux state.

### Session Kill

**OPS-12** When `KillSession` is called on a session that has a live tmux pane and `ConfirmedStuck` is false, the system shall return `ErrActiveSessionKill` and not execute the kill.

**OPS-13** When `KillSession` is called on a session that was active within the recency threshold and `Force` is false, the system shall return `ErrKillProtected` and not execute the kill.

**OPS-14** When `KillSession` is called with `DryRun = true`, the system shall return the would-be result without mutating tmux or storage.

**OPS-15** When `KillSession` is called on an already-archived session, the system shall return an error indicating the session is not killable.

### Session Archive

**OPS-16** When `ArchiveSession` is called on a session that has a live tmux pane and `Force` is false, the system shall block the archive and return an error identifying the active pane.

**OPS-17** When `ArchiveSession` is called on a session whose working-directory verification finds critical issues and `Force` is false, the system shall block the archive and surface the verification failures.

**OPS-18** When `ArchiveSession` is called on a session whose name matches a supervisor pattern (e.g. "orchestrator", "meta-orchestrator", "overseer") and `Force` is false, the system shall block the archive.

**OPS-19** When `ArchiveSession` succeeds, the system shall in sequence: set `Lifecycle = archived`, update storage, record a trust event, deregister from the monitor, kill MCP processes, kill the tmux process group, and run worktree/sandbox cleanup.

**OPS-20** When `ArchiveSession` is called on a session that is already archived, the system shall return an error without re-running any cleanup step.

**OPS-51** When `ArchiveSession` completes its durable AGM cleanup, the system shall report a harness-neutral external archive outcome without reverting the archived lifecycle state.

**OPS-52** While `ArchiveSession` is executing with `DryRun = true`, the system shall not invoke an external session archive adapter.

**OPS-53** When immediate, bulk, garbage-collection, or asynchronous-reaper archival reaches the durable archive transition, the system shall execute that transition through `ArchiveSession` rather than mutating lifecycle storage in the caller.

**OPS-54** When a bulk archive processes multiple candidates, the system shall preserve per-session `ArchiveSession` guards, outcome stamping, external archive outcomes, and cleanup results while reporting aggregate success and failure counts.

**OPS-55** When an async reaper validates an active session before stopping its pane, the system shall permit that expected active pane only for preflight while preserving supervisor, completion-verification, and pending-delegation guards; the final archive shall enforce pane death again.

**OPS-81** When archive cleanup receives an explicit working directory or falls back to the context project, the system shall resolve and preserve the repository primary checkout and any merely name-matching branch, record the intentional preservation, and continue safe non-worktree cleanup; for linked worktrees, the system shall resolve a surviving primary checkout before removing the linked checkout, use that surviving path for subsequent prune and branch cleanup, and delete a branch only after resolving and successfully removing a worktree whose inventory branch exactly matches that deletion target.

**OPS-56** When `ArchiveSession` uses an isolated SQLite store or archives a manifest marked as a test session, the system shall preserve lifecycle, explicit legacy-directory moves, and injected external-archive behavior without mutating host trust, monitor, process, pending-message, worktree, branch, temporary-file, sandbox, or configuration state.

**OPS-57** When an archive caller supplies a request context, the system shall propagate its cancellation through tracked-worktree storage cleanup instead of replacing it with a background context.

### Claude UI Archive Reconciliation

**OPS-67** When `ArchiveUISessions` evaluates a desktop record whose `cliSessionId` or `sessionId` belongs to a running process, the system shall skip that record regardless of the requested status; working-directory equality shall not be treated as session liveness because multiple historical sessions may share one directory.

**OPS-68** When `ArchiveUISessions` uses the default `idle` status, the system shall mutate only records that differ from the requested archive state, have no matching live session identity, and are older than the configured threshold.

**OPS-69** When Claude UI storage has an unknown schema or ambiguous device or account selection, the system shall refuse or skip the affected record without rewriting it.

### Garbage Collection

**OPS-21** When `GC` is called, the system shall perform a pre-flight health check by listing all sessions; if storage is unreachable it shall abort with a 503-equivalent error before touching any session.

**OPS-22** When `GC` evaluates a session, the system shall skip it with reason `GCSkipActiveTmux` if the session has a live tmux pane.

**OPS-23** When `GC` evaluates a session, the system shall skip it with reason `GCSkipActiveState` if the session manifest state is any of: WORKING, PERMISSION_PROMPT, COMPACTING, WAITING_AGENT, LOOPING, BACKGROUND_TASKS, USER_PROMPT, READY.

**OPS-24** When `GC` evaluates a session, the system shall skip it with reason `GCSkipProtectedRole` if the session name contains any protected role substring (default: "orchestrator", "meta-orchestrator", "overseer"; case-insensitive).

**OPS-25** When `GC` is called with `OlderThan` set, the system shall skip any session whose `max(UpdatedAt, StateUpdatedAt)` is more recent than the threshold, with reason `GCSkipTooRecent`.

**OPS-26** When `GC` is called with `DryRun = true`, the system shall record GC intent in the log entries but not mutate storage or call `ArchiveSession`.

**OPS-27** When `GC` archives a session, the system shall write an entry to `gc.jsonl` via the gclog subsystem for every action taken (skip or archive).

### Message Delivery

**OPS-28** When `SendMessage` is called with an empty recipient, the system shall return a validation error without attempting delivery.

**OPS-29** When `SendMessage` is called with an empty message body, the system shall return a validation error without attempting delivery.

**OPS-30** When `SendMessage` is called targeting an archived session, the system shall return an error before attempting delivery.

**OPS-31** When `SendMessage` targets a local interactive session, the system shall require the injected tmux runtime's atomic input capability and shall combine harness-aware readiness, target ownership, and exact-pane delivery in one mutation boundary without a parallel manager fallback.

**OPS-82** When `SendMessage` resolves an `openai` or `gpt` manifest, the system shall route before every configured tmux capability through the shared stable-session-ID API transaction, reload lifecycle under the archive-compatible lock, reconstruct the adapter from persisted non-secret runtime configuration, accept only active or idle adapter status, and require context-aware provider delivery; CLI, MCP, and other callers shall therefore neither inspect nor send to a tmux pane for a pure API session.

**OPS-83** When CLI, MCP, or daemon code attempts direct message delivery, the system shall use `SendMessage` as the operation that resolves the recipient, selects API or tmux transport, decides readiness, and performs the send.

**OPS-84** When direct delivery succeeds, the `SendMessage` operation shall return the stable session ID used by the transaction so callers can perform post-delivery bookkeeping without resolving a mutable name again.

**OPS-85** When `SendMessage` cannot deliver because the recipient is not ready, the system shall return `AGM-016` with the authoritative readiness state.

**OPS-86** When `SendMessage` returns a typed not-ready outcome, the operation shall not have sent input.

**OPS-87** The CLI and daemon direct-delivery paths shall not call `session.CheckSessionDelivery`.

**OPS-88** The daemon direct-delivery path shall not own a separate tmux sender.

**OPS-89** When `SendMessage` returns an outcome, the CLI and daemon adapters shall translate it into presentation, queue, defer, retry, and acknowledgment policy.

**OPS-90** When `SendMessage` resolves a tmux-backed session, the operation shall lock its stable session ID, reload mutable lifecycle and delivery identity, and retain that lifecycle boundary across atomic readiness and exact-pane input.

### Stall Detection

**OPS-32** When a session has been in `PERMISSION_PROMPT` state for longer than `PermissionTimeout` (default 5 minutes), the stall detector shall classify it as a critical stall.

**OPS-33** When a session named or tagged as "worker" has been in `WORKING` state for longer than `NoCommitTimeout` (default 15 minutes) with zero git commits since `StateUpdatedAt`, the stall detector shall classify it as a warning stall.

**OPS-34** When the tmux pane output of a session contains any error pattern repeated `ErrorRepeatThreshold` or more times, the stall detector shall classify it as a warning stall.

**OPS-35** When a stall is detected, the system shall record it via `recordErrorMemory` and publish a stall event to the `eventbus.Broadcaster`

### Codex Session Creation

**OPS-40** When `CreateSession` creates a `codex-cli` session, the system shall attempt to create a Codex remote-control thread before sending the harness launch command to tmux.

**OPS-41** When Codex remote-control thread creation succeeds for a `codex-cli` session, the system shall persist the returned Codex thread id in the session manifest and launch Codex by resuming that thread with `codex resume --remote unix://`.

**OPS-42** When Codex remote-control thread creation fails and `AGM_CODEX_REQUIRE_REMOTE_CONTROL` is set to `1`, the system shall fail session creation instead of silently starting an untracked Codex thread.

**OPS-48** When `CreateSession` creates a `codex-cli` session, the system shall record the session's working directory as a trusted Codex project in `$CODEX_HOME/config.toml` before creating the Codex thread or sending the launch command, so a fresh non-git sandbox directory cannot block Codex startup on its interactive trust prompt (ce-cmsq); if the pre-trust write fails, the system shall warn and still attempt the launch.

### Shared Session Creation Lifecycle

**OPS-58** When a CLI creation surface, including `session create-child` and current-tmux creation, or the MCP surface creates a session, the surface shall delegate tmux creation, optional Codex remote setup, launch-command construction, manifest registration, completion ordering, and rollback to `CreateSessionWithContext`; child creation shall carry its parent relationship, selected context, and explicit initial prompt through the shared creation request, and shared validation shall reject every normalized AGY child without that identity-creating prompt before mutation.

**OPS-59** When `CreateSessionWithContext` advances a new session, the system shall order the durable lifecycle as tmux creation, bounded Codex setup when applicable, runtime launch, manifest registration, and runtime completion.

**OPS-60** When a creation request declares a caller surface, the system shall return that caller as result provenance and persist a matching `source:<caller>` manifest tag.

**OPS-61** If any creation step fails after a new tmux session is created, the system shall remove the newly-created tmux session, delete any completed session registration, and remove only the manifest directory created by that operation.

**OPS-62** If creation reuses an existing tmux session or manifest directory, the system shall preserve those pre-existing artifacts during rollback.

**OPS-63** When optional Codex remote setup is attempted, the system shall apply a finite deadline to the complete remote-control setup sequence and shall honor cancellation from the calling surface.

**OPS-64** When any creation adapter or fresh-session resume fallback builds a harness command, the system shall use `BuildHarnessLaunchCommand` rather than assemble a surface-specific command variant.

**OPS-65** When create-session rollback cannot delete a registration, stop a newly-created tmux session, or remove its newly-created manifest directory, the system shall report the cleanup failure instead of silently discarding it.

**OPS-66** When an optional manifest directory cannot be created, the system shall continue without registration and shall provide no manifest path to runtime completion.

**OPS-77** When AGM creates an AGY session, the shared launch-command owner shall preserve the selected model, permission mode, work directory, additional directories, and persistence policy while using AGY's native bare interactive entry point.

**OPS-78** When AGM cold-resumes an AGY conversation, the shared AGY command owner shall preserve a known stored model and permission mode, include the canonical conversation ID, and apply the same quoting, directory, and persistence policy used by creation; if the native model is unknown, the command shall omit `--model` so AGY retains the conversation's saved selection.

**OPS-79** When a creation request is canceled after registration but before runtime completion, the shared lifecycle shall skip completion and enter rollback before a startup prompt or other completion side effect can run.

**OPS-80** When any shared creation surface launches AGY, the system shall use `CreateSessionWithContext` to resolve the existing workspace to one canonical physical path for locking, tmux creation, launch, identity correlation, registration, and persisted metadata while holding the workspace-create lock across the fail-closed pre-launch snapshot through registration and releasing it before surface-specific completion, including any blocking interactive attach, without surrendering normal rollback ownership.

**OPS-81** When `ArchiveSession` resolves a session, the system shall lock its immutable session ID, reload mutable lifecycle state under that lock, and serialize the archive mutation with delivery; for a pure API session, archive and delivery shall use a provider-appropriate bounded lock wait that exceeds the ordinary lifecycle wait while honoring caller cancellation, so either an in-flight completed turn commits before archive or delivery observes archive before provider work.

**OPS-82** When `ArchiveSession` reloads sandbox ownership metadata, the system shall authorize sandbox cleanup only for a complete valid record whose ID matches the stable session ID and whose merged boundary is exactly the current host sandbox base's identified `merged` child; incomplete, mismatched, legacy, or out-of-base metadata shall preserve every sandbox path.

### Shared Session Resume Lifecycle

**OPS-83** When `ResumeSession` receives a stable session ID, the operation shall acquire that ID's lifecycle lock before its first mutable storage read, reload the current session under the lock, classify worktree and tmux health before mutation, and reject archived, unhealthy, or unverifiable targets without creating or commanding a tmux session.

**OPS-84** When `ResumeSession` cold-starts a harness, the operation shall create one exact tmux identity, build the native resume command from persisted harness metadata, restore the current harness default when a legacy Codex session has no model, wait for the harness-specific process and composer boundary, and persist the canonical tmux name only after readiness; when the expected runtime already exists, the operation shall preserve it and shall not submit another launch command except for a proven restartable Pi shell.

**OPS-85** If cold resume fails before an irreversible prompt boundary, the operation shall restore only the metadata revision owned by that attempt and remove only its exact tmux creation identity; if metadata compensation is rejected or otherwise unproven, it shall preserve the ready runtime rather than leave canonical metadata pointing at a resource it destroyed.

**OPS-86** When optional Codex prompt submission is confirmed or its final acknowledgement is lost, `ResumeSession` shall treat work as possibly started, complete ownership with a cancellation-independent context, and return success with the uncertainty fact; a positive pre-submission failure shall roll back an owned cold runtime, while the same failure against a pre-existing runtime shall warn and preserve attachment behavior.

**OPS-87** When `ResumeSession` cold-resumes AGY, it shall hold the canonical workspace lifecycle lock across command submission and native readiness, preserve a known stored model and permission mode, and omit an unknown or ambiguous legacy model override; when it evaluates an existing Pi pane, it shall require exact configured-harness liveness or a proven restartable shell.

**OPS-88** When `ResumeSession` reports progress, the system shall provide observer callbacks only read-only lifecycle facts, prevent those callbacks from authorizing, skipping, reordering, or aborting operation phases, and permit interactive terminal attachment only in the caller after the operation returns and releases the stable-session lock.

**OPS-89** While `ResumeSession` remains before its irreversible prompt boundary, the system shall honor caller cancellation before each later lifecycle phase and compensate any owned cold runtime; after work may have started, later cancellation shall not convert the operation into a retryable failure.

**OPS-90** When `ResumeSession` reports a successful lifecycle result, the system shall update durable activity only after native readiness and canonical runtime ownership, and shall return the exact stable ID, harness, tmux name, creation and launch facts, health facts, prompt uncertainty, and warnings needed by non-owning surfaces.

**OPS-76** When a fresh AGY session has a startup prompt, `CreateSessionWithContext` shall deliver that prompt once after native readiness but before identity discovery, discover and register the resulting provider identity while retaining the workspace lock, and mark the prompt consumed so CLI, MCP, and fallback completion paths cannot resend it; bootstrap failure or caller cancellation shall roll back the owned tmux session before registration, and a fresh AGY request without a startup prompt shall fail before mutation.

**OPS-77** When shared session creation launches a harness, the system shall prove the expected harness process and harness-specific prompt or composer readiness after launch and before registration, and shall atomically revalidate the exact pane immediately before initial prompt delivery, regardless of whether a surface `CreateSessionRuntime` is present; a runtime may skip the shared startup observation only by explicitly reporting that it already verified both process and composer ownership, an MCP AGY text-only onboarding wait shall remain unverified, and either readiness failure shall enter the existing creation rollback transaction.

**OPS-78** When supported current-tmux creation queues a harness behind the foreground AGM process, the runtime may explicitly defer readiness until the caller exits only for a reused tmux pane with no initial prompt; unsupported harnesses and every other deferred-readiness claim shall fail before registration.

**OPS-79** When `SendMessage` has a configured tmux delivery mechanism, the system shall normalize supported legacy harness names and serialize exact-pane readiness with delivery under one mutation boundary, proving the expected canonical harness process and current composer in the resolved active pane immediately before sending to that same pane ID; queue identity shall use the complete style-preserving logical composer with terminal-wrapped rows joined. For every supported harness, an explicit force request or autonomous-session policy may replace input only when the latest queued-input marker and a syntactically complete generated AGM message header are bound inside that same current composer within the atomic boundary, accepting an optional reply-to value under the same public message-ID contract as `agm send`; visible pasted-text line or character counts shall bind payload-ending whitespace while excluding only capture framing, and terminal idle or managed chrome shall exclude later active output. It shall clear that verified queue without submission and re-prove the expected foreground harness plus an empty composer on the same exact pane before pasting the replacement. A successful recovery shall report `Forced` with the post-clear `YES` state and shall be accepted as delivered so callers cannot retry already-sent input. A failed clear or recheck, changed pane, historical or partial header, opaque Codex pasted-content chip without an observable bound header or exact character extent, post-turn Codex queue without independent idle-cursor proof, human draft, generic busy composer, active work, shell, wrong or dead harness, stale prompt, onboarding or permission prompt, overlay, missing session, missing atomic delivery capability, or unverified readiness shall return typed non-delivery without sending replacement input or Enter.

**OPS-80** If the request is cancelled, or the tmux backend cannot provide or complete a required startup or input-readiness check, the shared create or send operation shall return an error rather than report a successful launch or delivery; CLI and MCP surfaces shall propagate their request context through the shared readiness boundary.

**OPS-36** While a session's state is OFFLINE, READY, or DONE, the stall detector shall skip error-loop detection for that session.

### Field Mask Projection

**OPS-37** When `ApplyFieldMask` is called with a non-empty field list, the system shall return a JSON object containing only the requested top-level keys.

**OPS-38** When `ApplyFieldMask` is called on a value that is not a JSON object, the system shall return the value unchanged.

**OPS-39** When `agm session list` JSON output applies a field mask for per-session row fields (`name`, `status`, `harness`, `workspace`, `tags`, etc.), the system shall preserve the `sessions` envelope and shall not produce `{}` solely because the requested fields are not top-level list result keys.

### Harness-Process Liveness (ce-axsr)

**OPS-43** When `KillSession` evaluates whether a session is active, the system shall require both tmux session existence and a verified harness process in the pane tree; a session whose harness process is provably dead shall be killable without `ConfirmedStuck`.

**OPS-44** When `KillSession` kills a session whose harness process was proven dead, the result shall report the dead-harness verdict, any orphaned agm zombie-writer, and the pane-tree evidence so the caller can say why the session was treated as dead.

**OPS-45** When `KillSession` evaluates the recent-activity protection, the system shall accept either `Force` or `ConfirmedStuck` as sufficient confirmation, so that no combination of two flags is ever required to kill a session.

**OPS-46** When session status is computed and the tmux backend can verify process liveness, a session whose tmux session exists but whose harness process is dead shall report status `zombie` rather than `active`.

**OPS-71** When `KillSession` returns success outside dry-run mode, the shared operation shall have removed the exact resolved tmux session and verified that the target no longer exists.

**OPS-72** If the tmux existence probe, kill command, or post-kill absence check fails, `KillSession` shall return the backend or verification error and shall not report a successful kill.

**OPS-73** When `KillSession` runs in dry-run mode, the system shall evaluate the active-session guard and report the resolved tmux target without invoking the kill mutation.

**OPS-74** When `KillSession` resolves an identifier, the system shall lock the immutable session ID, reload mutable session and tmux identity under that lock, and honor caller cancellation before the irreversible tmux mutation.

**OPS-75** The system shall assign a unique code to every stable RFC 7807 error in the shared operations catalog so programmatic callers can distinguish lifecycle guards and failures without parsing human-readable text.

**OPS-76** When a shared operation wraps a typed backend failure in its stable RFC 7807 error envelope, the in-process error chain shall preserve the original cause for `errors.Is` and `errors.As` while JSON and human-readable output retain the stable operation code and actionable detail.

**OPS-47** When a process-liveness scan fails or the tmux backend cannot verify process liveness, status and kill decisions shall fall back to tmux session existence (fail-safe: an unverifiable session is treated as active).

**OPS-48** If `Sweep` is asked to execute without a caller-confirmed active-session set, then the system shall return `ErrActiveSessionsUnknown` before discovering or removing any worktree.

**OPS-49** When `Sweep` runs in dry-run mode without a caller-confirmed active-session set, the system shall still classify every worktree and shall remove nothing.

---

## Key Invariants

- **No surface bypasses ops lifecycle ownership.** CLI, MCP, and Skills all go
  through `OpContext` functions for shared operations; adapters are limited to
  dependency construction and the `CreateSessionRuntime` seam.
- **Creation has one owner.** A runtime adapter may launch a harness and
  safely deliver AGY's identity-creating startup prompt when explicitly asked
  by `CreateSessionWithContext`, and complete surface-specific work after
  registration, but cannot insert, reorder, or skip shared lifecycle phases.
- **GC skip priority is deterministic.** The `gcSkipReason` function applies
  checks in a fixed order: already-archived → reaping → protected-role →
  active-tmux → active-state → too-recent. The first matching check wins.
- **State field removed.** The `State` field was removed from `SessionSummary`
  because it produced false positives causing cascading bad decisions. Do not
  re-add it without an ADR.
- **One archive transition.** `ArchiveSession` is the only implementation that
  writes `lifecycle=archived`. The reaper's `lifecycle=reaping` tombstone is a
  distinct crash-recovery transition and must not grow a copied finalizer.
- **UI archival is a separate namespace.** `ArchiveUISessions` reconciles
  Claude desktop/UI records and is not part of AGM internal session archival
  (ADR-026).

## BDD Traceability

- Feature: `agm/test/bdd/features/trust_protocol.feature`
- Feature: `agm/test/bdd/features/scan_loop.feature`
- Feature: `agm/test/bdd/features/stall_detection.feature`
