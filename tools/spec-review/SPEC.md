# Product Specification: Spec-Review Marketplace

**Version:** 2.0.0
**Last Updated:** 2026-03-17
**Status:** Production-Ready (All 10 Phases Complete, Security Hardened)
**Contributors:** Claude Sonnet 4.5
**Stakeholders:** Engram users, Wayfinder users, Documentation teams, Architecture teams

---

## 1. Vision

**What is this:** A cross-CLI plugin marketplace that provides:
1. **LLM-as-judge documentation review** (SPEC.md, ARCHITECTURE.md, ADR reviews)
2. **Diagram-as-code capabilities** (C4 Model, D2, Structurizr DSL, Mermaid)
3. **Automated diagram generation** from codebase analysis
4. **Diagram-code sync detection** to prevent documentation drift
5. **Unified abstraction layer** supporting Claude Code, Gemini CLI, OpenCode, and Codex

**Problem Statement:**

Software documentation is frequently low quality or missing:
1. **No enforcement**: Documentation is optional (nice-to-have not required)
2. **No quality standards**: Unclear what "good" documentation looks like
3. **No feedback loop**: Teams don't know if docs are useful until too late
4. **Discovery happens late**: Design flaws found in implementation (not design phase)
5. **Stale diagrams**: Architecture diagrams outdated, stored as binary images, disconnected from code

This leads to:
- Architectural drift (implementation diverges from intent)
- Knowledge loss (future maintainers can't understand decisions)
- Repeated mistakes (same architectural debates resurface)
- Poor AI agent performance (agents hallucinate without clear specifications)
- Misleading diagrams (outdated architecture diagrams cause confusion)

**Who Benefits:**

- **Primary users:** AI agents executing Wayfinder/Swarm projects
  - Get clear specifications before implementation
  - Avoid hallucinating requirements
  - Produce testable, verifiable work

- **Secondary users:** Human developers using Engram workflows
  - Understand project goals before implementation
  - Reference architectural decisions during implementation
  - Onboard to projects faster (read SPEC/ARCHITECTURE/ADRs)

- **Tertiary users:** Project stakeholders and reviewers
  - Review specifications before expensive implementation
  - Provide feedback at design stage
  - Track architectural decisions over time

**Product Vision:**

Every software project has living, high-quality documentation that guides implementation, enables validation, and captures architectural knowledge - validated through automated quality gates powered by LLM-as-judge, with architecture diagrams automatically generated from code and kept in sync through CI/CD integration.

---

## 2. User Personas

### Persona 1: AI Agent (Claude Executing Wayfinder Project)

**Demographics:** Claude Sonnet 4.5, autonomous execution mode, multi-phase Wayfinder project

**Goals:**
- Understand project requirements before implementation
- Validate work against clear success criteria
- Avoid hallucinating features or requirements
- Produce work that passes quality gates

**Pain Points:**
- Vague user requests lead to incorrect implementations
- No clear success metrics → can't verify work quality
- Architectural decisions undocumented → repeated design debates
- Low-quality existing docs → hard to extend/modify systems

**Behaviors:**
- Prefers declarative specifications (what/why not how)
- Needs measurable success criteria (testable outcomes)
- References documentation throughout implementation
- Validates work against specifications continuously

**Jobs-to-be-Done:**
- When starting a project, I need clear requirements so I implement the right solution
- When making architectural decisions, I need documentation so future agents understand rationale
- When validating work, I need success criteria so I know if it's correct

### Persona 2: Human Developer (Using Engram for Complex Project)

**Demographics:** Software engineer, 5+ years experience, multi-week feature development

**Goals:**
- Document architectural decisions for future reference
- Get stakeholder approval before extensive implementation
- Create specifications that guide AI agent execution
- Maintain knowledge base of design rationale

**Pain Points:**
- No time to write documentation (pressure to ship)
- Unclear documentation standards (what to include?)
- Documentation gets stale (not updated with changes)
- Poor documentation quality blocks project progress

**Jobs-to-be-Done:**
- When designing a system, I need to document architecture so others can extend it
- When making trade-off decisions, I need to capture rationale so future me understands
- When onboarding to a project, I need clear specifications so I understand quickly

---

## 3. Critical User Journeys (CUJs)

### CUJ 1: AI Agent Validates SPEC.md Before Implementation (Toothbrush)

**Goal:** Ensure SPEC.md meets quality standards before proceeding to implementation

**Lifecycle Stage:** Adoption (daily usage in Wayfinder workflow)

**Tasks:**

#### Task 1: Reach Wayfinder D4 Phase
- **Intent:** Complete discovery phases, ready to document requirements
- **Action:** Execute D1-D3 phases (problem validation, existing solutions, approach decision)
- **Success Criteria:** D3 complete with chosen approach documented

#### Task 2: Invoke SPEC Review
- **Intent:** Validate SPEC.md quality before stakeholder alignment
- **Action:** Run `/review-spec docs/SPEC.md`
- **Success Criteria:**
  - LLM-as-judge scores SPEC.md on quality rubric
  - Multi-Persona review provides feedback
  - Pass/fail decision (≥8/10 = pass)

#### Task 3: Review Feedback
- **Intent:** Understand quality issues
- **Action:** Review structured feedback
  - Overall score (0-10)
  - Dimension scores (Vision, CUJs, Metrics, Scope, Living Doc)
  - Specific improvement suggestions
- **Success Criteria:** Feedback is actionable (specific sections, not generic advice)

#### Task 4: Improve SPEC.md (If Score < 8/10)
- **Intent:** Fix quality issues
- **Action:** Update SPEC.md based on feedback
- **Success Criteria:** Apply improvements, re-run validation

#### Task 5: Pass Quality Gate
- **Intent:** Proceed to S4 phase
- **Action:** Achieve score ≥8/10 OR explicitly override with rationale
- **Success Criteria:** Quality gate passed, ready for stakeholder alignment

**Metrics:**
- % completing all tasks: Target ≥90%
- Time-to-pass: p95 <10 minutes (includes fixes)
- Review cost: <$0.50 per validation
- Review quality: ≥85% of users find feedback actionable

---

### CUJ 2: Developer Reviews Architecture Before Implementation (Pivotal)

**Goal:** Validate ARCHITECTURE.md quality before implementation planning

**Lifecycle Stage:** Acquisition (first-time architecture review)

**Tasks:**

#### Task 1: Reach Wayfinder S6 Phase
- **Intent:** Complete research and stakeholder alignment
- **Action:** Execute S4-S5 phases

#### Task 2: Invoke Architecture Review
- **Intent:** Validate architecture design quality
- **Action:** Run `/review-architecture docs/ARCHITECTURE.md`
- **Success Criteria:**
  - Dual-layer validation (traditional + agentic architecture)
  - Visual diagrams present (C4 Model)
  - Architectural decisions reference ADRs

#### Task 3: Pass Architecture Gate
- **Intent:** Proceed to S7 (implementation planning)
- **Action:** Achieve score ≥8/10
- **Success Criteria:** Architecture validated, ready for implementation

**Metrics:**
- % completing all tasks: Target ≥85%
- Architecture quality: Average score ≥8.5/10

---

## 4. Goals & Success Metrics

### Goal 1: Enforce High-Quality Documentation

**Description:** LLM-as-judge validation ensures documentation meets research-based quality standards.

**North Star Metric:** "≥95% of Wayfinder projects have high-quality documentation (score ≥8/10)"

**Success Criteria:**

#### Primary Metrics
- **Documentation existence rate:** ≥95% of Wayfinder projects have SPEC.md by D4
- **Quality gate pass rate:** ≥80% pass on first attempt (score ≥8/10)
- **Override rate:** <15% override quality gates (indicates gates are reasonable)

#### Secondary Metrics (Quality)
- **Agent hallucination reduction:** Agents reference specifications before claiming completion
- **Specification drift:** Zero cases of implementation diverging from SPEC without updating SPEC
- **Documentation staleness:** <10% flagged as outdated (last updated >30 days)

#### Efficiency Metrics (Agentic)
- **Validation time:** p95 <5 minutes
- **Validation cost:** <$0.50 per validation
- **Token usage:** <20K tokens per validation

**How to Measure:**
- Data source: Wayfinder telemetry, quality gate logs
- Collection method: Automatic logging of validation scores, pass/fail rates
- Validation approach: Weekly analysis of gate effectiveness

### Goal 2: Provide Actionable Feedback

**Description:** When documentation fails quality gates, provide specific improvement suggestions.

**North Star Metric:** "Users fix documentation issues in <5 minutes using feedback"

**Success Criteria:**

#### Primary Metrics
- **Improvement effectiveness:** ≥85% who apply suggestions pass on retry
- **Feedback actionability:** ≥90% rated "actionable" by users
- **Time to fix:** p95 <10 minutes from feedback to passing

#### Secondary Metrics
- **Feedback specificity:** Suggestions reference specific sections
- **No contradictions:** Multi-Persona feedback doesn't contradict
- **Learning curve:** Users improve scores over time (skill building)

### Goal 3: Enable Cross-CLI Skill Portability

**Description:** Same review skills work on all supported CLIs.

**North Star Metric:** "100% of marketplace skills work on all 4 CLIs"

**Success Criteria:**

#### Primary Metrics
- **CLI compatibility rate:** 100% of skills work on all 4 CLIs
- **Cross-CLI consistency:** <5% output variance
- **Adoption rate:** ≥50% of users try skills on multiple CLIs

---

## 5. Feature Prioritization (MoSCoW)

### Must Have (Phase 1 - Complete)

**M1: CLI Abstraction Layer (Python)**
- Why critical: Foundation for cross-CLI compatibility
- Effort: 6 hours
- Status: ✅ Complete

**M2: Marketplace Structure**
- Why critical: Defines skill organization
- Effort: 4 hours
- Status: ✅ Complete

**M3: Testing Infrastructure**
- Why critical: Ensures cross-CLI compatibility
- Effort: 4 hours
- Status: ✅ Complete

**M4: Quality Rubric**
- Why critical: Defines what "good" documentation looks like
- Effort: 2 hours
- Status: ✅ Complete

### Must Have (Phase 3 - Complete)

**M5: review-spec Skill**
- Why critical: Core use case (SPEC.md validation)
- Effort: 4 hours (migrated)
- Status: ✅ Complete

**M6: review-adr Skill**
- Why critical: ADR validation
- Effort: 4 hours (migrated)
- Status: ✅ Complete

**M7: review-architecture Skill**
- Why critical: Architecture validation (ENHANCED with diagram validation)
- Effort: 4 hours (migrated) + 8 hours (diagram enhancement)
- Status: ✅ Complete (Phase 6 enhanced)

**M8: create-spec Skill**
- Why critical: Automated spec generation
- Effort: 6 hours
- Status: ✅ Complete

### Must Have (Phases 2-5 - Diagram-as-Code - Complete)

**M9: render-diagrams Skill**
- Why critical: Compile diagram-as-code to images (PNG/SVG/PDF)
- Effort: 12 hours (Phase 2)
- Status: ✅ Complete
- Formats: D2, Structurizr DSL, Mermaid, PlantUML
- Tests: 16 tests passing

**M10: create-diagrams Skill**
- Why critical: Auto-generate C4 diagrams from codebase
- Effort: 20 hours (Phase 3)
- Status: ✅ Complete
- Features: Multi-language analysis (Go, Python, TypeScript, Java), template-based generation
- Tests: Unit tests passing

**M11: review-diagrams Skill**
- Why critical: Multi-persona diagram quality validation
- Effort: 16 hours (Phase 4)
- Status: ✅ Complete
- Rubric: C4 correctness (30%), Visual clarity (25%), Technical accuracy (25%), Documentation (10%), Maintainability (10%)
- Personas: System Architect, Technical Writer, Developer, DevOps

**M12: diagram-sync Skill**
- Why critical: Detect drift between diagrams and code
- Effort: 14 hours (Phase 5)
- Status: ✅ Complete
- Features: Diagram parser, codebase analyzer, sync scoring, patch generation
- Integration: CI/CD workflows, pre-commit hooks

### Must Have (Phase 6 - Existing Skill Enhancements - Complete)

**M13: Enhanced review-architecture**
- Enhancement: Integrated C4 diagram validation (syntax + quality)
- Diagram weight: Increased from 10% to 20%
- Status: ✅ Complete
- Tests: 25 tests passing

**M14: Enhanced create-spec**
- Enhancement: Diagram generation option during spec creation
- Status: ✅ Complete

**M15: Enhanced review-spec**
- Enhancement: Diagram reference validation
- Status: ✅ Complete

### Must Have (Phases 7-8 - Advanced Features & Documentation - Complete)

**M16: Visual Regression Testing**
- Implementation: ImageMagick pixel-diff solution
- Thresholds: <1% auto-pass, 1-5% flag, >20% block
- Status: ✅ Complete (Phase 7)

**M17: Comprehensive Documentation**
- C4 Model Primer (610 lines)
- PlantUML Migration Guide (473 lines)
- Diagram-as-Code Best Practices (629 lines)
- Troubleshooting Guide (659 lines)
- Example Diagrams (7 diagrams: microservices, monolith, event-driven)
- Status: ✅ Complete (Phase 8)

### Should Have

**S1: Backfill Skills**
- Important: Documentation for existing codebases
- Effort: 8 hours per skill type

### Could Have

**C1: Validate-Spec Skill**
- Nice-to-have: Schema validation
- Effort: 4 hours

---

## 6. Scope Boundaries

### In Scope

**Functional Features:**
- LLM-as-judge validation (SPEC, ARCHITECTURE, ADR)
- Multi-Persona review (complementary validation)
- Quality gate integration (Wayfinder phase transitions)
- Cross-CLI compatibility (4 CLIs supported)
- Research-based rubrics
- **Diagram-as-code capabilities (NEW - Phases 2-8):**
  - C4 Model framework (Context, Container, Component levels)
  - Automated diagram generation from codebase analysis
  - Multi-format support (D2, Structurizr DSL, Mermaid, PlantUML)
  - Diagram syntax validation
  - Diagram-code sync detection and scoring
  - Visual regression testing
  - Template-based generation (microservices, monolith, event-driven)

**Non-Functional Requirements:**
- Performance: Validation <5 minutes, diagram generation <30s (small), <5min (large)
- Cost: <$0.50 per validation
- Reliability: ≥95% execution success
- Usability: ≥85% find feedback actionable
- **Diagram Quality:** ≥90% C4 compliance, ≥85% sync score average

**Target User Segments:**
- AI agents (Wayfinder/Swarm projects)
- Human developers (Engram workflows)
- Documentation teams

**Supported Platforms:**
- Claude Code (≥1.0.0)
- Gemini CLI (≥0.1.0)
- OpenCode (≥1.0.0)
- Codex (≥1.0.0)

### Out of Scope (Explicit Exclusions)

**Features Deferred:**
- Automated doc updates on code changes (too complex)
- Custom quality rubrics per project (configuration burden)
- Real-time documentation monitoring (separate concern)
- Integration with Confluence/Notion (external platforms)

---

## 7. Assumptions & Constraints

### Assumptions

**Assumption 1:** Users value high-quality documentation
- Impact: If false, will override gates frequently
- Validation: Track override rate

**Assumption 2:** LLM-as-judge provides reliable scoring
- Impact: If false, validation is meaningless
- Validation: Test consistency, compare to human ratings

**Assumption 3:** 8/10 threshold is reasonable
- Impact: If too high, frustrates users; too low, passes poor docs
- Validation: Track pass rate, adjust based on learnings

### Constraints

**Technical:**
- Must integrate with Wayfinder phase structure
- Must use Anthropic Claude models
- Must work within CLI skill framework

**Organizational:**
- Team size: 1 AI agent + 1 human
- Timeline: 5 weeks total (Phase 1 complete)

**Resource:**
- Cost budget: <$50 total for development
- Token budget: <500K tokens

---

## 8. Agent-Specific Specifications

### Agent Goals (Declarative)

**Goal:** Enable agents to validate documentation quality through LLM-as-judge.

**Constraints:**
- Quality threshold: 8/10 minimum
- Validation methods: LLM-as-judge + Multi-Persona
- Documentation location: `/docs/` directory

**Success Criteria:** See Goal 1 in Section 4

**Explicitly Unacceptable:**
- Gaming quality scores (keyword stuffing)
- Overriding gates without rationale
- Creating compliance documentation (not useful)
- Hallucinating documentation content

---

## 9. Living Document Process

### When to Update

- New validation methods discovered
- Design decisions impacting approach
- Assumption changes (threshold adjustments)
- Major milestone completions

### How to Update

1. Propose change with rationale
2. Update SPEC.md (increment version)
3. Document in LEARNINGS.md
4. Reference in ADR if architectural

### Related Documents

- **ARCHITECTURE.md:** Technical architecture
- **ROADMAP.md:** Implementation phases
- **RUBRICS:** Quality assessment criteria

---

## 10. Success Metrics (Diagram-as-Code)

### Goal 4: Enable Living Architecture Diagrams

**Description:** Teams maintain up-to-date architecture diagrams that evolve with code.

**North Star Metric:** "≥80% of projects have architecture diagrams with ≥85% sync score"

**Success Criteria:**

#### Primary Metrics
- **Diagram adoption rate:** ≥70% of Wayfinder projects have C4 diagrams by S6
- **Sync score:** ≥85% average (diagrams match code reality)
- **Diagram quality:** ≥8.0/10 average multi-persona review score
- **Stale diagram rate:** <10% of diagrams outdated (sync <60%)

#### Secondary Metrics (Quality)
- **C4 compliance:** ≥90% of diagrams pass C4 validation
- **Syntax validation:** 100% of diagrams compile without errors
- **Visual regression:** <5% false positive rate

#### Efficiency Metrics
- **Generation time:** <30s (small codebases), <5min (large codebases)
- **Rendering time:** <10s (simple diagrams), <60s (complex diagrams)
- **Review time:** <2min (multi-persona validation)

**How to Measure:**
- Data source: Wayfinder telemetry, diagram-sync reports, quality gate logs
- Collection method: Automatic logging of diagram generation, sync scores, quality scores
- Validation approach: Weekly analysis of diagram health across projects

---

## 11. Version History

| Version | Date | Changes | Rationale |
|---------|------|---------|-----------|
| 1.0.0 | 2026-03-11 | Initial SPEC for spec-review marketplace | Phase 1 completion documentation |
| 2.0.0 | 2026-03-13 | Add diagram-as-code capabilities (Phases 2-8) | Integrated C4 Model, D2, Structurizr, Mermaid support; 4 new skills (render-diagrams, create-diagrams, review-diagrams, diagram-sync); enhanced review-architecture with diagram validation; comprehensive documentation (2,371 lines) and examples (7 diagrams); visual regression testing |

---

## Appendix

### A. Quality Rubric

**SPEC.md Quality (0-10 scale):**

1. **Vision/Goals** (0-3 points): Clear vision, measurable goals, stakeholder alignment
2. **User Journeys** (0-2 points): Complete CUJs with tasks, intent, success criteria
3. **Success Metrics** (0-2 points): Measurable, anti-reward-hacking, baselines/targets
4. **Scope Boundaries** (0-2 points): Explicit in/out scope, future considerations
5. **Living Document** (0-1 point): Update process, ownership, version control

**Threshold:** ≥8/10 required for quality gate passage

### B. Technical References

**Core Infrastructure:**
- **CLI Detection:** `lib/cli-detector.py`
- **CLI Abstraction:** `lib/cli-abstraction.py`
- **Test Suite:** `tests/test_cli_abstraction.py`
- **Quality Rubrics:** `rubrics/spec-quality-rubric.yml`, `rubrics/architecture-quality-rubric.yml`, `rubrics/diagram-quality-rubric.yml`

**Diagram-as-Code Skills:**
- **render-diagrams:** `skills/render-diagrams/render_diagrams.py` (386 lines, 16 tests passing)
- **create-diagrams:** `skills/create-diagrams/create_diagrams.py` (260 lines, Python wrapper for Go binary)
- **review-diagrams:** `skills/review-diagrams/review_diagrams.py`
- **diagram-sync:** `skills/diagram-sync/diagram_sync.py`

**Enhanced Skills:**
- **review-architecture:** `skills/review-architecture/review_architecture.py` (655 lines, 25 tests passing, diagram validation integrated)
- **review-spec:** `skills/review-spec/review_spec.py` (19 tests passing)
- **review-adr:** `skills/review-adr/review_adr.py`

**Documentation:**
- **C4 Model Primer:** `docs/c4-model-primer.md` (610 lines)
- **Migration Guide:** `docs/migration-from-plantuml.md` (473 lines)
- **Best Practices:** `docs/diagram-as-code-guide.md` (629 lines)
- **Troubleshooting:** `docs/troubleshooting.md` (659 lines)

**Examples:**
- **Microservices:** `examples/microservices/` (3 diagrams: context, container, component)
- **Monolith:** `examples/monolith/` (2 diagrams: context, container)
- **Event-Driven:** `examples/event-driven/` (2 diagrams: context in Mermaid, container in D2)

**CI/CD Templates:**
- **GitHub Actions:** `templates/github-actions-render-diagrams.yml`
- **Pre-commit Hook:** `templates/pre-commit-diagram-validation.sh`
- **Visual Regression:** `scripts/visual-regression.sh`
