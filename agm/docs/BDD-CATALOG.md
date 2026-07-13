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
sentinel intake, tmux inspection, and verification-skip recovery policies.

**Key scenarios:**
- Every listed supervision and recovery package has a co-located `SPEC.md`.
- Every package SPEC points back to the executable guardrail feature.
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

**Drives:** `agm/internal/agent` harness/model registry plus Codex terminal
state detection contracts.

**Key scenarios:**
- A Codex CLI composer pane is detected as `ready`.
- An idle Codex composer allows direct delivery.
- A Codex trust prompt is queued rather than treated as a sendable prompt.
- Active harnesses are exactly Claude Code, Codex CLI, AGY, and OpenCode.
- Gemini CLI remains deprecated compatibility, not active parity.
- Active harness factories use canonical names.
- Active harness adapters satisfy the shared non-I/O conformance suite.
- AGM runtime helper commands keep co-located SPEC coverage.
- AGM backend implementations keep co-located SPEC coverage.
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
- Claude Code, Codex CLI, AGY, and OpenCode expose the required PreToolUse
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
- Active harnesses have `.claude`, `.codex`, `.agents`, and `.opencode`
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
`agm/internal/terminal`, and `agm/internal/validate` SPEC coverage.

**Key scenarios:**
- AGM control-plane packages keep co-located SPEC coverage.
- AGM control-plane package SPECs point back to their executable BDD feature.

**Why this matters:** The AGM control plane ties CLI, MCP, status, discovery,
tmux terminal handling, validation, and delegation behavior together. These
packages need explicit contracts so parity work does not regress into
harness-specific or Claude-only assumptions.

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

### Shared Runtime Policy Package Guardrails

**File:** [`shared_runtime_policy_guardrails.feature`](../test/bdd/features/shared_runtime_policy_guardrails.feature)

**Drives:** Co-located strict EARS contracts and route-neutral implementation
checks for shared configuration, backlog, code generation, code intelligence,
enforcement, evaluation, event, markdown, graceful-exit, and health packages.

**Key scenarios:**
- Every shared runtime policy package carries a reciprocal SPEC reference.
- Every package contract names all four active harnesses and seven model families.
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
filesystem safety, content validation, VCS, versioning, W0, and workspace state.

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

**Why this matters:** Parity work should not land as untraceable test-only or
doc-only changes. The coverage matrix keeps SPEC and BDD artifacts paired while
legacy repo-wide SPEC coverage is burned down incrementally. The diff-based
package guard prevents new feature work from deepening the SPEC backlog.

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
- Normal session commands parse V2 only while legacy reads remain isolated to
  explicit migration commands.
- Non-migration runtime source cannot reintroduce retired phase identifiers.

**Why this matters:** Wayfinder is the repository's planning gate. A V1 default
or hidden legacy executor makes phase enforcement ambiguous and violates the
canonical V2 and broken-windows policies.

### Wayfinder Internal Package Guardrails

**File:** [`wayfinder_internal_package_guardrails.feature`](../test/bdd/features/wayfinder_internal_package_guardrails.feature)

**Drives:** canonical command entrypoint, Beads, configuration, explicit
migration, Git, history, lint context, telemetry, tracker, project-discovery,
and preset package specifications.

**Key scenarios:**
- Every surviving Wayfinder support package keeps a co-located strict EARS SPEC.
- The exact canonical nine-phase AST contract is preserved across all four
  supported harnesses and seven model families.

**Why this matters:** Wayfinder support code participates in the same planning
gate regardless of which harness or model executes a task. Structural package
coverage prevents migration or tooling helpers from becoming ungoverned paths.

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

---

### Test Support Package Guardrails

**File:** [`test_support_package_guardrails.feature`](../test/bdd/features/test_support_package_guardrails.feature)

**Drives:** strict co-located contracts for all remaining AGM, Engram, shared,
and Wayfinder test-support package boundaries.

**Key scenarios:**
- Every residual test and support package retains a co-located strict EARS SPEC.
- Every SPEC references the executable feature that enforces it.
- The complete support contract is validated across all four active harnesses
  and all seven supported model families.
- Live harness contracts use canonical guarded session and message commands.

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
