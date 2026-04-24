# Wayfinder Plugin - Specification

**Version**: 0.1.0
**Last Updated**: 2026-02-11
**Status**: Active Development
**Plugin**: wayfinder (core/cortex)

---

## Vision

The Wayfinder plugin provides a structured SDLC workflow for AI-assisted development through 9 sequential phases (W0 charter + D1-D4 discovery + S6-S8 implementation + S11 retrospective). It solves the problem of unstructured AI development where critical steps are skipped, leading to incomplete requirements, missing design reviews, insufficient testing, and no retrospectives.

AI agents excel at rapid implementation but struggle with self-critique and comprehensive planning. Wayfinder acts as a navigation system, guiding AI agents through mandatory checkpoints with multi-persona validation, progressive rigor adaptation, and automated domain expert detection.

---

## Goals

### 1. Structured SDLC Navigation

Provide a sequential waypoint system that ensures AI agents complete all critical development phases before proceeding to implementation.

**Success Metric**: 100% of Wayfinder sessions complete all mandatory phases (W0 → D1-D4 → S6-S8 → S11) with validated artifacts at each step.

### 2. Multi-Persona Validation

Enable comprehensive review by automatically detecting and engaging appropriate domain experts (Security, QA, DevOps, ML Engineer, etc.) based on project context.

**Success Metric**: 70% of identified issues are HIGH impact (prevent critical bugs), 30% MEDIUM impact, 0% waste. Achieve 5:1 ROI (9 hours invested → 45 hours saved in rework).

### 3. Progressive Rigor Adaptation

Automatically adjust workflow depth (Minimal/Standard/Thorough/Comprehensive) based on project complexity signals, reducing overhead for simple tasks while ensuring thorough review for complex/sensitive features.

**Reasoning Mode Guidance**: Critical phases (D1 escalated, S6, S9 Tier 3) include conditional `ultrathink` guidance to activate Claude Code's maximum reasoning depth (31,999 token thinking budget) for complex scenarios.

**Success Metric**: Confidence-based auto-escalation (≥80% confidence) eliminates decision fatigue while catching 90%+ of high-risk projects requiring deeper review.

### 4. Phase Isolation & Scope Validation

Prevent scope creep by validating that phase artifacts contain only content appropriate for that phase, not future-phase content.

**Success Metric**: Detect and prevent 95%+ of scope violations (e.g., implementation details in D3 Approach Decision, acceptance criteria in D1 Problem Validation).

### 5. Context-Efficient Workflow

Reduce context token usage by 40-50% for long projects through waypoint summaries while preserving all critical information.

**Success Metric**: D1-S8 projects use 40-50% fewer tokens via automatic summarization of completed waypoints (2+ steps back).

---

## Architecture

### High-Level Design

Wayfinder uses a **waypoint orchestrator** pattern with three layers: session management (Go), phase execution (TypeScript), and validation (Go + TypeScript).

```
┌──────────────────────────────────────────────────────────┐
│                    Claude Code / AI Agent                │
│                    (User Interaction Layer)              │
└────────────────────────┬─────────────────────────────────┘
                         │
                         v
┌──────────────────────────────────────────────────────────┐
│                Wayfinder Session Manager                 │
│                 (cmd/wayfinder-session)                  │
│                                                           │
│  - Session lifecycle (start/end/status)                  │
│  - Phase navigation (next/start/complete)                │
│  - Validation orchestration                              │
│  - Filesystem-as-truth architecture                      │
└────────┬─────────────────────────────┬───────────────────┘
         │                             │
         v                             v
┌──────────────────────┐    ┌──────────────────────────┐
│  Phase Orchestrator  │    │   Validation Engine      │
│  (lib/*.ts)          │    │   (internal/validator)   │
│                      │    │                          │
│  - Context compiler  │    │  - Frontmatter signing   │
│  - Signal detector   │    │  - Phase boundaries      │
│  - Template builder  │    │  - Git claim validation  │
│  - Scope validator   │    │  - Deliverable checks    │
└──────────┬───────────┘    └──────────┬───────────────┘
           │                           │
           v                           v
┌──────────────────────────────────────────────────────────┐
│              Waypoint Artifacts (Filesystem)             │
│                                                           │
│  W0-project-charter.md                                   │
│  D1-problem-validation.md  ← signed, validated           │
│  D2-existing-solutions.md  ← signed, validated           │
│  ...                                                      │
│  S11-retrospective.md      ← signed, validated           │
│  WAYFINDER-STATUS.md       ← session state (regenerable) │
└──────────────────────────────────────────────────────────┘
```

### Components

**Component 1: Session Manager (Go)**
- **Purpose**: Manage Wayfinder session lifecycle and state
- **Responsibilities**:
  - Start/end sessions with project naming
  - Navigate phases (next-phase, start-phase, complete-phase)
  - Track session status via filesystem (stateless architecture)
  - Coordinate validation and signing
- **Interfaces**: CLI commands (`wayfinder-session start`, `wayfinder-session next-phase`, etc.)

**Component 2: Phase Orchestrator (TypeScript)**
- **Purpose**: Execute individual waypoints with compiled context
- **Responsibilities**:
  - Compile phase context from prior artifacts
  - Detect signals for progressive rigor
  - Build templates for phase execution
  - Validate scope isolation
- **Interfaces**: `PhaseOrchestrator.executePhase()`, `ContextCompiler.compile()`

**Component 3: Validation Engine (Go + TypeScript)**
- **Purpose**: Validate and sign waypoint artifacts
- **Responsibilities**:
  - Sign validated artifacts with cryptographic checksums
  - Check phase boundary violations (scope creep)
  - Validate deliverables against requirements
  - Verify git claims (D2, S9)
- **Interfaces**: `validator.ValidateAndSign()`, `ScopeValidator.validate()`

**Component 4: Signal Detector (TypeScript)**
- **Purpose**: Detect project complexity signals for progressive rigor
- **Responsibilities**:
  - Keyword analysis (HIPAA, OAuth, compliance, etc.)
  - Effort estimation from context
  - Domain expert detection (ML, Mobile, Fintech, etc.)
  - Confidence scoring (0.0-1.0)
- **Interfaces**: `SignalDetector.detect()`, `DomainDetector.detect()`

**Component 5: W0 Detector (TypeScript)**
- **Purpose**: Detect vague requests requiring project framing
- **Responsibilities**:
  - Analyze user request for 5 vagueness signals
  - Calculate vagueness score (0.0-1.0)
  - Generate framing questions if score ≥ 0.60
  - Skip W0 if detailed charter exists
- **Interfaces**: `W0Detector.shouldActivate()`, `W0Questions.generate()`

### Data Flow

1. **Session Initialization**:
   - User starts Wayfinder session: `wayfinder-session start "Add OAuth"`
   - Session manager creates WAYFINDER-STATUS.md with initial state
   - W0 Detector analyzes request, triggers framing if vague (score ≥ 0.60)

2. **Waypoint Execution**:
   - User requests next phase: `/wayfinder-next-phase` (skill invocation)
   - Session manager calls `next-phase` → returns D1
   - Phase orchestrator compiles context (no prior artifacts for D1)
   - AI agent executes D1 methodology, creates D1-problem-validation.md
   - Validation engine signs artifact with frontmatter checksum

3. **Phase Navigation**:
   - User completes D1: `wayfinder-session complete-phase D1 --outcome success`
   - Validator checks D1-problem-validation.md signature
   - Session manager marks D1 complete, updates WAYFINDER-STATUS.md
   - Next call to `next-phase` returns D2

4. **Progressive Rigor**:
   - Signal detector analyzes D1-D3 outputs for complexity signals
   - Detects "OAuth" keyword + "security" → confidence 0.85
   - Auto-escalates to Thorough rigor for S4-S11
   - Reports reasoning: "Using thorough level (confidence 0.85) because: OAuth security implications, 2 integration points"

5. **Multi-Persona Validation**:
   - S6 (Design) triggers reviewer selection
   - Domain detector analyzes context → detects ML project
   - Assigns reviewers: Tech Lead, Security, QA, ML Engineer
   - Each persona reviews design document, provides feedback

6. **Context Compression**:
   - After completing S7 (Plan), summaries generated for D1-D4
   - S8 loads: Full S7 (1 step back) + Summaries D1-D4 (2+ steps back)
   - Token usage: 2000 tokens → 800 tokens (60% reduction)

### Key Design Decisions

- **Decision: Filesystem as Source of Truth** (See ADR-001)
  - Phase files contain all state via YAML frontmatter signatures
  - No SQLite database or hidden state files
  - Git history provides complete audit trail

- **Decision: 9-Phase Structure** (See ADR-002, superseded by ADR-001-phase-consolidation)
  - W0 charter + D1-D4 discovery + S6-S8 implementation + S11 retrospective
  - Sequential, mandatory checkpoints (cannot skip)
  - Validation gates between waypoints

- **Decision: Progressive Rigor with Auto-Escalation** (See ADR-003)
  - 4 rigor levels (Minimal/Standard/Thorough/Comprehensive)
  - Confidence-based auto-escalation (≥80% confidence)
  - Multi-signal detection (keywords, effort, context)

- **Decision: Multi-Persona Review** (See ADR-004)
  - 9 core personas + 4 domain experts (auto-detected)
  - Phase-specific reviewer assignment (S6 critical)
  - 5:1 ROI target (70% HIGH impact, 30% MEDIUM, 0% waste)

---

## Success Metrics

### Primary Metrics

- **Waypoint Completion Rate**: 100% of sessions complete all mandatory waypoints
- **Validation Pass Rate**: 95%+ of first-attempt validations pass (indicates clear methodology)
- **Rework Reduction**: Post-completion fixes reduced from 33.7% to <10%

### Secondary Metrics

- **Multi-Persona ROI**: 5:1 return (9 hours invested → 45 hours saved)
- **Token Efficiency**: 40-50% reduction for D1-S8 projects via summaries
- **Auto-Escalation Accuracy**: 90%+ of high-risk projects correctly identified
- **Phase Isolation**: 95%+ scope violations detected and prevented

### Quality Metrics

- **Test Coverage**: ≥80% for TypeScript orchestration, ≥70% for Go validation
- **Documentation**: README, SPEC, ARCHITECTURE, ADRs for all major decisions
- **Signal Detection Accuracy**: Precision ≥85% for domain expert detection

---

## What This SPEC Doesn't Cover

- **Multi-Project Sessions**: One Wayfinder session per project (no simultaneous projects)
- **Branch Management**: Wayfinder doesn't manage git branches (recommendation: use git-worktrees plugin)
- **Code Review Integration**: No GitHub/GitLab PR integration (manual PR creation)
- **Team Coordination**: Single-user sessions (no multi-user collaboration)
- **Custom Waypoints**: Fixed 9-phase structure (no custom phases)
- **Waypoint Reordering**: Strict sequential order (no phase skipping or reordering)

Future considerations:
- Multi-project session support (track multiple Wayfinder sessions)
- Git branch automation (auto-create feature branches)
- PR template generation (auto-generate PR descriptions from waypoints)
- Team session sharing (shared Wayfinder sessions across team)

---

## Assumptions & Constraints

### Assumptions

- AI agents run in environments with filesystem access (read/write phase artifacts)
- Git repository exists for project (used for validation, archiving)
- Claude Code or compatible AI agent environment (skill system, tool access)
- UTF-8 text files for all artifacts (no binary format support)
- Single-threaded session execution (no concurrent phase execution)

### Constraints

- **Dependency Constraints**:
  - Node.js ≥18.0.0 for TypeScript orchestration
  - Go ≥1.21 for session management and validation
  - Git for version control and validation
  - Claude 3.5 Haiku for waypoint summarization (~$0.01-0.03 per summary)
- **Phase Constraints**:
  - Maximum 9 phases (W0 + D1-D4 + S6-S8 + S11)
  - Sequential execution only (no parallel phases)
  - Artifacts must be markdown files with YAML frontmatter
- **Session Constraints**:
  - One active session per project directory
  - Session state stored in WAYFINDER-STATUS.md (regenerable from artifacts)
  - Cannot resume abandoned sessions (must start new)

---

## Dependencies

### External Libraries

**TypeScript/Node.js**:
- `unified` + `remark-parse` - Markdown parsing for scope validation
- `fast-levenshtein` - Fuzzy section name matching (75% threshold)
- `vitest` - Unit and integration testing

**Go**:
- `github.com/spf13/cobra` - CLI framework for wayfinder-session
- `gopkg.in/yaml.v3` - YAML frontmatter parsing
- `golang.org/x/crypto/sha256` - Cryptographic checksums for signatures

### Internal Dependencies

- **engram/core/pkg/progress** - Progress indicators for phase execution
- **engram/core/pkg/eventbus** - Event telemetry (optional)
- **engram/core/pkg/ecphory** - Knowledge base integration (optional)

---

## API Reference

### Session Management (CLI)

```bash
# Start new session
wayfinder-session start "Project Name"
wayfinder-session -C /path/to/project start "Project Name"

# Get next phase
wayfinder-session next-phase

# Start specific phase
wayfinder-session start-phase D1
wayfinder-session start-phase S6

# Complete phase
wayfinder-session complete-phase D1 --outcome success
wayfinder-session complete-phase D2 --outcome blocked --reason "Needs approval"

# Session status
wayfinder-session status
wayfinder-session status --force-fs  # Rebuild from filesystem

# End session
wayfinder-session end
```

### Phase Orchestration (TypeScript)

```typescript
import { PhaseOrchestrator } from '@wayfinder/core';

const orchestrator = new PhaseOrchestrator({
  projectPath: '/path/to/project',
  sessionId: 'uuid-v4',
  startPhase: 'D1',  // Optional: resume from phase
});

const result = await orchestrator.executePhase('D1');
// Returns: PhaseResult with artifact, status, tokenCount
```

### Validation (Go)

```go
import "github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/validator"

// Validate and sign artifact
result := validator.ValidateAndSign("D1-problem-validation.md")

// Check phase boundaries
violations := validator.CheckPhaseBoundaries("D3", "D3-approach-decision.md")

// Verify signature
valid := validator.VerifySignature("D1-problem-validation.md")
```

### Signal Detection (TypeScript)

```typescript
import { SignalDetector } from '@wayfinder/core';

const detector = new SignalDetector();
const signals = detector.detect({
  userRequest: "Add OAuth authentication",
  priorArtifacts: ["D1-problem-validation.md"],
});
// Returns: { confidence: 0.85, level: 'thorough', reasons: [...] }
```

---

## Testing Strategy

### Unit Tests (TypeScript)

- Phase orchestration logic (context compilation, template building)
- Signal detection accuracy (keyword matching, confidence scoring)
- Scope validation (section parsing, boundary detection)
- W0 vagueness detection (5 signal analysis)

**Coverage Target**: ≥80%

### Unit Tests (Go)

- Session lifecycle management (start/end/status)
- Phase navigation (next-phase, complete-phase)
- Validation logic (frontmatter parsing, signature verification)
- Filesystem state reconstruction

**Coverage Target**: ≥70%

### Integration Tests

- End-to-end session flow (W0 → D1 → ... → S11)
- Multi-persona validation workflow
- Progressive rigor escalation
- Context compression (summaries)

### Manual Tests

- Signal detection accuracy (test with real projects)
- Domain expert detection (ML, Mobile, Fintech)
- Scope validation error messages (user-friendly)
- Automated execution (`/wayfinder-run-all-phases`)

---

## Version History

- **0.1.0** (2026-02-11): Initial specification and documentation backfill

---

**Note**: This plugin is in active development. The phase structure (9 phases, consolidated from V1's 13 waypoints) is stable, but individual features (progressive rigor, domain detection) are evolving based on usage data. See ARCHITECTURE.md for detailed design and docs/adr/ for decision rationale.
