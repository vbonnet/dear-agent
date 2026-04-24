# Spec-Review-Marketplace Architecture Diagrams

This directory contains C4 Component diagrams showing the internal architecture of the Spec-Review-Marketplace module.

## Diagrams

### C4 Component: Spec-Review-Marketplace Internal Architecture

**File:** `c4-component-spec-review.d2`
**Level:** Component (C4 Level 3)
**Purpose:** Shows internal module architecture with multi-language components and data flow

**Coverage:**
- 6 major containers (Skill Registry, CLI Abstraction, Doc Skills, Diagram Skills, Shared Services, Go Core Services)
- 35+ internal components
- CLI Adapters for 4 CLIs (Claude Code, Gemini, OpenCode, Codex)
- 4 data stores (marketplace.json, rubrics, examples, templates)
- 3 external systems (Anthropic API, diagram tools, user codebase)

**Key Features:**
- Multi-language architecture (Python, Go, TypeScript)
- Cross-CLI compatibility layer
- LLM-as-judge validation pipeline
- Diagram-as-code generation workflow
- Sync detection and drift analysis

## Rendering

To render diagrams to visual formats:

```bash
# Render to SVG
d2 c4-component-spec-review.d2 rendered/c4-component-spec-review.svg

# Render to PNG
d2 c4-component-spec-review.d2 rendered/c4-component-spec-review.png

# Render to PDF
d2 c4-component-spec-review.d2 rendered/c4-component-spec-review.pdf
```

## References

- **ARCHITECTURE.md**: See section "Component Architecture (C4 Level 3)" for detailed component descriptions
- **SPEC.md**: Product specification with user journeys and success metrics
- **marketplace.json**: Complete skill registry with 8 skills
