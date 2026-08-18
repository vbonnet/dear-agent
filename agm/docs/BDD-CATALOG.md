# BDD Scenario Catalog

Behavior-Driven Development (BDD) test scenarios for AGM. These scenarios make
AGM's SPEC invariants executable: each one is driven directly against the real
`internal/ops` and `internal/contracts` packages with `godog`.

---

## Overview

Scenarios are written in Gherkin and executed by `godog` via `TestFeatures`.
There is **no tag filter**: every `.feature` file under `test/bdd/features/`
runs on every build. A scenario whose steps are not implemented fails as
`undefined` rather than being skipped — so this catalog can never drift back
into listing tests that do not actually run.

**Location:** [`test/bdd/features/`](../test/bdd/features/)

**How to run:** See [test/bdd/README.md](../test/bdd/README.md)

> History: an earlier, larger catalog listed a multi-agent adapter suite
> (session lifecycle, agent selection, conversation persistence, etc.) marked
> "passing". Those feature files were never wired to step definitions and never
> ran — they were deleted in the "end BDD limbo" cleanup. Only the
> SPEC-invariant features below remain, because only they are actually
> enforced.

---

## Feature Files

### AGY Saved-Session Discovery

**File:** [`agy_saved_session_discovery.feature`](../test/bdd/features/agy_saved_session_discovery.feature)

**Drives:** `agm/internal/agysession` cache-first metadata lookup and bounded
newest-first Antigravity log fallback.

**Key scenarios:**
- Native conversation IDs are validated as bounded safe path components before
  saved-session database or transcript lookup.
- Cache hits never enter provider log discovery.
- Log discovery inspects at most 257 directory entries, using the 257th only as
  an exhaustion sentinel and processing at most 256; it then orders regular
  candidates by modification time and limits scanning to the newest 64 files.
- Each candidate read is limited to 2 MiB; known-ID matches inside that budget
  remain valid, while latest-workspace lookup rejects a truncated prefix or a
  match in an older file after a truncated newer candidate; a post-scan probe
  also detects bytes appended during the bounded read.
- Directory-entry, candidate, or byte exhaustion is distinguishable from a
  complete miss, and oversized lines fail explicitly.
- Directory exhaustion retains the bounded candidates so a known-ID match can
  remain conclusive, while latest-workspace lookup rejects any match because
  an unprocessed entry could be newer.
- A log removed by provider rotation after enumeration is skipped as a stale
  snapshot during metadata collection or scan open, while other metadata,
  open, and scan failures remain explicit.

**Why this matters:** Import, association, and post-create metadata capture must
not inherit unbounded latency from a large or stale provider log directory.

### Declarative Runtime Guardrails

**File:** [`declarative_runtime_guardrails.feature`](../test/bdd/features/declarative_runtime_guardrails.feature)

**Drives:** harness and plugin manifests, GitHub automation and rulesets, AGM
contracts, schemas and schedules, deployment service definitions, workflow
configuration, and language-specific code-intelligence rules.

**Key scenarios:**
- Every declarative runtime directory has a co-located strict EARS
  specification.
- Every specification retains reciprocal executable BDD traceability.
- All active harnesses preserve these contracts across all supported model
  families.

**Why this matters:** Configuration controls executable behavior. Excluding it
from repository-wide governance would leave CI, plugin loading, protocols,
schedules, and deployment outside the same parity contract as Go code.

### Declarative Fixture Guardrails

**File:** [`declarative_fixture_guardrails.feature`](../test/bdd/features/declarative_fixture_guardrails.feature)

**Drives:** manifest and provenance testdata, golden configuration and agent
interactions, archived and corrupt session states, installer images, benchmark
baselines, configuration loaders, lint contexts, and canonical status fixtures.

**Key scenarios:**
- Every declarative fixture directory has a co-located strict EARS
  specification.
- Every specification retains reciprocal executable BDD traceability.
- All active harnesses preserve these contracts across all supported model
  families.

**Why this matters:** Fixtures define the boundary between accepted and
rejected behavior. Unspecified fixture drift can make tests pass against the
wrong contract just as readily as implementation drift.

---

### BDD Repository Root Portability

**File:** [`bdd_root_portability.feature`](../test/bdd/features/bdd_root_portability.feature)

**Drives:** shared BDD checkout discovery used by package, harness, hook,
workflow, Wayfinder, and SPEC coverage step definitions.

**Key scenarios:**
- Nested package execution resolves the nearest `go.mod` and `agm` ancestor.
- Root discovery does not depend on compiler source paths and remains valid
  for binaries built with `-trimpath`.

**Why this matters:** A BDD gate that only works with absolute build-time source
paths can silently fail in reproducible CI builds instead of enforcing parity.

### Trust Protocol

**File:** [`trust_protocol.feature`](../test/bdd/features/trust_protocol.feature)

**Drives:** `ops.TrustRecord` / `ops.TrustScore` / `ops.TrustHistory`

**Key scenarios:**
- Trust score is always clamped to `[0, 100]`.
- Base score for a new session is `50`.
- Trust events are append-only and chronologically ordered.
- `gc_archived` has zero score impact; `false_completion` is the heaviest penalty.
- Empty session names and invalid event types are rejected.

**Why this matters:** Orchestrator delegation decisions depend on trust scores
reflecting agent reliability within well-defined bounds.

---

### Scan Loop

**File:** [`scan_loop.feature`](../test/bdd/features/scan_loop.feature)

**Drives:** `ops.DefaultCrossCheckConfig` and the scan-loop SLO contracts.

**Key scenarios:**
- Auto-approve matches only the RBAC allowlist (`Read`/`Glob`/`Grep`, never
  `rm`/`sudo`).
- Well-known tmux sessions (`main`, `default`) are excluded from unmanaged checks.
- Health status escalates `healthy → warning → critical` by severity.
- Scan loop reads its thresholds from the SLO contracts (intervals, timeouts,
  capture depth, list limits).

**Why this matters:** The scan loop is the orchestrator's situational
awareness; its safety (allowlist) and cadence (SLO thresholds) must be pinned.

---

### AGM Supervision And Recovery Guardrails

**File:** [`agm_supervision_recovery_guardrails.feature`](../test/bdd/features/agm_supervision_recovery_guardrails.feature)

**Drives:** co-located SPEC coverage for conservative PR, process, worktree,
sentinel intake, configured-socket isolation, tmux inspection, and
verification-skip recovery policies.

**Key scenarios:**
- Every listed supervision and recovery package has a co-located `SPEC.md`.
- Every package SPEC points back to the executable guardrail feature.
- Sentinel discovery and lifecycle tests stay on the exact configured tmux
  socket rather than inspecting ambient user sessions.
- Nested AGM recovery commands inherit the exact configured socket instead of
  falling back to the ambient default server.
- Unknown cleanup evidence remains conservative rather than destructive.

---

### Stall Detection

**File:** [`stall_detection.feature`](../test/bdd/features/stall_detection.feature)

**Drives:** stall-type invariants and stall-detection SLO contracts.

**Key scenarios:**
- Permission-prompt stalls are `critical` severity.
- Error messages are normalized (paths/line numbers stripped) before counting.
- Detector thresholds come from the SLO contracts (permission timeout,
  no-commit timeout, error-repeat threshold).
- Exactly three stall types exist: `permission_prompt`, `no_commit`, `error_loop`.
- VROOM flow stalls escalate once after the threshold and reset when flow resumes.
- VROOM worker-health and ready-bead probes inherit cancellation and enforce timeouts.

**Why this matters:** The multi-agent system only makes forward progress if
stalled sessions are detected and recovered against agreed thresholds.

---

### AGM Product Surface Guardrails

**File:** [`agm_product_surface_guardrails.feature`](../test/bdd/features/agm_product_surface_guardrails.feature)

**Drives:** co-located SPEC coverage for AGM gateway middleware, plugin commands,
generated surface metadata, workflow-bus signaling, and accessible operator UIs.

**Key scenarios:**
- Every listed AGM product surface has a co-located `SPEC.md`.
- Every surface SPEC points back to the executable guardrail feature.
- UI accessibility and shared-registry boundaries remain explicit contracts.

---

### Harness Parity

**File:** [`harness_parity.feature`](../test/bdd/features/harness_parity.feature)

**Drives:** `agm/internal/agent` harness/model registry,
`agm/internal/launchparity` startup contracts, `agm/cmd/agm` current-pane
creation, and terminal state detection.

**Key scenarios:**
- Harness discovery exposes only name, version, and descriptive capabilities;
  required behavior is expressed through consumer-owned capability interfaces.
- Pure API delivery requires context-aware readiness and message delivery at
  compile time, while adapter constructors return concrete types and the finite
  discovery catalog has no duplicate mutable runtime registry.
- A Codex CLI composer pane is detected as `ready` only with an explicit empty
  cursor on both the initial and post-turn forms.
- An idle Codex composer allows direct delivery.
- A typed draft or Unicode collapsed-paste chip remains queued even when the
  normal Codex model footer is visible.
- A stale Codex composer followed by newer shell output remains queued.
- A newer tail-owned initial composer remains ready after stale post-turn
  footer history from a prior Codex process.
- A Codex trust prompt is queued rather than treated as a sendable prompt.
- A Codex executable-hook review selector fails startup promptly with explicit
  operator guidance, is classified as review-required, and receives no
  automated input; a later live composer supersedes retained selector text.
- The top-level new command routes in-tmux, non-detached Claude, Codex,
  OpenCode, Pi, and deprecated Gemini creation into the current pane and queues
  canonical launch commands without an impossible wait behind the AGM process
  that owns the pane; Codex also validates credentials and its executable,
  Pi uses its managed canonical launch contract, while Claude's SessionStart
  hook persists the conversation UUID after the queued launch begins.
- Shared creation requires process and composer readiness before registration
  or startup-prompt delivery even when a CLI or MCP surface runtime owns the
  launch. Prompt-free current-pane Claude, Codex, OpenCode, Pi, and deprecated
  Gemini creation explicitly defer readiness until the foreground AGM process
  exits because each command is queued behind that process.
- Shared tmux sends serialize exact-pane readiness with delivery under one
  mutation boundary, and MCP creation atomically revalidates the harness and
  composer after registration immediately before delivering its startup prompt.
  A concurrent sender or readiness change cannot reuse the earlier proof. A
  measured pasted-content marker binds its complete payload, so prompt-like
  glyphs inside that payload cannot displace the composer anchor that owns it.
- Pure API single-send preflight resolves the registered delivery surface
  before any tmux probe, while both single-recipient and fan-out delivery
  restore the session's persisted model, storage locator, endpoint, and Azure
  settings without persisting credentials. Every sequential fan-out recipient
  receives a fresh finite deadline that still inherits caller cancellation.
  Stable-lock acquisition, reconstruction, and readiness use a separate
  bounded preflight context; the completed-turn phase then retains the
  adapter's complete provider budget. Targeted reconstruction loads only the
  requested session and cannot be delayed or failed by unrelated session
  directories.
  Under the same stable session-ID
  boundary as archive, delivery reloads lifecycle before adapter construction,
  rejects reaping and archived sessions, and uses a provider-appropriate wait, so archive linearizes before or after
  a bounded completed turn. Lock acquisition and provider work honor caller
  cancellation; direct adapter callers retain context-aware store-level
  serialization and a finite provider ceiling; completion errors, cancellation,
  and timeout leave history unchanged. Delivery otherwise fails closed unless
  adapter status is active or idle. A successful API send does not run tmux
  state resolution or persist `OFFLINE` for the pane-less session.
- Clearing API history reloads and preserves the current reconstruction
  metadata while atomically emptying only messages under the store-level
  transaction lock, including updates made by another process. Completed-turn
  commits reload the same metadata, and title, directory, and runtime-setting
  writers serialize on that lock and apply only their requested field.
- OpenAI-compatible history reload accepts valid JSONL records larger than the
  standard scanner token limit. Conversation import converts the parsed batch
  once and persists it with one history transaction, while an empty import
  performs no history transaction.
- Shared startup readiness honors its total deadline while a slow launch
  wrapper still owns the pane, but fails promptly if an already-observed
  harness process later stops.
- Shared readiness rejects a retained Claude prompt followed by current
  working output, recognizes styled Claude ghost placeholders as empty without
  accepting unstyled human drafts, requires structural tail-owned Gemini and
  OpenCode composers rather than generic glyphs or borders, requires AGY's bare
  prompt to own the tail, requires Pi's latest managed state to be ready rather
  than stale readiness followed by work, and distinguishes harness-specific
  Node launch arguments from unrelated background Node descendants. Permission,
  onboarding, model-upgrade, and survey prompts block only while their UI owns
  the tail; resolved dialogs and ordinary Allow/Deny output before a newer
  composer do not. Liveness, styled capture, and delivery stay pinned to one
  resolved pane ID even if session focus changes. Legacy AGY manifest names
  normalize to canonical `agy`, and the `pi` alias normalizes to canonical
  `pi-cli`, before shared send readiness.
- MCP AGY's native onboarding wait remains an unverified transition; shared
  creation still proves the live AGY process and tail-owned composer before
  registration or prompt delivery.
- Claude SessionStart association retries asynchronously across the detached
  registration race for longer than the maximum launch-readiness window and
  reports READY only after the payload UUID is persisted; the installable
  command hook is the sole repository source for that destination.
- Shared Gemini startup detects first-run directory trust, sends option `1`
  plus Enter to the exact pane that displayed the dialog, and still requires a
  later tail-owned Gemini composer before reporting readiness.
- Active harnesses are exactly Claude Code, Codex CLI, AGY, OpenCode, and Pi.
- Gemini CLI remains deprecated compatibility, not active parity.
- Active harness factories use canonical names.
- Active harness adapters satisfy the shared non-I/O conformance suite.
- The Codex factory uses `CodexCLIAdapter`, while the OpenAI API adapter
  remains independent of Codex tmux state.
- CLI and MCP lifecycle surfaces delegate to shared operations. Resume uses one
  stable-ID `internal/ops.ResumeSession` transaction; the CLI retains only
  identifier and prompt-file input, presentation, and post-operation attach.
- Harness parity requirement identifiers are unique.
- Active harness launch commands preserve native startup mode and persistence.
- Imported AGY conversations preserve unknown native-model provenance through
  the real storage adapter instead of acquiring Claude's legacy default, and
  cold resume clears the ambiguous model guessed by older import/association
  paths before command construction.
- AGY model-switch provenance requires a new exact confirmation, and root
  cancellation reaches AGY readiness stabilization and post-resume multiline
  readiness, direct and fan-out tmux delivery, and metadata association retry
  before delivery, mutation, or attach.
- Root cancellation also reaches Claude post-create prompt delivery and retry
  verification plus model, mode, and compaction slash-command readiness before
  later delivery, persistence, liveness validation, or attach work.
- The root command remains the sole process-signal owner, and continuous scan,
  watchdog, event-watch, stalled-session watch, and compaction-monitor loops
  consume its Cobra context and return promptly when canceled.
- Structured verify-result, work-request, and wake-loop sends preserve the same
  root context through multiline composer readiness and delivery.
- Resume rechecks the root context after metadata lookup and before tmux
  creation, command delivery, metadata updates, or warm-session attach.
- Every production resume entry, including last-session and bulk resume,
  acquires the stable session-ID lock before health or transaction reads and
  releases it after finalization but before an interactive attachment.
- A confirmed Codex prompt or a lost acknowledgement after the final Enter
  creates the irreversible success boundary; the latter preserves the pane and
  warns that work may have started. A paste positively proven to remain parked
  is still a delivery failure, and a failed send on an existing pane cannot
  hide a later attach error.
- Cold Codex resume retains tmux's server-local ID plus a random per-creation
  token, including when a later command in the tmux creation queue fails or ID
  output is lost while the exact random provisional name still exists;
  serializes concurrent resume attempts by stable session ID; persists a
  canonical name under an opaque cross-dialect ownership revision before
  optional prompt submission; treats ordinary prompt failures
  as transactional failures, compensates owned metadata before removing those
  exact identities, and preserves the ready tmux session when a concurrent
  writer supersedes metadata ownership or a post-write reload leaves
  compensation unproven. A stale full-session writer preserves the current
  name while applying unrelated fields and advancing the identity revision;
  every writer advances that revision so multiple stale snapshots stay unable
  to restore the old name, and rollback preserves the canonical tmux session
  without timestamp guesses. Activity-only finalization preserves the
  provisional revision; cancellation before or immediately after that touch
  restores the prior activity timestamp and canonical name before removing the
  exact created pane. A commit
  error is re-read against the complete prior and provisional revisions before
  cleanup proceeds. It also avoids killing a same-named or
  server-restart replacement. Reopening a
  persistent pre-revision SQLite test store upgrades its schema idempotently
  while preserving existing sessions before those lifecycle mutations run;
  compensation restores the prior activity timestamp with the prior name.
- Authoritative `agm session rename` updates both stored names through the
  exact revision it observed and holds the same stable-ID lifecycle lock as
  cold resume across all rename effects. A concurrent identity advance returns
  a conflict and compensates the already-moved tmux name even after caller
  cancellation, joining rollback failures. Stale broad writers preserve both
  current identity names while applying unrelated metadata. Lost tmux responses are reconciled through a
  server-local ID plus random option marker so name or ID reuse cannot adopt a
  replacement. Lost storage responses first fence the observed revision with
  a competing compare-and-swap before the prior identity can authorize rollback.
- Administrative parent-link and plan-session backfill repairs persist the
  parent and optional inherited display name atomically through the exact
  identity revision they read, advance that revision on success, and surface a
  stale writer as a conflict instead of claiming an unapplied repair succeeded.
- Once a transactional Codex resume prompt is submitted, or its final Enter
  loses an acknowledgement after tmux received the request, later caller
  cancellation cannot report a retryable failure that would duplicate work.
- Final creation liveness validation derives from the root context and rechecks
  cancellation before title update, attach, or detached-success reporting.
- AGY feedback survey handling dismisses once and recognizes the subsequent
  composer even while stale survey text remains in captured pane history;
  downstream state, direct-delivery, and idle predicates use the same
  last-marker rule.
- The AGY adapter captures provider-native conversation identity before fresh
  create succeeds, fails before tmux mutation when its pre-create identity
  snapshot is unreadable or incomplete, preserves known model provenance on
  cold resume, and omits a model override when an imported conversation's
  native selection is unknown.
- Fresh AGY startup prompts bootstrap lazy provider identity under the shared
  workspace lock before registration, remain out of process arguments, and are
  marked consumed so CLI, MCP, adapter, and completion paths deliver them only
  once; missing prompts and bootstrap failures fail before durable success.
- AGY direct, fan-out, queued-daemon, structured, and fresh-startup message
  surfaces preserve attribution plus multiline bodies as one bracketed native
  composer submission while retaining legacy paste behavior for other harnesses.
- AGY creation normalizes relative workspaces and shares cancellation-aware
  native identity serialization across CLI, MCP, and adapter lifecycle paths;
  launch, resume, and history reject unsafe provider identifiers before
  external mutation or path lookup.
- AGY cold resume distinguishes a restartable bare shell from a pane containing
  another live harness and never injects its command into the latter.
- AGY adapter create and cold resume require native readiness, roll back tmux
  sessions created by a failed operation, and use exact AGY process and native
  transcript truth for status and history.
- AGM runtime helper commands keep co-located SPEC coverage.
- AGM production Go sources use the single `session.RealTmux` local-runtime
  type, expose no parallel manager runtime, and retain its compile-time safety
  capability proofs.
- AGM cleanup and process support packages keep co-located SPEC coverage.
- Supported model families include Anthropic, OpenAI, Gemini, GLM, DeepSeek,
  Nemotron, and Qwen.

**Why this matters:** AGM's delivery contract is harness-neutral. Different
terminal chrome, adapter names, and model aliases must still route through one
shared registry instead of harness-specific assumptions.

### Agent Selection Guardrails

**File:** [`agent_selection_guardrails.feature`](../test/bdd/features/agent_selection_guardrails.feature)

**Drives:** `agm/internal/agents` SPEC coverage for AGENTS.md keyword-based
harness routing compatibility.

**Key scenarios:**
- Agent selection packages keep co-located SPEC coverage.

**Why this matters:** AGM still carries a legacy AGENTS.md keyword-routing
path. Its fallback and first-match behavior must remain explicit while the
newer harness and model-family parity registries evolve.

### Model Family Parity

**File:** [`model_family_parity.feature`](../test/bdd/features/model_family_parity.feature)

**Drives:** `agm/internal/agent` model-family defaults and
`pkg/llm/provider` resolver/factory routing.

**Key scenarios:**
- OpenRouter-hosted GLM, DeepSeek, Nemotron, and Qwen model identifiers resolve
  through the OpenRouter provider family.
- AGM model-family defaults for the priority open-model families resolve through
  the lower-level LLM provider resolver.
- The provider factory constructs an OpenRouter provider when API-key
  authentication is available.
- OpenRouter capabilities advertise the priority model-family defaults.

**Why this matters:** Model-family parity must be executable below the AGM
alias registry. The default routes for the new families should reach a concrete
provider instead of stopping at documentation or harness-only aliases.

### LLM Runtime Guardrails

**File:** [`llm_runtime_guardrails.feature`](../test/bdd/features/llm_runtime_guardrails.feature)

**Drives:** `pkg/llm/auth`, `pkg/llm/config`, `pkg/llm/delegation`, and
`pkg/llm/router` SPEC coverage.

**Key scenarios:**
- LLM runtime packages keep co-located SPEC coverage.
- LLM runtime package SPECs point back to their executable BDD feature.

**Why this matters:** Model-family parity depends on lower-level auth,
configuration, delegation, and role-routing contracts remaining explicit across
providers instead of becoming incidental behavior in individual tests.

### Sandbox Provider Guardrails

**File:** [`sandbox_provider_guardrails.feature`](../test/bdd/features/sandbox_provider_guardrails.feature)

**Drives:** `internal/sandbox/bubblewrap`, `internal/sandbox/apfs`,
`internal/sandbox/gvisor`, `internal/sandbox/overlayfs`, and
`wayfinder/pkg/sandbox` SPEC coverage.

**Key scenarios:**
- Sandbox provider packages keep co-located SPEC coverage.
- Sandbox provider package SPECs point back to their executable BDD feature.

**Why this matters:** Sandbox behavior is a permissions boundary for AGM and
Wayfinder. Provider-specific isolation, secrets, cleanup, and path-resolution
contracts need executable traceability without requiring privileged mounts in
BDD.

### Instruction Parity

**File:** [`instruction_parity.feature`](../test/bdd/features/instruction_parity.feature)

**Drives:** root instruction entrypoints and `internal/instructions`.

**Key scenarios:**
- `CLAUDE.md`, `GEMINI.md`, `CODEX.md`, `AGY.md`, and `OPENCODE.md` import
  `AGENTS.md` first.
- Harness-specific files do not duplicate shared policy sections.

**Why this matters:** The shared policy is the source of truth; model-specific
entrypoints should only add model/harness-specific guidance.

### Hook Parity

**File:** [`hook_parity.feature`](../test/bdd/features/hook_parity.feature)

**Drives:** repository hook manifests and `internal/hookparity`.

**Key scenarios:**
- Claude Code, Codex CLI, AGY, OpenCode, and Pi expose the required PreToolUse
  guardrails.
- Stop and SubagentStop feedback hooks are configured.
- Non-Claude harnesses expose Beads lifecycle hooks through their native hook
  manifests.
- The repository post-merge hook exposes its lifecycle safeguards.

**Why this matters:** Safety and dogfooding guardrails must travel with every
hook-capable harness, not only Claude Code.

### Permission Parity

**File:** [`permission_parity.feature`](../test/bdd/features/permission_parity.feature)

**Drives:** `agm/internal/permissionparity` and session manifest permission
policy persistence.

**Key scenarios:**
- Every active harness has a permission policy target.
- Resolved permission policy includes default and profile permissions.
- The manifest records the resolved policy as the shared source of truth.

**Why this matters:** Harnesses have different native permission features, so
AGM must preserve one durable policy contract across them.

### Quota Parity

**File:** [`quota_parity.feature`](../test/bdd/features/quota_parity.feature)

**Drives:** `agm/internal/quotaparity`, model-family defaults, and pricing
coverage policy.

**Key scenarios:**
- Every active harness has explicit context, cost, rate-limit, persistence, and
  degradation surfaces.
- Every supported model family has priced coverage or an explicit unpriced
  policy.

**Why this matters:** Quota monitoring must degrade honestly instead of falling
back to Claude-specific or Opus-specific assumptions.

### Quota Monitoring Guardrails

**File:** [`quota_monitoring_guardrails.feature`](../test/bdd/features/quota_monitoring_guardrails.feature)

**Drives:** cost tracking, Claude Code usage monitoring, and CLI usage tracker
SPEC coverage.

**Key scenarios:**
- Quota monitoring packages keep co-located SPEC coverage.
- Quota monitoring package SPECs point back to their executable BDD feature.

**Why this matters:** Quota parity policy is only useful if the concrete
pricing, budget, transcript-scan, burn-rate, and usage-log implementations
remain specified and executable.

### MCP Parity

**File:** [`mcp_parity.feature`](../test/bdd/features/mcp_parity.feature)

**Drives:** `agm/internal/mcpparity`, MCP tool schemas, and the shared ops
registry.

**Key scenarios:**
- Every active harness has an MCP create-session surface.
- MCP create-session uses shared harness/model validation.
- Deprecated Gemini compatibility remains detectable as compatibility.
- Lifecycle operations are exposed through the MCP operation registry.
- MCP server startup guards fail loudly when workspace or database access is missing.

**Why this matters:** MCP clients should call the same harness-neutral session
contract as the CLI rather than receiving a Claude-only subset.

### MCP Command Guardrails

**File:** [`mcp_command_guardrails.feature`](../test/bdd/features/mcp_command_guardrails.feature)

**Drives:** `cmd/dear-agent-mcp` and `cmd/recommendation-mcp` SPEC coverage.

**Key scenarios:**
- Top-level MCP command packages keep co-located SPEC coverage.
- MCP command package SPECs point back to their executable BDD feature.

**Why this matters:** The standalone MCP binaries are client-facing integration
points. Their workflow, source, recommendation, and backlog contracts need the
same executable traceability as AGM and Engram MCP surfaces.

### Marketplace Parity

**File:** [`marketplace_parity.feature`](../test/bdd/features/marketplace_parity.feature)

**Drives:** `.dear-agent/marketplace.json`, `.claude-plugin/marketplace.json`,
and `agm/internal/marketplaceparity`.

**Key scenarios:**
- The neutral marketplace catalog is valid.
- Claude's native marketplace mirror matches the neutral catalog.
- Published plugin assets exist for declared capabilities.
- Every active harness has an explicit discovery surface.

**Why this matters:** Claude may have native plugin marketplace support, but
other harnesses still need a neutral discovery path.

### Engram Parity

**File:** [`engram_parity.feature`](../test/bdd/features/engram_parity.feature)

**Drives:** `agm/internal/engramparity`, manifest Engram metadata, and ops
discovery.

**Key scenarios:**
- Every active harness has an Engram injection surface.
- Engram metadata is persisted through harness-neutral manifest fields.
- Engram metadata remains discoverable through shared ops.

**Why this matters:** Memory retrieval should not depend on Claude-only state or
native memory APIs.

### Wayfinder Parity

**File:** [`wayfinder_parity.feature`](../test/bdd/features/wayfinder_parity.feature)

**Drives:** `agm/internal/wayfinderparity`, Wayfinder assets, phase Engrams, and
MCP operations.

**Key scenarios:**
- Every active harness has Wayfinder discovery and execution surfaces.
- Wayfinder publishes SKILL, plugin, command, and MCP status surfaces.
- Wayfinder phase guidance resolves through harness-neutral Engrams.

**Why this matters:** Wayfinder is the planning layer for consequential work;
non-Claude harnesses need an executable path to the same workflow.

### Wayfinder Status Guardrails

**File:** [`wayfinder_status_guardrails.feature`](../test/bdd/features/wayfinder_status_guardrails.feature)

**Drives:** Wayfinder V2 status and phase dependency package SPEC coverage.

**Key scenarios:**
- Wayfinder core status packages keep co-located SPEC coverage.

**Why this matters:** Wayfinder V2 is the canonical planning workflow, so its
state schema and dependency graph need executable traceability alongside the
harness parity surfaces that consume it.

### Wayfinder Lifecycle Guardrails

**File:** [`wayfinder_lifecycle_guardrails.feature`](../test/bdd/features/wayfinder_lifecycle_guardrails.feature)

**Drives:** Wayfinder archive, workspace detection, and review package
SPEC coverage.

**Key scenarios:**
- Wayfinder lifecycle packages keep co-located SPEC coverage.
- Wayfinder lifecycle package SPECs point back to their executable BDD feature.

**Why this matters:** Wayfinder lifecycle safety depends on resumable project
detection, pre-rewind archive snapshots, workspace isolation, and risk-adaptive
review gates staying explicit and executable.

### Config Directory Parity

**File:** [`config_directory_parity.feature`](../test/bdd/features/config_directory_parity.feature)

**Drives:** repo-local harness config directories and
`agm/internal/configdirparity`.

**Key scenarios:**
- Active harnesses have `.claude`, `.codex`, `.agents`, `.opencode`, and `.pi`
  directory surfaces.
- Deprecated Gemini compatibility keeps `.gemini` available without making it
  active parity.

**Why this matters:** Harness-specific config belongs in explicit repo-local
surfaces so hook, instruction, and marketplace support can be audited.

### Harness Configuration Surface Guardrails

**File:** [`harness_config_surface_guardrails.feature`](../test/bdd/features/harness_config_surface_guardrails.feature)

**Drives:** `.agents`, `.claude`, `.codex`, `.gemini`, `.opencode`,
`.dear-agent`, and `.claude-plugin` SPEC coverage.

**Key scenarios:**
- Harness configuration and marketplace surface directories keep co-located SPEC coverage.
- Surface directory SPECs point back to their executable BDD feature.

**Why this matters:** The config-directory and marketplace parity tests validate
behavior, but the directories themselves are supported integration surfaces.
They need local contracts so harness-specific files do not drift into
undocumented Claude-only assumptions.

### Database Persistence Guardrails

**File:** [`db_persistence_guardrails.feature`](../test/bdd/features/db_persistence_guardrails.feature)

**Drives:** `agm/internal/db` SQLite schema, session persistence, and FTS5
search contracts.

**Key scenarios:**
- The embedded schema exposes session, message, escalation, FTS, and view
  surfaces.
- Session persistence preserves harness-neutral manifest metadata.
- FTS search applies harness filters across stored sessions.

**Why this matters:** AGM persistence must remain harness-neutral and searchable
instead of encoding Claude-only state or untested schema assumptions.

### Engram Knowledge Guardrails

**File:** [`engram_knowledge_guardrails.feature`](../test/bdd/features/engram_knowledge_guardrails.feature)

**Drives:** Engram retrieval, document, and corpus-callosum knowledge package
SPEC coverage.

**Key scenarios:**
- Engram knowledge packages keep co-located SPEC coverage.
- Engram knowledge package SPECs point back to their executable BDD feature.

**Why this matters:** Engram is the repo's shared knowledge layer for recall,
Wayfinder phase guidance, and cross-harness work. Its core retrieval,
document, and schema-sharing packages need executable traceability rather than
being covered only by package tests.

### Engram Analysis And Configuration Guardrails

**File:** [`engram_analysis_configuration_guardrails.feature`](../test/bdd/features/engram_analysis_configuration_guardrails.feature)

**Drives:** Engram Wayfinder analytics, layered configuration, provider-neutral
memory consolidation, and instruction detector SPEC coverage.

**Key scenarios:**
- Engram analysis and configuration packages keep co-located SPEC coverage.
- Engram analysis and configuration SPECs point back to executable BDD.

**Why this matters:** Telemetry, policy precedence, durable memory, and
violation detection are shared governance inputs. Their contracts must not
change based on the calling harness or selected model provider.

### Engram Core Context Guardrails

**File:** [`engram_core_context_guardrails.feature`](../test/bdd/features/engram_core_context_guardrails.feature)

**Drives:** Engram agent and project context detection, identity, episodic
memory, metacontext, profile, prompt-boundary, and scratchpad SPEC coverage.

**Key scenarios:**
- Engram core context packages keep co-located SPEC coverage.
- Engram core context package SPECs point back to their executable BDD feature.

**Why this matters:** Engram carries durable context across harness and model
families. Its trust sources, canonical `AGENTS.md` constitution, Wayfinder
memory integration, prompt boundaries, and execution sandbox need explicit,
executable contracts rather than package tests alone.

### Engram CLI Support Guardrails

**File:** [`engram_cli_support_guardrails.feature`](../test/bdd/features/engram_cli_support_guardrails.feature)

**Drives:** Engram CLI errors, security and validation helpers, enhanced slash
commands, and structured table formatting SPEC coverage.

**Key scenarios:**
- Engram CLI support packages keep co-located SPEC coverage.
- Engram CLI support SPECs point back to executable BDD.

**Why this matters:** Every harness ultimately reaches operator-facing command
surfaces. Their path boundaries, dynamic completion, and machine-readable
output must remain safe and deterministic.

### Engram Hook Guardrails

**File:** [`engram_hook_guardrails.feature`](../test/bdd/features/engram_hook_guardrails.feature)

**Drives:** Engram hook runtime, built-in checks, Bash validator, worktree
isolation, verification escalation, and denial analyzer SPEC coverage.

**Key scenarios:**
- Engram hook packages keep co-located SPEC coverage.
- Engram hook package SPECs point back to their executable BDD feature.

**Why this matters:** Hook parity is one of the repo's safety boundaries.
Engram's hook runtime and enforcement binaries must remain documented and
executable across harnesses instead of accumulating Claude-only assumptions.

### Engram Governance Runtime Guardrails

**File:** [`engram_governance_runtime_guardrails.feature`](../test/bdd/features/engram_governance_runtime_guardrails.feature)

**Drives:** Engram enforcement, guidance search, harness-effort generation,
and composed platform runtime SPEC coverage.

**Key scenarios:**
- Engram governance runtime packages keep co-located SPEC coverage.
- Engram governance runtime package SPECs point back to their executable BDD feature.

**Why this matters:** Enforcement and runtime composition must remain
harness-neutral, while effort configuration must preserve custom model-provider
identifiers even where harnesses expose different native configuration formats.

### Engram Security And Token Guardrails

**File:** [`engram_security_token_guardrails.feature`](../test/bdd/features/engram_security_token_guardrails.feature)

**Drives:** Engram plugin security, signing and revocation, token estimation,
and tokenizer registry SPEC coverage.

**Key scenarios:**
- Engram security and token packages keep co-located SPEC coverage.
- Engram security and token SPECs point back to executable BDD.

**Why this matters:** Plugin trust, provider credential handling, and token
budgets are cross-harness boundaries. Credential protection must cover
Anthropic, OpenAI, Gemini, and OpenRouter-routed model families uniformly.

### Engram Reflection And Storage Guardrails

**File:** [`engram_reflection_storage_guardrails.feature`](../test/bdd/features/engram_reflection_storage_guardrails.feature)

**Drives:** Engram reflection synthesis, project scanners, and the simple
provider's memory, session, context, and artifact SPEC coverage.

**Key scenarios:**
- Engram reflection and storage packages keep co-located SPEC coverage.
- Engram reflection and storage SPECs point back to executable BDD.
- Reflection storage documents the exact filename-safe session ID allowlist.
- Simple provider storage documents artifact path containment, temp-file flush
  before atomic replacement, and serialized filesystem operations.

**Why this matters:** Cross-session learning requires both safe signal
collection and a complete provider-neutral persistence implementation, not a
harness-specific or partially implemented memory path.

### Engram Observability Guardrails

**File:** [`engram_observability_guardrails.feature`](../test/bdd/features/engram_observability_guardrails.feature)

**Drives:** Engram provider-neutral token tracking, retrieval metadata, and
analytics dashboard SPEC coverage.

**Key scenarios:**
- Engram observability packages keep co-located SPEC coverage.
- Engram observability SPECs point back to executable BDD.

**Why this matters:** Quota and quality comparisons require normalized evidence
across Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen routes
rather than a Claude-only accounting path.

### Local Development Guardrails

**File:** [`local_development_guardrails.feature`](../test/bdd/features/local_development_guardrails.feature)

**Drives:** safe local development wrapper SPEC coverage for `safe-push`,
`safe-pr`, `safe-merge`, `safe-rebase`, `safe-unlock`, and their internal
policy packages.

**Key scenarios:**
- Safe local development command packages keep co-located SPEC coverage.
- Safe local development internal policy packages keep co-located SPEC coverage.

**Why this matters:** Agents must use audited wrappers for push, PR, merge,
rebase, and stale-lock cleanup instead of raw mutation commands.

### Workflow Tooling Guardrails

**File:** [`workflow_tooling_guardrails.feature`](../test/bdd/features/workflow_tooling_guardrails.feature)

**Drives:** workflow support command SPEC coverage for `agm-worktree-audit` and
`resolve-review-threads`.

**Key scenarios:**
- Workflow tooling command packages keep co-located SPEC coverage.

**Why this matters:** Agent cleanup and review-thread workflows need audited
command contracts before they are used in local development and merge gates.

### Workflow Command Guardrails

**File:** [`workflow_command_guardrails.feature`](../test/bdd/features/workflow_command_guardrails.feature)

**Drives:** workflow command SPEC coverage for `workflow-run`,
`workflow-status`, `workflow-list`, `workflow-logs`, `workflow-cancel`,
`workflow-approve`, `workflow-audit`, `workflow-codemod`, `workflow-dev`,
`workflow-inspector`, `workflow-lint`, `workflow-migrate`, and
`workflow-roles`.

**Key scenarios:**
- Workflow command packages keep co-located SPEC coverage.
- Workflow command package SPECs point back to their executable BDD feature.

**Why this matters:** Workflow commands are the human and automation control
surface for persisted workflow execution. They need explicit contracts for
state changes, audit trails, migration, development loops, and machine-readable
output before parity work can rely on them.

### Workflow Package Guardrails

**File:** [`workflow_package_guardrails.feature`](../test/bdd/features/workflow_package_guardrails.feature)

**Drives:** workflow implementation package SPEC coverage for
`agm/internal/workflow`, `agm/internal/workflow/deepresearch`,
`pkg/workflow/codemod`, `pkg/workflow/dev`, and `pkg/workflow/roles`.

**Key scenarios:**
- Workflow implementation packages keep co-located SPEC coverage.
- Workflow implementation package SPECs point back to their executable BDD feature.

**Why this matters:** Workflow command parity is only enforceable if the
registry, role resolver, codemods, dev loop, and specialized workflow adapters
behind those commands have explicit contracts.

### Audit Package Guardrails

**File:** [`audit_package_guardrails.feature`](../test/bdd/features/audit_package_guardrails.feature)

**Drives:** audit support package SPEC coverage for `agm/internal/audit`,
`internal/driftaudit`, `pkg/audit/config`, and `pkg/audit/verifiers`.

**Key scenarios:**
- Audit support packages keep co-located SPEC coverage.
- Audit support package SPECs point back to their executable BDD feature.

**Why this matters:** Repo-wide parity governance depends on trustworthy audit
inputs: AGM session audits, durable drift evidence, audit plan config, and
verifier dispatch all need stable contracts.

### AGM Diagnostics Package Guardrails

**File:** [`agm_diagnostics_package_guardrails.feature`](../test/bdd/features/agm_diagnostics_package_guardrails.feature)

**Drives:** co-located SPEC coverage for AGM SLO contracts, benchmark evaluation,
logging, trace evidence, Git/worktree safety, quality baselines, and verification.

**Key scenarios:**
- Every listed diagnostics package has a co-located `SPEC.md`.
- Every package SPEC points back to the executable guardrail feature.
- Repository safety and evidence contracts remain test-enforced support surfaces.

---

### API And Gateway Package Guardrails

**File:** [`api_gateway_package_guardrails.feature`](../test/bdd/features/api_gateway_package_guardrails.feature)

**Drives:** `cmd/dear-agent-api`, `pkg/api`, `pkg/gateway`, and gateway adapter
SPEC coverage.

**Key scenarios:**
- API and gateway packages keep co-located SPEC coverage.
- API and gateway package SPECs point back to their executable BDD feature.

**Why this matters:** Workflow, HITL, audit, and run-triggering control
surfaces must remain harness-neutral across loopback, Tailscale, CLI, HTTP, and
future adapters instead of drifting into transport-specific behavior.

### AGM Conversation And Discovery Guardrails

**File:** [`agm_conversation_discovery_guardrails.feature`](../test/bdd/features/agm_conversation_discovery_guardrails.feature)

**Drives:** co-located SPEC coverage for AGM conversation formats, harness-specific
history adapters, UUID detection, orphan import, transcript context, and search.

**Key scenarios:**
- Every listed conversation and discovery package has a co-located `SPEC.md`.
- Every package SPEC points back to the executable guardrail feature.
- Claude-only storage details remain explicit adapters rather than shared contracts.
- Pi imports preserve provider-qualified native model provenance and leave the
  model override empty when the native transcript does not record one.

---

### AGM Runtime Package Guardrails

**File:** [`agm_runtime_package_guardrails.feature`](../test/bdd/features/agm_runtime_package_guardrails.feature)

**Drives:** AGM runtime support SPEC coverage for artifacts, backups,
capacity, compaction, deadlock detection, freshness, locks, monitoring,
reservations, state detection, and failure tracking.

**Key scenarios:**
- AGM runtime packages keep co-located SPEC coverage.
- AGM runtime package SPECs point back to their executable BDD feature.

**Why this matters:** AGM parity is not only command syntax. The operational
substrate behind every harness needs explicit contracts for preserving state,
avoiding unsafe concurrency, detecting interactive readiness, and recovering
from stale or wedged sessions.

### AGM Capacity Platform Detection

**File:** [`agm_capacity_platform.feature`](../test/bdd/features/agm_capacity_platform.feature)

**Drives:** the real `agm/internal/capacity` native memory detector on the
current Linux or macOS test host.

**Key scenarios:**
- Supported development platforms resolve native total and available memory.
- Total memory is positive, available memory is non-negative, and available
  memory never exceeds total memory.

**Why this matters:** `agm capacity` is an operator-facing safety surface. A
Linux-only `/proc` probe made the command unusable on macOS even though AGM's
session lifecycle and circuit breaker support both platforms.

### Observability Package Guardrails

**File:** [`observability_package_guardrails.feature`](../test/bdd/features/observability_package_guardrails.feature)

**Drives:** observability SPEC coverage for `cmd/jaeger-health`,
`cmd/otel-local`, `internal/metrics`, telemetry agent, telemetry analysis,
telemetry enrichment, telemetry error rendering, and `pkg/otelsetup`.

**Key scenarios:**
- Observability packages keep co-located SPEC coverage.
- Observability package SPECs point back to their executable BDD feature.

**Why this matters:** Quota monitoring, drift audits, local trace collectors,
agent launch telemetry, and Engram session summaries are only useful when every
harness and model family reports through documented, testable contracts.

### Plugin And Skill Package Guardrails

**File:** [`plugin_skill_package_guardrails.feature`](../test/bdd/features/plugin_skill_package_guardrails.feature)

**Drives:** plugin and skill governance SPEC coverage for
`engram/internal/plugin`, `pkg/plugin`, `pkg/skilllint`, and
`tools/skill-lint`.

**Key scenarios:**
- Plugin and skill packages keep co-located SPEC coverage.
- Plugin and skill package SPECs point back to their executable BDD feature.

**Why this matters:** Plugin marketplace and skill parity require stable
contracts for manifests, discovery, execution, integrity checks, EventBus
subscriptions, and cost-governed skill frontmatter.

### Source And Knowledge Package Guardrails

**File:** [`source_knowledge_package_guardrails.feature`](../test/bdd/features/source_knowledge_package_guardrails.feature)

**Drives:** `pkg/source`, source adapter backends, `pkg/papersearch`, and
`pkg/wikibrain` SPEC coverage.

**Key scenarios:**
- Source and knowledge packages keep co-located SPEC coverage.
- Source and knowledge package SPECs point back to their executable BDD
  feature.

**Why this matters:** Workflow durability, search, engram support, paper
retrieval, and wiki graph analysis all share this knowledge substrate. Adapter
and graph behavior must stay explicit across implementations rather than
drifting into backend-specific assumptions.

### Quality Command Guardrails

**File:** [`quality_command_guardrails.feature`](../test/bdd/features/quality_command_guardrails.feature)

**Drives:** `cmd/ears-lint`, `cmd/ears-to-bdd`, `cmd/test-affected`,
`cmd/repo-health`, `cmd/structural-health`, and `cmd/src-health` SPEC coverage.

**Key scenarios:**
- Repo quality command packages keep co-located SPEC coverage.
- Repo quality command package SPECs point back to their executable BDD feature.

**Why this matters:** These commands enforce EARS, BDD, affected-test, and repo
health contracts. They are part of the parity safety net and need the same
traceability as the features they guard.

### AGM Control Surface Guardrails

**File:** [`agm_control_surface_guardrails.feature`](../test/bdd/features/agm_control_surface_guardrails.feature)

**Drives:** `agm/internal/api`, `agm/internal/cli`,
`agm/internal/delegation`, `agm/internal/discovery`, `agm/internal/surface`,
`agm/internal/terminal`, `agm/internal/validate`, and `agm/cmd/agm` command-test
isolation.

**Key scenarios:**
- AGM control-plane packages keep co-located SPEC coverage.
- AGM control-plane package SPECs point back to their executable BDD feature.
- Cobra validation tests use fresh commands or restore every mutated flag and
  remain stable across repeated execution orders.

**Why this matters:** The AGM control plane ties CLI, MCP, status, discovery,
tmux terminal handling, validation, and delegation behavior together. These
packages need explicit contracts so parity work does not regress into
harness-specific or Claude-only assumptions. The CLI also keeps global Cobra
objects for production registration, so its tests must isolate command state
to avoid order-dependent false passes and failures.

### Context Management Parity

**File:** [`context_management_parity.feature`](../test/bdd/features/context_management_parity.feature)

**Drives:** `pkg/context` harness detection, explicit heuristic labeling,
model-family context windows, and provider-neutral compaction defaults.

**Key scenarios:**
- Every active harness resolves context usage for every supported model family.
- Fallback usage is visibly estimated and retains the selected model route.
- Every supported family has a positive registered context window.
- Every active harness rejects explicit counters outside the platform integer range.
- Every active harness selects competing nested counter sets deterministically.

**Why this matters:** Quota and compaction policy cannot be parity-complete when
non-Claude harnesses return not-implemented errors or silently inherit a Claude
summarizer model.

### Pi Custom Model Context

**File:** [`pi_custom_context.feature`](../test/bdd/features/pi_custom_context.feature)

**Drives:** `agm/internal/pisession` provider provenance and
`agm/internal/session` native Pi context-window selection from a bounded custom
model catalog.

**Key scenarios:**
- A managed Pi transcript preserves the provider-qualified custom model ID.
- AGM reports the latest native prompt footprint against the exact configured
  custom model context window.
- A built-in provider model outside AGM's static window table still honors
  Pi's topmost user override.
- A provider added after AGM's audited Pi release still honors an override for
  the exact provider-qualified route recorded in native history.
- An OpenRouter route with a nested vendor-qualified model ID retains its own
  route-specific native context window rather than a direct-provider default.
- A custom model ID remains opaque when it begins with its own provider name;
  AGM does not collapse the repeated provider segment during qualification.
- An explicit null custom context window is rejected instead of receiving the
  default reserved for an omitted field.
- An integral exponent-spelled context window resolves to the same exact token
  count that Pi uses, without accepting nearby fractional values.
- Provider-less legacy history rejects two matching providers even when their
  declared windows agree, rather than guessing a route from equal values.
- Credential command strings in Pi's model catalog remain inert data.

**Why this matters:** Custom providers are a supported Pi route. Using a static
fallback for their configured windows makes AGM's percentage disagree with Pi,
while evaluating unrelated credential configuration would cross the harness
permission boundary.

### Shared Runtime Policy Package Guardrails

**File:** [`shared_runtime_policy_guardrails.feature`](../test/bdd/features/shared_runtime_policy_guardrails.feature)

**Drives:** Co-located strict EARS contracts and route-neutral implementation
checks for shared configuration, backlog, code generation, code intelligence,
enforcement, evaluation, event, markdown, graceful-exit, and health packages.

**Key scenarios:**
- Every shared runtime policy package carries a reciprocal SPEC reference.
- Every package contract names all five active harnesses and seven model families.
- Production string literals do not embed a harness or model-family route.

**Why this matters:** Shared policy behavior must remain identical across caller
routes; hard-coded route defaults in these packages silently break parity above
the adapter layer.

### Agent Utility Parity

**File:** [`agent_utility_parity.feature`](../test/bdd/features/agent_utility_parity.feature)

**Drives:** Shared agent utility SPEC coverage, model-family cache policy,
cross-harness hook normalization, and harness-neutral session synchronization.

**Key scenarios:**
- Eleven shared utility packages carry co-located executable specifications.
- Every active harness and supported model family resolves a cache policy.
- Hook and synchronization boundaries preserve the selected route without
  inheriting Anthropic or Claude Code wire assumptions.

**Why this matters:** Utility packages sit below adapters and are reused by
every route; a provider-specific default here propagates across the codebase.

### Validation and Workspace Parity

**File:** [`validation_workspace_parity.feature`](../test/bdd/features/validation_workspace_parity.feature)

**Drives:** Co-located contracts and route-neutral implementation checks for
filesystem safety, content validation, VCS, versioning, and workspace state.

**Key scenarios:**
- Every validation and workspace package carries a strict EARS specification.
- Production string literals remain free of harness and model-family defaults.
- The complete four-harness by seven-family route matrix is exercised.

**Why this matters:** Validation and workspace policy is shared infrastructure;
route-specific defaults would produce different safety outcomes for identical work.

### Evaluation and Control Parity

**File:** [`evaluation_control_parity.feature`](../test/bdd/features/evaluation_control_parity.feature)

**Drives:** Benchmark orchestration, model-route preservation, Engram migration,
and Discord/GitHub human approval transport SPEC coverage.

**Key scenarios:**
- Every benchmark, migration, and HITL package carries a co-located SPEC.
- Every harness and model family preserves the selected benchmark model route.
- Engram tier migration remains independent of the invoking harness.

**Why this matters:** Comparative evidence and human approval decisions must
not change because a task entered through a different harness or model family.

### Root Lifecycle Command Guardrails

**File:** [`root_lifecycle_command_guardrails.feature`](../test/bdd/features/root_lifecycle_command_guardrails.feature)

**Drives:** PR babysitting, Bead closure and PR synchronization, merge audit,
and merge-loop command and policy package SPEC coverage.

**Key scenarios:**
- Every listed lifecycle package has reciprocal co-located SPEC traceability.
- Merge-loop repair sessions preserve all active harness routes.
- Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model routes are passed through AGM's shared validation path.

**Why this matters:** Repository automation must use safe wrappers and shared
AGM routing rather than embedding Claude-only credentials or model assumptions.

### Root Maintenance Command Guardrails

**File:** [`root_maintenance_command_guardrails.feature`](../test/bdd/features/root_maintenance_command_guardrails.feature)

**Drives:** burndown worker maintenance, merge velocity, PR linkification,
golden source recovery, post-merge hook installation, and chezmoi deployment
SPEC coverage.

**Key scenarios:**
- Every listed maintenance package has reciprocal co-located SPEC traceability.
- Burndown workers preserve every active AGM harness route.
- Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen routes reach AGM unchanged.

**Why this matters:** Host maintenance must preserve golden-source and safe-push
rules while remaining independent of a single harness or model provider.

### Root Safety Command Guardrails

**File:** [`root_safety_command_guardrails.feature`](../test/bdd/features/root_safety_command_guardrails.feature)

**Drives:** native Claude settings and OAuth adapters, deployment drift,
language-server pressure, log rotation, and shared filesystem-hook adapter SPEC
coverage.

**Key scenarios:**
- Every listed safety package has reciprocal co-located SPEC traceability.
- Claude-native credential/configuration tools declare their adapter boundary.
- Filesystem guards also point to the active cross-harness hook parity suite.

**Why this matters:** Native harness files are valid adapter details, but shared
permissions, hooks, and policy must remain in neutral contracts.

### Root Intelligence Command Guardrails

**File:** [`root_intelligence_command_guardrails.feature`](../test/bdd/features/root_intelligence_command_guardrails.feature)

**Drives:** deterministic backlog, code intelligence, source search, signal,
and trace-to-eval command SPEC coverage.

**Key scenarios:**
- Every listed intelligence package has reciprocal co-located SPEC traceability.
- Specs require shared schemas and deterministic behavior across harnesses and models.

**Why this matters:** Search, supervision, dispatch, and evaluation evidence
must not change meaning based on which model produced or consumed it.

### Root Operations Command Guardrails

**File:** [`root_operations_command_guardrails.feature`](../test/bdd/features/root_operations_command_guardrails.feature)

**Drives:** webhook, benchmark, Bumblebee, flywheel, retrospective, and VROOM
prompt generation SPEC coverage.

**Key scenarios:**
- Every listed operations package has reciprocal co-located SPEC traceability.
- Generated VROOM prompts preserve every active harness route.
- Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen worker routes remain selectable.

**Why this matters:** Autonomous dispatch prompts are control-plane artifacts;
they must enforce safety rules without hardcoding one harness or model.

### Internal Foundation Guardrails

**File:** [`internal_foundation_guardrails.feature`](../test/bdd/features/internal_foundation_guardrails.feature)

**Drives:** Shared baseline, benchmark, local CI, drift, EARS, atomic file,
log rotation, safety override, SQLite, and Engram tracking packages.

**Key scenarios:**
- Every internal foundation package keeps a co-located executable SPEC.
- High-risk override policy keeps one provider-neutral identity across the full
  supported harness and model-family matrix.
- The deterministic override floor cannot be weakened by a model classifier.

**Why this matters:** Shared host and persistence utilities sit beneath every
harness. Their behavior must remain explicit, traceable, and independent of the
agent or model route that invokes them.

### Session Protocol And Signal Guardrails

**File:** [`session_protocol_guardrails.feature`](../test/bdd/features/session_protocol_guardrails.feature)

**Drives:** A2A session transport and client behavior, Wayfinder acceptance
criteria, privacy-preserving agent traces, and project health signal packages.

**Key scenarios:**
- Every session-protocol and signal package keeps co-located SPEC coverage.
- A2A cards advertise the selected harness across all supported harness and
  model-family routes.
- A2A transport presentation remains independent of model-provider choice.

**Why this matters:** Delegation protocols, acceptance policy, observability,
and health signals are shared infrastructure. Hard-coded Claude presentation
or unredacted trace content would break parity or privacy for every caller.

### SPEC and BDD Coverage

**File:** [`spec_coverage.feature`](../test/bdd/features/spec_coverage.feature)

**Drives:** `internal/speccoverage`.

**Key scenarios:**
- Every parity-critical surface has a registered `SPEC.md`.
- Every parity-critical surface has an executable BDD feature.
- Every parity `SPEC.md` declares EARS requirements.
- Every parity `SPEC.md` passes strict EARS lint.
- Every parity `SPEC.md` references its executable BDD feature.
- Every parity BDD feature references its governing `SPEC.md`.
- Every registered parity `SPEC.md` has a completed audit marker.
- Every `*_parity.feature` file is registered in the coverage matrix.
- Changed production Go package directories carry a co-located `SPEC.md`.
- Changed production Go package `SPEC.md` files pass strict EARS lint.
- The actual checkout gives every implementation directory strict co-located
  SPEC and reciprocal executable BDD coverage across supported source formats.
- Every SPEC artifact, including doc-only and hidden policy contracts, retains
  strict EARS and reciprocal executable BDD traceability.

**Why this matters:** Parity work should not land as untraceable test-only or
doc-only changes. The coverage matrix keeps SPEC and BDD artifacts paired, the
diff-based package guard gives fast changed-package diagnostics, and the
actual-checkout gate prevents any implementation directory from remaining
outside strict SPEC and executable BDD enforcement.

### Visible Markdown Classification

**File:** [`markdown_visibility.feature`](../test/bdd/features/markdown_visibility.feature)

**Drives:** provider-neutral whole-document CommonMark classification used by
SPEC policy tools. It preserves source line alignment while excluding complete
indented-code, fenced-code, raw-HTML, and inline-comment ranges from normative
prose. Container-nested fenced blocks are exercised through the same shared
classifier rather than through a harness adapter.

**Key scenario:** Hidden CommonMark examples, including container-prefixed
fences, do not become normative requirements while following visible prose
retains its original line position.

### SPEC Audit Tooling Evidence Boundary

**File:** [`spec_governance_tooling.feature`](../test/bdd/features/spec_governance_tooling.feature)

**Drives:** focused `tools/specaudit` unit tests for the root-module command's
pinned inventory, validation, and offline HTML rendering behavior. It does not
exercise skill discovery, skill invocation, provider behavior, or maintainer
decisions.

The runner compares only build-selected `TestGoFiles` and `XTestGoFiles` at its
pre-test and post-test observation points to validate exact selected test
declarations. This check does not cover or make immutable production Go files,
module files, embed inputs, or dependencies. It also cannot detect a
mid-run swap that is restored before the post-test observation. The
implementation test source must already be trusted; the runner's bounded
process, environment, and cleanup controls reduce accidental leakage and
residue but are not a filesystem, network, or syscall sandbox.

Each nested selection uses bounded `go test -json` output and is accepted only
when every requested exact top-level name has exactly one `run` event and one
terminal `pass`, followed by a package pass. Missing, duplicate, malformed,
skipped, failed, out-of-order, or unrequested test events fail the BDD step even
if the child process exits zero. Before each launch, the runner revalidates the
captured Go and Git executable identities, ownership, and canonical ancestry.
The default contract rejects world-writable executables and ancestors plus
group-writable ancestors owned by another user; current-user-owned
group-writable ancestors remain admissible for standard package-manager
layouts. Git always uses that strict contract.

GitHub-hosted Ubuntu is one explicit compatibility boundary because its image
deliberately makes `/opt/hostedtoolcache` writable. The Go executable may use a
runner-context fallback only when GitHub-defined runner variables name
the `github-hosted` Linux/X64 environment, the image metadata is well formed,
`RUNNER_TOOL_CACHE` resolves to exactly `/opt/hostedtoolcache`, no `GOROOT`
override is present, and the compiled runtime version and GOROOT select exactly
that tool cache's matching `go/<version>/x64/bin/go`. The runner retains the Go
file and GOROOT identities, hashes the bounded Go executable, and revalidates
the complete context, identities, and digest before every launch. This is a
runner-context compatibility gate, not provider attestation: the writable
GOROOT toolchain remains a trusted input and ordinary in-place and pre-exec
races remain. Its task root is canonical, absolute, current-user-owned, and
identity-bound; cleanup refuses a missing, replaced, wrong-owner, or wrong-mode
root. None of these controls sandbox trusted test code.

**Key scenarios:**
- Exact pinned Git-object inventory ignores dirty worktree content.
- Duplicate IDs, exact bodies, shared BDD paths, identical files, and harness
  terminology remain deterministic review leads rather than semantic verdicts.
- Missing and nonreciprocal SPEC/BDD links remain visible diagnostics.
- Positive findings must match pinned Git-resolved evidence, identify one
  shared reciprocal feature across current owners, and carry a structurally
  complete ownership-preservation proposal that remains pending maintainer
  approval; forged evidence and unsafe positive verdicts fail validation.
- Offline HTML remains escaped, self-contained, and bounded.
- The runner observes exact selected test declarations before and after their
  execution without claiming complete build-input integrity or immutability.
- The runner requires machine-readable evidence that every requested exact
  top-level test ran once and passed; process exit status alone is insufficient.
- Successful inventory, validation, and rendering emit their expected stdout
  while preserving tracked bytes and status, index identity and content,
  `HEAD`, refs, and relevant SPEC and feature bytes in the target repository.

**Why this matters:** The audit command cannot credibly supply review evidence
if dirty bytes can alter pinned evidence, lexical similarity becomes a merge
verdict, reciprocal BDD drift is hidden, or a report mutates product state
before maintainer review. The focused checks are not evidence that a skill is
discoverable, that a maintainer accepted a recommendation, or that every Go
build input remained unchanged while the selected tests ran.

### VROOM Runtime Guardrails

**File:** [`vroom_runtime_guardrails.feature`](../test/bdd/features/vroom_runtime_guardrails.feature)

**Drives:** `pkg/vroom/decisiontrail`, `pkg/vroom/escalation`,
`pkg/vroom/goplswatch`, `pkg/vroom/supervisor`, and `pkg/vroom/vroom` SPEC
coverage and provider-neutral runtime behavior.

**Key scenarios:**
- VROOM runtime packages keep co-located SPEC coverage.
- Every supported harness preserves every supported model-family route.
- Unspecified worker models remain provider-selected instead of defaulting to
  an Anthropic model.

**Why this matters:** VROOM is shared coordination infrastructure. Its worker
dispatch, adjudication, decision trail, and process-pressure behavior must not
silently pin Claude when the active harness or model provider is different.

### Wayfinder V2 Command Guardrails

**File:** [`wayfinder_v2_command_guardrails.feature`](../test/bdd/features/wayfinder_v2_command_guardrails.feature)

**Drives:** canonical Wayfinder root/session commands, V2 status defaults,
roadmap tasks, validator gates, stop-hook enforcement, and rewind retrospective
persistence.

**Key scenarios:**
- Changed command packages keep co-located strict EARS specifications.
- Built root help exposes the nine canonical phases and no retired executors.
- Normal session commands parse schema 2.0 status only.
- The active runtime cannot reintroduce retired phase identifiers or migration
  commands.

**Why this matters:** Wayfinder is the repository's planning gate. A numeric-phase default
or hidden legacy executor makes phase enforcement ambiguous and violates the
canonical V2 and broken-windows policies.

### Wayfinder Internal Package Guardrails

**File:** [`wayfinder_internal_package_guardrails.feature`](../test/bdd/features/wayfinder_internal_package_guardrails.feature)

**Drives:** canonical command entrypoint, Beads, configuration, Git, history,
lint context, telemetry, tracker, project-discovery, and preset package
specifications.

**Key scenarios:**
- Every surviving Wayfinder support package keeps a co-located strict EARS SPEC.
- The exact canonical nine-phase AST contract is preserved across all four
  supported harnesses and seven model families.

**Why this matters:** Wayfinder support code participates in the same planning
gate regardless of which harness or model executes a task. Structural package
coverage prevents tooling helpers from becoming ungoverned paths.

### Developer Tool Package Guardrails

**File:** [`developer_tool_package_guardrails.feature`](../test/bdd/features/developer_tool_package_guardrails.feature)

**Drives:** Session skill extraction, Git hook integration tests, CI drift and
dead-link checks, devlog command and support packages, the stop quality guard,
and schema-registry MCP, query, persistence, and validation packages.

**Key scenarios:**
- Every covered developer-tool package keeps a co-located, audited SPEC.
- Shared developer-tool contracts remain available across the canonical four
  harnesses and seven supported model families.

**Why this matters:** Repository automation is part of the agent control plane.
Provider-specific assumptions in these tools can block one harness even when
the product runtime itself claims parity.

---

### Legacy Specification Strictness Guardrails

**File:** [`legacy_spec_strictness_guardrails.feature`](../test/bdd/features/legacy_spec_strictness_guardrails.feature)

**Drives:** EARS and BDD convergence for maintained legacy AGM, Engram,
Wayfinder, shared-package, and developer-tool specifications.

**Key scenarios:**
- Every selected legacy specification passes strict EARS lint.
- Every selected specification links reciprocally to its executable feature.
- The strictness contract is invariant across all active harnesses and all
  supported model families.

**Why this matters:** Historical prose specifications are not enforceable until
their maintained requirements can be linted and exercised through the same BDD
surface as newly governed packages.

---

### Legacy NFR EARS Guardrails

**File:** [`legacy_nfr_ears_guardrails.feature`](../test/bdd/features/legacy_nfr_ears_guardrails.feature)

**Drives:** strict EARS conversion for maintained Ecphory, ranking, monitoring,
telemetry, and Definition of Done functional and non-functional requirements.

**Key scenarios:**
- Every converted requirement retains a stable canonical identifier.
- Every converted specification passes strict EARS lint.
- Converted contracts remain enforced across all harness and model routes.

**Why this matters:** Uppercase prose `SHALL` bullets looked normative but
could not be consumed by the repository's deterministic SPEC gate. Canonical
EARS makes those existing promises executable without discarding them.

---

### Legacy Specification BDD Linkage Guardrails

**File:** [`legacy_spec_bdd_linkage_guardrails.feature`](../test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature)

**Drives:** reciprocal executable BDD linkage for maintained strict legacy
specifications across AGM, VROOM, Wayfinder, audit, deployment, telemetry,
workflow, and safety surfaces.

**Key scenarios:**
- Every listed specification passes strict EARS lint.
- Every listed specification and feature reference each other.
- Linkage remains enforced across all active harness and model-family routes.

**Why this matters:** Strict requirements that are not connected to an
executable scenario can still drift unnoticed. Reciprocal linkage makes the
maintained contract discoverable and test-enforced.

### Cross-Language Implementation Guardrails

**File:** [`cross_language_implementation_guardrails.feature`](../test/bdd/features/cross_language_implementation_guardrails.feature)

**Drives:** repository wrappers, harness hooks, AGM and Engram shell/TypeScript
surfaces, migrations, infrastructure, test suites, and Wayfinder shell support.

**Key scenarios:**
- Every executable implementation directory has a co-located strict EARS
  specification.
- Every specification retains reciprocal executable BDD traceability.
- Claude Code, Codex, Antigravity, and OpenCode preserve the same contract
  across Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen families.
- End-to-end harness lookup retains exact CLI mappings while running under the
  macOS system Bash 3.2 baseline.

**Why this matters:** Repository-wide coverage cannot exclude behavior merely
because it is implemented outside a Go package. Hooks, installers, migrations,
infrastructure, and protocol services are part of the shipped control plane.

---

### Dangerous Override Governance

**File:** [`dangerous_override_governance.feature`](../test/bdd/features/dangerous_override_governance.feature)

**Drives:** the shared contract behind every escape hatch that switches off a
safety control — today the Codex hook-trust bypass and the admission brake
override.

**Key scenarios:**
- The dangerous-override package carries a co-located strict EARS specification
  with reciprocal executable traceability.

**Why this matters:** An override is meant to be an exception, and its failure
mode is becoming routine — the control it disables then dies without anyone
deciding that. Holding the contract to the same spec discipline as every other
package is what stops a new override kind from being added beside the gates
(reason, human approval, ledger, recurring audit) instead of through them.

---

### Test Support Package Guardrails

**File:** [`test_support_package_guardrails.feature`](../test/bdd/features/test_support_package_guardrails.feature)

**Drives:** strict co-located contracts for all remaining AGM, Engram, shared,
and Wayfinder test-support package boundaries.

**Key scenarios:**
- Every residual test and support package retains a co-located strict EARS SPEC.
- Every SPEC references the executable feature that enforces it.
- The complete support contract is validated across all five active harnesses
  and all seven supported model families.
- Live harness contracts use canonical guarded session and message commands.
- Trust hooks run only for trust scenarios, restore process environment, reuse
  shared Go caches, and remove only their exact owned temporary directory,
  including read-only module trees.
- Real Codex lifecycle tests own their source binary, short tmux socket,
  provider fixture, persisted state, and exact cleanup.
- Named test environments validate paths, use a private effective-user short
  root, activate owned legacy environments in place, and remove exact state.

**Why this matters:** Test infrastructure is an enforcement surface. Ungoverned
helpers and suites can silently skip harnesses, consume the wrong provider
credentials, touch host state, or stop executing while production code remains
green.

---

## Running

```bash
cd agm
make test-bdd          # godog feature tests (TestFeatures)
go test ./test/bdd/... # features + SPEC invariants + contract drift
```

CI runs this package on every PR via the root `ci.yml` "Build & Test" job
(`go test -race ./...`).

---

## Adding a scenario

See [test/bdd/README.md](../test/bdd/README.md#adding-a-scenario). In short: add
the scenario, implement every step in `steps/<name>_steps.go`, register the
step group in `main_test.go`, and run `go test ./test/bdd/...`. An
unimplemented step fails the build — that is the enforcement mechanism.

---

## Next steps

- **Run scenarios:** [test/bdd/README.md](../test/bdd/README.md)
- **Choose agent:** [AGENT-COMPARISON.md](AGENT-COMPARISON.md)
- **Troubleshoot:** [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
