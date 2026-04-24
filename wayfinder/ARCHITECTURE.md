# Wayfinder Plugin — Architecture

## System Overview

Wayfinder uses a waypoint orchestrator pattern with three layers: session
management (Go CLI), phase execution (TypeScript), and validation (Go + TS).

```
┌──────────────────────────────────────────────────┐
│              Claude Code / AI Agent              │
└──────────────────┬───────────────────────────────┘
                   │ /wayfinder:start, :next, :close
                   ▼
┌──────────────────────────────────────────────────┐
│           Wayfinder Session Manager              │
│            (cmd/wayfinder-session)               │
│                                                  │
│  Session lifecycle (start/end/status)            │
│  Phase navigation (next/start/complete)          │
│  Validation orchestration                        │
│  Filesystem-as-truth state management            │
└────────┬──────────────────────┬──────────────────┘
         │                      │
         ▼                      ▼
┌────────────────────┐  ┌─────────────────────────┐
│ Phase Orchestrator │  │  Validation Engine      │
│ (TypeScript)       │  │  (Go + TypeScript)      │
│                    │  │                         │
│ Context compiler   │  │ Frontmatter signing     │
│ Signal detector    │  │ Phase boundary checks   │
│ Template builder   │  │ Git claim validation    │
│ Scope validator    │  │ Deliverable checks      │
└────────────────────┘  └─────────────────────────┘
         │                      │
         ▼                      ▼
┌──────────────────────────────────────────────────┐
│           Waypoint Artifacts (Filesystem)        │
│                                                  │
│  W0-charter.md          WAYFINDER-STATUS.md      │
│  D1-problem.md          (regenerable state)      │
│  D2-research.md                                  │
│  ...                                             │
│  S11-retrospective.md                            │
└──────────────────────────────────────────────────┘
```

## Key Packages

### `cmd/wayfinder-session` (Go)

Session lifecycle CLI built with Cobra.

| Subcommand | File | Purpose |
|------------|------|---------|
| `start` | `commands/start.go` | Create session, WAYFINDER-STATUS.md |
| `status` | `commands/status.go` | Show session state |
| `next-phase` | `commands/next_phase.go` | Advance to next phase |
| `start-phase` | `commands/start_phase.go` | Start specific phase |
| `complete-phase` | `commands/complete_phase.go` | Mark phase complete |
| `end` | `commands/end.go` | End session |
| `verify` | `commands/verify.go` | Validate and sign artifact |
| `rewind` | `commands/rewind.go` | Rewind to earlier phase |
| `coord` | `commands/coord.go` | Cross-session coordination |

### `internal/buildloop` (Go)

TDD enforcement state machine for BUILD phase:

```
TEST_FIRST → CODING → GREEN → REFACTOR → VALIDATION → DEPLOY
```

Tracks iteration count, enforces test-first discipline.

### `internal/config` (Go)

Session configuration: project type, risk level, phase graph.

### `internal/git` (Go)

Git operations: worktree management, branch validation, claim verification.

### `internal/history` (Go)

Session history tracking with typed events.

### `internal/lintcontext` (Go)

JIT lint context injection — detects project linter configuration
(ESLint, golangci-lint, pyproject.toml) and injects relevant rules
into BUILD phase context.

### `internal/archive` (Go)

Session archival: compress and store completed session artifacts.

### `internal/migrate` (Go)

V1 → V2 migration tools for session format conversion.

### `cmd/wayfinder-features` (Go)

Feature detection and progress tracking:

| File | Purpose |
|------|---------|
| `commands/init.go` | Initialize feature tracking |
| `commands/start.go` | Start tracking a feature |
| `commands/verify.go` | Verify feature completion |
| `commands/status.go` | Feature status report |
| `internal/progress/` | Progress I/O and schema |
| `internal/s7/parser.go` | S7 plan parser |

## Phase Context Management

Phases load context from dependencies via a directed graph
(`core/cortex/config/phase-dependencies.yaml`):

- **full**: Load complete prior-phase artifact
- **summary**: Load 100-200 token summary
- **(absent)**: Skip entirely

Example: BUILD loads PLAN (full) + CHARTER (summary) + DESIGN (summary).
Does NOT load PROBLEM or RESEARCH.

## Data Flow

### Session Lifecycle

```
wayfinder-session start "Project"
  → Create WAYFINDER-STATUS.md (V2 schema, 9 phases)
  → W0 detector: analyze request vagueness (score ≥ 0.60 → framing questions)
  → Publish session.started event

wayfinder-session next-phase
  → Read WAYFINDER-STATUS.md → determine current phase
  → Phase orchestrator compiles context from dependency graph
  → Signal detector analyzes for progressive rigor
  → AI executes phase methodology → creates artifact
  → Validation engine signs artifact (frontmatter checksum)

wayfinder-session complete-phase D1 --outcome success
  → Verify D1 artifact signature
  → Mark D1 complete in WAYFINDER-STATUS.md
  → Next call to next-phase returns D2
```

### Progressive Rigor

```
Signal detector analyzes D1-D3 outputs
  → Keywords (OAuth, HIPAA, compliance) + effort + context
  → Confidence score (0.0-1.0)
  → ≥ 0.80 confidence → auto-escalate rigor level
  → 4 levels: Minimal / Standard / Thorough / Comprehensive
```

## Design Decisions

- **Filesystem as source of truth**: Phase files contain all state via YAML
  frontmatter signatures. No SQLite or hidden state. Git history is the
  audit trail.
- **9-phase structure**: Consolidated from V1's 13 waypoints. Sequential,
  mandatory checkpoints with validation gates.
- **Dual-language**: Go for CLI/validation (fast, typed), TypeScript for
  phase orchestration (flexible prompt construction).
- **Progressive rigor with auto-escalation**: Reduces overhead for simple
  tasks while ensuring thorough review for complex/sensitive features.
