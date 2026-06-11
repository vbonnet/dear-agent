// Package supervisor implements the canonical 3-supervisor VROOM mesh —
// Meta-Orchestrator, Orchestrator, Overseer — as long-lived loops with
// the mutual-unblock-first invariant.
//
// # Mental model
//
//	┌───────────────────────── Mesh ─────────────────────────┐
//	│                                                          │
//	│   Meta-Orchestrator       Orchestrator       Overseer    │
//	│     ┌─────────┐            ┌─────────┐       ┌─────────┐ │
//	│     │  Loop   │            │  Loop   │       │  Loop   │ │
//	│     └────┬────┘            └────┬────┘       └────┬────┘ │
//	│          │                      │                 │      │
//	│          ▼                      ▼                 ▼      │
//	│   1. check(orch)         1. check(over)    1. check(meta)│
//	│   2. check(over)         2. check(meta)    2. check(orch)│
//	│   3. Tick:scan roadmap   3. Tick:dispatch  3. Tick:probe │
//	│   4. heartbeat           4. heartbeat      4. heartbeat  │
//	│   5. sleep(interval)     5. sleep          5. sleep      │
//	└──────────────────────────────────────────────────────────┘
//	                           │
//	                           ▼
//	            decisiontrail.Trail (append-only JSONL)
//
// # Vocabulary
//
// Vocabulary is normative in /CONTEXT.md; architectural rationale is in
// /docs/adr/ADR-002. If this package and those documents ever disagree,
// CONTEXT.md wins for definitions and ADR-002 wins for rationale.
//
// Critical distinction: the three Roles here (RoleMetaOrchestrator,
// RoleOrchestrator, RoleOverseer) are *supervisors* — long-lived agents in
// a peer mesh. They are NOT the per-task Primary/Secondary/Tertiary
// ownership roles (which are responsibilities any agent may hold for a
// specific task). The retired five-role V/R/O/O/M model (Verifier /
// Requester / Orchestrator / Overseer / Meta-Orchestrator) is superseded
// by /docs/adr/ADR-002.
//
// # Extension points
//
// Three seams let real-world adapters plug in without changing this
// package:
//
//   - Roadmap — the substrate the Meta-Orchestrator drives. PR 1 ships
//     InMemoryRoadmap; follow-ups will add a beads-backed adapter.
//   - Queue — the work queue the Orchestrator drives. PR 1 ships
//     InMemoryQueue; follow-ups will add an AGM-session-spawning adapter.
//   - ResourceProbe — what the Overseer samples. PR 1 ships
//     InMemoryResourceProbe; follow-ups will add real disk / memory /
//     worktree-stranded probes.
//
// The CheckSkill seam lets the mutual-unblock-first implementation evolve
// from a pure-heartbeat HeartbeatCheckSkill (PR 1) to an AGM-aware skill
// that detects permission-prompt blockage in peer Claude sessions (PR 2+).
package supervisor
