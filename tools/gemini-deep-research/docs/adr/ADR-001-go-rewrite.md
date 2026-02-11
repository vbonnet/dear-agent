# ADR-001: Go Rewrite from Bash

**Status**: Accepted
**Date**: 2024-12-15 (backfilled 2025-02-11)
**Deciders**: Engineering Team
**Tags**: architecture, language, tooling

## Context

The original Gemini Deep Research tool was implemented as a Bash script (`gemini-deep-research.sh`) with the following characteristics:

- Single-purpose: YouTube video analysis only
- Environment variable-based configuration
- Limited error handling
- Platform-specific (Linux/macOS)
- Difficult to test and maintain
- Ad-hoc architecture without clear module boundaries

As requirements grew to support multiple content types (arXiv, HuggingFace, web articles), competitive analysis mode, and caching, the Bash implementation became increasingly complex and difficult to maintain.

### Problems with Bash Implementation

1. **Type Safety**: No compile-time validation, runtime errors only
2. **Error Handling**: Difficult to propagate errors cleanly through pipeline
3. **Testing**: Limited testing infrastructure, hard to mock dependencies
4. **Cross-Platform**: Windows support requires WSL or separate implementation
5. **Maintainability**: Large monolithic script, poor code organization
6. **Extensibility**: Adding new content extractors requires significant refactoring
7. **Performance**: Shell overhead for process spawning and pipe operations

## Decision

Rewrite the tool in Go (version 1.21+) with the following architecture:

1. **Modular Package Structure**:
   - `detector`: Content type detection
   - `extractors`: Content extraction (YouTube, arXiv, HuggingFace, web)
   - `gemini`: Topic analysis with Gemini CLI
   - `research`: Deep Research API client
   - `config`: Configuration management
   - `cmd`: Command orchestration and pipeline
   - `types`: Shared type definitions

2. **CLI Interface**:
   - Positional URL argument (not environment variable)
   - Command-line flags for configuration
   - Environment variable fallbacks

3. **Error Handling**:
   - Specific exit codes for different error types
   - Structured error types with context
   - Clear error messages with troubleshooting steps

4. **Testing Strategy**:
   - Unit tests for all packages (target: >80% coverage)
   - Integration tests for E2E pipeline
   - Mock interfaces for external dependencies

## Consequences

### Positive

1. **Type Safety**: Compile-time validation catches errors early
2. **Cross-Platform**: Single binary works on Linux, macOS, Windows
3. **Performance**: Faster execution, lower memory usage
4. **Maintainability**: Clear package boundaries, modular design
5. **Testability**: Comprehensive test coverage, easy to mock dependencies
6. **Extensibility**: Plugin-based extractor factory, easy to add new content types
7. **Error Handling**: Structured errors with proper context propagation
8. **Documentation**: Go's built-in documentation tools (godoc)

### Negative

1. **Migration Effort**: Existing users need to migrate from Bash script
2. **Build Step**: Requires compilation (vs. direct script execution)
3. **Language Change**: Team needs Go expertise (vs. shell scripting)
4. **Binary Distribution**: Need to distribute compiled binaries or build from source
5. **Dependency Management**: Go modules vs. no dependencies in Bash

### Neutral

1. **Learning Curve**: Go is straightforward for developers familiar with C-like languages
2. **Ecosystem**: Rich Go ecosystem for HTTP, JSON, testing
3. **Deployment**: Binary deployment simpler than managing script dependencies

## Implementation

### Migration Path

1. **Phase 1**: Core pipeline (detection, extraction, analysis, research)
2. **Phase 2**: Competitive analysis mode
3. **Phase 3**: Caching and performance
4. **Phase 4**: Customization (prompts, templates)
5. **Migration Guide**: Document bash→go migration (MIGRATION.md)

### Backward Compatibility

- Provide bash compatibility wrapper for legacy scripts
- Maintain similar output structure (with enhancements)
- Support same environment variables where possible
- Document all breaking changes in MIGRATION.md

### Testing Strategy

```
Unit Tests:
├── detector_test.go (content type detection)
├── extractor_test.go (each extractor)
├── gemini_test.go (topic analysis)
├── research_test.go (API client)
├── config_test.go (configuration)
└── cmd_test.go (command logic)

Integration Tests:
└── integration_test.go (E2E pipeline)
```

Target: >80% code coverage

### Validation Criteria

- [ ] All bash functionality ported to Go
- [ ] Unit tests achieve >80% coverage
- [ ] Integration tests pass for all content types
- [ ] Performance meets or exceeds bash version
- [ ] Migration guide complete
- [ ] Cross-platform testing (Linux, macOS, Windows)

## Alternatives Considered

### 1. Python Rewrite

**Pros**:
- Rich ecosystem for data processing
- Easier JSON/HTTP handling
- Better string processing

**Cons**:
- Slower startup time (interpreter)
- Dependency management complexity (pip, venv)
- Distribution requires Python runtime
- Type safety requires additional tooling (mypy)

**Decision**: Rejected due to distribution complexity and performance overhead

### 2. Rust Rewrite

**Pros**:
- Excellent performance
- Strong type safety
- Modern tooling

**Cons**:
- Steeper learning curve
- Slower compilation times
- Smaller ecosystem for our use case
- Overkill for this application

**Decision**: Rejected due to complexity vs. benefit trade-off

### 3. Enhance Bash Script

**Pros**:
- No migration needed
- Familiar to team
- No build step

**Cons**:
- Fundamental limitations remain (type safety, error handling)
- Cross-platform support difficult
- Testing infrastructure limited
- Extensibility poor

**Decision**: Rejected due to fundamental limitations

### 4. TypeScript/Node.js

**Pros**:
- Type safety with TypeScript
- Rich ecosystem
- Modern tooling

**Cons**:
- Requires Node.js runtime
- Larger distribution size
- Slower startup time
- npm dependency complexity

**Decision**: Rejected due to runtime requirement and distribution overhead

## Related Decisions

- ADR-002: Extractor Factory Pattern
- ADR-003: Caching Strategy
- ADR-004: Competitive Analysis Mode
- ADR-005: Template-Based Prompts
- ADR-006: Error Handling Strategy

## References

- [Go Programming Language](https://golang.org/)
- [MIGRATION.md](../../MIGRATION.md)
- [Original Bash Script](../../.archived/gemini-deep-research-v1.sh)
- [Architecture Documentation](../../ARCHITECTURE.md)

## Notes

This decision represents a significant architectural shift from a simple script to a full application. The modular design enables future extensibility while maintaining simplicity for common use cases.

The migration guide (MIGRATION.md) provides a compatibility layer for teams that need gradual migration.
