# ADR-003: Separate Go Modules (Independent Versioning)

**Status:** Accepted
**Date:** 2026-03-12
**Deciders:** Claude Sonnet 4.5, User
**Phase:** Phase 1 (Foundation & Tool Integration)

---

## Context

The diagram library consists of multiple packages with different concerns:
- **renderer/** - Diagram rendering abstractions (evolves with new formats)
- **c4model/** - C4 Model semantic framework (stable, rarely changes)
- **validator/** - Syntax validation wrappers (delegates to renderers)

We must decide: One go.mod for entire lib/diagram, or separate go.mod per package?

## Decision

We will use **separate Go modules** with independent go.mod files:

```
lib/diagram/
├── go.mod                    # Root module (empty, no code)
├── renderer/
│   ├── go.mod                # Module: .../renderer
│   └── *.go
├── c4model/
│   ├── go.mod                # Module: .../c4model
│   └── *.go
└── validator/
    ├── go.mod                # Module: .../validator
    └── *.go
```

Each package is an independent Go module with its own version and dependencies.

## Rationale

### 1. Independent Versioning

**Different stability levels:**
- **c4model:** Stable (v1.0.0 → changes rarely, semantic versioning)
- **renderer:** Evolving (v0.x → new formats added frequently)
- **validator:** Wrapper (follows renderer versions)

**Version independence:**
```
c4model v1.0.0     → v1.0.1 (bug fix)
renderer v0.3.0    → v0.4.0 (add LikeC4 support)
validator v0.2.0   → v0.2.1 (minor update)
```

Users can upgrade renderer without changing c4model (if only rendering changes).

### 2. Dependency Isolation

**c4model has zero dependencies:**
```go
// c4model/go.mod
module github.com/engram/plugins/spec-review-marketplace/lib/diagram/c4model
go 1.21
// No external dependencies - pure business logic
```

**renderer has CLI tool dependencies:**
```go
// renderer/go.mod
module github.com/engram/plugins/spec-review-marketplace/lib/diagram/renderer
go 1.21
// Future: Native D2 library
require (
    oss.terrastruct.com/d2 v0.7.1
)
```

**Benefit:** Importing c4model doesn't pull renderer dependencies.

### 3. Clear Ownership Boundaries

**Package responsibilities:**
- **c4model:** C4 Model validation logic ONLY
  - No rendering concerns
  - No file I/O
  - Pure functions + data structures

- **renderer:** Rendering orchestration ONLY
  - No C4 validation
  - Delegates to CLI tools
  - I/O and error handling

- **validator:** Thin wrapper ONLY
  - Delegates to renderer.Validate()
  - Format detection (future)

**No circular dependencies:**
```
validator → renderer (allowed)
renderer  → c4model  (NOT needed, renderer is format-agnostic)
c4model   → renderer (NOT allowed, would be circular)
```

### 4. Reusability

**c4model is reusable standalone:**
```go
import "github.com/engram/plugins/spec-review-marketplace/lib/diagram/c4model"

// Use C4 validation without renderer
validator := &c4model.Validator{}
result := validator.Validate(diagram, nil)
```

**Use cases:**
- Validate C4 diagrams without rendering
- Lint diagrams in CI/CD
- Custom tooling (diagram generators, linters)

**renderer is reusable standalone:**
```go
import "github.com/engram/plugins/spec-review-marketplace/lib/diagram/renderer"

// Use rendering without C4 validation
r, _ := renderer.Get(renderer.FormatD2)
r.Render(ctx, source, dest, opts)
```

**Use cases:**
- Render non-C4 diagrams (flowcharts, sequences)
- Custom rendering pipelines
- Integration with other tools

## Consequences

### Positive

**Semantic versioning per module:**
```
c4model@v1.2.0    # Stable API, backward compatible
renderer@v0.5.3   # Pre-1.0, breaking changes allowed
validator@v0.3.1  # Follows renderer
```

**Smaller dependency graphs:**
- Importing c4model: 0 transitive dependencies
- Importing renderer: Only CLI tools (future: d2 library)
- Total savings: Megabytes of unused code not downloaded

**Parallel development:**
- c4model changes don't block renderer releases
- renderer experiments don't destabilize c4model
- Different teams can own different modules

**Clear API contracts:**
```go
// c4model exports public API
type Validator struct { ... }
func (v *Validator) Validate(...) *ValidationResult

// renderer exports public API
type Renderer interface { ... }
func Get(format Format) (Renderer, error)

// No cross-module coupling
```

### Negative

**More go.mod files to manage:**
- 3 modules = 3 go.mod + 3 go.sum files
- Must run `go mod tidy` in each module
- More git churn (go.sum changes)

**Import paths longer:**
```go
// Single module
import "github.com/engram/.../diagram/c4model"

// Separate modules (same, no difference)
import "github.com/engram/.../diagram/c4model"
```
(Actually no difference - paths are identical)

**Cross-module changes harder:**
- Can't change c4model + renderer in single PR (different versions)
- Must coordinate releases (release c4model first, then renderer)
- Mitigated: Modules are loosely coupled (rare cross-module changes)

**Testing cross-module integration:**
```bash
# Must test each module independently
cd c4model && go test ./...
cd renderer && go test ./...
cd validator && go test ./...
```

## Alternatives Considered

### Alternative 1: Monolithic Module

**Approach:** Single go.mod for entire lib/diagram

```
lib/diagram/
├── go.mod               # Everything in one module
├── renderer/
├── c4model/
└── validator/
```

**Rejected because:**
- c4model changes trigger renderer version bumps (unnecessary coupling)
- Importing c4model pulls renderer dependencies (bloat)
- No independent versioning (can't stabilize c4model at v1.0 while renderer is v0.x)
- Harder to reuse subsets (all-or-nothing import)

### Alternative 2: Workspaces

**Approach:** Use Go 1.18+ workspaces

```
lib/diagram/go.work:
go 1.21

use (
    ./renderer
    ./c4model
    ./validator
)
```

**Rejected because:**
- Workspaces are for development, not distribution
- Users don't see workspaces (go get downloads modules individually)
- Doesn't solve versioning problem (still need separate go.mod files)
- Adds complexity without benefit

### Alternative 3: Monorepo Tool (Bazel/Pants)

**Approach:** Use monorepo build tool for dependency management

**Rejected because:**
- Massive complexity (Bazel learning curve steep)
- Breaks standard Go tooling (go build, go test)
- Not justified for 3 modules
- Engram uses standard Go modules (consistency)

## Implementation

### Module Structure

```
lib/diagram/
├── go.mod                              # Root (empty placeholder)
├── renderer/
│   ├── go.mod                          # Independent module
│   ├── renderer.go
│   ├── renderer_test.go
│   ├── d2.go
│   └── ...
├── c4model/
│   ├── go.mod                          # Independent module
│   ├── model.go
│   ├── validator.go
│   └── validator_test.go
└── validator/
    ├── go.mod                          # Independent module
    └── validator.go
```

### Module Initialization

```bash
# Initialize each module
cd renderer && go mod init github.com/engram/.../renderer && go mod tidy
cd c4model && go mod init github.com/engram/.../c4model && go mod tidy
cd validator && go mod init github.com/engram/.../validator && go mod tidy
```

### Testing Each Module

```bash
# Test independently
cd renderer && go test -v ./...
cd c4model && go test -v ./...
cd validator && go test -v ./...

# Or use script
./test-all-modules.sh
```

### Versioning Strategy

**c4model (stable):**
- v1.0.0: Initial release (Phase 1 complete)
- v1.0.x: Bug fixes only
- v1.1.0: Backward-compatible additions
- v2.0.0: Breaking changes (rare)

**renderer (evolving):**
- v0.1.0: MVP (D2, Mermaid, Structurizr)
- v0.2.0: Add PlantUML
- v0.3.0: Native D2 integration (Phase 2)
- v1.0.0: Stable API (Phase 3+)

**validator (wrapper):**
- Follows renderer versions (v0.x until renderer v1.0)

### Import Examples

**Using c4model only:**
```go
import "github.com/engram/plugins/spec-review-marketplace/lib/diagram/c4model"

validator := &c4model.Validator{}
result := validator.Validate(diagram, nil)
```

**Using renderer only:**
```go
import "github.com/engram/plugins/spec-review-marketplace/lib/diagram/renderer"

r, _ := renderer.Get(renderer.FormatD2)
r.Render(ctx, source, dest, opts)
```

**Using both (typical):**
```go
import (
    "github.com/engram/plugins/spec-review-marketplace/lib/diagram/c4model"
    "github.com/engram/plugins/spec-review-marketplace/lib/diagram/renderer"
)

// Validate C4 compliance
validator := &c4model.Validator{}
result := validator.Validate(diagram, nil)

// Render if valid
if result.Passed {
    r, _ := renderer.Get(renderer.FormatD2)
    r.Render(ctx, source, dest, opts)
}
```

## Validation

**Phase 1 metrics:**
- ✅ 3 independent modules created
- ✅ c4model: 0 external dependencies
- ✅ renderer: 0 external dependencies (CLI wrappers)
- ✅ All modules compile (`go build ./...`)
- ✅ All tests pass independently

**Future validation (Phase 2+):**
- c4model reaches v1.0.0 (API stable)
- renderer reaches v1.0.0 (API stable)
- No breaking changes in c4model (major version stays at 1)
- Breaking changes in renderer isolated (doesn't affect c4model users)

## Related

- **ADR-001** - Polyglot Architecture
- **ADR-002** - CLI Wrapper Pattern
- **Go Modules Documentation:** https://go.dev/doc/modules/
- **Semantic Versioning:** https://semver.org/

---

**Change Log:**
- 2026-03-12: Initial ADR created (Phase 1 completion)
