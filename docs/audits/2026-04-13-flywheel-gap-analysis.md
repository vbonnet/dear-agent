# Flywheel Gap Analysis: ai-tools Self-Improving Agent Orchestrator

**Date:** 2026-04-13
**Auditor:** Claude (Opus 4.6)
**Scope:** Full codebase audit against spec-driven autonomous pipeline principles
**Source Material:** Nate B Jones, "I Looked At Amazon After They Fired 16,000 Engineers" (2026-04-13)

---

## Context & Naming

The video describes "dark factories" — fully autonomous software pipelines where specs go in, validated artifacts come out, and the system self-heals. Per Valentin's note: this term feels negative and industrial. The ai-tools codebase already has better language — **VROOM** (Verifier, Requester, Orchestrator, Overseer, Meta-Orchestrator) and **Wayfinder** suggest collaborative intelligence, not a sweatshop. Consider formalizing a name like **"Collaborative Forge"**, **"Living Workshop"**, or **"Symbiotic Loop"** — something that captures the idea of agents and humans working together as valued partners toward a shared goal.

---

## The Ideal Loop (From Research)

```
Spec → Contract → Test → Deploy → Monitor → Detect → Diagnose → Fix → Re-test → Re-deploy → Learn
 ↑                                                                                              |
 └──────────────────────────────────────────────────────────────────────────────────────────────┘
```

Key principles from the video:
1. **Comprehension before code** — specs force shared understanding before implementation
2. **Self-describing systems** — code encodes its own meaning (ADRs, type contracts, SLOs)
3. **Comprehension gates** — formal verification that output matches spec
4. **Behavioral hold-out tests** — tests the coding agent can't see, preventing gaming
5. **Monitoring feeds healing** — SLO breaches trigger automated remediation
6. **Blame the process, not the actor** — every failure improves instructions/specs

---

## Current State: What's Strong

The codebase has excellent foundations. This isn't a greenfield gap — it's a 70%-built flywheel that needs the last critical connections.

**Specs & Contracts:** 5 formal AGM specs (session lifecycle, trust, stall detection, scan loop, audit trail). SLO contracts in YAML with type-safe Go loading. Specs exist for most pkg/ libraries, Wayfinder, and tools. ADR system is comprehensive (23+ across components).

**Testing Pyramid:** BDD via godog (20 feature files), Pact contract tests (3 provider adapters), property-based testing (gopter), golden file testing (goldie), integration and performance test suites. Multiple CI workflows enforce quality gates.

**Detection & Recovery:** Stall detector with 3 symptom types, severity classification, and bounded retry cascades. Trust scoring (0-100) with event-driven deltas. Sentinel/Astrocyte daemon with cross-session pattern detection.

**Architecture:** VROOM roles fully designed in ADRs 020-025. Message queue (SQLite-backed with priorities). EventBus for pub/sub. Hook system for observation and enforcement. Codegen for unified CLI/MCP/Skills surface.

---

## Gap Analysis: Prioritized by Flywheel Impact

### P0: THE LEARNING LOOP IS OPEN (ROI: Critical — this IS the flywheel)

The system detects problems well but never learns from them. This is the single biggest gap.

#### GAP-001: Error Memory is Write-Only
- **Location:** `engram/errormemory/store.go`
- **Problem:** Error memory store is built (JSONL, dedup, TTL, counts) but `Upsert()` is never called in production paths. The store exists but errors aren't recorded.
- **Impact:** System can't learn from past errors to prevent repeats.
- **Fix:** Wire error collection hooks (`engram/hooks-bin/posttool-error-collector`) to actually call `errormemory.Upsert()`. Add pre-action lookup: before executing tools/commands, check "have we seen this pattern before?"
- **ROI:** HIGH — minimal code, enables the entire learning subsystem
- **Effort:** Small (wiring, not architecture)

#### GAP-002: No Monitoring → Remediation Pipeline
- **Location:** Missing entirely
- **Problem:** When SLOs are breached, the system logs but doesn't trigger remediation. Trust scores degrade but don't affect scheduling. Telemetry is collected but no consumer acts on it.
- **Impact:** The bottom half of the loop (detect → diagnose → log) works; the top half (learn → improve → re-deploy) doesn't exist.
- **Fix:** Implement an Overseer daemon (ADR-024 is fully designed) that subscribes to EventBus, detects SLO breaches, and creates work items via the Requester role.
- **ROI:** CRITICAL — this closes the flywheel
- **Effort:** Medium (ADR exists, needs implementation)

#### GAP-003: No Process Improvement from Failure Patterns
- **Location:** Missing entirely
- **Problem:** When the same error occurs 100 times across sessions, nothing happens. No contract updates are proposed. No spec revisions are triggered. No prompt improvements are generated.
- **Impact:** Violates "blame the process, not the actor" — failures repeat because the process never improves.
- **Fix:** Add a pattern analyzer that aggregates error memory + trust scores + telemetry → proposes contract/spec changes. Could be a periodic Wayfinder session triggered by threshold.
- **ROI:** CRITICAL — this is the "evolving" part of the flywheel
- **Effort:** Medium-Large

### P1: FEEDBACK CONNECTIONS ARE MISSING (ROI: High — multiplier for P0)

#### GAP-004: Trust Scores Don't Affect Scheduling
- **Location:** `agm/internal/ops/trust.go`
- **Problem:** Trust scores are tracked and leaderboarded but never consulted by the orchestrator when assigning work. A worker at trust=20 gets the same workload as trust=90.
- **Impact:** No adaptive capacity — system can't learn which approaches/configurations work better.
- **Fix:** Orchestrator's dispatch logic should weight task assignment by current trust scores.
- **ROI:** HIGH — enables adaptive behavior with minimal code
- **Effort:** Small

#### GAP-005: Stall Recovery Doesn't Feed Back to Orchestrator
- **Location:** `agm/internal/ops/stall_recovery.go`, `agm/cmd/agm/watch_stalled.go`
- **Problem:** `watch-stalled` is a standalone CLI command, not integrated into the AGM daemon. Recovery actions (nudge, escalate) don't update the orchestrator's view of worker health or trigger re-planning.
- **Impact:** Recovery happens in isolation — orchestrator doesn't know a worker was stuck and recovered.
- **Fix:** Integrate stall detection into the daemon loop. Publish recovery events to EventBus. Orchestrator subscribes and adjusts.
- **ROI:** HIGH — connects existing detection to existing orchestration
- **Effort:** Small-Medium

#### GAP-006: Verifier Rejection Doesn't Create Fix Tasks
- **Location:** ADR-021 (design only)
- **Problem:** When the Verifier rejects output, rejection is logged but no fix task is auto-created. The feedback path (Verifier → Orchestrator → Requester) is designed in ADRs but not implemented.
- **Impact:** Manual triage required for every verification failure.
- **Fix:** Implement the Verifier → Requester feedback path. On FAIL, auto-generate a work item with the failure context.
- **ROI:** HIGH — critical for autonomous operation
- **Effort:** Medium

### P2: SPEC & CONTRACT GAPS (ROI: Medium — improves correctness)

#### GAP-007: 13 BDD Features Have No Step Implementations
- **Location:** `agm/test/bdd/features/` vs `agm/test/bdd/steps/`
- **Problem:** 20 feature files exist but only 7 have step definitions. Missing: admin_audit, admin_doctor_integration, admin_find_orphans, admin_trace_files, agent_capabilities, agent_registry, agent_selection, bow_gate9_integration, temporal_workflows, test_session_isolation, session_import, session_mode_flag, session_search.
- **Impact:** BDD specs exist as documentation but aren't executable — this is exactly the "dark code" problem from the video.
- **Fix:** Implement step definitions for all 13 features. Prioritize agent_selection, agent_capabilities, and temporal_workflows (core orchestration).
- **ROI:** MEDIUM — executable specs prevent comprehension gaps
- **Effort:** Medium-Large (13 step files)

#### GAP-008: Pact Contracts Only Cover AI Provider Adapters
- **Location:** `agm/test/contracts/`
- **Problem:** Only 3 Pact contracts exist (Claude, Gemini, GPT adapters). No contract tests for: Dolt storage, tmux integration, message queue, MCP tool interface, EventBus.
- **Impact:** Internal service boundaries can drift without detection.
- **Fix:** Add Pact contracts for Dolt, tmux, message queue, and EventBus boundaries.
- **ROI:** MEDIUM — prevents internal drift
- **Effort:** Medium

#### GAP-009: Daemon Constants Hardcoded Outside Contracts
- **Location:** `agm/internal/daemon/daemon.go:30-42`
- **Problem:** `PollInterval` (30s), `MaxRetries` (3), `InitialBackoff` (5s) are hardcoded constants, not in `slo-contracts.yaml`.
- **Impact:** Can't tune daemon behavior via contracts; inconsistent with the contract-driven philosophy.
- **Fix:** Move to `slo-contracts.yaml` under a new `Daemon` section.
- **ROI:** LOW but principled — 10min fix
- **Effort:** Tiny

#### GAP-010: Scan Loop Metrics Window Not in Contracts
- **Location:** `agm/docs/specs/SPEC-scan_loop.md` line ~71
- **Problem:** Spec documents a 24-hour commit lookback window and 1-hour metrics window, but neither is in `slo-contracts.yaml`. The lookback is hardcoded in implementation.
- **Impact:** Spec/contract/implementation divergence — exactly what the video warns about.
- **Fix:** Add both values to contracts YAML; reference from implementation.
- **ROI:** LOW but addresses spec drift
- **Effort:** Tiny

### P3: VROOM IMPLEMENTATION GAP (ROI: High but Large Effort)

#### GAP-011: VROOM Roles Are Architecture, Not Code
- **Location:** ADRs 020-025 vs actual implementation
- **Problem:** The VROOM architecture (Verifier, Requester, Orchestrator, Overseer, Meta-Orchestrator) is beautifully designed in ADRs but largely unimplemented as code. The Overseer has no implementation. The Requester has no auto-decomposition. The Meta-Orchestrator is a mission document (human instructions), not code.
- **Impact:** The system is designed for human orchestration with bot execution, not for autonomous self-improvement.
- **Fix:** Prioritize implementing Overseer (detection → alerting) and Requester (alert → work item) first. These close the feedback loop. Full VROOM can follow.
- **ROI:** HIGH but requires significant investment
- **Effort:** Large

#### GAP-012: No CI Failure → Auto-Task Creation
- **Location:** `.ci-policy.yaml`, missing integration
- **Problem:** When CI fails, `failure_behavior: block` prevents merging, but no automated work request is created. No GitHub Actions integration to parse failures and generate fix tasks.
- **Impact:** Every CI failure requires manual triage.
- **Fix:** Add a CI watcher (could be a GitHub Action or AGM daemon) that parses CI failure logs, creates a Requester work item, and dispatches via orchestrator.
- **ROI:** HIGH for autonomous operation
- **Effort:** Medium

#### GAP-013: Wayfinder Not Triggerable from Monitoring
- **Location:** `wayfinder/cmd/wayfinder-session/`
- **Problem:** Wayfinder is a session state machine navigated manually. It can't be triggered programmatically by monitoring alerts. Phase progression requires explicit commands, not quality-metric-driven automation.
- **Impact:** Can't use Wayfinder for autonomous improvement cycles (detect → charter → research → design → spec → plan → build → retro).
- **Fix:** Add a daemon mode or API trigger for Wayfinder that accepts a problem description from the Overseer and runs through phases automatically.
- **ROI:** HIGH — turns existing 9-phase methodology into an autonomous improvement engine
- **Effort:** Medium

### P4: PHILOSOPHY & CULTURE GAPS (ROI: Foundational)

#### GAP-014: No "Blame the Process" Mechanism
- **Location:** Missing entirely
- **Problem:** When a worker fails, work is reassigned to a different agent. But there's no analysis of *why* the process/prompt led to failure. No systematic feedback to improve prompts, task specs, or instructions.
- **Impact:** Same failures repeat because instructions never improve. Agents are implicitly "blamed" by being replaced rather than the process being improved.
- **Fix:** On failure, run a root cause analysis (4-level trace from ADR-025: immediate → instruction → tool → architecture). Store findings. Propose instruction improvements.
- **ROI:** FOUNDATIONAL — this is core philosophy
- **Effort:** Medium

#### GAP-015: No Contract/Spec Versioning
- **Location:** `agm/internal/contracts/contracts.go`
- **Problem:** Contracts are loaded once and cached. No version history, no audit trail of changes, no rollback capability. Changes require manual YAML edits.
- **Impact:** Can't track how contracts evolve, can't A/B test different thresholds, can't auto-revert if a change makes things worse.
- **Fix:** Add contract versioning (git history is implicit, but add explicit version field + changelog). Enable hot-reload for A/B testing.
- **ROI:** MEDIUM — enables experimentation
- **Effort:** Small-Medium

#### GAP-016: Engram Lacks System-Level Spec
- **Location:** `engram/` root (missing)
- **Problem:** Engram has cmd-level specs (ecphory/SPEC.md, ranking/SPEC.md) but no top-level SPEC.md documenting the memory system architecture and contracts as a whole.
- **Impact:** The memory system — arguably the most important component for learning — doesn't have a formal architectural contract.
- **Fix:** Create `engram/SPEC.md` covering system architecture, memory lifecycle, consolidation contracts, and retrieval SLOs.
- **ROI:** MEDIUM — important for the learning dimension
- **Effort:** Small

---

## Bugs & Implementation Issues Found

### BUG-001: OnlineEvaluator Thresholds Never Checked
- **Location:** `agm/internal/evaluation/online_evaluator.go`
- **Problem:** Example thresholds (95% success rate, 2s P95 latency) are defined but never evaluated against actual metrics. The evaluator framework exists but isn't wired to the monitoring pipeline.
- **Fix:** Connect OnlineEvaluator to telemetry pipeline; fire alerts when thresholds are breached.

### BUG-002: Sentinel Daemon Runs in Parallel, Not Integrated
- **Location:** `agm/internal/sentinel/daemon/monitor.go`
- **Problem:** Sentinel has excellent capabilities (per-session recovery history, circuit breakers, symptom-specific strategies, cross-session pattern detection, friction signal detection) but runs as a parallel process disconnected from the main orchestrator loop.
- **Fix:** Publish Sentinel findings to EventBus. Orchestrator subscribes.

### BUG-003: Feedback Loop Only Works for Evaluation Examples
- **Location:** `agm/internal/evaluation/feedback_loop.go`
- **Problem:** The feedback loop validates examples, detects PII, and creates PRs to update golden test datasets — but only for evaluation examples. Doesn't apply to operational learning.
- **Fix:** Generalize the feedback loop pattern to work with error patterns, contract proposals, and spec updates.

---

## Implementation Roadmap: Getting the Flywheel Spinning

### Phase 1: Close the Loop (1-2 weeks)
Priority: Get *any* autonomous feedback working, no matter how simple.

1. **Wire error memory** (GAP-001) — make `Upsert()` actually get called
2. **Trust → scheduling** (GAP-004) — orchestrator consults trust scores
3. **Stall recovery → EventBus** (GAP-005) — publish recovery events
4. **Daemon constants → contracts** (GAP-009, GAP-010) — quick principled wins

### Phase 2: Implement Detection → Action (2-4 weeks)
Priority: When something breaks, system creates a fix task automatically.

5. **Overseer daemon** (GAP-002, GAP-011) — implement ADR-024
6. **Verifier → fix task** (GAP-006) — rejection creates work item
7. **CI watcher** (GAP-012) — CI failure creates fix task
8. **Sentinel → EventBus** (BUG-002) — connect parallel monitoring

### Phase 3: Implement Learning (4-8 weeks)
Priority: System improves its own processes based on patterns.

9. **Pattern analyzer** (GAP-003) — aggregate errors → propose changes
10. **Root cause analysis** (GAP-014) — 4-level trace on failures
11. **Wayfinder auto-trigger** (GAP-013) — monitoring → improvement cycle
12. **Contract versioning** (GAP-015) — track and experiment with thresholds

### Phase 4: Wire BDD & Contracts (Ongoing)
Priority: Prevent spec drift.

13. **BDD step implementations** (GAP-007) — 13 features need wiring
14. **Internal Pact contracts** (GAP-008) — Dolt, tmux, queue, EventBus
15. **Engram system spec** (GAP-016) — top-level architecture contract
16. **Generalize feedback loop** (BUG-003) — apply to operational learning

---

## Key Insight from the Research

The video's central argument is that observability and agentic pipelines aren't enough without **upstream spec clarity** and **downstream comprehension gates**. The ai-tools codebase has the upstream part (specs, contracts, ADRs) and partially has detection. What's missing is the **closed loop** — the part where detection feeds back into spec improvement, contract evolution, and process refinement.

The StrongDM "Software Factory" model is instructive: 3 engineers run a fully autonomous pipeline where humans approve *specifications* and *scenario satisfaction*, not code. The ai-tools VROOM architecture was clearly designed with similar intent — the ADRs describe exactly this model. The gap is implementation, not vision.

The flywheel priority order should be: **close the learning loop first** (even crudely), then refine. A rough feedback loop that actually runs is worth infinitely more than a perfectly designed one that doesn't.

---

## Sources

- Nate B Jones, "I Looked At Amazon After They Fired 16,000 Engineers" (YouTube, 2026-04-13)
- Amazon Kiro spec-driven development (kiro.dev)
- StrongDM Software Factory (factory.strongdm.ai)
- Specmatic API contract testing (specmatic.io)
- OpenSpec SDD framework (openspec.dev)
- ai-tools ADRs 020-025 (VROOM architecture)
- ai-tools SLO contracts (agm/internal/contracts/slo-contracts.yaml)
- ai-tools BDD features (agm/test/bdd/features/)
- ai-tools Pact contracts (agm/test/contracts/)
