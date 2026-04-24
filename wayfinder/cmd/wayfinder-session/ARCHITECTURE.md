# Wayfinder V2 Architecture

**Version**: 2.0
**Last Updated**: 2026-02-20
**Status**: Phase 1 Complete

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture Diagram](#architecture-diagram)
3. [Package Structure](#package-structure)
4. [Core Components](#core-components)
5. [Data Flow](#data-flow)
6. [Phase Consolidation](#phase-consolidation)
7. [Key Design Decisions](#key-design-decisions)
8. [Integration Points](#integration-points)

---

## Overview

Wayfinder V2 consolidates the 13-phase SDLC workflow into 9 streamlined phases with native task/roadmap management. The architecture consists of 5 core packages implementing schema V2, phase orchestration, task management, BUILD loop, and multi-persona review.

### Architectural Goals

1. **Phase Reduction**: 13 phases → 9 phases (31% reduction)
2. **Native Task Tracking**: Roadmap embedded in WAYFINDER-STATUS.md
3. **TDD Enforcement**: BUILD loop with tight test-first feedback
4. **Risk-Adaptive Review**: Per-task (L/XL) vs batch (XS/S/M) review
5. **Zero Data Loss**: 100% V1→V2 migration with validation

### Design Principles

- **Single Source of Truth**: WAYFINDER-STATUS.md contains all project state
- **Fail Fast**: Validate at phase boundaries, not project end
- **Test-First Discipline**: Tests written in D4/S6, enforced in S8
- **Separation of Concerns**: Each package has clear responsibility
- **Backward Compatible**: V1 projects coexist during migration

---

## Architecture Diagram

### High-Level System Context

```
┌──────────────────────────────────────────────────────────────────────┐
│                     Wayfinder V2 System                              │
└──────────────────────────────────────────────────────────────────────┘
                                  │
                    ┌─────────────┴─────────────┐
                    │  WAYFINDER-STATUS.md V2   │  (Single source of truth)
                    │  YAML + Roadmap + Tasks   │
                    └─────────────┬─────────────┘
                                  │
              ┌───────────────────┼───────────────────┐
              │                   │                   │
        ┌─────▼──────┐     ┌─────▼──────┐     ┌─────▼──────┐
        │   Status   │     │Orchestrator│     │Task Manager│
        │  (Schema)  │────▶│  (Phases)  │────▶│   (CLI)    │
        └────────────┘     └─────┬──────┘     └────────────┘
                                 │
                      ┌──────────┴──────────┐
                      │                     │
               ┌──────▼─────┐        ┌─────▼──────┐
               │ BUILD Loop │        │   Review   │
               │ (S8 TDD)   │───────▶│(Multi-Pers)│
               └────────────┘        └────────────┘
                      │
                      │  Task Iteration
                      ▼
       TEST_FIRST → CODE → GREEN → REFACTOR → VALIDATE → DEPLOY
```

### C4 Component Diagram

For detailed component-level architecture showing internal components, data flows, and interactions:

**Diagrams**:
- [C4 Component Diagram (SVG)](diagrams/rendered/c4-component-wayfinder.svg)
- [C4 Component Diagram (PNG)](diagrams/rendered/c4-component-wayfinder.png)
- [Source (D2)](diagrams/c4-component-wayfinder.d2)

The C4 Component diagram illustrates:
- **Phase Orchestrator V2**: Manages the 9-phase workflow with transition validation
- **Status Manager**: Handles WAYFINDER-STATUS.md read/write with YAML parsing
- **Build Loop Engine**: TDD state machine (Red→Green→Refactor→Review)
- **Multi-Persona Review**: Risk-adaptive review with 5 personas (Security, Performance, Maintainability, UX, Reliability)
- **Task Manager**: CRUD operations for roadmap tasks with dependency tracking
- **History Logger**: Append-only event log for audit trail
- **Migration Engine**: V1→V2 schema migration with validation
- **Data flows** between components and external systems (Git, Beads, Filesystem)

---

## Package Structure

```
cmd/wayfinder-session/
├── SPEC.md                    # V2 specification (348 lines)
├── ARCHITECTURE.md            # This file
├── internal/
│   ├── status/                # V2 schema package (Task 1.1)
│   │   ├── types_v2.go        # Go types for V2 YAML schema
│   │   ├── parser_v2.go       # YAML parser
│   │   ├── validator_v2.go    # Schema validator
│   │   ├── *_test.go          # 49 tests, 81.2% coverage
│   │   └── README.md
│   │
│   ├── orchestrator/          # Phase orchestrator (Task 1.2)
│   │   ├── orchestrator_v2.go # 9-phase orchestrator
│   │   ├── transitions_v2.go  # Phase transition rules
│   │   ├── build_loop.go      # S8 BUILD loop integration
│   │   ├── *_test.go          # 28 tests, 81.7% coverage
│   │   └── README.md
│   │
│   ├── taskmanager/           # Task management CLI (Task 1.3)
│   │   ├── taskmanager.go     # CRUD operations
│   │   ├── types.go           # Task structures
│   │   ├── dependency_validator.go  # Cyclic detection (DFS)
│   │   ├── *_test.go          # 25 tests, 88.1% coverage
│   │   └── README.md
│   │
│   ├── buildloop/             # BUILD loop state machine (Task 1.4)
│   │   ├── buildloop.go       # Main executor
│   │   ├── states.go          # 8-state machine
│   │   ├── iteration_tracker.go # Task iteration logic
│   │   ├── *_test.go          # 67 tests, 78.1% coverage
│   │   └── README.md
│   │
│   └── review/                # Multi-persona review (Task 1.5)
│       ├── review_engine.go   # Review executor
│       ├── personas.go        # 5 personas (Sec, Perf, UX, Maint, Rel)
│       ├── risk_adapter.go    # Risk-adaptive strategy
│       ├── *_test.go          # 24 tests, 87.3% coverage
│       └── README.md
│
└── commands/                  # CLI commands
    ├── task.go                # Task management commands
    ├── next_phase.go          # Phase advancement
    └── verify.go              # Gate validation
```

**Test Summary**: 193 total tests, 100% pass rate, 78-88% coverage

---

## Core Components

### 1. Status Package (Schema V2)

**Purpose**: Parse, validate, and manage WAYFINDER-STATUS.md V2 schema

**Key Types**:
- `WayfinderStatusV2`: Root structure with roadmap
- `RoadmapPhase`: Phase with task array
- `Task`: Individual task with dependencies
- `QualityMetrics`: Coverage, assertion density, persona scores

**Responsibilities**:
- YAML parsing with validation
- Cyclic dependency detection
- Quality metrics calculation
- Schema version migration

**Key Functions**:
- `ParseV2StatusFile(path string) (*WayfinderStatusV2, error)`
- `ValidateV2Schema(status *WayfinderStatusV2) error`
- `SaveV2StatusFile(path string, status *WayfinderStatusV2) error`

---

### 2. Orchestrator Package (Phase Management)

**Purpose**: Enforce 9-phase sequence and transition rules

**9 Phases**:
1. W0 - Project Intake & Bootstrapping
2. D1 - Problem Definition & Research
3. D2 - Solution Exploration
4. D3 - Detailed Design
5. D4 - Requirements Sign-off (merged S4)
6. S6 - Implementation Planning (merged S5)
7. S7 - Development Environment Setup
8. S8 - BUILD Loop (merged S8/S9/S10)
9. S11 - Documentation & Knowledge Transfer

**Phase Merges**:
- S4 → D4: Requirements + Stakeholder approval
- S5 → S6: Research + Implementation planning
- S8/S9/S10 → S8: Test + Code + Deploy unified

**Responsibilities**:
- Phase sequence enforcement (D1→D2→D3→D4→S6→S7→S8→S11)
- Transition validation (exit criteria checks)
- Phase dependency graph
- Merged phase data preservation

**Key Functions**:
- `CanTransition(from, to Phase) (bool, error)`
- `ValidatePhaseComplete(phase Phase, status *WayfinderStatusV2) error`
- `GetNextPhase(current Phase) (Phase, error)`

---

### 3. Task Manager Package (CLI)

**Purpose**: CRUD operations for tasks in roadmap

**CLI Commands**:
- `wayfinder-session add-task {phase} {title} --effort {days}`
- `wayfinder-session complete-task {task-id}`
- `wayfinder-session roadmap` (markdown view)
- `wayfinder-session task-status` (progress)

**Responsibilities**:
- Task creation with validation
- Dependency management (depends_on, blocks)
- Cyclic dependency detection (DFS algorithm)
- Status updates with timestamps
- Roadmap markdown generation

**Key Functions**:
- `AddTask(status *WayfinderStatusV2, task Task) error`
- `CompleteTask(status *WayfinderStatusV2, taskID string) error`
- `ValidateDependencies(tasks []Task) error`

---

### 4. BUILD Loop Package (S8 State Machine)

**Purpose**: Execute TDD iteration cycle for S8 phase

**8 States**:
1. **TEST_FIRST**: Run tests (must fail)
2. **CODING**: Write minimal code to pass
3. **GREEN**: Tests pass
4. **REFACTOR**: Improve code quality
5. **VALIDATION**: Multi-persona review
6. **DEPLOY**: Integration testing
7. **MONITORING**: Observe behavior
8. **COMPLETE**: Task done, next task

**Responsibilities**:
- Enforce test-first discipline (fail → pass cycle)
- Task iteration (for each task in roadmap)
- Assertion density validation (prevent `assert true` gaming)
- Coverage tracking (≥80% target)
- Integration with review engine

**Key Functions**:
- `ExecuteBUILDLoop(status *WayfinderStatusV2) error`
- `RunTestsExpectFailure(task Task) error`
- `RunTestsExpectSuccess(task Task) error`
- `CalculateAssertionDensity(files []string) (float64, error)`

**Assertion Density**: Minimum 0.3 assertions per 10 LOC (prevents tautologies)

---

### 5. Review Package (Multi-Persona)

**Purpose**: Risk-adaptive code review with 5 personas

**5 Personas**:
1. **Security**: SQL injection, XSS, secrets detection
2. **Performance**: N+1 queries, memory leaks, algorithmic complexity
3. **Maintainability**: Cyclomatic complexity, TODOs, code smells
4. **UX**: Error messages, accessibility, user feedback
5. **Reliability**: Error handling, edge cases, nil checks

**Risk Levels**:
- **XS** (1-50 LOC): Batch review, minimal personas
- **S** (51-200 LOC): Batch review, basic personas
- **M** (201-500 LOC): Batch review, includes UX
- **L** (501-1000 LOC): Per-task review, includes reliability
- **XL** (1001+ LOC): Per-task review, all personas

**Severity Scoring**:
- P0 (Critical): -25 points
- P1 (High): -15 points
- P2 (Medium): -8 points
- P3 (Low): -3 points

**Blocking Rules**:
- P0/P1: Always block deployment
- P2: Blocks XL projects only
- P3: Never blocks

**Responsibilities**:
- Risk level detection (LOC, file paths, patterns)
- Persona selection (adaptive to risk)
- Pattern-based code review
- Severity-based scoring
- Report generation (markdown + JSON)

**Harness Profiles**:
- `HarnessProfile`: Three levels — `ProfileLite` (XS/S), `ProfileStandard` (M), `ProfileDeep` (L/XL)
- `ProfileConfig`: Controls skipped phases, evaluator requirement, review persona count, max retries
- `ClassifyRisk(task, projectDir)`: Returns `(RiskLevel, ProfileConfig)` — maps a task to its risk level and corresponding profile configuration

**Key Functions**:
- `DetectRiskLevel(files []string, loc int) RiskLevel`
- `ReviewTask(task Task, files []string) (ReviewReport, error)`
- `ReviewBatch(tasks []Task, files []string) (ReviewReport, error)`
- `ClassifyRisk(task *Task, projectDir string) (RiskLevel, ProfileConfig)`
- `GetProfileForRisk(risk RiskLevel) HarnessProfile`
- `GetProfileConfig(profile HarnessProfile) ProfileConfig`

---

## Data Flow

### Phase Advancement Flow

```
User runs: wayfinder-session next-phase
                    │
                    ▼
         ┌──────────────────────┐
         │ Read STATUS file V2  │
         └──────────┬───────────┘
                    │
                    ▼
         ┌──────────────────────┐
         │ Orchestrator: Validate│
         │ current phase complete│
         └──────────┬───────────┘
                    │
              ┌─────┴─────┐
              │           │
          ✅ Valid    ❌ Invalid
              │           │
              ▼           ▼
      ┌──────────┐   ┌────────────┐
      │Transition│   │ Show errors│
      │to next   │   │ Exit       │
      └─────┬────┘   └────────────┘
            │
            ▼
   ┌────────────────┐
   │ Update STATUS  │
   │ current_phase  │
   └────────┬───────┘
            │
            ▼
   ┌────────────────┐
   │ Save STATUS    │
   │ Git commit     │
   └────────────────┘
```

### BUILD Loop Flow (S8)

```
For each task in roadmap (status: pending):
    │
    ▼
┌─────────────────┐
│ STATE: TEST_FIRST│  Run tests (expect failure)
└────────┬────────┘
         │
         ▼ Tests fail ✅
┌─────────────────┐
│ STATE: CODING   │  Write minimal code
└────────┬────────┘
         │
         ▼ Code written
┌─────────────────┐
│ STATE: GREEN    │  Run tests (expect pass)
└────────┬────────┘
         │
         ▼ Tests pass ✅
┌─────────────────┐
│ STATE: REFACTOR │  Improve code quality
└────────┬────────┘
         │
         ▼ Refactor done
┌─────────────────┐
│ STATE: VALIDATION│ Multi-persona review
└────────┬────────┘
         │
         ▼ No P0/P1 issues ✅
┌─────────────────┐
│ STATE: DEPLOY   │  Integration tests
└────────┬────────┘
         │
         ▼ Integration pass ✅
┌─────────────────┐
│STATE: MONITORING│ Observe behavior
└────────┬────────┘
         │
         ▼ Stable ✅
┌─────────────────┐
│ STATE: COMPLETE │  Mark task complete
└────────┬────────┘
         │
         ▼
    Next task
```

---

## Phase Consolidation

### Before (V1): 13 Phases

```
W0 → D1 → D2 → D3 → D4 → S4 → S5 → S6 → S7 → S8 → S9 → S10 → S11
                      ↑    ↑    ↑              ↑    ↑    ↑
                      └────┴────┴──────────────┴────┴────┘
                           Merged in V2
```

### After (V2): 9 Phases

```
W0 → D1 → D2 → D3 → D4* → S6* → S7 → S8* → S11
                     ↑      ↑         ↑
                     │      │         └── S8/S9/S10 unified (BUILD loop)
                     │      └── S5 merged (research → S6)
                     └── S4 merged (stakeholder → D4)
```

**Rationale**:
- **S4 → D4**: Requirements validation and stakeholder sign-off are sequential, not separate phases
- **S5 → S6**: Research informs implementation planning, merge for tight feedback
- **S8/S9/S10 → S8**: Test/code/deploy cycle unified into BUILD loop for TDD enforcement

---

## Key Design Decisions

### Decision 1: Native Roadmap in STATUS File

**Problem**: 3 overlapping roadmap systems (Wayfinder, Swarm, Autonomous)

**Solution**: Embed roadmap directly in WAYFINDER-STATUS.md V2 schema

**Benefits**:
- Single source of truth
- Git-tracked task history
- No duplicate systems
- CLI-queryable task status

**Trade-offs**: YAML complexity vs simplicity

---

### Decision 2: BUILD Loop State Machine

**Problem**: S8/S9/S10 linear phases discourage TDD iteration

**Solution**: 8-state BUILD loop with test-first enforcement

**Benefits**:
- Tight TDD feedback cycle
- Assertion density validation
- Risk-adaptive review integration
- Task-level iteration tracking

**Trade-offs**: Complexity vs discipline

---

### Decision 3: Risk-Adaptive Review

**Problem**: Per-task review overkill for small changes, batch review misses critical bugs

**Solution**: Risk level determines review strategy (XS/S/M batch, L/XL per-task)

**Benefits**:
- Efficient for small tasks
- Thorough for risky tasks
- Persona selection adaptive
- P0/P1 blocking enforced

**Trade-offs**: Complexity vs effectiveness

---

### Decision 4: Big Bang Migration

**Problem**: Gradual migration creates dual-maintenance burden

**Solution**: Automated V1→V2 converter with 100% validation

**Benefits**:
- One-time migration event
- Clean cutover
- Validation ensures zero data loss
- Backups enable rollback

**Trade-offs**: Risk vs maintenance cost

---

### Decision 5: Document Quality Gates (D3/D4)

**Problem**: Low-quality architecture decisions and requirements propagate to implementation, causing rework

**Solution**: LLM-as-judge review skills validate D3 (ARCHITECTURE.md + ADRs) and D4 (SPEC.md) documents before phase completion

**Implementation**:
- **D3 Gate**: Validates ARCHITECTURE.md (≥8.0/10) and all ADR-*.md files (≥8.0/10 each)
  - Uses `review-architecture` and `review-adr` skills
  - Blocks phase completion if any document scores <8.0
  - Catches missing context, unclear rationale, incomplete analysis
- **D4 Gate**: Validates SPEC.md (≥8.0/10)
  - Uses `review-spec` skill
  - Blocks phase completion if document scores <8.0
  - Catches vague requirements, unmeasurable metrics, missing acceptance criteria

**Validator Location**: `internal/validator/doc_quality_gate.go`

**Key Functions**:
- `validateDocQuality(phaseName, projectDir string, forceOverride bool) error`
- `validateD3Documents(projectDir string, forceOverride bool) error`
- `validateSingleDocument(projectDir, phaseName, docFile, skillName string) error`
- `runReviewSkill(skillName string, docPath string) (float64, []string, error)`

**Caching**:
- SHA-256 hash-based caching for fast re-validation
- Cache location: `{project}/.wayfinder-cache/doc-quality-scores.json`
- Cache entry: `{file_hash, score, timestamp}`

**Benefits**:
- Prevents low-quality decisions from reaching S6/S8
- Reduces rework in later phases (fixing decisions is cheaper in D3 than S8)
- Enforces systematic decision documentation (ADRs for key choices)
- Measurable quality threshold (8.0/10 based on W0 charter)

**Trade-offs**:
- Adds LLM API dependency (requires ANTHROPIC_API_KEY)
- 5-10 second latency per document (first run, cached after)

**Error Example**:
```
❌ D3 document quality gate failed

Documents reviewed:
- ARCHITECTURE.md: 6.5/10 ⚠️  FAILED
- ADR-001-database.md: 9.0/10 ✅ PASSED
- ADR-002-api-design.md: 7.2/10 ⚠️  FAILED

Minimum score required: 8.0/10

Fix failing documents and re-run:
  wayfinder session complete-phase D3
```

**Integration**:
- Called automatically during `wayfinder session complete-phase D3/D4`
- Phase engrams updated with quality gate documentation
- Documented in README.md Wayfinder section

---

## Integration Points

### Wayfinder CLI

```bash
# Phase management
wayfinder-session next-phase
wayfinder-session rewind {phase}
wayfinder-session complete-phase {phase}

# Task management
wayfinder-session add-task {phase} {title}
wayfinder-session complete-task {task-id}
wayfinder-session roadmap
```

### Swarm Coordination

**Read-only integration**: Swarm reads WAYFINDER-STATUS.md, doesn't manage phases

```bash
# Swarm reads Wayfinder tasks
wayfinder-session roadmap --json | swarm-coordinator parse
```

### A2A Protocol

**Optional integration**: A2A messages can reference Wayfinder task IDs

```json
{
  "type": "task_delegation",
  "wayfinder_task_id": "S8-3",
  "agent": "worker-1"
}
```

---

## Success Metrics (Phase 1)

- ✅ All 5 packages implemented
- ✅ 193 tests, 100% pass rate
- ✅ 78-88% test coverage (exceeds 70% target)
- ✅ Schema validates V2 YAML
- ✅ BUILD loop enforces TDD
- ✅ Multi-persona review detects issues
- ✅ Task dependency validation prevents cycles
- ✅ Phase orchestrator prevents invalid transitions

---

## References

- [SPEC.md](./SPEC.md) - Complete V2 specification
- [Validator ARCHITECTURE.md](./internal/validator/ARCHITECTURE.md) - Gate system architecture
- [ADR-001](./internal/validator/ADR-001-gate-9-working-code-verification.md) - Code verification gate

---

**Version**: 2.0
**Status**: Phase 1 Complete
**Last Updated**: 2026-02-20
