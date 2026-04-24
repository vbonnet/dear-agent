# Architecture: Spec-Review Marketplace

**Version:** 2.0.0
**Last Updated:** 2026-03-13
**Status:** Production (Phases 1-8 Complete)
**Architecture Style:** Plugin-based marketplace with CLI abstraction layer

---

## Executive Summary

The Spec-Review Marketplace is a cross-CLI plugin that provides LLM-as-judge documentation validation and diagram-as-code capabilities. It supports 4 CLIs (Claude Code, Gemini, OpenCode, Codex) through a unified abstraction layer, enabling teams to validate documentation quality (SPEC.md, ARCHITECTURE.md, ADRs) and maintain living architecture diagrams synchronized with code.

**Key Architectural Decisions:**
- **Dual-layer architecture:** Traditional documentation skills + diagram-as-code capabilities
- **CLI abstraction:** Python layer for cross-CLI compatibility
- **Multi-language implementation:** Python (CLI adapters), Go (diagram core), TypeScript (Mermaid service)
- **LLM-as-judge pattern:** Multi-persona review with self-consistency validation
- **C4 Model framework:** Standardized semantic notation for architecture diagrams

---

## Traditional Architecture

### System Context (C4 Level 1)

```d2
direction: down

# People
developer: {
  shape: person
  label: "Developer / AI Agent"
  description: "Uses skills to validate documentation and generate diagrams"
}

architect: {
  shape: person
  label: "System Architect"
  description: "Reviews architecture diagrams and documentation quality"
}

# Systems
marketplace: {
  label: "Spec-Review Marketplace"
  description: "Plugin providing documentation validation and diagram-as-code"
  style.fill: "#1168bd"
}

anthropic_api: {
  label: "Anthropic API"
  description: "Claude Sonnet 4.5 for LLM-as-judge validation"
  style.fill: "#999999"
  style.stroke-dash: 3
}

d2_binary: {
  label: "D2 Binary"
  description: "Native D2 diagram rendering (oss.terrastruct.com/d2)"
  style.fill: "#999999"
  style.stroke-dash: 3
}

mermaid_cli: {
  label: "Mermaid CLI"
  description: "Mermaid diagram rendering (@mermaid-js/mermaid-cli)"
  style.fill: "#999999"
  style.stroke-dash: 3
}

codebase: {
  label: "User Codebase"
  description: "Source code to analyze for diagram generation"
  style.fill: "#999999"
  style.stroke-dash: 3
}

# Relationships
developer -> marketplace: "Invokes skills (review-spec, create-diagrams, etc.)"
architect -> marketplace: "Reviews validation results"
marketplace -> anthropic_api: "LLM-as-judge calls (HTTPS/JSON)"
marketplace -> d2_binary: "Render D2 diagrams (CLI)"
marketplace -> mermaid_cli: "Render Mermaid diagrams (CLI)"
marketplace -> codebase: "Analyze code (static analysis)"
```

**External Systems:**
- **Anthropic API:** Claude Sonnet 4.5 for multi-persona document validation
- **D2 Binary:** Native Go library for D2 diagram rendering (syntax validation + PNG/SVG/PDF output)
- **Mermaid CLI:** Node.js-based Mermaid rendering
- **Structurizr CLI:** Java-based Structurizr DSL export (optional)
- **User Codebase:** Target codebase for diagram generation (Go, Python, TypeScript, Java)

---

### Container Architecture (C4 Level 2)

```d2
direction: down

# Python CLI Abstraction Layer
cli_abstraction: {
  label: "CLI Abstraction Layer"
  description: "Python module for cross-CLI compatibility"
  style.fill: "#438dd5"

  cli_detector: "CLI Detector (detects Claude Code, Gemini, etc.)"
  cli_wrapper: "CLI Wrapper (prompt caching, batch processing)"
}

# Documentation Review Skills (Python)
doc_skills: {
  label: "Documentation Review Skills"
  description: "LLM-as-judge validation (Python)"
  style.fill: "#438dd5"

  review_spec: "review-spec.py (SPEC.md validation)"
  review_architecture: "review-architecture.py (ARCHITECTURE.md + diagrams)"
  review_adr: "review-adr.py (ADR validation)"
  create_spec: "create-spec.py (spec generation)"
}

# Diagram-as-Code Skills (Multi-language)
diagram_skills: {
  label: "Diagram-as-Code Skills"
  description: "C4 diagram generation and validation"
  style.fill: "#438dd5"

  render_diagrams: "render-diagrams.py (Python, 386 lines)"
  create_diagrams: "create-diagrams.py (Python wrapper → Go binary)"
  review_diagrams: "review-diagrams.py (Python, multi-persona)"
  diagram_sync: "diagram-sync.py (Python, drift detection)"
}

# Go Core Services
go_services: {
  label: "Go Core Services"
  description: "High-performance diagram processing"
  style.fill: "#1168bd"

  d2_renderer: "D2 Renderer (native oss.terrastruct.com/d2)"
  codebase_analyzer: "Codebase Analyzer (AST parsing for Go, regex for Python/TS/Java)"
  c4_builder: "C4 Model Builder (map code → C4 elements)"
  sync_engine: "Sync Engine (diagram ↔ code comparison)"
}

# TypeScript Service
mermaid_service: {
  label: "Mermaid Service"
  description: "TypeScript service for Mermaid rendering"
  style.fill: "#1168bd"

  renderer: "Renderer (mermaid npm package)"
  cli_bridge: "CLI Bridge (Node.js subprocess)"
}

# Shared Libraries
shared_libs: {
  label: "Shared Libraries"
  description: "Common utilities and models"
  style.fill: "#438dd5"

  llm_judge: "llm-judge.py (multi-persona validation)"
  c4_model: "c4-model.py (C4 semantic framework)"
  rubrics: "Quality Rubrics (YAML)"
}

# Data Stores
rubrics_db: {
  shape: cylinder
  label: "Quality Rubrics"
  description: "YAML files (spec, architecture, diagram quality)"
  style.fill: "#438dd5"
}

examples_db: {
  shape: cylinder
  label: "Example Diagrams"
  description: "Reference diagrams (microservices, monolith, event-driven)"
  style.fill: "#438dd5"
}

docs_db: {
  shape: cylinder
  label: "Documentation"
  description: "C4 primer, migration guides, troubleshooting"
  style.fill: "#438dd5"
}

# External Dependencies
anthropic: {
  shape: cylinder
  label: "Anthropic API"
  style.fill: "#999999"
  style.stroke-dash: 3
}

diagram_tools: {
  shape: cylinder
  label: "Diagram Tools"
  description: "d2, mmdc, structurizr-cli"
  style.fill: "#999999"
  style.stroke-dash: 3
}

# Relationships - CLI Abstraction
cli_abstraction -> doc_skills: "Provides unified interface"
cli_abstraction -> diagram_skills: "Provides unified interface"

# Documentation Skills
doc_skills -> shared_libs: "Uses LLM-judge pattern"
doc_skills -> rubrics_db: "Loads quality rubrics"
doc_skills -> anthropic: "API calls (HTTPS/JSON)"

# Diagram Skills
diagram_skills -> go_services: "Calls Go binaries (subprocess)"
diagram_skills -> mermaid_service: "Calls Mermaid service (subprocess)"
diagram_skills -> diagram_tools: "CLI invocations"
diagram_skills -> examples_db: "Uses templates"
diagram_skills -> docs_db: "References documentation"

# Go Services
go_services -> diagram_tools: "Native D2 library"

# Shared Libraries
shared_libs -> rubrics_db: "Loads rubrics"
```

**Container Breakdown:**

1. **CLI Abstraction Layer (Python)**
   - Detects active CLI (Claude Code, Gemini, OpenCode, Codex)
   - Provides unified interface for prompt caching, batch processing
   - Handles CLI-specific quirks transparently

2. **Documentation Review Skills (Python)**
   - review-spec: SPEC.md quality validation
   - review-architecture: ARCHITECTURE.md + diagram validation (ENHANCED Phase 6)
   - review-adr: ADR quality validation
   - create-spec: Automated spec generation

3. **Diagram-as-Code Skills (Multi-language)**
   - render-diagrams.py: Compile diagram-as-code to PNG/SVG/PDF
   - create-diagrams.py: Auto-generate C4 diagrams from codebase (Python wrapper → Go)
   - review-diagrams.py: Multi-persona diagram quality validation
   - diagram-sync.py: Detect drift between diagrams and code

4. **Go Core Services**
   - D2 Renderer: Native D2 library integration (oss.terrastruct.com/d2)
   - Codebase Analyzer: AST parsing (Go), regex analysis (Python/TS/Java)
   - C4 Model Builder: Map code structure to C4 elements (System, Container, Component)
   - Sync Engine: Compare diagram elements vs actual codebase

5. **TypeScript Mermaid Service**
   - Mermaid rendering using official npm package (@mermaid-js/mermaid-cli)
   - Node.js subprocess bridge for Python skills

6. **Shared Libraries**
   - llm-judge.py: Multi-persona LLM validation with self-consistency
   - c4-model.py: C4 Model semantic framework
   - Quality rubrics: YAML-based scoring criteria

**Data Stores:**
- Quality Rubrics (YAML): Scoring criteria for SPEC, ARCHITECTURE, diagrams
- Example Diagrams: Reference diagrams for 3 patterns (microservices, monolith, event-driven)
- Documentation: C4 primer (610L), migration guide (473L), best practices (629L), troubleshooting (659L)

---

### Component Architecture (C4 Level 3)

#### Component Diagram: Marketplace Plugin Architecture

**Diagram:** [diagrams/c4-component-marketplace.d2](diagrams/c4-component-marketplace.d2) | [SVG](diagrams/rendered/c4-component-marketplace.svg) | [PNG](diagrams/rendered/c4-component-marketplace.png)

This component diagram shows the internal architecture of the Spec-Review-Marketplace plugin, including:
- **Skill Registry**: Discovery, loading, caching, and version management
- **CLI Abstraction Layer**: Cross-CLI compatibility (Claude Code, Gemini, OpenCode, Codex)
- **Documentation Skills**: LLM-as-judge validation (review-spec, review-architecture, review-adr, create-spec)
- **Diagram-as-Code Skills**: C4 diagram generation and validation (create-diagrams, review-diagrams, render-diagrams, diagram-sync)
- **Shared Services**: LLM Judge Engine, Dependency Resolver, Parallel Executor, Security Utils
- **Go Core Services**: D2 Renderer, C4 Validator, Codebase Analyzer, Sync Engine
- **CLI Adapters**: Skill-specific implementations for each supported CLI

**Component Interactions:**
1. Skill Registry discovers and loads skills on demand
2. CLI Abstraction detects environment and provides unified interface
3. Documentation Skills use LLM Judge for multi-persona validation
4. Diagram Skills leverage Go Core Services for high-performance processing
5. CLI Adapters delegate to CLI Abstraction for cross-platform compatibility

---

#### Component Diagram: review-architecture Skill

```d2
direction: down

# Entry Point
main: {
  label: "main() Entry Point"
  description: "CLI argument parsing, orchestration"
}

# Quick Validation Gate
quick_validate: {
  label: "quick_validate()"
  description: "Fail-fast on missing sections, diagrams, ADRs"
}

# Diagram Validation
diagram_validator: {
  label: "validate_diagrams()"
  description: "Syntax validation (D2, Structurizr, Mermaid)"
}

# Persona Selection
persona_selector: {
  label: "select_personas()"
  description: "Choose personas based on content analysis"
}

# LLM Validation
llm_validator: {
  label: "call_claude()"
  description: "Multi-persona LLM-as-judge validation"
}

# Response Parser
response_parser: {
  label: "parse_json_response()"
  description: "Extract scores, feedback, decision from LLM response"
}

# Output Formatter
output_formatter: {
  label: "output_terminal()"
  description: "Rich terminal output with colors, panels"
}

# External Dependencies
rubric_loader: {
  label: "load_rubric()"
  description: "Load architecture quality rubric from YAML"
}

cli_abstraction: {
  label: "CLIAbstraction"
  description: "Cross-CLI compatibility layer"
  style.fill: "#999999"
}

anthropic_api: {
  shape: cylinder
  label: "Anthropic API"
  style.fill: "#999999"
}

diagram_files: {
  shape: cylinder
  label: "Diagram Files"
  description: ".d2, .dsl, .mmd files"
  style.fill: "#999999"
}

# Flow
main -> quick_validate: "1. Quick gate"
main -> diagram_validator: "2. Validate diagrams"
diagram_validator -> diagram_files: "Read & validate syntax"
main -> persona_selector: "3. Select personas"
main -> rubric_loader: "4. Load rubric"
main -> llm_validator: "5. LLM validation"
llm_validator -> cli_abstraction: "Uses caching"
llm_validator -> anthropic_api: "API call"
main -> response_parser: "6. Parse response"
main -> output_formatter: "7. Display results"
```

**Key Components:**

1. **quick_validate()**: Fail-fast gate checking for required sections, diagram files, ADR references
2. **validate_diagrams()**: Validates diagram syntax using native tools (d2 fmt, mmdc, structurizr-cli validate)
3. **select_personas()**: Content-based persona selection (System Architect always, DevOps/Developer conditional)
4. **call_claude()**: Multi-persona LLM-as-judge validation with retry logic
5. **parse_json_response()**: Robust JSON parsing with markdown code block handling
6. **output_terminal()**: Rich terminal output using rich library

**Validation Flow:**
1. Quick validation (fail-fast if incomplete)
2. Diagram syntax validation (D2/Structurizr/Mermaid)
3. Persona selection (content analysis)
4. Rubric loading (YAML)
5. LLM validation (multi-persona with self-consistency)
6. Response parsing (JSON extraction)
7. Terminal output (formatted results)

---

## Agentic Architecture

### Agent Interaction Patterns

**Pattern 1: Wayfinder Integration (Primary)**

```
Wayfinder Phase → Skill Invocation → Validation → Quality Gate

D4 (Requirements):
  User → /create-spec
       → Generates SPEC.md with optional skeleton C4 Context diagram
       → /review-spec validates quality (≥8/10 required)
       → Quality gate: PASS/FAIL/WARN

S6 (Design):
  User → /create-diagrams (auto-generate from code)
       → Generates C4 Context, Container, Component diagrams
       → /review-diagrams validates quality (≥8/10 required)
       → /review-architecture validates ARCHITECTURE.md + diagrams (20% weight)
       → Quality gate: PASS/FAIL/WARN

S8 (Implementation):
  CI/CD → /diagram-sync (detect drift)
       → Calculates sync score (diagram vs code)
       → Quality gate: ≥80% sync score required
       → Blocks commit if <60% sync

S11 (Retrospective):
  User → /review-diagrams (final validation)
       → Multi-persona quality review
       → Sync status validation
       → Quality gate: ≥8/10 required
```

**Pattern 2: Autonomous Agent Workflow**

```
AI Agent (Claude Executing Wayfinder):
  1. Reads SPEC.md/ARCHITECTURE.md (source of truth)
  2. References diagrams for system understanding
  3. Validates work against specifications
  4. Updates diagrams when architecture changes
  5. Runs /diagram-sync before marking tasks complete
  6. Never proceeds with <80% sync score
```

**Pattern 3: Multi-Persona Review**

```
LLM-as-Judge Pattern:
  Input: Document content + Quality rubric
  ↓
  Generate 5 independent evaluations (self-consistency check)
  ↓
  Persona-specific feedback:
    - System Architect (40% weight): Architecture correctness, C4 compliance
    - Technical Writer (30% weight): Clarity, documentation quality
    - Developer (20% weight): Implementability, technical accuracy
    - DevOps (10% weight): Deployment feasibility, observability
  ↓
  Variance analysis (threshold: <0.5 for reliable scoring)
  ↓
  Decision: PASS (≥8/10), WARN (6-7.9/10), FAIL (<6/10)
```

### Agent Coordination Strategies

**1. Sequential Validation Chain:**
```
create-spec → review-spec → create-diagrams → review-diagrams → review-architecture
```
Each step validates previous step's output before proceeding.

**2. Parallel Diagram Generation:**
```
create-diagrams (codebase analysis)
  ↓
  ├─→ D2 generation (concurrent)
  ├─→ Structurizr DSL generation (concurrent)
  └─→ Mermaid generation (concurrent)
  ↓
render-diagrams (all formats in parallel)
```

**3. Feedback Loop:**
```
Diagram generation → Review (multi-persona) → Low score (<8/10)?
  ↓ Yes
Feedback analysis → Improvement suggestions → Regenerate → Review again
  ↓ No
PASS (proceed to next phase)
```

### State Management

**Diagram Lifecycle States:**
1. **Generated:** Freshly created from codebase (sync score = 100%)
2. **Validated:** Passed quality review (score ≥8/10)
3. **Synced:** Code changes, but diagram updated (sync score ≥80%)
4. **Drifted:** Code changed without diagram update (sync score <80%)
5. **Stale:** Significantly outdated (sync score <60%, age >30 days)

**Transitions:**
- Generated → Validated: Pass review-diagrams
- Validated → Synced: diagram-sync detects changes, updates made
- Synced → Drifted: Code changes without diagram update
- Drifted → Stale: Time passes without update
- Stale → Generated: Regenerate from current code

---

## Deployment Architecture

### Development Environment

```
Developer Machine:
├── Python 3.10+ (CLI adapters, skills)
├── Go 1.21+ (diagram core services)
├── Node.js 18+ (Mermaid service)
├── D2 binary (diagram rendering)
├── Mermaid CLI (npm global install)
└── Structurizr CLI (optional, Java-based)
```

### CI/CD Integration

```
GitHub Actions Workflow:
  1. Checkout code
  2. Install dependencies (d2, mmdc, structurizr-cli)
  3. Run /diagram-sync (detect drift)
  4. Fail build if sync <80%
  5. Render diagrams (PNG/SVG for docs)
  6. Visual regression test (ImageMagick pixel-diff)
  7. Upload diagram artifacts
  8. Comment PR with sync status
```

**Pre-commit Hook:**
```bash
#!/bin/bash
# Validate diagram syntax before commit
for diagram in diagrams/**/*.d2; do
  d2 fmt "$diagram" || exit 1
done

for diagram in diagrams/**/*.mmd; do
  mmdc -i "$diagram" -o /dev/null || exit 1
done
```

### Runtime Environment

**Execution Model:** Subprocess invocation

```
Claude Code (main process)
  ↓
  Invokes: python3 render_diagrams.py diagram.d2 output.svg
  ↓
  Python process spawns: d2 diagram.d2 output.svg
  ↓
  D2 binary executes, outputs SVG
  ↓
  Python process returns: exit code 0 (success)
  ↓
  Claude Code displays: "✓ Rendered successfully: output.svg"
```

**Performance Characteristics:**
- Diagram generation: <30s (small), <5min (large codebases)
- Rendering: <10s (simple), <60s (complex diagrams)
- LLM validation: <2min (multi-persona review)
- Sync detection: <30s (medium codebase)

---

## Technology Stack

### Languages & Runtimes

| Component | Language | Rationale |
|-----------|----------|-----------|
| CLI Abstraction | Python 3.10+ | Existing spec-review-marketplace uses Python; cross-CLI compatibility layer |
| Documentation Skills | Python 3.10+ | LLM-as-judge pattern, rich library for terminal output, pydantic for type safety |
| Diagram Core (create-diagrams) | Go 1.21+ | Native D2 library (oss.terrastruct.com/d2), AST parsing, performance |
| Mermaid Service | TypeScript/Node.js 18+ | Official @mermaid-js/mermaid-cli npm package |
| Visual Regression | Bash + ImageMagick | Simple pixel-diff solution, free/open-source |

### Key Dependencies

**Python:**
- `anthropic` - Claude API client
- `pydantic` - Type-safe data models
- `rich` - Terminal output formatting
- `pytest` - Testing framework

**Go:**
- `oss.terrastruct.com/d2` - Native D2 library
- Standard library for AST parsing, file I/O

**TypeScript/Node.js:**
- `@mermaid-js/mermaid-cli` - Official Mermaid renderer
- `typescript` - Type safety

**External Tools:**
- `d2` - D2 diagram rendering
- `mmdc` - Mermaid CLI
- `structurizr-cli` - Structurizr DSL export (optional)
- `ImageMagick` - Visual regression testing

### Build & Deployment

**Python Skills:**
```bash
# No build step required (interpreted)
# Install dependencies:
pip install -r requirements.txt

# Run tests:
pytest skills/*/tests/
```

**Go Services:**
```bash
# Build create-diagrams binary:
cd skills/create-diagrams/cmd/create-diagrams
go build -o create-diagrams

# Install globally:
go install
```

**TypeScript Service:**
```bash
# Build Mermaid service:
cd skills/render-diagrams/mermaid-service
npm install
npm run build
```

---

## Data Flow

### Diagram Generation Flow

```
1. User invokes: create-diagrams ~/project diagrams/
   ↓
2. Python wrapper (create_diagrams.py):
   - Validates codebase path
   - Creates output directory
   - Finds Go binary
   ↓
3. Go binary (codebase analyzer):
   - Discovers source files (*.go, *.py, *.ts, *.java)
   - Parses imports (AST for Go, regex for others)
   - Builds dependency graph (nodes = packages, edges = imports)
   ↓
4. C4 Model Builder:
   - Maps packages → C4 Containers
   - Maps modules → C4 Components
   - Infers external systems from import paths
   ↓
5. Diagram Generator:
   - Generates D2 syntax (Context, Container, Component)
   - Optionally generates Structurizr DSL
   - Optionally generates Mermaid C4
   ↓
6. Output:
   - diagrams/c4-context.d2
   - diagrams/c4-container.d2
   - diagrams/c4-component-{service}.d2
```

### Diagram Validation Flow

```
1. User invokes: review-diagrams diagrams/c4-context.d2
   ↓
2. Syntax Validation:
   - Runs: d2 fmt diagrams/c4-context.d2
   - Checks exit code (0 = valid, non-zero = syntax error)
   ↓
3. C4 Compliance Validation:
   - Parse diagram → extract elements and relationships
   - Check C4 level rules:
     * Context: Only Person, Software System, External System
     * Container: Only Container, Database, Message Queue
     * Component: Only Component
   - Verify no level mixing
   ↓
4. Multi-Persona LLM Review:
   - System Architect: C4 correctness, architecture quality
   - Technical Writer: Visual clarity, documentation
   - Developer: Technical accuracy, implementability
   - DevOps: Deployment feasibility (if infrastructure present)
   ↓
5. Self-Consistency Check:
   - Generate 5 independent scores
   - Calculate variance (threshold: <0.5)
   - Flag if inconsistent (high variance = unreliable scoring)
   ↓
6. Decision:
   - Score ≥8.0: PASS
   - Score 6.0-7.9: WARN
   - Score <6.0: FAIL
```

### Diagram-Code Sync Flow

```
1. User invokes: diagram-sync diagrams/ ~/project
   ↓
2. Diagram Parser:
   - Parse D2/Structurizr/Mermaid diagrams
   - Extract component names, relationships
   - Build diagram component graph
   ↓
3. Codebase Analyzer:
   - Analyze actual codebase structure
   - Extract package/module names, dependencies
   - Build code component graph
   ↓
4. Diff Engine:
   - Compare diagram graph vs code graph
   - Identify: missing components, extra components, changed relationships
   - Calculate sync score: (matched / total) × 100
   ↓
5. Patch Generator (optional):
   - LLM-assisted suggestions for diagram updates
   - Generate D2/Structurizr/Mermaid patches
   ↓
6. Output:
   - Sync score (0-100%)
   - Missing components list
   - Extra components list
   - Suggested patches
```

---

## Security Considerations

### Input Validation

**Diagram Injection Prevention:**
- Sanitize user input before generating diagrams
- Escape special characters in D2/Structurizr/Mermaid syntax
- Validate file paths (prevent path traversal: `../../etc/passwd`)

**Command Injection Prevention:**
- Use subprocess with argument lists (not shell=True)
- Validate CLI arguments before passing to external tools
- Whitelist allowed diagram formats, output formats

### API Security

**Anthropic API:**
- API keys stored in environment variables (never in code)
- Vertex AI support for GCP environments
- Retry logic with exponential backoff
- Timeout limits (60s default)

### DoS Prevention

**Diagram Complexity Limits:**
- Max nodes: 500 per diagram
- Max edges: 1000 per diagram
- Max nesting depth: 10 levels
- Rendering timeout: 5 minutes

---

## Testing Strategy

### Unit Tests

**Coverage by Skill:**
- review-architecture: 25 tests (quick validation, diagram validation, persona selection, rubric loading, prompt building, JSON parsing, CLI integration)
- review-spec: 19 tests (rubric loading, prompt construction, response parsing, CLI detection, data models)
- render-diagrams: 16 tests (format detection, validation, CLI interface, integration)

**Test Types:**
- Format detection tests
- Validation tests (syntax, C4 compliance)
- Persona selection tests
- LLM response parsing tests
- CLI abstraction tests
- Integration tests (requires tools installed)

### Integration Tests

**End-to-End Workflows:**
```bash
# Test complete workflow
create-diagrams ~/project diagrams/
review-diagrams diagrams/c4-context.d2
render-diagrams diagrams/c4-context.d2 output.svg
diagram-sync diagrams/ ~/project
```

**Cross-CLI Compatibility:**
- Test same skill on all 4 CLIs (Claude Code, Gemini, OpenCode, Codex)
- Verify output consistency (<5% variance)
- Check CLI-specific features (prompt caching, batch processing)

### Visual Regression Tests

**ImageMagick Pixel-Diff:**
```bash
# Compare baseline vs current render
compare -metric RMSE baseline.png current.png diff.png

# Thresholds:
# <1%: Auto-pass (anti-aliasing tolerance)
# 1-5%: Flag for manual review
# >20%: Block (likely error)
```

---

## Performance Optimization

### Diagram Generation

**Caching Strategy:**
- Cache codebase analysis results (invalidate on file changes)
- Incremental analysis (only analyze changed files)
- Parallel file processing (Go concurrency)

**Performance Targets:**
- Small codebase (<100 files): <30s
- Medium codebase (100-1000 files): <2min
- Large codebase (>1000 files): <5min

### LLM Validation

**Token Optimization:**
- Prompt caching (Claude Code supports this)
- Batch processing (Gemini supports this)
- Streaming responses (reduce latency)
- Token budgets: <20K tokens per validation

**Cost Optimization:**
- Cache rubric prompts (reuse across validations)
- Use cheaper models for syntax validation
- Use expensive models only for multi-persona review

---

## Monitoring & Observability

### Metrics

**Skill Execution:**
- Execution time (p50, p95, p99)
- Success rate (%)
- Error rate by type
- Token usage (for LLM calls)

**Diagram Quality:**
- Diagram generation success rate
- C4 compliance rate (% passing validation)
- Sync score distribution (histogram)
- Stale diagram rate (% with sync <60%)

### Logging

**Structured Logging:**
```json
{
  "skill": "review-architecture",
  "file": "/path/to/ARCHITECTURE.md",
  "duration_ms": 45231,
  "score": 8.5,
  "decision": "PASS",
  "personas": ["System Architect", "DevOps Engineer"],
  "variance": 0.234,
  "diagram_count": 3,
  "diagram_syntax_valid": true
}
```

---

## Architectural Decision Records (ADRs)

### ADR-001: Multi-Language Implementation Strategy

**Status:** Accepted
**Date:** 2026-03-12

**Context:**
Diagram-as-code tools have native implementations in different languages:
- D2: Go native library
- Mermaid: JavaScript/TypeScript npm package
- Structurizr: Java SDK (archived Python SDK)
- Existing spec-review-marketplace: Python

**Decision:**
Use multi-language implementation:
- Python: CLI adapters (compatibility with existing skills)
- Go: Diagram core services (native D2 library, performance)
- TypeScript: Mermaid service (official npm package)

**Consequences:**
- ✅ Best-in-class tool integration (native libraries)
- ✅ Performance (Go for compute-intensive tasks)
- ✅ Type safety (Go, TypeScript)
- ❌ Increased build complexity (3 languages to compile)
- ❌ Larger dependency footprint

---

### ADR-002: C4 Model as Semantic Framework

**Status:** Accepted
**Date:** 2026-03-12

**Context:**
Need standardized notation for architecture diagrams to ensure consistency across teams.

**Decision:**
Adopt C4 Model (Context, Container, Component, Code) as semantic framework for all generated diagrams.

**Consequences:**
- ✅ Standardized notation (System, Container, Component)
- ✅ Progressive disclosure (4 levels of detail)
- ✅ Industry standard (widely recognized)
- ✅ Validation possible (enforce level-specific rules)
- ❌ Learning curve for teams unfamiliar with C4
- ❌ Opinionated (not all architectures fit C4 perfectly)

---

### ADR-003: Diagram-Code Sync Detection

**Status:** Accepted
**Date:** 2026-03-12

**Context:**
Architecture diagrams become stale as code evolves. Need automated drift detection.

**Decision:**
Implement diagram-sync skill with sync scoring (0-100%):
- Parse diagrams → extract components
- Analyze code → extract actual structure
- Compare → calculate sync score
- Quality gate: ≥80% required

**Consequences:**
- ✅ Prevents stale diagrams (CI/CD enforcement)
- ✅ Measurable drift (0-100% score)
- ✅ Actionable feedback (missing/extra components)
- ❌ False positives (name mismatches)
- ❌ Maintenance burden (keep parser updated for new diagram formats)

---

### ADR-004: ImageMagick for Visual Regression Testing

**Status:** Accepted
**Date:** 2026-03-13

**Context:**
Need visual regression testing to detect unintended diagram changes. Options:
- Percy ($449/month)
- Chromatic ($149/month)
- ImageMagick (free, open-source)

**Decision:**
Use ImageMagick pixel-diff solution with thresholds (<1% pass, 1-5% flag, >20% block).

**Consequences:**
- ✅ Free and open-source (no vendor lock-in)
- ✅ Simple pixel-diff approach (easy to understand)
- ✅ CI/CD friendly (command-line tool)
- ❌ No web UI (Percy/Chromatic have better UX)
- ❌ Manual threshold tuning (requires experimentation)

---

## Related Documents

- **[SPEC.md](./SPEC.md)** - Product specification (v2.0.0, diagram-as-code integrated)
- **[ROADMAP.md](../../swarm/diagram-as-code-spec-enhancement/ROADMAP.md)** - Implementation roadmap (10 phases)
- **[C4 Model Primer](./docs/c4-model-primer.md)** - Understanding C4 levels and best practices
- **[Migration Guide](./docs/migration-from-plantuml.md)** - Migrating from PlantUML to D2/Mermaid/Structurizr
- **[Best Practices](./docs/diagram-as-code-guide.md)** - Diagram-as-code conventions and workflows
- **[Troubleshooting](./docs/troubleshooting.md)** - Common issues and solutions

---

**Document Version:** 2.0.0
**Last Updated:** 2026-03-13
**Maintained by:** Engram Team
