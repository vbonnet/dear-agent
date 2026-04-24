---
name: review-architecture
description: >-
  Reviews architecture documentation for quality and best practices (dual-layer: traditional + agentic).
  TRIGGER when: user asks to review architecture docs, validate ARCHITECTURE.md, check architecture quality, or says "review architecture".
  DO NOT TRIGGER when: reviewing ADRs (use review-adr) or reviewing specs (use review-spec).
metadata:
  version: 1.0.0
  author: engram
  activation_patterns:
    - "/review-architecture"
---

# review-architecture: ARCHITECTURE.md Quality Validation Skill

**Purpose**: Validate ARCHITECTURE.md files against dual-layer architecture framework (traditional + agentic) with multi-persona quality assessment.

**Invocation**: `/review-architecture <architecture-file-path>`

**Quality Threshold**: 8/10 minimum to pass

**Cost Target**: <$0.50 per validation (typical: $0.05-$0.08)

**Latency Target**: p95 <5 minutes (typical: 10-30s)

---

## Validation Rubric

### Traditional Architecture (45% weight)
- Component architecture: 15%
- C4 diagram presence and quality: 10%
- Deployment architecture: 10%
- Data flow documentation: 10%

### Agentic Architecture (25% weight)
- Agent patterns documented: 8%
- Coordination strategy: 9%
- State management approach: 8%

### ADR Integration (10% weight)
- ADR references exist: 5%
- Decision rationale linked: 5%

### Visual Diagrams (20% weight) - **ENHANCED**
- C4 diagrams present: 10%
- Diagram syntax validation: 5%
- Diagram references in doc: 5%

**Diagram-as-Code Formats Supported:**
- D2 (.d2) - Native syntax validation via `d2 compile --dry-run`
- Structurizr DSL (.dsl) - Validated via `structurizr-cli validate`
- Mermaid (.mmd, .mermaid) - Basic syntax check
- Traditional formats (.puml, .png, .svg) - Presence check only

---

## Multi-Persona Validation

### System Architect (always included)
- Overall architecture quality
- Component design
- Pattern usage

### DevOps Engineer (conditional)
- Deployment architecture
- Observability
- Scalability
- **Included if:** deployment/infrastructure sections present

### Developer (conditional)
- Code organization
- Module structure
- Implementability
- **Included if:** code architecture or agentic patterns present

---

## Quick Validation Gate

Auto-fail without LLM call if:
- Missing Traditional Architecture section
- Missing Agentic Architecture section
- No C4 diagrams found
- No ADR references

---

## Diagram Validation (NEW)

**Automatic diagram-as-code validation:**

Before LLM evaluation, the skill now validates all diagram files:

**Supported Formats:**
- **D2** (.d2): Syntax validated via `d2 compile --dry-run`
- **Structurizr DSL** (.dsl): Validated via `structurizr-cli validate`
- **Mermaid** (.mmd, .mermaid): Basic non-empty file check
- **Traditional** (.puml, .png, .svg): Presence check only

**Validation Output:**
```
Validating architecture diagrams...
Found 3 diagram(s)
✓ All diagrams have valid syntax
Diagram quality score: 10.0/10.0
```

**Syntax Errors Detected:**
```
Validating architecture diagrams...
Found 2 diagram(s)
⚠️  Syntax errors found:
  - context.d2: unexpected token at line 5
Diagram quality score: 5.0/10.0
```

**Integration with LLM Scoring:**
- Diagram validation results are passed to LLM as context
- Visual Diagrams dimension score heavily weights syntax validation
- Syntax errors significantly reduce overall score

---

## CLI Abstraction Support

This skill supports multiple AI coding assistant CLIs:

- **Claude Code**: Optimized with prompt caching
- **Gemini CLI**: Batch processing mode
- **OpenCode**: MCP integration
- **Codex**: Completion mode

### CLI-Specific Adapters

Each CLI has a dedicated adapter for optimal performance:

```bash
# Claude Code (with prompt caching)
python cli-adapters/claude-code.py ~/docs/ARCHITECTURE.md

# Gemini CLI (with batch mode)
python cli-adapters/gemini.py ~/docs/ARCHITECTURE.md

# OpenCode (with MCP)
python cli-adapters/opencode.py ~/docs/ARCHITECTURE.md

# Codex (with completion mode)
python cli-adapters/codex.py ~/docs/ARCHITECTURE.md
```

The adapters automatically detect the current CLI environment and apply appropriate optimizations.

---

## Usage

### Basic Usage

```bash
# Validate ARCHITECTURE.md
/review-architecture ~/docs/ARCHITECTURE.md

# Direct invocation
python review_architecture.py ~/docs/ARCHITECTURE.md

# With JSON output for CI/CD
python review_architecture.py ~/docs/ARCHITECTURE.md --output-json report.json
```

### CLI Adapter Usage

```bash
# Auto-detect CLI and optimize
python cli-adapters/claude-code.py ~/docs/ARCHITECTURE.md

# With custom API key
python review_architecture.py ~/docs/ARCHITECTURE.md --api-key sk-ant-...
```

### Environment Variables

```bash
# Anthropic API
export ANTHROPIC_API_KEY=sk-ant-...

# Or use Vertex AI
export CLAUDE_CODE_USE_VERTEX=1
export ANTHROPIC_VERTEX_PROJECT_ID=your-project
export CLOUD_ML_REGION=us-east5
```

**Exit Codes:**
- 0: PASS (score ≥8.0)
- 1: FAIL (score <6.0)
- 2: WARN (score 6.0-7.9)
- 3: ERROR (file not found, API error)

---

## Examples

### Example 1: Basic Validation

```bash
$ /review-architecture ARCHITECTURE.md

Running quick validation gate...
✓ Quick validation passed

Running LLM-based validation...
Detected CLI: claude-code
Selected personas: System Architect, DevOps Engineer, Developer

╭─ ARCHITECTURE.md Validation Report ────────────────────────╮
│ Overall Score: 8.5/10.0 PASS                               │
│                                                             │
│ Dimension Scores:                                          │
│   Traditional Architecture: 8.8/10.0 (50% weight)          │
│   Agentic Architecture: 8.2/10.0 (30% weight)              │
│   ADR Integration: 8.0/10.0 (10% weight)                   │
│   Visual Diagrams: 9.0/10.0 (10% weight)                   │
│                                                             │
│ Persona Feedback:                                          │
│                                                             │
│ System Architect (score: 8.7/10.0)                         │
│   Excellent component architecture with clear C4 diagrams. │
│   Good separation of concerns. Minor: add more detail on   │
│   failure handling patterns.                               │
│                                                             │
│ DevOps Engineer (score: 8.5/10.0)                          │
│   Deployment architecture is well documented. Good K8s     │
│   patterns. Suggestion: add more observability details.    │
│                                                             │
│ Developer (score: 8.3/10.0)                                │
│   Clear agent patterns and coordination strategy. Code     │
│   structure is implementable. Consider adding more state   │
│   management details.                                      │
│                                                             │
│ Self-Consistency:                                          │
│   Variance: 0.234 (threshold: <0.5)                        │
│                                                             │
│ CLI: claude-code                                           │
╰─────────────────────────────────────────────────────────────╯
```

### Example 2: Failed Validation

```bash
$ /review-architecture incomplete-arch.md

Running quick validation gate...
FAIL: Quick validation failed

Missing sections: Agentic Architecture
Missing C4 diagrams (checked docs/, diagrams/, architecture/)
Missing ADR references
```

### Example 3: JSON Output for CI/CD

```bash
$ python review_architecture.py ARCHITECTURE.md --output-json report.json
JSON output saved to report.json

$ cat report.json
{
  "overall_score": 8.5,
  "dimension_scores": {
    "traditional_architecture": 8.8,
    "agentic_architecture": 8.2,
    "adr_integration": 8.0,
    "visual_diagrams": 9.0
  },
  "self_consistency": {
    "scores": [8.4, 8.6, 8.5, 8.3, 8.7],
    "mean": 8.5,
    "variance": 0.234
  },
  "personas": [
    {
      "role": "System Architect",
      "score": 8.7,
      "feedback": "Excellent component architecture..."
    }
  ]
}
```

---

## Integration with spec-review-marketplace

This skill is part of the **spec-review-marketplace** plugin and integrates with:

- **Rubrics Directory**: Uses `~/plugins/spec-review-marketplace/rubrics/spec-quality-rubric.yml`
- **CLI Abstraction**: Leverages `lib/cli_abstraction.py` for cross-CLI support
- **CLI Detection**: Auto-detects current CLI environment via `lib/cli_detector.py`

### Plugin Structure

```
spec-review-marketplace/
├── lib/
│   ├── cli_abstraction.py    # CLI abstraction layer
│   └── cli_detector.py        # CLI detection
├── rubrics/
│   └── spec-quality-rubric.yml
└── skills/
    └── review-architecture/
        ├── review_architecture.py
        ├── cli-adapters/
        │   ├── claude-code.py
        │   ├── gemini.py
        │   ├── opencode.py
        │   └── codex.py
        ├── tests/
        │   └── test_review_architecture.py
        ├── skill.yml
        └── SKILL.md
```

---

## Performance Optimization

### Prompt Caching (Claude Code)

The Claude Code adapter enables prompt caching to reduce API costs:

- **Rubric Caching**: The evaluation rubric is cached across invocations
- **Cost Reduction**: ~90% reduction for repeated validations
- **Automatic**: Enabled by default in claude-code.py adapter

### Batch Processing (Gemini CLI)

The Gemini CLI adapter uses batch processing:

- **Batch Size**: 20 (optimized for Gemini)
- **Parallel Reviews**: Persona reviews can run in parallel
- **Throughput**: Higher throughput for multiple files

### MCP Integration (OpenCode/Codex)

OpenCode and Codex adapters leverage MCP for tool integration:

- **Tool Registry**: Access to file system and git tools
- **MCP Protocol**: Standard tool invocation format
- **Extensibility**: Easy to add custom tools

---

## Troubleshooting

### API Key Issues

```bash
Error: Anthropic API key or Vertex AI configuration required
```

**Solution**: Set `ANTHROPIC_API_KEY` or configure Vertex AI

### Missing Diagrams

```bash
FAIL: Quick validation failed
Missing C4 diagrams (checked docs/, diagrams/, architecture/)
```

**Solution**: Add C4 diagrams to one of the checked directories:
- `docs/architecture/`
- `docs/diagrams/`
- `diagrams/`
- `architecture/`

### Low Score

```bash
Overall Score: 5.2/10.0 FAIL
```

**Solution**: Review persona feedback for specific improvement areas

---

**Version**: 1.0
**Created**: 2026-03-11
**Migrated**: 2026-03-11
**Author**: Engram Plugin Development Team
