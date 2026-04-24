# Wayfinder V2 Specification

## Overview

Wayfinder V2 consolidates 13 phases into 9 streamlined phases with native task/roadmap management embedded in WAYFINDER-STATUS.md. This eliminates the need for separate ROADMAP.md files and provides tight integration with the Engram ecosystem.

## Executive Summary

**Problem**: Three overlapping systems for roadmap/phase management create confusion:
- Wayfinder phases (13 phases)
- Swarm ROADMAP.md (manual markdown)
- Autonomous TASK-QUEUE.yaml (auto-generated, duplicative)

**Solution**: Consolidate into Wayfinder V2:
- 9 phases (down from 13)
- Native task tracking in WAYFINDER-STATUS.md
- BUILD loop for S8 (tight TDD feedback cycle)
- Risk-adaptive multi-persona review

## Phase Structure

### 9-Phase Wayfinder V2

| Phase | Name | Purpose |
|-------|------|---------|
| CHARTER | Project Intake & Bootstrapping | Framing, scope, constraints |
| PROBLEM | Problem Definition & Research | Validate problem exists |
| RESEARCH | Solution Exploration | Build/buy/enhance analysis |
| DESIGN | Detailed Design | Architecture, ADRs |
| SPEC | Requirements Sign-off & Test Outline | Requirements + stakeholder approval (merged S4) |
| PLAN | Implementation Planning & Test Scenarios | Gherkin BDD, task breakdown (merged S5) |
| SETUP | Development Environment Setup | CI/CD, dependencies |
| BUILD | BUILD Loop | TDD cycle: test-writer -> implementer -> reviewer (merged S8/S9/S10) |
| RETRO | Documentation & Knowledge Transfer | Retrospective, learnings |

### Phase Merges

**S4 → SPEC**: Requirements validation + Stakeholder sign-off unified
**S5 → PLAN**: Research + Implementation planning combined
**S8/S9/S10 → BUILD**: Test → Code → Deploy unified into BUILD loop

### Sub-Agent Architecture

Each phase delegates work to a specialized sub-agent with restricted tools (5-7 per agent)
to keep context windows small and focused. Agent definitions in `.claude/agents/`:

| Phase | Agent | Model | Key Tools |
|-------|-------|-------|-----------|
| PROBLEM, RESEARCH | researcher | sonnet | Read, Grep, Glob, WebSearch, WebFetch |
| DESIGN | designer | opus | Read, Grep, Glob, Write, Edit |
| PLAN | planner | sonnet | Read, Grep, Glob, Write, Edit |
| BUILD (tests) | test-writer | sonnet | Read, Write, Edit, Bash, Grep, Glob |
| BUILD (code) | implementer | sonnet | Read, Write, Edit, Bash, Grep, Glob |
| BUILD (review) | reviewer | sonnet | Read, Grep, Glob, Bash |

The BUILD phase uses a multi-agent TDD loop:
1. test-writer produces failing tests (red)
2. implementer makes tests pass (green)
3. reviewer verifies against spec (pass/fail)
4. If fail: loop to step 2 (max 3 retries)

Context budgets are model-aware via `engram context check` (no hardcoded sizes).

## WAYFINDER-STATUS.md V2 Schema

```yaml
schema_version: "2.0"
session_id: "abc123"
project_path: "/path/to/project"
project_name: "Example Project"
project_type: "feature|bugfix|refactor|infrastructure"
risk_level: "XS|S|M|L|XL"
current_phase: "CHARTER|PROBLEM|RESEARCH|DESIGN|SPEC|PLAN|SETUP|BUILD|RETRO"
current_task: "BUILD-1"
status: "planning|in_progress|blocked|completed"
lifecycle_state: "active|archived|abandoned"

# Timestamps
created_at: "2026-02-20T10:00:00Z"
updated_at: "2026-02-20T15:30:00Z"
completion_date: "2026-02-25T18:00:00Z"

# Phase history
phases:
  - name: "W0"
    status: "completed"
    started_at: "2026-02-20T10:00:00Z"
    completed_at: "2026-02-20T12:00:00Z"
    deliverables:
      - "SPEC.md"
      - "ARCHITECTURE.md"

# Native roadmap structure
roadmap:
  phases:
    - id: "S8"
      name: "BUILD Loop"
      status: "in_progress"
      tasks:
        - id: "S8-1"
          title: "Implement authentication"
          effort_days: 2.0
          status: "completed"
          deliverables:
            - "src/auth/login.go"
            - "src/auth/login_test.go"
          tests_status: "passed"
          depends_on: []
          blocks: ["S8-2"]
        - id: "S8-2"
          title: "Add authorization"
          effort_days: 1.5
          status: "in_progress"
          depends_on: ["S8-1"]
          blocks: []

# Quality metrics
quality:
  coverage_percent: 85.5
  coverage_target: 80.0
  assertion_density: 3.2
  assertion_density_target: 3.0
  multi_persona_score: 88.0
  security_score: 92.0
  performance_score: 85.0
  reliability_score: 90.0
  maintainability_score: 82.0
  p0_issues: 0
  p1_issues: 3
  p2_issues: 7

# Test tracking
tests:
  outline_path: "TESTS.outline"
  feature_path: "TESTS.feature"
  outline_created_at: "2026-02-21T10:00:00Z"
  feature_created_at: "2026-02-22T14:00:00Z"
```

## BUILD Loop (S8 Phase)

### State Machine

The BUILD loop replaces the linear S8→S9→S10 sequence with a tight TDD feedback cycle:

```
┌─────────────────────────────────────────────┐
│              BUILD LOOP (S8)                 │
└─────────────────────────────────────────────┘
         │
         ▼
    ┌──────────────┐
    │ TEST_FIRST   │  Tests fail as expected
    └──────────────┘
         │
         ▼
    ┌──────────────┐
    │   CODING     │  Write minimal code to pass
    └──────────────┘
         │
         ▼
    ┌──────────────┐
    │    GREEN     │  Tests pass
    └──────────────┘
         │
         ▼
    ┌──────────────┐
    │  REFACTOR    │  Improve code quality
    └──────────────┘
         │
         ▼
    ┌──────────────┐
    │ VALIDATION   │  Multi-persona review
    └──────────────┘
         │
         ▼
    ┌──────────────┐
    │   DEPLOY     │  Integration testing
    └──────────────┘
         │
         ▼
    ┌──────────────┐
    │ MONITORING   │  Observe behavior
    └──────────────┘
         │
         ▼
    ┌──────────────┐
    │  COMPLETE    │  Task done, next task
    └──────────────┘
```

### TDD Enforcement

**Discipline**: Tests MUST fail before implementation, pass after.

**Assertion Density**: Minimum 0.3 assertions per 10 LOC to prevent gaming (e.g., `assert true`).

**Coverage Target**: 80% minimum for new code.

## Test-First Workflow

### D4: Test Outline

High-level acceptance criteria:

```markdown
## Authentication

**AC1**: User can log in with email/password
**AC2**: Invalid credentials return 401
**AC3**: Successful login returns JWT token
**AC4**: Token expires after 1 hour
```

### S6: Concrete Test Scenarios

Convert outline to Gherkin BDD scenarios:

```gherkin
Feature: User Authentication

  Scenario: Successful login
    Given a user with email "user@example.com" and password "secret123"
    When the user submits login credentials
    Then the response status is 200
    And the response contains a valid JWT token
    And the token expires in 3600 seconds

  Scenario: Invalid credentials
    Given a user with email "user@example.com"
    When the user submits password "wrongpassword"
    Then the response status is 401
    And the response contains error "Invalid credentials"
```

## Task Management

### CLI Commands

```bash
# Add task to roadmap
wayfinder-session task add S8 "Implement OAuth" --effort 3.0 --priority P0

# Update task status
wayfinder-session task update S8-1 --status completed --tests-status passed

# List tasks
wayfinder-session task list --phase S8 --status in_progress

# Show task details
wayfinder-session task show S8-1

# Delete task
wayfinder-session task delete S8-2
```

### Dependency Management

**Cyclic Detection**: DFS-based algorithm prevents circular dependencies.

**Validation**: All `depends_on` and `blocks` references must exist.

**Auto-blocking**: Completing a task automatically unblocks dependent tasks.

## Risk-Adaptive Review

### Risk Levels

| Level | LOC Range | Review Strategy |
|-------|-----------|-----------------|
| XS    | 1-50      | Batch review    |
| S     | 51-200    | Batch review    |
| M     | 201-500   | Batch review    |
| L     | 501-1000  | Per-task review |
| XL    | 1001+     | Per-task review |

### Multi-Persona Review

**Personas**:
- Security: SQL injection, XSS, hardcoded secrets
- Performance: N+1 queries, memory leaks, inefficient algorithms
- Maintainability: Code complexity, TODOs, code smells
- UX: Error messages, accessibility, user feedback
- Reliability: Error handling, edge cases, nil checks

**Scoring**: Severity-based (P0=-25, P1=-15, P2=-8, P3=-3)

**Blocking**: P0/P1 always block, P2 blocks XL projects only

## Migration

### V1 → V2 Conversion

**Phase Mapping**:
- W0 → W0 (unchanged)
- D1-D4 → D1-D4 (D4 absorbs S4 stakeholder data)
- S5 → S6 (research notes preserved)
- S6 → S7 (environment setup)
- S8/S9/S10 → S8 (BUILD loop)
- S11 → S11 (documentation)

**Data Preservation**: 100% of V1 data migrated to V2 fields.

**Validation**: Schema validator ensures no data loss.

## Quality Gates

### Phase Transition Gates

**D4 Exit Criteria**:
- TESTS.outline created
- Stakeholder approved
- Requirements signed off

**S6 Exit Criteria**:
- TESTS.feature created
- All scenarios in Gherkin format
- Dependencies validated

**S8 Exit Criteria**:
- All tasks completed
- All tests passing
- Coverage ≥ 80%
- Assertion density ≥ 0.3
- P0/P1 issues = 0

## Implementation

### Module Structure

```
cmd/wayfinder-session/internal/
├── status/           # V2 schema parser & validator
├── orchestrator/     # Phase transition logic
├── taskmanager/      # Task CRUD & CLI
├── buildloop/        # S8 state machine
└── review/           # Multi-persona review engine
```

### External Integration

**Engram Swarm**: Reads Wayfinder tasks via CLI, no phase management.

**Beads**: Git-native issue tracking, optional integration.

**A2A**: Agent-to-agent communication for multi-agent workflows.

## Success Criteria

✅ All 5 packages compile successfully
✅ All tests pass (100% pass rate)
✅ Test coverage >70% for all packages
✅ Schema validates V2 YAML files
✅ BUILD loop enforces TDD discipline
✅ Multi-persona review detects P0/P1 issues
✅ Task dependency validation prevents cycles
✅ Phase orchestrator prevents invalid transitions

---

**Version**: 2.0
**Status**: Implemented
**Last Updated**: 2026-02-20
