# ADR-001: Polyglot Architecture (Go + Python + TypeScript)

**Status:** Accepted
**Date:** 2026-03-12
**Deciders:** Claude Sonnet 4.5, User
**Phase:** Phase 0 (Swarm Setup & Discovery)

---

## Context

The diagram-as-code enhancement requires integrating multiple diagram tools (D2, Structurizr, Mermaid, PlantUML) with the existing spec-review-marketplace infrastructure. Each tool has different implementation languages and API maturity:

- **D2:** Go-native with official `oss.terrastruct.com/d2` library
- **Mermaid:** JavaScript-native with official npm package `mermaid`
- **Structurizr:** Java-native with official SDK (Python wrapper archived/unmaintained)
- **PlantUML:** Java-native (no actively maintained Go/Python libraries)
- **Existing marketplace:** Python CLI adapters (claude-code.py, gemini.py, etc.)

## Decision

We will use a **polyglot architecture** with language choices optimized for each component:

1. **Go** - Core library implementation
   - Diagram rendering abstractions (renderer package)
   - C4 Model validation (c4model package)
   - CLI wrapper orchestration
   - Reason: D2 native support, excellent CLI execution (os/exec), type safety

2. **Python** - CLI adapter layer
   - Cross-CLI compatibility (Claude Code, Gemini, OpenCode, Codex)
   - Type hints (PEP 484) with mypy strict mode
   - Existing pattern in marketplace
   - Reason: Maintains consistency, excellent for glue code

3. **TypeScript** - Mermaid service (Phase 2+)
   - Native Mermaid integration
   - Type-safe rendering API
   - Reason: Official Mermaid package is npm, TypeScript provides safety

## Consequences

### Positive

**Native library access:**
- D2: Use official Go library (fastest, most reliable)
- Mermaid: Use official npm package (best support)
- No dependency on community wrappers (often unmaintained)

**Type safety across stack:**
- Go: Compile-time type checking, interface enforcement
- Python: mypy strict mode, Literal types for enums
- TypeScript: tsc --strict mode
- Reduces runtime errors, improves IDE support

**Performance:**
- Go: Fast CLI execution, efficient I/O streaming
- No overhead from cross-language FFI/RPC
- Each language optimized for its task

**Maintainability:**
- Each component in its "natural" language
- Easier to find developers (Go, Python, TypeScript are mainstream)
- Official libraries reduce bus factor

### Negative

**Complexity:**
- Multiple toolchains required (Go, Node.js, Python)
- Build process more complex (3 compilation steps)
- More dependencies to manage

**Coordination overhead:**
- Interface contracts between languages (CLI args, JSON, stdout/stderr)
- Testing requires all toolchains installed
- Deployment bundles larger

**Learning curve:**
- Developers need polyglot skills
- Debugging crosses language boundaries
- Different idioms per language

## Alternatives Considered

### Alternative 1: Pure Python

**Approach:** Use Python for everything (py-d2, mermaid-py wrappers)

**Rejected because:**
- py-d2 is community-maintained (not official, could become stale)
- mermaid-py is community wrapper (adds layer of indirection)
- Python subprocess overhead for CLI calls
- No compile-time safety (even with type hints)

### Alternative 2: Pure Go

**Approach:** Use Go for everything, wrap Mermaid via exec

**Rejected because:**
- Mermaid CLI via exec loses type safety (JSON serialization overhead)
- No official Mermaid Go library
- Structurizr requires Java (same exec overhead)
- Breaks existing Python CLI adapter pattern

### Alternative 3: Pure TypeScript

**Approach:** Use Node.js for everything

**Rejected because:**
- No official D2 npm package
- C4 validation logic awkward in JavaScript (weakly typed)
- Performance worse than Go for CLI orchestration
- Breaks existing Python marketplace pattern

## Implementation Notes

**Module boundaries:**
```
Go (lib/diagram/)
├── renderer/        # Rendering abstraction
├── c4model/         # C4 semantic validation
└── validator/       # Syntax validation wrapper

Python (skills/*/cli-adapters/)
├── claude-code.py   # CLI adapter (calls Go binaries)
├── gemini.py
├── opencode.py
└── codex.py

TypeScript (skills/render-diagrams/mermaid-service/)
└── src/renderer.ts  # Mermaid native integration
```

**Communication:**
- Python → Go: subprocess.run() with JSON args
- Go → CLI tools: os/exec.Command() with pipes
- TypeScript → Mermaid: Native npm package import

**Testing:**
- Go: `go test ./...` (unit + integration)
- Python: `pytest` (interface tests, mypy type checking)
- TypeScript: `jest` (Mermaid rendering tests)

## Validation

**Success criteria:**
- ✅ All toolchains verified (Phase 0, Task 0.5)
- ✅ Go modules compile (`go build ./...`)
- ✅ Python type hints pass strict mypy
- ✅ Cross-CLI tests pass (all 4 adapters)

**Metrics (Phase 1 complete):**
- Go library: 10 files, ~1500 LOC, 100% tests pass
- Python adapters: 5 files, ~500 LOC, 100% tests pass
- TypeScript: Deferred to Phase 2
- Build time: <5 seconds (Go), instant (Python)

## Related

- **Decision D001** - Go for D2 Integration
- **Decision D002** - TypeScript for Mermaid Integration
- **Decision D003** - Python CLI Adapters
- **ADR-002** - CLI Wrapper Pattern
- **ADR-003** - Separate Go Modules

---

**Change Log:**
- 2026-03-12: Initial ADR created (Phase 1 completion)
