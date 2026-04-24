---
name: create-spec
description: >-
  Generate SPEC.md files from codebase analysis and requirements.
  TRIGGER when: user asks to create a spec, generate a product specification, write a SPEC.md, or says "create spec".
  DO NOT TRIGGER when: reviewing an existing SPEC.md (use review-spec) or creating architecture diagrams (use create-diagrams).
allowed-tools:
  - "Bash"
  - "Read"
  - "Grep"
  - "Glob"
  - "Write"
  - "AskUserQuestion"
metadata:
  version: 1.0.0
  author: engram
  disable-model-invocation: true
  activation_patterns:
    - "/create-spec"
    - "create spec"
    - "generate spec"
    - "write SPEC.md"
---

# create-spec Skill

LLM-powered SPEC.md generation from project requirements and codebase analysis. Automates creation of high-quality product specifications by analyzing your codebase, asking clarifying questions, and rendering a comprehensive SPEC.md.

## Key Features

- **Codebase Analysis**: Automatically analyzes project structure, technologies, and patterns
- **Interactive Question Generation**: Asks targeted questions to gather requirements
- **Template-Based Rendering**: Generates comprehensive SPEC.md from proven template
- **Skeleton Diagram Generation**: Optionally creates C4 Context diagram in D2 format
- **Quality Validation**: Validates generated SPEC against quality rubric
- **Cross-CLI Support**: Works on all 4 supported CLIs with optimizations

## Workflow

1. **Analyze** - CodebaseAnalyzer scans project files, detects technologies, extracts key files
2. **Question** - QuestionGenerator creates contextual questions; collects answers (interactive or defaults)
3. **Render** - SPECRenderer loads template, prepares context, outputs SPEC.md
4. **Validate** - SpecValidator checks structure, completeness, and quality (threshold: 8.0/10.0)

## Usage

```bash
# Basic interactive
python create_spec.py /path/to/project

# Non-interactive with defaults
python create_spec.py /path/to/project --no-interactive

# Custom output path
python create_spec.py /path/to/project -o /custom/path/SPEC.md

# Skip validation
python create_spec.py /path/to/project --no-validate

# Custom template
python create_spec.py /path/to/project --template my-template.md

# With skeleton C4 Context diagram
python create_spec.py /path/to/project --generate-diagrams
```

### Diagram Generation

When using `--generate-diagrams` (or `-d`):
- Creates C4 Context diagram in D2 format at `<project>/diagrams/c4-context.d2`
- Includes user actor, system boundary, placeholder for external systems
- Render with: `d2 diagrams/c4-context.d2 diagrams/c4-context.svg`

### CLI Adapters

Each CLI has an optimized adapter:

| CLI | Adapter | Key Optimization |
|-----|---------|-----------------|
| Claude Code | `cli-adapters/claude-code.py` | 200K context, prompt caching |
| Gemini | `cli-adapters/gemini.py` | Batch mode, parallel processing |
| OpenCode | `cli-adapters/opencode.py` | MCP integration |
| Codex | `cli-adapters/codex.py` | Completion mode, MCP support |

## Components

| Component | Purpose |
|-----------|---------|
| `CodebaseAnalyzer` | Scans project, detects languages/technologies, extracts metadata |
| `QuestionGenerator` | Generates contextual questions across 6 categories |
| `SPECRenderer` | Mustache-style template rendering with metadata injection |
| `SpecValidator` | Quality scoring (structure 40%, completeness 30%, quality 30%) |

## Validation Scoring

- **Structure** (40%): Required sections present
- **Completeness** (30%): Sections have content
- **Quality** (30%): Specific metrics, examples
- **Pass threshold**: 8.0/10.0

## Wayfinder Integration

Integrates with Wayfinder phase D4 (Requirements Documentation). Generated SPEC.md feeds into S4 (Stakeholder Alignment).

## Related Skills

- **review-spec**: Validate SPEC.md quality (LLM-as-judge)
- **review-architecture**: Validate ARCHITECTURE.md
- **review-adr**: Validate ADR documents

---

## Reference Documentation

Detailed documentation is available in the `references/` subdirectory:

- [Architecture & Workflow](references/architecture.md) - Directory structure, pipeline details
- [Examples](references/examples.md) - Full interactive session transcripts
- [CLI Adapters](references/cli-adapters.md) - Per-CLI optimization details
- [Components](references/components.md) - API details, data structures, template variables
- [Configuration](references/configuration.md) - Template customization, quality thresholds
- [Testing](references/testing.md) - Test suite, coverage, results
- [Troubleshooting](references/troubleshooting.md) - Common issues and solutions
- [Integration & Metadata](references/integration.md) - Wayfinder integration, changelog, contributing
