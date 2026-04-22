# Gemini Adapter Documentation Index

**Component:** Google Gemini 2.0 Integration for AGM
**Last Updated:** 2026-02-11
**Status:** Implemented (V1)

## Quick Navigation

### Getting Started
- [README.md](README.md) - Overview, quick start, usage examples
- [SPEC.md](SPEC.md) - Technical specification and requirements
- [ARCHITECTURE.md](ARCHITECTURE.md) - System architecture and design

### Architecture Decision Records (ADRs)
- [ADR-001: File-Based Storage](ADR-001-file-based-storage.md) - JSONL history persistence
- [ADR-002: Google Generative AI SDK](ADR-002-google-genai-sdk-selection.md) - SDK selection rationale
- [ADR-003: Full History Context](ADR-003-full-history-context.md) - Context management strategy

### Implementation
- [gemini_adapter.go](../gemini_adapter.go) - Main adapter implementation
- [gemini_adapter_test.go](../gemini_adapter_test.go) - Unit tests
- [gemini_parity_test.go](../gemini_parity_test.go) - Cross-adapter compatibility tests

### Related Documentation
- [Agent Interface](../interface.go) - Unified agent interface definition
- [Session Store](../session_store.go) - Session metadata storage
- [GPT Adapter](../gpt/) - Alternative adapter for comparison
- [Claude Adapter](../claude/) - CLI-based adapter reference

## Document Purpose Map

### For New Users
**Start here:** [README.md](README.md)
1. Quick start guide
2. Basic usage examples
3. Configuration options
4. Common troubleshooting

### For Developers
**Start here:** [SPEC.md](SPEC.md)
1. Read SPEC.md for requirements
2. Review ARCHITECTURE.md for design
3. Check ADRs for decision context
4. Examine source code and tests

### For Maintainers
**Start here:** [ARCHITECTURE.md](ARCHITECTURE.md)
1. Understand system design
2. Review ADRs for historical decisions
3. Check test coverage
4. Update documentation with changes

## Documentation Standards

### Document Types

#### README.md
- **Purpose:** User-facing overview and quick start
- **Audience:** Developers using the adapter
- **Content:** Usage examples, configuration, troubleshooting
- **Style:** Tutorial/guide format

#### SPEC.md
- **Purpose:** Technical specification and requirements
- **Audience:** Developers and architects
- **Content:** Functional/non-functional requirements, API contracts
- **Style:** Formal specification format

#### ARCHITECTURE.md
- **Purpose:** System design and implementation details
- **Audience:** Developers and maintainers
- **Content:** Component architecture, data flow, design patterns
- **Style:** Technical documentation with diagrams

#### ADR-XXX.md
- **Purpose:** Architecture decision records
- **Audience:** Future developers and maintainers
- **Content:** Problem, options, decision, consequences
- **Style:** ADR template (context → decision → consequences)

### Maintenance Guidelines

#### When to Update Documentation

**README.md:**
- New features added
- Configuration options changed
- Common issues identified
- Usage patterns change

**SPEC.md:**
- Requirements added or modified
- API contracts change
- Capabilities updated
- Limitations change

**ARCHITECTURE.md:**
- Component design changes
- Data flow modifications
- Integration points change
- Design patterns evolve

**ADRs:**
- Major architectural decisions made
- Technology choices changed
- Design patterns adopted
- Trade-offs re-evaluated

#### Documentation Review Checklist

Before committing changes:
- [ ] All affected documents updated
- [ ] Code examples tested and working
- [ ] Version numbers updated if needed
- [ ] Cross-references checked and valid
- [ ] Diagrams reflect current implementation
- [ ] Status fields updated (if applicable)

## Document Relationships

### Dependency Graph

```
README.md (Entry Point)
    ├─► SPEC.md (Requirements)
    │   └─► ARCHITECTURE.md (Design)
    │       ├─► ADR-001 (Storage Decision)
    │       ├─► ADR-002 (SDK Decision)
    │       └─► ADR-003 (Context Decision)
    │
    ├─► ../interface.go (API Contract)
    ├─► ../gemini_adapter.go (Implementation)
    └─► ../gemini_adapter_test.go (Tests)
```

### Cross-Reference Index

#### Storage
- SPEC.md § FR2 (Session Management)
- ARCHITECTURE.md § Storage Architecture
- ADR-001 (File-Based Storage)

#### API Integration
- SPEC.md § FR3 (Google AI API Integration)
- ARCHITECTURE.md § Integration Points
- ADR-002 (SDK Selection)

#### Context Management
- SPEC.md § Limitations (Context Window)
- ARCHITECTURE.md § Data Flow
- ADR-003 (Full History Context)

#### Testing
- SPEC.md § NFR4 (Testability)
- ARCHITECTURE.md § Testing Architecture
- gemini_adapter_test.go

## Version History

### V1.0 (2026-02-11)
- Initial documentation backfill
- SPEC.md created
- ARCHITECTURE.md created
- ADR-001, ADR-002, ADR-003 created
- README.md created
- INDEX.md created

### Future Versions (V2 Planned)
- Streaming support documentation
- Function calling documentation
- Vision input documentation
- Context window management
- Performance optimization guides

## External References

### Google Documentation
- [Gemini API Overview](https://ai.google.dev/docs)
- [Go SDK Reference](https://pkg.go.dev/github.com/google/generative-ai-go/genai)
- [Model Documentation](https://ai.google.dev/models/gemini)
- [Best Practices](https://ai.google.dev/docs/best_practices)

### AGM Documentation
- [Agent Interface Design](../doc.go)
- [Session Store Implementation](../session_store.go)
- [Registry Pattern](../registry.go)
- [Factory Pattern](../factory.go)

### Related Adapters
- [GPT Adapter Docs](../gpt/INDEX.md)
- [Claude Adapter Docs](../claude/)

## Contributing to Documentation

### Adding New Documents

1. Create document in appropriate location
2. Follow template for document type (ADR, SPEC, etc.)
3. Add entry to this INDEX.md
4. Update cross-references in related docs
5. Submit PR with documentation changes

### Improving Existing Documents

1. Make changes following style guide
2. Update "Last Updated" date
3. Update version history if significant
4. Check cross-references still valid
5. Submit PR

### Reporting Documentation Issues

- File issue in main project tracker
- Tag with `documentation` label
- Reference specific document and section
- Suggest improvement if possible

## Template References

### ADR Template
```markdown
# ADR-XXX: Title

**Status:** Proposed/Accepted/Deprecated/Superseded
**Date:** YYYY-MM-DD
**Deciders:** Team/Role
**Context:** V1/V2/etc.

## Context and Problem Statement
[Problem description]

## Decision Drivers
[Key factors]

## Considered Options
### Option 1: [Name]
**Pros:**
**Cons:**

## Decision Outcome
[Chosen option and rationale]

## Consequences
[Positive/Negative/Neutral impacts]

## References
[Related docs]
```

### SPEC.md Template
See [GPT SPEC.md](../gpt/SPEC.md) for reference structure.

### ARCHITECTURE.md Template
See [GPT ARCHITECTURE.md](../gpt/ARCHITECTURE.md) for reference structure.

## Maintenance Schedule

### Regular Updates (Quarterly)
- Review all documents for accuracy
- Update version numbers
- Check external links
- Update examples with latest API

### Ad-Hoc Updates (As Needed)
- Feature additions
- Bug fixes
- API changes
- User feedback

## Contact

For documentation questions or suggestions:
- File issue in main project repository
- Tag with `documentation` label
- Reference this INDEX.md for context
