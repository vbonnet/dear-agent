# GPT Adapter Documentation Index

**Package:** `github.com/vbonnet/ai-tools/claude-session-manager/internal/agent/gpt`
**Version:** 1.0
**Last Updated:** 2026-02-11

## Overview

This directory contains the GPT adapter implementation for OpenAI GPT-4 integration with the Claude Session Manager's unified Agent interface.

## Documentation Structure

### User Documentation

#### [README.md](README.md)
**Audience:** Users, developers integrating the adapter
**Content:**
- Quick start guide
- Installation and setup
- Usage examples
- API reference
- Troubleshooting guide
- Feature list and limitations

### Technical Documentation

#### [SPEC.md](SPEC.md)
**Audience:** Developers, architects, QA engineers
**Content:**
- Functional requirements (FR1-FR6)
- Non-functional requirements (NFR1-NFR4)
- API contract and validation rules
- Data structures
- Error conditions
- Acceptance criteria
- V1 limitations and V2 roadmap

#### [ARCHITECTURE.md](ARCHITECTURE.md)
**Audience:** Developers, architects, system designers
**Content:**
- System overview and component diagrams
- Core components (Adapter, Session, Translator, Errors)
- Data flow diagrams (message send, session creation, export/import)
- Concurrency model (RWMutex strategy)
- Integration points (OpenAI API, Agent Registry)
- Error handling architecture
- Storage architecture (in-memory vs. file-based)
- Security architecture
- Testing architecture

### Architecture Decision Records (ADRs)

#### [ADR-001: In-Memory Session Storage](ADR-001-in-memory-storage.md)
**Decision:** Use in-memory map storage for V1
**Context:** Need to store conversation sessions quickly
**Rationale:** Prioritize development speed over persistence
**Trade-offs:** Sessions lost on restart (acceptable for V1)
**Status:** Accepted

#### [ADR-002: Exponential Backoff for Rate Limits](ADR-002-exponential-backoff.md)
**Decision:** Implement exponential backoff retry logic for 429 errors
**Context:** OpenAI API enforces rate limits
**Rationale:** Industry standard, high success rate, balanced trade-offs
**Parameters:** 5 retries, 1s base delay, max 31s total wait
**Status:** Accepted

#### [ADR-003: Message Translation Strategy](ADR-003-message-translation-strategy.md)
**Decision:** Use dedicated translator functions in separate file
**Context:** Need to convert between agent.Message and OpenAI formats
**Rationale:** Separation of concerns, testability, reusability
**Implementation:** Pure functions in `translator.go`
**Status:** Accepted

#### [ADR-004: Thread Safety with RWMutex](ADR-004-thread-safety-rwmutex.md)
**Decision:** Use sync.RWMutex for session map protection
**Context:** Concurrent access from multiple goroutines
**Rationale:** Read-heavy workload benefits from concurrent reads
**Alternative:** sync.Mutex (no concurrent reads), sync.Map (type erasure)
**Status:** Accepted

#### [ADR-005: OpenAI SDK Selection](ADR-005-openai-sdk-selection.md)
**Decision:** Use sashabaranov/go-openai third-party SDK
**Context:** Need to integrate with OpenAI Chat Completion API
**Rationale:** Development speed (3 days vs. 2 weeks), reliability, type safety
**Alternative:** Custom HTTP client (too much code, error-prone)
**Status:** Accepted

## Implementation Files

### Source Code

| File | Purpose | Lines of Code |
|------|---------|---------------|
| `adapter.go` | Main adapter implementing agent.Agent interface | ~420 |
| `session.go` | Session data structure | ~30 |
| `translator.go` | Message format conversion (agent ↔ OpenAI) | ~48 |
| `errors.go` | Error definitions and types | ~42 |
| `adapter_test.go` | Comprehensive test suite | ~300+ |
| `verify_integration.go` | Integration test helpers | ~50 |

**Total Production Code:** ~540 lines
**Total Test Code:** ~350 lines
**Test Coverage:** >90%

### Documentation Files

| File | Type | Purpose |
|------|------|---------|
| `README.md` | User Guide | Usage, setup, examples |
| `SPEC.md` | Technical Spec | Requirements, API contract |
| `ARCHITECTURE.md` | Architecture Doc | System design, data flow |
| `ADR-001-*.md` | Decision Record | In-memory storage |
| `ADR-002-*.md` | Decision Record | Exponential backoff |
| `ADR-003-*.md` | Decision Record | Message translation |
| `ADR-004-*.md` | Decision Record | Thread safety (RWMutex) |
| `ADR-005-*.md` | Decision Record | OpenAI SDK selection |
| `INDEX.md` | This file | Documentation index |

## Quick Links

### For New Users
1. Start with [README.md](README.md) for setup and basic usage
2. Review examples in README for common patterns
3. Check troubleshooting section for common issues

### For Developers
1. Read [SPEC.md](SPEC.md) for requirements and API contract
2. Review [ARCHITECTURE.md](ARCHITECTURE.md) for system design
3. Read ADRs for context on key design decisions
4. Check `adapter_test.go` for usage examples

### For Architects
1. Review [ARCHITECTURE.md](ARCHITECTURE.md) for system overview
2. Read all ADRs for design rationale
3. Check V2 roadmap sections in SPEC and ARCHITECTURE

## Document Maintenance

### When to Update Documentation

| Change Type | Update Required |
|-------------|-----------------|
| New feature added | SPEC.md (requirements), ARCHITECTURE.md (design), README.md (usage) |
| API contract changed | SPEC.md (API contract section) |
| Architecture decision | New ADR file |
| Bug fix | README.md (troubleshooting if user-visible) |
| Performance optimization | ARCHITECTURE.md (if design changed) |
| V2 enhancement | SPEC.md and ARCHITECTURE.md (V2 roadmap sections) |

### Documentation Review Checklist
- [ ] All code changes reflected in docs
- [ ] Examples still work (copy-paste into test)
- [ ] Version numbers updated
- [ ] "Last Updated" dates current
- [ ] Links between docs still valid
- [ ] Diagrams match current implementation

## Related Documentation

### Parent Package
- [Agent Interface](../interface.go) - Unified agent interface definition
- [Agent Package Doc](../doc.go) - Agent ecosystem overview

### Other Adapters
- Claude Adapter: `../claude/adapter.go`
- Gemini Adapter: `../gemini_adapter.go`

### External References
- [OpenAI API Documentation](https://platform.openai.com/docs/api-reference/chat)
- [sashabaranov/go-openai SDK](https://pkg.go.dev/github.com/sashabaranov/go-openai)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-02-11 | Initial documentation (V1 completion) |

## Contact

For questions or issues:
- **Code Issues:** File issue in ai-tools repository
- **Documentation Issues:** File issue with "docs" label
- **Architecture Questions:** Review ADRs or contact maintainers

---

**Navigation:** [README](README.md) | [SPEC](SPEC.md) | [ARCHITECTURE](ARCHITECTURE.md) | [ADRs](#architecture-decision-records-adrs)
