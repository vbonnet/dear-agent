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

### Stall Detection

**File:** [`stall_detection.feature`](../test/bdd/features/stall_detection.feature)

**Drives:** stall-type invariants and stall-detection SLO contracts.

**Key scenarios:**
- Permission-prompt stalls are `critical` severity.
- Error messages are normalized (paths/line numbers stripped) before counting.
- Detector thresholds come from the SLO contracts (permission timeout,
  no-commit timeout, error-repeat threshold).
- Exactly three stall types exist: `permission_prompt`, `no_commit`, `error_loop`.

**Why this matters:** The multi-agent system only makes forward progress if
stalled sessions are detected and recovered against agreed thresholds.

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

**Drives:** Wayfinder archive, resume, workspace detection, and review package
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
