# AGM CLI - Technical Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`
- Feature: `agm/test/bdd/features/agm_control_surface_guardrails.feature`
- Feature: `agm/test/bdd/features/local_development_guardrails.feature`
- Feature: `agm/test/bdd/features/harness_parity.feature`

<!-- Last audited at: 2026-07-21 -->

**Version:** 2.0
**Status:** Production (Phase 6 Complete - Dolt-Only Architecture, YAML Backend Removed)
**Last Updated:** 2026-07-21

## Overview

The AGM (AI/Agent Gateway Manager) CLI is a command-line interface for managing multi-agent AI sessions with tmux integration. It provides unified session management across multiple AI providers (Claude, Gemini, GPT) through a consistent, user-friendly command structure.

## Purpose

Provide a production-ready CLI that:
- Enables creation, resumption, and lifecycle management of AI agent sessions
- Integrates seamlessly with tmux for persistent terminal sessions
- Supports multiple AI agents through a unified interface
- Provides robust session discovery and fuzzy matching
- Ensures session health through validation and diagnostics
- Maintains backward compatibility with Agent Session Manager (AGM)

## EARS Requirements

**CLI-01** When AGM creates a Codex CLI session without an explicit model, the system shall use the shared codex-cli harness default from the agent model registry.

**CLI-02** When AGM builds a Codex CLI launch or resume command, the system shall resolve the selected model alias to the full Codex model name before invoking the harness.

**CLI-03** When `agm supervisor heartbeat` runs inside a tmux session, the system shall record that tmux session name in the heartbeat record so status can verify a harness process is actually running where the heartbeat claims to come from.

**CLI-04** When `agm supervisor status` finds a fresh heartbeat, the system shall cross-check the supervisor's tmux pane process tree; a fresh heartbeat with no live harness process shall report as ZOMBIE, count as stale for exit codes and mesh-recovery reachability, and include a reason naming any orphaned agm writer (ce-axsr/ce-qkf7).

**CLI-05** When a supervisor's process liveness cannot be verified (probe error, or no tmux session recorded and no known role mapping), the system shall keep the heartbeat-based verdict in `agm supervisor status` and annotate the row as unverified rather than marking it dead.

**CLI-06** When `agm session kill` completes against a session whose harness process was proven dead, the system shall tell the user why the kill did not require `--confirmed-stuck`, including the pane-tree evidence and any orphaned agm heartbeat writer.

**CLI-07** When AGM starts or resumes a Codex CLI harness in a working directory, the system shall record that directory as a trusted Codex project in `$CODEX_HOME/config.toml` before sending the launch command, so a fresh non-git sandbox directory cannot block startup on Codex's interactive trust prompt (ce-cmsq).

**CLI-08** If pre-trusting the Codex working directory fails, the system shall emit a warning and still attempt the harness launch.

**CLI-09** When AGM installs a launchd schedule from an embedded plist template, the system shall set the job's WorkingDirectory to the invoking user's home directory, so launchd does not start the job with cwd=/ (ce-k414).

**CLI-10** When AGM starts AGY with startup permission mode `auto`, the system shall launch AGY with its skip-permissions mechanism and shall treat the mode as already applied at startup.

**CLI-11** When AGM builds a session manifest with a startup permission mode, the system shall persist the permission mode, source, and update timestamp so resume and audit surfaces reflect the launch contract.

**CLI-12** When `agm capture` reads pane output for any active harness, the system shall use the canonical AGM tmux socket for both session discovery and capture rather than the host default socket.

**CLI-13** When `agm supervisor run` launches Claude Code without an explicit model, the system shall pass the non-credit-gated `sonnet-200k` model alias resolved to its full Claude model name so unattended supervisor boot cannot stop at a model-switch prompt.

**CLI-14** When `agm supervisor run` launches Claude Code, the system shall pass startup auto-mode flags so the persistent supervisor can execute boot and tick-loop setup without a plan-exit approval prompt.

**CLI-15** When `agm supervisor run` launches Claude Code by default, the system shall not pass development-channel flags; it shall load development channels only when the caller explicitly opts in because the Claude Code development-channel confirmation prompt blocks unattended supervisor boot.

**CLI-16** When `agm supervisor heartbeat` receives a canonical VROOM supervisor ID, compact alias, or role name, the system shall normalize it to the canonical identity, derive omitted Primary/Tertiary peers from `pkg/vroom/supervisor`, and reject explicit peer values that contradict that topology.

**CLI-17** When AGM mirrors a canonical supervisor heartbeat into the flat VROOM heartbeat directory, the system shall use the compact alias and role from `pkg/vroom/supervisor` rather than maintaining an AGM-local alias table.

**CLI-18** When `agm session new` starts a session inside or outside tmux, the system shall declare caller provenance and delegate the creation lifecycle to `internal/ops`, while adapting interactive launch and post-registration completion through one `CreateSessionRuntime` implementation.

**CLI-19** When the CLI launches a new session or a resume fallback starts a fresh harness, the system shall obtain the shell command from the shared ops command builder so model resolution, permission flags, persistence, telemetry, and credential forwarding do not drift between creation paths.

**CLI-20** When current-tmux session creation cannot commit its manifest, the system shall record the commit failure in the CLI debug log instead of silently discarding it.

**CLI-21** When `agm sessions resume-all` successfully resumes a session, the system shall write `.agm/resume-timestamp` for supervisor coordination; failure to write that advisory timestamp shall warn without failing the resumed session.

**CLI-22** When AGM registers its Cobra tree, the system shall reject duplicate active sibling command paths before generating plugin command guidance.

**CLI-23** When `agm wiki query-save` receives user-controlled question or answer text, the system shall accept file-backed inputs, reject conflicting inline and file values, and preserve file content without shell evaluation.

**CLI-41** When installed AGM command guidance is generated, the system shall derive executable paths and supported flags from the live Cobra tree and shall fail if any installed command Markdown is outside the declared inventory.

**CLI-24** When AGM command tests execute Cobra commands or mutate command flags, the system shall use fresh command instances or restore the complete shared command state so test results remain independent of execution order.

**CLI-25** When `agm new` runs inside tmux without `--detached` for Claude Code, Codex, OpenCode, Pi, or deprecated Gemini compatibility, the system shall route into current-pane creation, require the harness's canonical executable, queue the canonical launch command behind the invoking AGM process, and finalize session metadata without synchronously waiting for the composer, because the pane shell cannot consume the command until AGM returns; Pi shall use its managed canonical launch contract, and Claude's SessionStart hook shall persist the resulting conversation UUID.

**CLI-26** When `agm audit resources --fix` cannot remove a linked worktree through a one-force Git operation, including when the worktree is locked, the system shall preserve the checkout and report the cleanup error instead of deleting the directory directly.

**CLI-27** When AGM creates or cold-resumes an AGY session, the system shall resolve and pass the selected AGY model rather than silently launching AGY's default model.

**CLI-28** When AGM launches AGY and a startup prompt must be delivered, the system shall wait through first-run trust and feedback-survey prompts until AGY is ready before sending that prompt, and shall propagate readiness failure.

**CLI-29** When a command-scoped create, model-change, mode-change, or compaction flow waits to deliver or verify a prompt or slash command, the system shall derive that wait from the Cobra command context and shall stop before later delivery, retry, persistence, liveness validation, or attach work when the context is canceled.

**CLI-30** When the AGM root owns SIGINT and SIGTERM handling, continuous scan, watchdog, event-watch, stalled-session watch, and bounded compaction-monitor loops shall consume the root Cobra context and return promptly on cancellation; subcommands shall not install competing process-global signal handlers.

**CLI-31** When `agm send verify`, `agm send work-request`, or `agm send wake-loop` performs structured multiline delivery, the system shall pass the Cobra command context through composer readiness and delivery so cancellation cannot later send the payload.

**CLI-32** When session resume metadata lookup returns after the Cobra command context has been canceled, the system shall stop before creating, commanding, updating, or attaching to a tmux session.

**CLI-33** When final creation liveness validation runs, the system shall derive the scan from the Cobra command context and recheck cancellation before title update, attach, or detached-success reporting.

**CLI-34** When current-pane creation selects AGY, the system shall fail before harness launch or session registration and direct the user to create a detached session with a different name, because the provider-native conversation identity cannot be safely correlated until the foreground AGM command exits.

**CLI-35** When AGM resumes a stopped `codex-cli` session, the system shall report success, update activity, deliver an optional prompt, or attach only after both the Codex process (including its Node wrapper) and interactive composer have been observed as ready, allowing up to 60 seconds for each readiness phase so a healthy cold startup is not rejected at the former advisory threshold.

**CLI-36** If AGM creates a tmux session for a resume attempt and creation finalization, command dispatch, harness readiness, ordinary or caller-canceled prompt delivery, or canonical-name persistence fails, the system shall route every production resume entry, including last-session and bulk resume, through the stable session-ID operation lock before resume reads, serialize the resolved transaction through finalization, release that lock before interactive attachment, compensate only the creation-specific tmux identity created by that attempt, and return the primary failure together with any cleanup failure. The identity shall include the server-local session ID and a random per-creation token represented by a provisional creation name until stored on the session, so every partial command boundary is cleanable; when ID output is lost, the exact random provisional name shall remain cleanable, and a replacement after a server restart shall be preserved. After successful readiness, canonical-name persistence shall use an opaque cross-dialect ownership revision before optional Codex prompt submission; a prompt failure proven to occur before submission or while the paste remains positively parked shall restore only that exact provisional metadata revision and its prior activity timestamp before removing the created tmux identity. A successful restore compare-and-swap shall be sufficient proof for tmux cleanup and shall not depend on a fallible follow-up metadata read. If commit reports an error, storage shall re-read the exact revision and retain the pending ownership change unless the complete prior state proves the write did not commit. If the post-write metadata reload fails, the system shall retain the exact pending ownership revision through rollback unless an immediate compensation was proven successful. If a newer writer superseded metadata ownership or restoration otherwise cannot be proven, the system shall preserve the ready tmux session so canonical metadata never points at a resource the failed transaction destroyed. Every full-session writer shall advance the tmux identity revision, including after a lost compare-and-swap, so any number of older snapshots remain unable to restore a stale tmux name. Confirmed prompt delivery or a lost acknowledgement after the final Enter shall rotate away from the provisional revision, shall become an irreversible success boundary, and shall not be reported as a retryable failure because of later caller cancellation or attachment failure; the latter shall warn that work may have started and preserve the pane. A failed prompt delivery on a pre-existing session shall not suppress a later attachment failure. A same-named replacement tmux session or a tmux session that existed before the attempt shall not be removed.

**CLI-37** When `agm session rename` moves a live tmux session, the system shall resolve its stable session ID, hold the same per-session lifecycle lock as resume while reloading current state and completing every rename effect, first claim that exact tmux creation with its server-local ID plus a random marker, then persist the user-visible and tmux names together through a narrow compare-and-swap against the exact storage revision read before the move. If either the tmux client or storage loses its success response, the command shall use bounded cancellation-independent ownership checks before deciding whether forward progress completed or compensation is safe: tmux shall still carry the claimed identity at the expected name, and storage shall first fence the observed revision with a competing compare-and-swap before re-reading it. If another writer advanced to a different identity, the claimed tmux identity is lost or replaced, or the caller is canceled after the move and storage is fenced as unchanged, the command shall report the primary failure, compensate only the claimed live tmux session, join any fencing, probe, or rollback failure, and never report rename success with metadata pointing at a nonexistent or unrelated tmux session.

**CLI-38** When `agm admin link-session-parent` or `agm admin backfill-plan-sessions` assigns a parent and optionally inherits its display name, the command shall persist both changes through one narrow compare-and-swap against the child identity revision it read and shall report a concurrent identity change as a failure instead of reporting a link or rename that storage did not apply.

**CLI-39** When `agm send msg` delivers directly to one or more registered CLI-harness sessions, every recipient shall route through shared `ops.SendMessage`, which shall atomically prove the manifest's canonical harness process and current composer before sending to the exact verified pane; a single-recipient `--force`, `--autonomous`, `AGM_AUTONOMOUS=1`, or autonomous-role send shall reach that shared atomic route before preliminary queue dispatch only for a manifest-confirmed supported CLI harness. Pure API harnesses intentionally have no tmux pane: single-recipient preflight shall resolve the registered harness before any tmux probe, and both single-recipient and fan-out delivery shall reconstruct the adapter from the session's persisted non-secret runtime configuration and obtain credentials only at runtime. A provider-appropriate stable session-ID mutation boundary shall serialize adapter construction, a locked lifecycle reload, active-or-idle readiness, bounded context-aware provider completion, and completed-turn history persistence across AGM processes while sharing that boundary with archive; every non-active lifecycle, including reaping and archived, shall fail before adapter construction or paid provider work. Adapter-level delivery shall independently serialize callers that bypass the CLI and bound its own provider work. Adapter errors, non-active lifecycle, caller cancellation, provider timeout, and suspended or terminated status shall not enter the tmux-only daemon queue and shall create neither pending-file nor delegation state. For every supported CLI harness the shared route may replace input only when the latest queued-input marker and a syntactically complete generated AGM message header are bound inside that same current composer after exact foreground-harness and pane ownership are proved from the complete style-preserving logical composer with terminal-wrapped rows joined, with visible pasted-text line counts and terminal idle or managed chrome excluding later active output. Replacement shall first clear the verified queue without submitting it and re-prove the expected foreground harness plus an empty composer on the same exact pane; a failed clear or recheck, changed pane, historical or partial header, opaque Codex pasted-content chip without an observable bound header, human draft, generic busy composer, active work, an unregistered tmux session, missing storage, wrong or dead harness, permission, overlay, onboarding, missing target, missing atomic delivery capability, or failed readiness check shall return non-delivery without pasting replacement input or creating a pending-file bypass.

**CLI-40** When CLI session creation has registered a supported harness and must deliver `--prompt` or `--prompt-file`, the completion boundary shall atomically revalidate that the expected harness owns the foreground terminal and an empty composer, then send to the exact verified pane under the same mutation boundary; an unready, background, suspended, wrong, or missing harness, a focus change, an invalid pane proof, or caller cancellation shall not deliver the startup prompt.

**CLI-41** When `agm session new` provisions a sandbox for any harness, the system shall start the shared harness lifecycle from the provider-mapped counterpart of the requested project directory and shall persist both that directory and the sandbox workspace root.

**CLI-42** When sandbox onboarding is enabled for a nested harness working directory, the system shall render onboarding template workspace-root data from the provider's merged path while writing the generated instructions to the harness working directory's project-scoped configuration.

**CLI-43** When automatic workspace discovery finds repositories that do not contain the requested project directory, the system shall retain those repositories, prepend the nearest Git repository containing the requested directory, and reject sandbox creation if the requested directory has no safe containing Git repository.

**CLI-44** When direct, fan-out, structured, daemon-queued, fresh-startup, or post-resume delivery targets an AGY session, the command shall propagate the resolved `agy` harness through the shared tmux delivery boundary so attribution and multiline bodies remain one native request; unresolved and non-AGY sessions shall retain their established delivery semantics.

## Requirements

### Functional Requirements

#### FR1: Session Lifecycle Management
- **ID:** FR1
- **Priority:** P0 (Critical)
- **Description:** CLI MUST support full session lifecycle operations
- **Commands:**
  - `agm new [session-name]` - Create new session with tmux integration
  - `agm resume [identifier]` - Resume existing session by UUID/name/fuzzy match
  - `agm session list` - List all non-archived sessions
  - `agm session archive [session-name]` - Mark session as archived
  - `agm session kill [session-name]` - Terminate active session
  - `agm session unarchive [session-name]` - Restore archived session
- **Validation:** All lifecycle commands update manifest and maintain session integrity

#### FR2: Smart Session Resolution
- **ID:** FR2
- **Priority:** P0 (Critical)
- **Description:** CLI MUST intelligently resolve session identifiers
- **Resolution Strategy:**
  1. Exact name match (highest priority)
  2. UUID prefix match (partial UUID accepted)
  3. Tmux session name match
  4. Fuzzy name matching (Levenshtein distance ≥ 0.6)
  5. Interactive picker (when no args provided)
- **Behavior:**
  - No args + sessions exist → Interactive TUI picker
  - No args + no sessions → Prompt to create new session
  - Name provided + exact match → Resume immediately
  - Name provided + fuzzy matches → "Did you mean" prompt
  - Name provided + no match → Offer to create new session

#### FR3: Multi-Agent Support
- **ID:** FR3
- **Priority:** P0 (Critical)
- **Description:** CLI MUST support multiple AI agent backends
- **Implementation:**
  - Agent selection via `--agent` flag (default: claude)
  - Supported agents: claude, gemini, gpt
  - Agent availability detection (API keys, CLI installation)
  - Agent-specific command translation
  - `agm agent list` - Show all available agents with capabilities
- **Error Handling:**
  - Warning if agent unavailable (missing API key/CLI)
  - Graceful degradation for unsupported agent features
  - Clear error messages with remediation steps

#### FR4: Tmux Integration
- **ID:** FR4
- **Priority:** P0 (Critical)
- **Description:** CLI MUST integrate with tmux for session persistence
- **Behavior:**
  - Create tmux session if not exists
  - Attach to existing tmux session if available
  - Send commands to tmux panes (cd, agent CLI invocation)
  - Detect running tmux sessions
  - Support detached mode (`--detached` flag)
- **Constraints:**
  - Cannot run `agm new` from within tmux (unless `--detached`)
  - Tmux session names must match AGM session names
  - Health checks validate tmux session state

#### FR5: Session Discovery and Search
- **ID:** FR5
- **Priority:** P1 (High)
- **Description:** CLI MUST provide robust session discovery
- **Commands:**
  - `agm search [query]` - Search sessions by name/project path
  - `agm session list --all` - Include archived sessions
  - `agm session list --json` - Machine-readable output
  - `agm session list --limit <n> --offset <n>` - Deterministic pagination for large inventories
  - `agm admin get-uuid [identifier]` - Get session UUID
  - `agm admin get-session-name [identifier]` - Get session name
- **Search Features:**
  - Fuzzy matching with similarity threshold
  - Filter by lifecycle state (active/stopped/archived)
  - Filter by current directory (project-scoped sessions)
  - Status computation (active/stopped/archived based on tmux state)
- **List Output Format:**
  - **Terminal width-based layouts:**
    - Minimal (<80 cols): NAME(20), UUID(8), WORKSPACE(8), AGENT(6), ACTIVITY(10)
    - Compact (80-99 cols): NAME(24), UUID(8), WORKSPACE(9), AGENT(6), PROJECT(20), ACTIVITY(10)
    - Full (≥100 cols): NAME, UUID, WORKSPACE, AGENT, PROJECT, ACTIVITY (dynamic widths)
  - **UUID column:** Shows first segment of Claude UUID (before first "-"), or "-" if no UUID
  - **Column headers:** Displayed above each status group (ACTIVE, STOPPED, STALE, ARCHIVED)
  - **Column alignment:**
    - Most columns: Left-aligned
    - ACTIVITY column: Right-aligned to make "ago" line up vertically
  - **Sort order:**
    - ACTIVE sessions: Sort by attachment status (● attached first, ◐ detached second), then alphabetically by name
    - All other groups: Sort alphabetically by name
  - **Status symbols:**
    - ● (filled circle): Active session with attached clients
    - ◐ (half-filled circle): Active session with no attached clients
    - ○ (empty circle): Stopped session
    - ⊗ (circled X): Stale session
    - ◉ (double circle): Archived session

#### FR6: Health Checks and Diagnostics
- **ID:** FR6
- **Priority:** P1 (High)
- **Description:** CLI MUST provide diagnostic capabilities
- **Commands:**
  - `agm admin doctor` - Quick structural health check
  - `agm admin doctor --validate` - Deep functional validation
  - `agm admin doctor --apply-fixes` - Auto-repair common issues
  - `agm admin fix-uuid [session-name]` - Fix UUID associations
- **Checks:**
  - Structural: Duplicate sessions, orphaned directories, invalid manifests
  - Functional: Session resumability, agent availability, tmux state
  - Auto-fixes: UUID conflicts, missing manifests, stale tmux sessions

#### FR7: Backup and Migration
- **ID:** FR7
- **Priority:** P2 (Medium)
- **Description:** CLI MUST support backup and migration operations
- **Commands:**
  - `agm backup [session-name]` - Create numbered backup
  - `agm backup list` - List all backups
  - `agm backup restore [session-name] [backup-id]` - Restore from backup
  - `agm migrate to-unified-storage` - Migrate legacy sessions
- **Behavior:**
  - Automatic backups before destructive operations (archive, UUID update)
  - Numbered backups with timestamps
  - Restore from specific backup number
  - Migration wizard for AGM → AGM transition

#### FR8: Workflow Automation
- **ID:** FR8
- **Priority:** P2 (Medium)
- **Description:** CLI MUST support predefined workflows
- **Commands:**
  - `agm workflow list` - List available workflows
  - `agm new --workflow [name]` - Create session with workflow
- **Workflows:**
  - `deep_research` - Gemini-based deep research workflow
  - Custom workflows via registration system
- **Features:**
  - Auto-detect agent for workflow
  - Validate workflow prerequisites
  - Inject initial prompt

#### FR9: Interactive UI Components
- **ID:** FR9
- **Priority:** P1 (High)
- **Description:** CLI MUST provide rich interactive experiences
- **Components:**
  - Session picker (Huh TUI library)
  - "Did you mean" prompt (fuzzy match selection)
  - Create confirmation prompt
  - Multi-step session creation form
  - Spinner for long-running operations
- **Accessibility:**
  - `--no-color` flag for WCAG AA compliance
  - `--screen-reader` flag for text-only symbols
  - Keyboard navigation (arrow keys, vim bindings)

#### FR10: Session Association
- **ID:** FR10
- **Priority:** P1 (High)
- **Description:** CLI MUST support manual and automatic session association
- **Commands:**
  - `agm session associate [session-name]` - Associate an AGM session with the current harness session
  - `agm session associate [session-name] --uuid [uuid]` - Specify Claude UUID
  - `agm session associate [session-name] --create --harness codex-cli` - Create/link a non-Claude harness session record
  - Auto-detection on session creation/resume
- **Detection:**
  - Hybrid detection algorithm (timestamp + tmux correlation)
  - Confidence scoring (high/medium/low)
  - Auto-detect only in high-confidence scenarios
  - Fallback to manual association

#### FR11: Message Sending and Queueing
- **ID:** FR11
- **Priority:** P0 (Critical)
- **Description:** CLI MUST support non-disruptive message sending to running sessions
- **Commands:**
  - `agm send msg [session-name] --prompt "message"` - Send or queue by verified readiness
  - `agm send msg [session-name] --prompt-file /path/to/file` - Send from file
  - `agm send msg [session-name] --force --prompt "recovery"` - Replace only positively identified queued AGM input
  - `agm send msg [session-name] --autonomous --prompt "msg"` - Mark the recipient unattended for the same narrow queued-AGM recovery
  - `agm send msg [session-name] --sender [name] --prompt "msg"` - Specify sender
  - `agm send msg [session-name] --reply-to [message-id] --prompt "reply"` - Thread messages
- **Behavior:**
  - **Default:** Deliver through shared atomic readiness when the registered harness owns an idle composer; queue a busy composer for later delivery
  - **Force/autonomous recovery:** For manifest-confirmed supported CLI harnesses only, bypass preliminary queue routing so the shared atomic operation can replace the latest queued-input artifact when a syntactically complete generated AGM message header is observably bound inside that same current composer; pure API harnesses bypass irrelevant tmux state but require adapter status active or idle at the final common single-recipient or fan-out delivery boundary and reject adapter errors, suspended status, or terminated status without entering the tmux-only daemon queue, and no path may infer AGM ownership from a historical or partial header or an opaque Codex pasted-content chip, replace a human draft, or bypass generic busy, active-work, permission, overlay, onboarding, wrong-harness, missing-target, or backend-error states
  - **State-based routing:**
    - READY state → Deliver to the exact verified pane
    - THINKING state → Queue message (wait for READY)
    - PERMISSION_PROMPT state → Queue message (wait for READY)
    - COMPACTING state → Reject with error (never interrupt compaction)
    - OFFLINE state → Reject with error
  - **Daemon integration:** Queued messages delivered automatically when daemon running
  - **Daemon offline warning:** Clear feedback when messages queued without daemon
- **Message Metadata:**
  - Unique message ID: `{timestamp}-{sender}-{seq}`
  - Sender attribution (auto-detected from AGM session or --sender flag)
  - Reply-to threading for conversation context
  - Audit trail logged to `~/.agm/logs/messages/`
- **Safety Guarantees:**
  - Compaction protection: NEVER interrupt during COMPACTING state (critical safety requirement)
  - Rate limiting: 10 messages per minute per sender
  - Message size limit: 10KB for prompt files
  - Sender name validation: alphanumeric, dash, underscore only
- **User Feedback:**
  - Queue confirmation: "⏳ Queued to 'session' (session READY) [ID: ...]"
  - Daemon warning: "⚠️ Message queued but delivery daemon is NOT running!"
  - Interrupt confirmation: "✓ Sent to 'session' from 'sender' ... [via: tmux]"
  - Compaction error: "❌ Cannot send to 'session' (session COMPACTING)"
- **Test Coverage:**
  - Unit tests: Flag validation, message formatting, sender validation
  - Integration tests: Queue behavior, interrupt mode, compaction protection, daemon warnings
  - Backward compatibility: Existing scripts continue to work

#### FR12: Bulk Session Operations
- **ID:** FR12
- **Priority:** P1 (High)
- **Description:** CLI MUST support bulk operations on multiple sessions for post-reboot recovery
- **Commands:**
  - `agm sessions resume-all` - Resume all stopped sessions
  - `agm sessions resume-all --workspace-filter [name]` - Resume sessions in specific workspace
  - `agm sessions resume-all --include-archived` - Also resume archived sessions
  - `agm sessions resume-all --dry-run` - Preview without executing
  - `agm sessions resume-all --detached` - Resume without attaching (default: true)
  - `agm sessions resume-all --continue-on-error` - Keep going if sessions fail (default: true)
- **Behavior:**
  - **Sequential Resume:** Resume sessions one at a time with 500ms delays (avoid tmux overload)
  - **Status-Based Filtering:** Only resume "stopped" sessions (skip active, error on archived unless --include-archived)
  - **Progress Indicators:** Charmbracelet Bubbles spinner + progress bar for visual feedback
  - **Error Collection:** Continue through failures (unless --continue-on-error=false), report all errors at end
  - **Workspace Filtering:** Filter by manifest.Workspace field (case-sensitive exact match)
  - **Detached Mode:** Resume without attaching to sessions (allows bulk operations without interruption)
- **Use Cases:**
  - **Post-Reboot Recovery:** `agm sessions resume-all` restores all stopped sessions after machine restart
  - **Workspace Isolation:** `agm sessions resume-all --workspace-filter=alpha` resumes only alpha workspace sessions
  - **Preview Changes:** `agm sessions resume-all --dry-run` shows which sessions would be resumed
- **Integration:**
  - **Orchestrator Coordination:** Writes `.agm/resume-timestamp` for supervisor detection
  - **Boot Automation:** Works with systemd service (`agm-resume-boot.service`) for automatic boot recovery
  - **Admin Commands:** Future `agm admin enable-auto-resume` and `disable-auto-resume` for opt-in boot automation
- **Performance:**
  - **Batch Status Computation:** Uses `session.ComputeStatusBatch()` for efficient stopped session detection
  - **Sequential Processing:** 500ms delays prevent tmux server overload
  - **Progress Feedback:** Real-time updates during long operations (20+ sessions)
- **Safety Guarantees:**
  - **No Archived by Default:** Archived sessions are skipped unless explicitly included with --include-archived
  - **Health Checks:** Each session validated before resume attempt (worktree exists, manifest valid)
  - **Error Isolation:** One failed session doesn't stop remaining resumes (unless --continue-on-error=false)
  - **Summary Reporting:** Clear success/failure counts with error details at completion
- **Error Handling:**
  - No stopped sessions → "No stopped sessions found. All sessions are active or archived."
  - Manifest read errors → Skip corrupted manifest with warning, continue with next
  - Resume failures → Collect errors, continue (default), show summary at end
  - Tmux unavailable → Clear error message with remediation steps
- **Test Coverage:**
  - Unit tests: `cmd/agm/resume_all_test.go` - filterNonArchived(), filterByWorkspace(), edge cases
  - Integration tests: `test/integration/lifecycle/resume_all_test.go` - basic functionality, workspace filtering, archived handling, error scenarios, large scale (50 sessions)
  - BDD scenarios: (future) End-to-end resume-all flows
- **References:**
  - Implementation: `cmd/agm/resume_all.go`
  - Systemd Service: `systemd/agm-resume-boot.service`

#### FR13: Send Command Reorganization
- **ID:** FR13
- **Priority:** P0 (Critical)
- **Description:** CLI MUST provide unified `agm send` command group for all communication operations
- **Commands:**
  - `agm send msg [recipient] --prompt "..."` - Send messages (replaces `agm session send`)
  - `agm send reject [session] --reason "..."` - Reject permission prompts (replaces `agm session reject`)
  - `agm send approve [session] --reason "..."` - Approve permission prompts (NEW)
- **Migration:**
  - Use `agm send msg` and `agm send reject`; the retired `agm session send` and `agm session reject` paths are not registered
  - Generated command guidance and examples use the active command tree
- **Benefits:**
  - Logical grouping of communication commands
  - Improved command discoverability
  - Future extensibility for additional send operations

#### FR14: Multi-Recipient Support
- **ID:** FR14
- **Priority:** P1 (High)
- **Description:** CLI MUST support sending messages to multiple recipients in one invocation
- **Syntax:**
  - Positional: `agm send msg session1,session2,session3 --prompt "..."`
  - Explicit flag: `agm send msg --to session1,session2 --prompt "..."`
  - Glob patterns: `agm send msg "*research*" --prompt "..."`
  - Workspace filtering: `agm send msg --workspace oss --prompt "..."`
- **Features:**
  - **Sequential delivery:** Recipients are delivered one at a time because tmux mutation is serialized and each send owns one atomic readiness-and-input boundary
  - **Recipient resolution:** Comma-separated lists, glob pattern expansion, workspace filtering
  - **Result aggregation:** Color-coded success/failure report for each recipient
  - **Error isolation:** One recipient failure doesn't block others
  - **Rate limiting:** Per-sender (not per-recipient), 10 messages/minute
- **Safety:**
  - One recipient's readiness proof and exact-pane send complete before the next begins
  - A failed recipient is reported without preventing later recipients from being attempted
- **Flags:**
  - `--to <recipients>` - Explicit recipient list (alternative to positional)
  - `--workspace <name>` - Filter sessions by workspace
- **Use Cases:**
  - Broadcast notifications: "Deploy complete" to all backend sessions
  - Status checks: "Health check" to all test sessions
  - Coordinated workflows: "Pause work" to multiple sessions

#### FR15: Permission Approval Command
- **ID:** FR15
- **Priority:** P1 (High)
- **Description:** CLI MUST provide symmetric approve command to complement reject
- **Command:**
  - `agm send approve [session] --reason "..." --auto-continue`
- **Features:**
  - **Automated approval:** Navigates to "Yes" option (usually default)
  - **Optional reasoning:** Add approval reason as additional instructions
  - **Auto-continue:** Automatically continue after approval (bypasses "Continue" prompt)
  - **Prompt detection:** Handles 2-option and 3-option permission prompts
  - **Smart extraction:** Extracts "## Standard Prompt (Recommended)" from markdown files
- **Flags:**
  - `--reason <text>` - Approval reason (optional)
  - `--reason-file <path>` - File containing approval reason
  - `--auto-continue` - Automatically continue after approval
- **Workflow:**
  1. Detect prompt type (2-option or 3-option)
  2. Navigate to "Yes" option if needed
  3. If `--reason` provided: Tab → reason text → Enter
  4. If `--auto-continue`: Send additional Enter
  5. Otherwise: Send Enter to approve
- **Symmetry with Reject:**
  - Same flags: `--reason`, `--reason-file`
  - Same markdown extraction logic
  - Opposite navigation (Yes instead of No)

#### FR16: Session Output Capture
- **ID:** FR16
- **Priority:** P1 (High)
- **Description:** CLI MUST expose tmux capture-pane functionality as public commands
- **Command:**
  - `agm capture [session] [flags]`
- **Modes:**
  - **Visible content** (default): Capture visible pane content
  - **Full history** (`--history`): Capture full scrollback buffer
  - **Tail** (`--tail N`): Capture last N lines only
- **Output Formats:**
  - **Text** (default): Line-by-line output
  - **JSON** (`--json`): Structured output with metadata
  - **YAML** (`--yaml`): YAML format with metadata
- **Features:**
  - **Regex filtering** (`--filter`): Filter lines matching pattern
  - **Line limiting** (`--lines N`): Limit output to N lines
  - **Metadata** (JSON/YAML): Includes session name, timestamp, line count
- **Flags:**
  - `--lines <N>` - Limit output to N lines
  - `--history` - Capture full scrollback history
  - `--tail <N>` - Capture last N lines only
  - `--json` - Output in JSON format
  - `--yaml` - Output in YAML format
  - `--filter <regex>` - Filter lines matching regex
- **Use Cases:**
  - **Debugging:** Capture error output for analysis
  - **Monitoring:** Extract specific log patterns
  - **Automation:** Parse structured output in scripts
  - **Multi-session coordination:** Capture responses from multiple sessions

#### FR17: Claude Code Commands
- **ID:** FR17
- **Priority:** P2 (Medium)
- **Description:** CLI MUST provide verified global Claude Code command files for the tracked AGM plugin command inventory
- **Installation:**
  - `make install-skills` - Compatibility alias that installs Markdown commands to `~/.claude/commands/`
- **Implementation:**
  - `agm/agm-plugin/commands/*.md` defines the complete command and argument contracts
  - Per-file `content-hash` fields and inventory tests detect stale or missing command surfaces

#### FR18: Test Session Isolation and Management
- **ID:** FR18
- **Priority:** P1 (High)
- **Description:** CLI MUST provide robust test session isolation and cleanup mechanisms
- **Commands:**
  - `agm new --test [session-name]` - Create isolated test session in `~/sessions-test/`
  - `agm new [session-name] --allow-test-name` - Override test pattern detection
  - `agm admin cleanup-test-sessions` - Clean up orphaned test sessions
  - `agm admin cleanup-test-sessions --dry-run` - Preview cleanup without deletion
  - `agm admin cleanup-test-sessions --pattern "regex"` - Custom pattern matching
  - `agm admin cleanup-test-sessions --message-threshold N` - Set trivial session threshold
- **Test Isolation:**
  - **--test flag behavior:**
    - Creates session in `~/sessions-test/` (isolated from production `~/.claude/sessions/`)
    - Tmux session prefixed with `agm-test-` (e.g., `agm-test-my-experiment`)
    - Not tracked in AGM database (ephemeral, no manifest created)
    - Perfect for experiments, CI/CD tests, temporary work
  - **Production workspace protection:**
    - Interactive prompt when session name contains "test" (case-insensitive)
    - PreToolUse hook blocks `test-*` patterns without `--test` flag
    - Clear error messages with remediation steps
- **Cleanup Mechanism:**
  - **Interactive multi-select:** Review and select sessions for deletion
  - **Message count analysis:** Identify trivial sessions (default threshold: 5 messages)
  - **Automatic backup:** Tarball backup to `~/.agm/backups/sessions/` before deletion
  - **Safe deletion:** Archive manifest (lifecycle: archived) then remove directory
  - **Pattern matching:** Default `^test-`, customizable via `--pattern` flag
  - **Dry-run mode:** Preview operations without making changes
- **Prevention Layers:**
  - **Layer 1 (Reactive):** Cleanup command removes existing pollution
  - **Layer 2 (Educational):** Interactive prompt teaches users `--test` flag usage
  - **Layer 3 (Preventive):** PreToolUse hook (`pretool-test-session-guard`) blocks violations
- **Override Mechanism:**
  - **--allow-test-name flag:** Bypasses test pattern detection for legitimate use cases
  - **Use cases:** Testing infrastructure work, TDD sessions, test-related documentation
  - **Guidance:** Use sparingly - most test work should use `--test` flag
- **Test Pattern Detection:**
  - **Pattern:** Case-insensitive substring match for "test"
  - **Triggers:** test-foo, TEST-FOO, Test-Bar, my-testing, contest (conservative)
  - **Does not trigger:** my-test-feature (different prefix), latest (not standalone)
  - **Rationale:** Conservative approach catches more cases, override available
- **Error Handling:**
  - **Hook failures:** Graceful degradation (allow command, log error)
  - **Missing sessions:** Clear error with suggested alternatives
  - **Backup failures:** Abort cleanup, preserve session safety
- **Test Coverage:**
  - Unit tests: Message counting, backup creation, pattern detection
  - Integration tests: Full cleanup workflow, hook integration, edge cases
  - BDD tests: User-facing scenarios (see `test/bdd/features/test_session_isolation.feature`)
  - Hook tests: 18 sub-tests covering all patterns and edge cases
- **Documentation:**
  - **User guide:** `docs/TEST-SESSION-GUIDE.md` (comprehensive examples, comparison table)
  - **ADR:** `cmd/agm/ADR-006-test-isolation-enforcement.md` (isolation boundary)
  - **Retrospective:** `vbonnet/engram-research` `retrospectives/RETROSPECTIVE-TEST-SESSION-CLEANUP.md` (implementation learnings)
  - **README:** Updated with test session quick start section
  - **CHANGELOG:** v2.4 release notes with feature details
- **Related:**
  - **ADR-006:** Test Isolation Enforcement (original PreToolUse hook rationale)
  - **Dolt test isolation:** `agm/internal/dolt/testing.go`
  - **NFR4:** Testability requirements (original `--test` flag purpose)

### Non-Functional Requirements

#### NFR1: Performance
- **ID:** NFR1
- **Priority:** P1 (High)
- **Description:** CLI MUST meet performance targets
- **Targets:**
  - Command startup: < 100ms (cold start)
  - Session list (100 sessions): < 200ms
  - Session picker (100 sessions): < 500ms
  - Doctor structural check: 1-5 seconds
  - Doctor functional validation: 5-30 seconds per session
- **Optimization:**
  - Batch status computation for session lists
  - Cache tmux health checks (5-second TTL)
  - Lazy loading of session manifests
  - Concurrent health checks

#### NFR2: Reliability
- **ID:** NFR2
- **Priority:** P0 (Critical)
- **Description:** CLI MUST handle errors gracefully
- **Guarantees:**
  - No panics (all errors handled with `PrintError`)
  - Automatic backups before destructive operations
  - Lock-free design (fine-grained locks per resource)
  - Idempotent operations (safe to retry)
  - Clear error messages with actionable remediation
- **Error Presentation:**
  - User-friendly error messages
  - Context-specific remediation steps
  - Debug mode for detailed stack traces (`--debug` or `AGM_DEBUG=true`)

#### NFR3: Backward Compatibility
- **ID:** NFR3
- **Priority:** P0 (Critical)
- **Description:** CLI MUST maintain AGM compatibility
- **Guarantees:**
  - Read AGM manifest v2 format
  - Write AGM manifest v3 format
  - Auto-upgrade manifests on first write
  - `csm` command symlinked to `agm` (deprecated)
  - AGM sessions discoverable in AGM
- **Migration:**
  - Wizard for AGM → AGM migration
  - Preserve all session metadata
  - Maintain UUID associations

#### NFR4: Testability
- **ID:** NFR4
- **Priority:** P1 (High)
- **Description:** CLI MUST have comprehensive test coverage
- **Requirements:**
  - Unit tests: >80% code coverage
  - Integration tests: End-to-end command execution
  - BDD tests: User-facing scenarios (Gherkin/Cucumber)
  - Test mode: `--test` flag for isolated testing
  - Mock tmux client for unit tests
- **Test Infrastructure:**
  - `~/sessions-test/` directory for test mode
  - Injected dependencies (`ExecuteWithDeps`)
  - Deterministic test fixtures

#### NFR5: Usability
- **ID:** NFR5
- **Priority:** P1 (High)
- **Description:** CLI MUST be intuitive and self-documenting
- **Features:**
  - Comprehensive help text for all commands
  - Examples in command descriptions
  - Tab completion (bash/zsh)
  - Colorized output (with `--no-color` fallback)
  - Progress indicators for long operations
- **Documentation:**
  - Inline examples in `--help` output
  - Error messages include remediation steps
  - Validation errors show expected format

#### NFR6: Configuration Management
- **ID:** NFR6
- **Priority:** P2 (Medium)
- **Description:** CLI MUST support flexible configuration
- **Configuration Sources:**
  1. Command-line flags (highest priority)
  2. Environment variables (AGM_DEBUG, ANTHROPIC_API_KEY, etc.)
  3. Config file (`~/.config/agm/config.yaml`)
  4. Smart defaults (lowest priority)
- **Configurable Options:**
  - `--sessions-dir` - Sessions directory (default: `~/.claude/sessions`)
  - `--log-level` - Logging verbosity (debug/info/warn/error)
  - `--timeout` - Tmux command timeout
  - `--skip-health-check` - Disable health checks
  - `-C, --directory` - Working directory (like `git -C`)

## Command Structure

### Root Command
```
agm
  Shows help and available subcommands.
  Note: The 'agm [session-name]' shortcut was removed.
  Use 'agm session resume <name>' to resume a session.
```

### Command Hierarchy

```
agm
├── (no default command)     # Use subcommands below
├── new [session-name]       # Create new session
├── resume [identifier]      # Resume existing session
├── session                  # Session lifecycle management
│   ├── new [session-name]
│   ├── resume [identifier]
│   ├── list [--all] [--json]
│   ├── archive [session-name]
│   ├── unarchive [session-name]
│   ├── kill [session-name]
│   └── associate [session-name]
├── sessions                 # Bulk session operations
│   └── resume-all [--workspace-filter] [--include-archived] [--dry-run]
├── agent                    # Agent management
│   └── list [--json]
├── search [query]           # Search sessions by name/project
├── workflow                 # Workflow automation
│   └── list
├── backup                   # Backup management
│   ├── [session-name]
│   ├── list
│   └── restore [session-name] [backup-id]
├── logs                     # Log management
│   ├── [session-name]
│   ├── clean [session-name]
│   ├── stats [session-name]
│   ├── thread [session-name] [thread-id]
│   └── query [session-name] [query]
├── send [session-name] [message]  # Send message to session
├── admin                    # Administrative commands
│   ├── doctor [--validate] [--apply-fixes]
│   ├── fix-uuid [session-name]
│   ├── get-uuid [identifier]
│   ├── get-session-name [identifier]
│   ├── clean                # Clean up stale sessions
│   ├── unlock               # Remove stale locks
│   └── test                 # Test infrastructure
├── migrate                  # Migration utilities
│   └── to-unified-storage
├── sync                     # Sync session metadata
└── version                  # Show version info
```

### Global Flags

```
-C, --directory <path>       Working directory (default: current)
--config <path>              Config file (default: ~/.config/agm/config.yaml)
--sessions-dir <path>        Sessions directory (default: ~/.claude/sessions)
--log-level <level>          Log level (debug/info/warn/error)
--debug                      Enable debug logging (shorthand for --log-level debug)
--timeout <duration>         Tmux command timeout
--skip-health-check          Skip health checks
--no-color                   Disable colored output
--screen-reader              Use text symbols (accessibility)
```

## Data Structures

### Command Context
```go
type CommandContext struct {
    Config          *config.Config
    SessionsDir     string
    ProjectDir      string      // From -C flag or cwd
    HealthChecker   *tmux.HealthChecker
    UIConfig        *ui.Config
}
```

### Session Creation Options
```go
type NewSessionOptions struct {
    SessionName   string
    AgentName     string      // claude/gemini/gpt
    WorkflowName  string      // Optional workflow
    ProjectID     string      // Optional project identifier
    Prompt        string      // Initial prompt
    PromptFile    string      // Initial prompt from file
    Detached      bool        // Create without attaching
}
```

## Command Execution Flow

### Smart Default Command (REMOVED)

The `agm [session-name]` shortcut was removed. Use explicit subcommands instead:
- `agm session resume <name>` - Resume a session
- `agm session new <name>` - Create a new session
- `agm session list` - List sessions

### New Session Flow (agm new [session-name])

```
1. FAIL FAST: Cannot run from within tmux (unless --detached)
2. Determine session name:
   - From args
   - From interactive form
   - From current tmux session (if inside tmux)
3. Validate agent availability
   - Check API keys
   - Check CLI installation
   - Warn if unavailable (non-blocking)
4. Check for workflow
   - Auto-detect agent from workflow
   - Inject workflow prompt
5. Check session name uniqueness
6. Generate session ID (UUID)
7. Create manifest
8. Create/attach tmux session
9. Start agent CLI in tmux pane
10. Associate Claude UUID (if Claude agent)
11. Print success message
```

### Session Initialization Sequence (Automatic)

**Critical User Journey**: After `agm session new --agent=claude` creates a tmux session and starts Claude, the InitSequence automatically executes initialization commands without user intervention.

**Test Coverage**: See `cmd/agm/new_init_sequence_test.go`

**Flow**:
```
1. Claude CLI starts in tmux pane
2. Wait for Claude prompt (❯) using capture-pane polling
   - Poll interval: 500ms
   - Timeout: 30 seconds
   - Detection: containsClaudePromptPattern("❯")
3. Send /rename command:
   - Command: `/rename <session-name>`
   - Purpose: Generate Claude UUID and set session name
   - Wait 5s for command completion
4. Send /agm:agm-assoc command:
   - Command: `/agm:agm-assoc <session-name>`
   - Purpose: Associate Claude UUID with AGM manifest
   - Triggers agm session associate which creates ready-file
5. Wait for ready-file signal:
   - File: ~/.agm/ready-<session-name>
   - Timeout: 60 seconds
   - Signals: agm associate binary completed
6. Wait for skill completion:
   - Detects Claude prompt return using capture-pane polling
   - Timeout: 10 seconds
   - Ensures: skill finished all output before returning control
7. Session initialization complete
   - User attached to session
   - Commands executed successfully
   - Session ready for interaction

ERROR HANDLING:
- Trust Prompt: Pre-authorized via additionalDirectories in settings.json
  - Directory added before Claude starts
  - No prompt appears (prevented, not answered)
- Timeout (30s): If Claude never appears:
  - Warning displayed to user
  - Session remains attached (not killed)
  - User can manually run `/rename` and `/agm:agm-assoc`
- Ready-file timeout (60s): If association fails:
  - Warning displayed
  - Session usable but UUID may be missing
  - User can run `agm sync` to fix

TECHNICAL IMPLEMENTATION:
- Uses capture-pane polling (not control mode)
- Both code paths (detached and in-tmux) use identical InitSequence
- Fixed bug (2026-02-17): startClaudeInCurrentTmux now uses InitSequence.Run()
- Proven approach from prompt_detector.go:WaitForClaudePrompt()
```

**Expected Behavior** (from BDD scenarios):
- ✓ Successful initialization completes within 90 seconds
- ✓ Session renamed to match tmux session name
- ✓ Session associated with AGM (ready-file created)
- ✓ Timeout handled gracefully (session remains accessible)
- ✓ Trust prompts handled via user input (no auto-answering)
- ✓ Parallel session creation works without race conditions

**Reference**:
- BDD Tests: `test/bdd/features/session_initialization.feature`
- Implementation: `internal/tmux/init_sequence.go`
- Architecture owner: `internal/tmux/init_sequence.go`

### Resume Session Flow (agm resume [identifier])

```
1. Resolve identifier:
   - Exact session name match
   - UUID prefix match
   - Tmux session name match
   - Fuzzy name match
   - Interactive picker (if no identifier)
2. Read manifest
3. Check lifecycle (error if archived)
4. Validate agent availability (warn if unavailable)
5. Check session health:
   - Worktree exists
   - Agent directories present
   - Tmux session state
6. Create/attach tmux session
7. Send cd command to worktree
8. Send agent resume command (with UUID/conversation_id)
9. Update manifest timestamp
10. Attach to tmux session
```

### Archive Session Flow (agm session archive [session-name])

**Storage Backend**: AGM lifecycle storage (`Dolt` in production; isolated
SQLite when `AGM_DB_PATH` is set by a named test environment)

```
1. Connect to Dolt storage adapter
2. Resolve session identifier through the shared ops storage contract
3. Check if already archived (error if yes)
4. Run shared supervisor, active-pane, completion-verification, and pending-
   delegation guards
   - Active sessions MUST use --async flag (error if omitted)
   - Stopped sessions do NOT use --async (error if included)
5. For stopped sessions, call ops.ArchiveSession immediately
6. For active sessions with --async:
   - preflight through ops.ArchiveSession without mutating
   - require the detached agm-reaper binary to prove the same embedded VCS revision and clean or dirty provenance as agm
   - wait for the exact detached child to acknowledge revision validation and durable log initialization before reporting success
   - spawn agm-reaper with force/keep-sandbox/outcome options preserved
   - mark lifecycle=reaping before stopping the pane
   - after pane death, call ops.ArchiveSession for the final transition
7. For --all, select inactive candidates and call ops.ArchiveSession once per
   candidate; aggregate success/failure counts without direct storage updates
8. The shared operation stamps the outcome, updates lifecycle storage, reports
   external provider archive outcomes, and performs cleanup
9. Print success message with restore instructions
```

### Cross-Harness Association Flow (agm session associate [session-name])

```
1. Resolve existing session by ID, name, or tmux session in Dolt/YAML.
2. Determine target harness:
   - Existing manifest harness wins.
   - `--harness auto` infers from live tmux pane commands.
   - Explicit `--harness claude-code|codex-cli|agy|opencode-cli|pi-cli|gemini-cli` is accepted.
   - Omitted harness defaults to `claude-code` for backward compatibility.
3. For Claude Code:
   - Detect or accept the Claude UUID.
   - Persist the UUID and create the ready-file signal.
4. For Codex, AGY, Gemini, and OpenCode:
   - Do not require or detect a Claude UUID.
   - Ensure the Dolt session record exists when `--create` is set.
   - Persist harness, tmux session name, workspace, and working directory metadata.
   - Create the ready-file signal.
```

**Invariant**: `/agm:agm-assoc` is a cross-harness command. It invokes
`agm session associate ... --harness auto`; Codex/AGY/Gemini/OpenCode association
must not fail only because no Claude UUID exists.

`agm session archive-ui` is intentionally outside this flow: it reconciles
Claude desktop/UI records through `ops.ArchiveUISessions` per ADR-026 and never
mutates AGM session lifecycle storage.

### Doctor Health Check Flow (agm admin doctor)

```
STRUCTURAL CHECKS (always performed):
1. Check Claude installation (history.jsonl exists)
2. Check tmux installation (tmux version)
3. List all manifests
4. Check for duplicate sessions (old vs new naming)
5. Check for UUID conflicts (multiple sessions same UUID)
6. Check for empty UUIDs
7. Check for orphaned directories
8. Check for invalid manifests

IF --validate flag:
  FUNCTIONAL CHECKS (per session):
  1. Attempt to resume session
  2. Classify resume errors
  3. Suggest fixes

  IF --apply-fixes flag:
    4. Auto-repair common issues:
       - Fix UUID conflicts
       - Remove orphaned directories
       - Repair invalid manifests
```

## Error Handling

### Error Categories

| Category | Handling | User Experience |
|----------|----------|-----------------|
| **Configuration Errors** | Print error + remediation steps | Exit code 1, clear instructions |
| **Session Not Found** | Fuzzy match suggestions or create prompt | Non-blocking, helpful suggestions |
| **Agent Unavailable** | Warning message + continue | Non-blocking, session still created |
| **Tmux Errors** | Detailed error + tmux diagnostics | Exit code 1, remediation steps |
| **Manifest Errors** | Backup + attempt repair | Auto-fix or manual intervention |
| **Lock Conflicts** | Retry or abort | Clear message about concurrent access |

### Error Presentation Format

```
❌ Error: Failed to resume session

  Session 'my-session' could not be resumed because Claude UUID is missing.

  To fix this:
    • Run: agm session associate my-session
    • Or manually add UUID to manifest: ~/sessions/session-abc123/manifest.yaml
    • Then try resuming again
```

## UI/UX Patterns

### Interactive Session Picker

```
┌─────────────────────────────────────────────────┐
│ Select a session to resume:                     │
│                                                  │
│ > my-project (active)     Updated: 2 mins ago  │
│   feature-auth (stopped)  Updated: 1 hour ago  │
│   bugfix-123 (active)     Updated: 5 hours ago │
│                                                  │
│ [↑/↓: Navigate | Enter: Select | q: Quit]       │
└─────────────────────────────────────────────────┘
```

### Fuzzy Match "Did You Mean" Prompt

```
Session 'my-proj' not found.

Did you mean one of these?
  1. my-project
  2. my-project-v2
  3. new-project
  4. Create new session "my-proj"

Choice [1-4]:
```

### Progress Spinner

```
⠋ Creating session 'my-project'...
⠙ Initializing tmux session...
⠹ Starting Claude CLI...
⠸ Associating session UUID...
✓ Session created successfully!
```

## Security Considerations

### API Key Handling
- Never log API keys
- Read from environment variables only
- Validate presence (not value) in availability checks
- No API keys stored in manifests or config files

### File System Security
- Manifests stored with 0600 permissions
- Session directories with 0700 permissions
- Backup files inherit source permissions
- No sensitive data in logs

### Tmux Socket Security
- Use default tmux socket permissions
- Validate tmux session ownership
- Prevent unauthorized session access

## Versioning

### Version Information

```go
var (
    Version   = "3.0.0"
    GitCommit = "abc1234"
    BuildDate = "2026-02-11"
)
```

Printed on every command execution:
```
agm 3.0.0 (/usr/local/bin/agm)
```

### Compatibility Matrix

| AGM Version | Manifest Version | AGM Compatible | Agents Supported |
|-------------|------------------|----------------|------------------|
| 3.0.0 | v3 (writes), v2 (reads) | Yes | claude, gemini, gpt |
| 2.x.x | v2 | Yes | claude only |
| 1.x.x | v1 | N/A | claude only |

## Dependencies

### External Libraries
- `github.com/spf13/cobra` - CLI framework
- `github.com/charmbracelet/huh` - Interactive TUI components
- `github.com/google/uuid` - UUID generation
- `gopkg.in/yaml.v3` - YAML parsing (manifests)

### Internal Packages
- `internal/agent` - Agent abstraction
- `internal/manifest` - Manifest schema
- `internal/tmux` - Tmux integration
- `internal/session` - Session management
- `internal/discovery` - Session discovery
- `internal/detection` - UUID detection
- `internal/ui` - UI components
- `internal/fuzzy` - Fuzzy matching
- `internal/config` - Configuration management

## Acceptance Criteria

### V1.0 Completion Checklist
- [x] All session lifecycle commands implemented
- [x] Smart session resolution with fuzzy matching
- [x] Multi-agent support (claude, gemini, gpt)
- [x] Tmux integration with detached mode
- [x] Interactive TUI components
- [x] Session discovery and search
- [x] Health checks and diagnostics
- [x] Backup and restore operations
- [x] Workflow automation support
- [x] Session association (manual + auto-detect)
- [x] Backward compatibility with AGM
- [x] Comprehensive error handling
- [x] Accessibility features
- [x] Tab completion
- [x] Configuration management
- [x] Test infrastructure

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## References

- [AGM Architecture](ARCHITECTURE.md)
- [Agent Interface](../../internal/agent/interface.go)
- [Manifest Schema](../../internal/manifest/manifest.go)
- [Cobra CLI Framework](https://github.com/spf13/cobra)
- [Huh TUI Library](https://github.com/charmbracelet/huh)
