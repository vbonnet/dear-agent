# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for the Gemini Deep Research tool.

## What are ADRs?

Architecture Decision Records (ADRs) are documents that capture important architectural decisions made along with their context and consequences. They help teams understand:

- **Why** decisions were made
- **What** alternatives were considered
- **What** the consequences are (positive, negative, and neutral)
- **When** the decision was made

## ADR Index

### Core Architecture

| ADR | Title | Status | Date | Tags |
|-----|-------|--------|------|------|
| [ADR-001](ADR-001-go-rewrite.md) | Go Rewrite from Bash | Accepted | 2024-12-15 | architecture, language, tooling |
| [ADR-002](ADR-002-extractor-factory-pattern.md) | Extractor Factory Pattern | Accepted | 2024-12-20 | architecture, design-pattern, extensibility |
| [ADR-006](ADR-006-error-handling-strategy.md) | Error Handling Strategy and Exit Codes | Accepted | 2025-01-05 | error-handling, user-experience, debugging, cli |

### Performance & Optimization

| ADR | Title | Status | Date | Tags |
|-----|-------|--------|------|------|
| [ADR-003](ADR-003-caching-strategy.md) | Caching Strategy | Accepted | 2025-01-10 | performance, api-efficiency, storage |

### Features

| ADR | Title | Status | Date | Tags |
|-----|-------|--------|------|------|
| [ADR-004](ADR-004-competitive-analysis-mode.md) | Competitive Analysis Mode | Accepted | 2025-01-15 | feature, templates, discovery, competitive-intelligence |
| [ADR-005](ADR-005-template-based-prompts.md) | Template-Based Prompts and Variable Substitution | Accepted | 2025-01-20 | customization, prompts, templates, variables |

## ADR Summary

### By Status

- **Accepted**: 6 ADRs (all current decisions are active)
- **Proposed**: 0 ADRs
- **Deprecated**: 0 ADRs
- **Superseded**: 0 ADRs

### By Category

- **Architecture**: 3 ADRs (Go rewrite, factory pattern, error handling)
- **Performance**: 1 ADR (caching)
- **Features**: 2 ADRs (competitive mode, templates)

### Key Decisions

1. **Language**: Chose Go over Bash/Python/Rust for cross-platform support, type safety, and performance
2. **Extensibility**: Factory pattern enables easy addition of new content extractors
3. **Performance**: File-based caching achieves 60%+ cache hit rate
4. **Features**: Competitive analysis mode automates competitor research workflows
5. **Customization**: Template-based prompts with variable substitution
6. **User Experience**: Structured error handling with specific exit codes

## Decision Timeline

```
2024-12-15  ADR-001  Go Rewrite from Bash
2024-12-20  ADR-002  Extractor Factory Pattern
2025-01-05  ADR-006  Error Handling Strategy
2025-01-10  ADR-003  Caching Strategy
2025-01-15  ADR-004  Competitive Analysis Mode
2025-01-20  ADR-005  Template-Based Prompts
```

## ADR Relationships

```
ADR-001 (Go Rewrite)
  ├── ADR-002 (Extractor Factory) - Enabled by Go's interface system
  ├── ADR-003 (Caching) - Enabled by Go's file I/O and struct marshaling
  ├── ADR-004 (Competitive Mode) - Enabled by modular architecture
  ├── ADR-005 (Templates) - Enabled by structured prompt system
  └── ADR-006 (Error Handling) - Enabled by Go's error interface

ADR-002 (Extractor Factory)
  ├── ADR-003 (Caching) - Extractors must support content hashing
  ├── ADR-004 (Competitive Mode) - Extractors support discovery pipeline
  └── ADR-006 (Error Handling) - Extractors return structured errors

ADR-003 (Caching)
  ├── ADR-004 (Competitive Mode) - Caching supports competitive workflows
  └── ADR-006 (Error Handling) - Graceful cache failure handling

ADR-004 (Competitive Mode)
  └── ADR-005 (Templates) - Uses template system for prompts

ADR-005 (Templates)
  └── ADR-006 (Error Handling) - Template errors must be clear
```

## How to Read ADRs

Each ADR follows this structure:

1. **Title**: Short, descriptive name
2. **Metadata**: Status, date, deciders, tags
3. **Context**: Problem statement and requirements
4. **Decision**: What was decided and how it works
5. **Consequences**: Positive, negative, and neutral impacts
6. **Implementation**: How to implement or extend
7. **Alternatives Considered**: What else was evaluated
8. **Related Decisions**: Links to other ADRs
9. **References**: External resources
10. **Notes**: Additional insights

## Creating New ADRs

When making significant architectural decisions:

1. Copy the template from an existing ADR
2. Number sequentially (ADR-007, ADR-008, etc.)
3. Use descriptive kebab-case filenames (`ADR-NNN-short-title.md`)
4. Fill in all sections with context and rationale
5. Update this README.md index
6. Link to related ADRs

## ADR Best Practices

- **Write ADRs before implementation**: Capture decisions as they're made
- **Be specific**: Include code examples and concrete alternatives
- **Show trade-offs**: List pros and cons for all alternatives
- **Link decisions**: Reference related ADRs
- **Update status**: Mark as Superseded/Deprecated when replaced
- **Keep history**: Never delete ADRs, only mark as superseded

## Related Documentation

- [ARCHITECTURE.md](../../ARCHITECTURE.md): Technical architecture overview
- [SPEC.md](../../SPEC.md): Product specification
- [README.md](../../README.md): User documentation
- [MIGRATION.md](../../MIGRATION.md): Migration from bash version

## Questions?

For questions about ADRs or architectural decisions:

1. Read the relevant ADR for context
2. Check related ADRs for dependencies
3. Review implementation code referenced in ADR
4. Open a GitHub issue if clarification needed
