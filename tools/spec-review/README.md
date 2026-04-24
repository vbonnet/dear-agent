# Spec Review Marketplace

**Version:** 2.0.0 | **Status:** Production-Ready

Specification, architecture, and diagram-as-code review marketplace with cross-CLI support for Claude Code, Gemini CLI, OpenCode, and Codex.

## Overview

This marketplace provides LLM-as-judge based reviews and diagram-as-code capabilities for:

### Documentation Review
- **SPEC.md Files**: Research-backed quality assessment
- **Architecture Decision Records (ADRs)**: Completeness and clarity review
- **Architecture Documentation**: Best practices validation
- **Spec Generation**: AI-assisted SPEC.md creation from requirements

### Diagram-as-Code (NEW in v2.0.0)
- **Diagram Generation**: C4 architecture diagrams from codebase analysis
- **Diagram Review**: Multi-persona quality validation with cost tracking
- **Diagram Rendering**: Compile to PNG/SVG/PDF with syntax validation
- **Diagram Sync**: Detect drift between diagrams and codebase reality

## Features

- **Multi-CLI Support**: Works with Claude Code, Gemini CLI, OpenCode, and Codex
- **CLI Abstraction Layer**: Unified interface for cross-platform skill compatibility
- **LLM-as-Judge**: Hybrid self-consistency scoring with multiple perspectives
- **Quality Rubrics**: Research-backed assessment criteria
- **Fast & Cost-Effective**: ~5 seconds, ~$0.05 per validation

## Installation

```bash
# Install marketplace
engram plugin marketplace install spec-review-marketplace

# List available skills
engram plugin marketplace skills spec-review-marketplace

# Install specific skill
engram plugin marketplace install-skill review-spec
```

## Skills

### review-spec ✅
Validates SPEC.md files against research-backed quality rubric.

**Status:** Implemented (19/20 tests passing)

```bash
# Review SPEC.md in current directory
python review_spec.py

# Review specific file
python review_spec.py path/to/SPEC.md

# JSON output
python review_spec.py --output-json report.json
```

### review-adr ✅
Reviews Architecture Decision Records for quality and completeness with multi-persona assessment.

**Status:** Implemented (comprehensive tests)

```bash
# Review ADR
python review_adr.py docs/adr/001-database-choice.md

# Multiple ADRs
python review_adr.py docs/adr/*.md
```

### review-architecture ✅
Reviews architecture documentation for quality and best practices with multi-persona assessment.

**Status:** Implemented (15+ tests)

```bash
# Review architecture docs
python review_architecture.py docs/ARCHITECTURE.md

# With specific personas
python review_architecture.py --personas system-architect,devops ARCHITECTURE.md
```

### create-spec ✅
LLM-based SPEC.md generation from codebase analysis and requirements.

**Status:** Implemented (25+ tests, 100% pass rate)

```bash
# Generate SPEC.md from project
python create_spec.py /path/to/project

# Interactive mode
python create_spec.py --interactive /path/to/project

# Non-interactive with defaults
python create_spec.py --non-interactive /path/to/project
```

### create-diagrams ✅ (NEW in v2.0.0)
Generate C4 architecture diagrams from codebase analysis in D2, Mermaid, or Structurizr DSL formats.

**Status:** Production-ready (10-1200x faster than targets, security hardened)

```bash
# Generate C4 context diagram in D2 format
create-diagrams --codebase ./src --output diagrams/ --format d2 --level context

# Generate container diagram in Mermaid
create-diagrams --codebase . --output diagrams/ --format mermaid --level container

# Multiple formats
create-diagrams --codebase ./src --output diagrams/ --format d2,mermaid,dsl --level context
```

**Performance**: 0.25-0.31s (constant time up to 300 files)

### review-diagrams ✅ (NEW in v2.0.0)
Multi-persona diagram quality validation with C4 compliance checking and cost tracking.

**Status:** Production-ready (45 security tests passing, WCAG AA framework)

```bash
# Review diagram with multi-persona gate
review-diagrams --diagram diagrams/c4-context.d2

# Dry-run mode (no LLM API costs)
review-diagrams --diagram diagrams/c4-container.mmd --dry-run

# JSON output with detailed verdicts
review-diagrams --diagram diagrams/c4-component.dsl --json
```

**Performance**: 0.30-0.40s (mock mode), adds ~2-4s with real LLM calls
**Cost Tracking**: Logs to `~/.engram/diagram-costs.jsonl`

### render-diagrams ✅ (NEW in v2.0.0)
Compile diagram-as-code to visual formats (PNG, SVG, PDF) with syntax validation.

**Status:** Production-ready (accessibility validated, WCAG AA partial compliance)

```bash
# Render D2 diagram to SVG
render-diagrams --input diagrams/c4-context.d2 --output rendered/context.svg

# Render Mermaid to PNG
render-diagrams --input diagrams/c4-container.mmd --output rendered/container.png

# Validate syntax only (no rendering)
render-diagrams --input diagrams/*.d2 --validate-only
```

**Performance**: 0.50-0.88s (validation only), rendering time depends on external tools
**Supported formats**: D2, Mermaid, Structurizr DSL

### diagram-sync ✅ (NEW in v2.0.0)
Detect and report drift between architecture diagrams and actual codebase structure.

**Status:** Production-ready (CI/CD integration ready)

```bash
# Check diagram sync status
diagram-sync --diagram diagrams/c4-container.d2 --codebase ./src

# Generate patch for outdated diagram
diagram-sync --diagram diagrams/c4-component.mmd --codebase ./src --patch

# CI/CD mode (exit code 1 if drift detected)
diagram-sync --diagram diagrams/*.d2 --codebase . --strict
```

**Performance**: 0.27-0.29s (90 files)
**Use case**: Pre-commit hooks, GitHub Actions, CI/CD pipelines

## CLI Compatibility

| CLI | Status | Optimizations |
|-----|--------|---------------|
| Claude Code | ✅ Full | Prompt caching, multimodal |
| Gemini CLI | ✅ Full | Batch mode |
| OpenCode | ✅ Full | MCP integration |
| Codex | ✅ Full | MCP integration |

## Quality Rubrics

### SPEC Quality Rubric
- **Vision/Goals** (30%): Clarity, measurability, stakeholder alignment
- **Critical User Journeys** (25%): Completeness, detail, edge cases
- **Success Metrics** (25%): Measurability, baseline, targets
- **Scope** (10%): Boundaries, exclusions, assumptions
- **Living Document** (10%): Updateability, versioning, ownership

### ADR Quality Rubric
- **Context** (25%): Problem statement, background, constraints
- **Decision** (30%): Chosen solution, rationale, alternatives
- **Consequences** (25%): Impacts, trade-offs, risks
- **Metadata** (20%): Status, date, stakeholders

## Marketplace Discovery

The marketplace includes powerful discovery tools for finding and exploring skills:

### Discovery CLI

```bash
# List all skills
marketplace-discover list

# Search for skills
marketplace-discover search "validation"

# Filter by tags
marketplace-discover filter validation quality-assessment

# Filter with AND logic
marketplace-discover filter-and spec validation

# Get skill information
marketplace-discover info review-spec

# Check CLI compatibility
marketplace-discover compatible claude-code

# Get recommendations
marketplace-discover recommend documentation

# Generate compatibility matrix
marketplace-discover matrix
```

### Programmatic Usage (Python)

```python
from lib.discovery import MarketplaceDiscovery

# Create discovery instance
discovery = MarketplaceDiscovery()

# Search for skills
results = discovery.search_skills("validation")

# Check compatibility
if discovery.is_skill_compatible("review-spec", "claude-code"):
    print("Compatible!")

# List compatible skills
compatible = discovery.list_compatible_skills("claude-code")

# Get recommendations
recommended = discovery.recommend_skills("spec")
```

**Documentation:** See [DISCOVERY-EXAMPLES.md](DISCOVERY-EXAMPLES.md) for comprehensive examples.

**Compatibility Matrix:** See [COMPATIBILITY-MATRIX.md](COMPATIBILITY-MATRIX.md) for full CLI compatibility details.

## Architecture

### C4 Component Diagram

![Marketplace Component Diagram](diagrams/rendered/c4-component-marketplace.png)

**Component Architecture** showing the internal structure of the Spec Review Marketplace plugin system:

- **PluginLoader**: Dynamic skill loading and registration
- **SkillRegistry**: Centralized skill discovery and metadata management
- **ValidationEngine**: Multi-persona review orchestration with LLM-as-judge
- **CLI Abstraction Layer**: Cross-CLI compatibility (Claude Code, Gemini, OpenCode, Codex)
- **Rubric System**: Research-backed quality assessment criteria
- **Diagram Skills**: C4 diagram generation, review, rendering, and sync capabilities

**Diagram Source**: `diagrams/c4-component-marketplace.d2`

### Directory Structure

```
spec-review-marketplace/
├── bin/
│   └── marketplace-discover     # Discovery CLI tool
├── lib/
│   ├── cli-detector.py          # CLI runtime detection
│   ├── cli-abstraction.py       # Common interface layer
│   ├── discovery.py             # Skill discovery and search
│   └── llm-judge.py             # LLM-as-judge utilities
├── skills/
│   ├── review-spec/             # SPEC.md review skill
│   ├── review-adr/              # ADR review skill
│   ├── review-architecture/     # Architecture review skill
│   └── create-spec/             # SPEC.md generation skill
├── rubrics/
│   ├── spec-quality-rubric.yml  # SPEC quality criteria
│   └── adr-quality-rubric.yml   # ADR quality criteria
├── tests/
│   ├── test_cli_abstraction.py  # CLI abstraction tests
│   ├── test_discovery.py        # Discovery tests
│   └── run-tests.sh             # Test runner
└── cli-adapters/                # CLI-specific adapters
```

## Development

### Adding a New Skill

1. Create skill directory: `skills/my-skill/`
2. Add skill metadata: `skills/my-skill/skill.yml`
3. Implement skill logic: `skills/my-skill/my_skill.py`
4. Create CLI adapters: `skills/my-skill/cli-adapters/`
5. Update marketplace.json

### Adding a New Rubric

1. Create rubric file: `rubrics/my-rubric.yml`
2. Define dimensions and criteria
3. Add scoring guidelines
4. Update marketplace.json
5. Create tests

## Testing

```bash
# Run unit tests
python -m pytest tests/

# Test CLI abstraction
python -m pytest tests/test_cli_abstraction.py

# Test cross-CLI compatibility
./tests/test-multi-cli.sh
```

## License

MIT
