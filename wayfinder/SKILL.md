---
name: wayfinder
description: SDLC workflow management with structured 9-phase methodology (CHARTER through RETRO).
allowed-tools:
  - "Bash"
  - "Read"
  - "Write"
  - "Edit"
  - "Glob"
  - "Grep"
  - "AskUserQuestion"
  - "Task"
metadata:
  version: 2.0.0
  author: engram
  activation_patterns:
    - "/wayfinder"
    - "wayfinder project"
    - "start wayfinder"
    - "SDLC workflow"
    - "discovery phases"
---

# wayfinder Skill

**Purpose**: Guide projects through structured SDLC workflow with 9 phases (V2): CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, RETRO.

**When to use**: Planning/implementing features, complex projects, multi-phase work requiring validation gates, stakeholder alignment.

**Invocation**: `/wayfinder-start "<project-description>"` or `wayfinder-start "<project-description>"`

---

## Workflow

### Phase 1: CHARTER

**Purpose**: Define problem, scope, success criteria, stakeholder questions

**Commands**:
```bash
wayfinder-start "<concise-project-description>"
```

**Creates**: Project directory with W0-charter.md

**Deliverable**: Charter with problem statement, scope, success criteria, stakeholder alignment questions

---

### Phase 2: PROBLEM

- Validate problem exists (evidence, impact, stakeholders)
- Search for existing solutions in codebase
- Decision: Real problem vs misunderstanding
- *Reasoning Mode*: `ultrathink` when auto-escalated to thorough/comprehensive

### Phase 3: RESEARCH

- Search for internal solutions (existing code, libraries)
- Research external solutions (tools, services, libraries)
- Decision: Build from scratch vs Buy vs Adapt existing (70% overlap threshold)

### Phase 4: DESIGN

- Compare viable approaches (pros/cons/tradeoffs)
- Technical feasibility, timeline, resource assessment
- Detailed design (architecture, data models, APIs)
- Design review
- *Reasoning Mode*: `ultrathink` for architecture decisions and multi-component designs

### Phase 5: SPEC

- Functional requirements (what system must do)
- Non-functional requirements (performance, security, scalability)
- Acceptance criteria (testable outcomes)

### Phase 6: PLAN

- Present approach to stakeholders, get alignment
- Break down into tasks with estimates
- Identify dependencies, risks
- Resource allocation

### Phase 7: SETUP

- Deep dive into selected approach
- Prototype, proof-of-concept, spikes
- Validate assumptions
- Set up development environment

### Phase 8: BUILD

- Execute plan with TDD enforcement (ADR-002 BUILD loop)
- State machine: TEST_FIRST -> CODING -> GREEN -> REFACTOR -> VALIDATION -> DEPLOY
- JIT linting context injected automatically (`lintcontext` package)
- Quality telemetry emitted (`telemetry` package — token-quality pipeline)
- Unit + integration tests

### Phase 9: RETRO

- What worked, what didn't, lessons learned
- Metrics (estimated vs actual effort)
- Update methodology based on learnings

---

## Phase Context Management

Each phase receives context from its dependencies via a **dependency graph** (not linear loading). The graph is configured in `core/cortex/config/phase-dependencies.yaml`:

- **full**: Load complete prior-phase artifact
- **summary**: Load 100-200 token summary
- **(absent)**: Skip entirely

Example: BUILD loads PLAN (full) + CHARTER (summary) + DESIGN (summary). It does NOT load PROBLEM or RESEARCH.

See ADR-005 (revised 2026-03-24) for details.

---

## V1 to V2 Phase Name Mapping

| V1 Name | V2 Name | Notes |
|---------|---------|-------|
| W0 | CHARTER | |
| D1 | PROBLEM | |
| D2 | RESEARCH | |
| D3 | DESIGN | Merged with S6 |
| D4 | SPEC | |
| S4 | PLAN | Merged with S7 |
| S5 | SETUP | |
| S6 | DESIGN | Merged with D3 |
| S7 | PLAN | Merged with S4 |
| S8 | BUILD | Includes S9/S10 (ADR-002) |
| S9 | BUILD | Merged into BUILD loop |
| S10 | BUILD | Merged into BUILD loop |
| S11 | RETRO | |

---

## Commands Reference

| Command | Purpose | Example |
|---------|---------|---------|
| `/wayfinder-start` | Create new project | `wayfinder-start "Implement OAuth"` |
| `/wayfinder-next-phase` | Execute next phase | Auto-progresses through phases |
| `/wayfinder-run-all-phases` | Autopilot mode | Runs all remaining phases automatically |
| `/wayfinder-stop` | Complete project | Mark project as completed/abandoned/blocked |
| `/wayfinder-rewind` | Rewind to earlier phase | Restart from RESEARCH if approach changed |

---

## Wayfinder vs Non-Wayfinder

**Use wayfinder when**:
- Multi-phase project (>1 day effort)
- Need stakeholder alignment
- Requires discovery (build/buy/adapt decision)
- Complex implementation with validation needs

**Don't use wayfinder when**:
- Simple task (<1 hour)
- Single-file change
- Bug fix with obvious solution
- No discovery needed (requirements clear)

---

## Example: OAuth Implementation

**CHARTER**: Problem: Users need Google OAuth login. Scope: Login flow only. Success: Users can login with Google.

**PROBLEM**: Validated problem (user requests, security requirement). Evidence: 45% user survey requested OAuth.

**RESEARCH**: Found Passport.js library (90% overlap, ADAPT decision). Alternative: Auth0 ($2300/month, rejected).

**DESIGN**: Selected Passport.js approach. Designed middleware, routes, session management.

**SPEC**: Requirements: OAuth flow <5s p95, CSRF protection, rate limiting. Tests: Unit + integration + E2E.

**PLAN**: Planned 5 tasks (8hr estimate): middleware (2hr), routes (2hr), UI (1hr), tests (2hr), docs (1hr).

**SETUP**: Prototyped Passport.js integration (validated <3s flow).

**BUILD**: Implemented all 5 tasks with TDD. Actual: 9hr, 12% over. 80% test coverage.

**RETRO**: Worked well, underestimated test time by 1hr.

---

## Troubleshooting

**Problem**: Phase stuck, can't proceed
- **Solution**: Use `/wayfinder-rewind` to restart from earlier phase with new approach

**Problem**: Autopilot makes wrong decision
- **Solution**: Manual phase execution (`/wayfinder-next-phase`) with oversight between phases

**Problem**: Project scope changed mid-flight
- **Solution**: Rewind to CHARTER, update charter, restart discovery

---

## Related Beads

- oss-b74m.4: Wayfinder phases can spawn sub-agents
- oss-b74m.5: Wayfinder phases-as-sub-agents (research)

---

## Documentation

- Phase dependencies: `core/cortex/config/phase-dependencies.yaml`
- ADRs: `docs/wayfinder/ADR-001-phase-consolidation.md`, `ADR-002-build-loop-tdd-enforcement.md`
- Go packages: `core/cortex/cmd/wayfinder-session/internal/{phasegraph,lintcontext,telemetry}/`
- TypeScript: `core/cortex/lib/{phase-definitions,context-compiler}.ts`
