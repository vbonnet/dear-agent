# Diagram Library Architecture

**Version:** 1.0.0
**Last Updated:** 2026-03-12
**Status:** Phase 1 Complete
**Module:** `github.com/engram/plugins/spec-review-marketplace/lib/diagram`

---

## 1. System Context

### Purpose

This library provides a unified abstraction layer for diagram-as-code tools (D2, Structurizr DSL, Mermaid, PlantUML) with C4 Model semantic framework validation. It enables:

1. **Unified rendering interface** - Single API for multiple diagram formats
2. **C4 compliance validation** - Enforce C4 Model level-specific rules
3. **Cross-CLI compatibility** - Works with Claude Code, Gemini, OpenCode, Codex

### System Boundary

**In Scope:**
- Diagram rendering (D2, Structurizr, Mermaid, PlantUML)
- C4 Model semantic validation (Levels 1-4)
- Syntax validation wrappers
- CLI execution and error handling

**Out of Scope:**
- Diagram editing/manipulation (use native tools)
- Visual regression testing (handled at skill level)
- Diagram generation from code (separate concern in create-diagrams skill)

### External Dependencies

**CLI Tools (called via exec):**
- `d2` - D2 compiler (v0.7.1+)
- `structurizr.sh` - Structurizr CLI (v2025.11.09+)
- `mmdc` - Mermaid CLI (@mermaid-js/mermaid-cli v11.12.0+)
- `java -jar plantuml.jar` - PlantUML (optional, migration support)

**Go Dependencies:**
- Standard library only (os/exec, io, context, errors, fmt)
- No external Go libraries (CLI wrappers, not native integrations)

---

## 2. Architecture Overview

### High-Level Design

```
┌─────────────────────────────────────────────────────┐
│ Skills Layer (Python CLI Adapters)                  │
│ - render-diagrams, create-diagrams, review-diagrams │
│ - CLI: claude-code.py, gemini.py, opencode.py       │
└──────────────────┬──────────────────────────────────┘
                   │ (subprocess calls)
                   ▼
┌─────────────────────────────────────────────────────┐
│ Diagram Library (Go)                                │
│                                                      │
│  ┌──────────────────┐   ┌──────────────────┐       │
│  │ renderer/        │   │ c4model/         │       │
│  │ - Renderer iface │   │ - Semantic model │       │
│  │ - D2Renderer     │   │ - Validator      │       │
│  │ - MermaidRenderer│   │ - Level rules    │       │
│  │ - StructurizrR   │   └──────────────────┘       │
│  │ - PlantUMLR      │                               │
│  │ - Registry       │   ┌──────────────────┐       │
│  └──────────────────┘   │ validator/       │       │
│                         │ - Syntax wrapper │       │
│                         └──────────────────┘       │
└──────────────────┬──────────────────────────────────┘
                   │ (os/exec)
                   ▼
┌─────────────────────────────────────────────────────┐
│ Native Diagram Tools                                │
│ - d2, structurizr.sh, mmdc, plantuml.jar            │
└─────────────────────────────────────────────────────┘
```

### Module Structure

**Independent Go modules (separate go.mod files):**

1. **renderer/** - Rendering abstraction
   - Module: `github.com/engram/plugins/spec-review-marketplace/lib/diagram/renderer`
   - Purpose: Unified interface for diagram rendering
   - Dependencies: None (CLI wrappers only)

2. **c4model/** - C4 semantic framework
   - Module: `github.com/engram/plugins/spec-review-marketplace/lib/diagram/c4model`
   - Purpose: C4 Model validation and compliance
   - Dependencies: None (pure business logic)

3. **validator/** - Syntax validation
   - Module: `github.com/engram/plugins/spec-review-marketplace/lib/diagram/validator`
   - Purpose: Wrapper for format-specific validation
   - Dependencies: None (delegates to renderers)

**Why separate modules?**
- Independent versioning (c4model rarely changes, renderers evolve)
- Clear dependency boundaries (c4model has zero deps)
- Testing isolation (each module tested independently)
- Reusability (c4model usable without renderers)

---

## 3. Component Details

### 3.1 Renderer Package

**File: `renderer/renderer.go`**

**Core Interface:**
```go
type Renderer interface {
    Render(ctx context.Context, source io.Reader, dest io.Writer, opts *RenderOptions) error
    Validate(ctx context.Context, source io.Reader) error
    SupportedFormats() []OutputFormat
    SupportedEngines() []LayoutEngine
    Format() Format
}
```

**Design Rationale:**
- **io.Reader/Writer** - Memory efficient, works with files/buffers/pipes
- **context.Context** - Timeout/cancellation support for long renders
- **RenderOptions** - Extensible configuration (layout, output format, theme)

**Registry Pattern:**
```go
var renderers = map[Format]Renderer{
    FormatD2: &D2Renderer{},
    FormatMermaid: &MermaidRenderer{},
    FormatStructurizr: &StructurizrRenderer{},
    // FormatPlantUML not auto-registered (migration only)
}

func Get(format Format) (Renderer, error)
```

**Why registry?**
- Dynamic renderer lookup (format string → implementation)
- Easy to extend (new renderers just register themselves)
- Decoupled from skill layer (skills don't import renderer types)

### 3.2 C4 Model Package

**File: `c4model/model.go`**

**Core Types:**
```go
type Level int

const (
    LevelContext   Level = 1  // System boundary
    LevelContainer Level = 2  // Services, apps, databases
    LevelComponent Level = 3  // Internal components
    LevelCode      Level = 4  // Classes (rarely used)
)

type ElementType string
type RelationshipType string
```

**C4 Model Rules (Level-Specific):**

**Level 1 (Context):**
- Allowed elements: Person, SoftwareSystem, ExternalSystem
- Allowed relationships: Uses
- Required: At least 1 Person, 1 SoftwareSystem
- Purpose: System boundary + external actors

**Level 2 (Container):**
- Allowed elements: Container, Database, MessageQueue
- Allowed relationships: Uses, ReadsFrom, WritesTo, SendsTo
- Required: Parent SoftwareSystem (focus system)
- Purpose: High-level technology choices

**Level 3 (Component):**
- Allowed elements: Component
- Allowed relationships: Uses, DependsOn
- Required: Parent Container (focus container)
- Purpose: Internal component structure

**Level 4 (Code):**
- Allowed elements: Class, Interface
- Allowed relationships: Extends, Implements, DependsOn
- Purpose: Code-level details (auto-generated)

**File: `c4model/validator.go`**

**Validation Algorithm:**
```go
func (v *Validator) Validate(diagram *Diagram) *ValidationResult {
    score := 100

    // 1. Check required elements (Level 1: Person + SoftwareSystem)
    if !hasElement(diagram, ElementTypePerson) {
        score -= 10
        errors = append(errors, "Missing Person element")
    }

    // 2. Validate element types allowed for level
    for _, element := range diagram.Elements {
        if !isElementAllowed(element.Type, diagram.Level) {
            score -= 10
            errors = append(errors, "Invalid element type")
        }
    }

    // 3. Validate relationships
    for _, rel := range diagram.Relationships {
        if !isRelationshipAllowed(rel.Type, diagram.Level) {
            score -= 10
            errors = append(errors, "Invalid relationship type")
        }
    }

    // 4. Strict mode: Check for orphaned elements
    if opts.StrictMode {
        // Elements without relationships
    }

    return &ValidationResult{Score: score, Errors: errors}
}
```

**Scoring System:**
- Base score: 100 points
- Each violation: -10 points
- Passing threshold: ≥70 points
- Perfect compliance: 100 points

### 3.3 Validator Package

**File: `validator/validator.go`**

**Unified Validation Interface:**
```go
func ValidateAll(source io.Reader, format Format) error {
    renderer, err := renderer.Get(format)
    if err != nil {
        return err
    }
    return renderer.Validate(context.Background(), source)
}
```

**Why wrapper?**
- Single entry point for syntax validation
- Format auto-detection (future enhancement)
- Consistent error handling

---

## 4. Data Flow

### Rendering Flow

```
1. Skill invokes Python CLI adapter
   └─> claude-code.py render --format=d2 input.d2 output.svg

2. Python adapter calls Go binary (future)
   └─> Or: subprocess.run(["d2", "input.d2", "output.svg"])

3. Renderer.Render() called
   ├─> Read source (io.Reader)
   ├─> Execute CLI: exec.Command("d2", "-", "-")
   ├─> Stream input via stdin
   ├─> Capture output via stdout
   └─> Write to dest (io.Writer)

4. Return rendered diagram or error
```

### Validation Flow

```
1. C4 Compliance Validation
   ├─> Parse diagram → extract elements/relationships
   ├─> Check level-specific rules
   ├─> Calculate score (100 - violations*10)
   └─> Return ValidationResult{Score, Errors, Warnings}

2. Syntax Validation
   ├─> Get renderer for format
   ├─> Call renderer.Validate()
   ├─> Execute CLI with --dry-run flag
   └─> Return syntax errors
```

---

## 5. Error Handling

### Error Categories

**1. CLI Execution Errors:**
```go
if err := cmd.Run(); err != nil {
    return fmt.Errorf("d2 render failed: %w (stderr: %s)", err, stderr.String())
}
```
- Include stderr output for debugging
- Preserve exit codes
- Add context (which tool, what command)

**2. Validation Errors:**
```go
type ValidationError struct {
    Level   Level
    Message string
    Element *Element  // Optional: specific element
}
```
- Structured errors (not just strings)
- Reference specific elements when possible
- Severity levels (error vs warning)

**3. File I/O Errors:**
```go
if err != nil {
    return fmt.Errorf("failed to read diagram source: %w", err)
}
```
- Wrap errors with context
- Use %w for error wrapping (Go 1.13+)

### Error Recovery

**Retry Strategy:**
- None (diagram rendering is deterministic)
- CLI failures are permanent (fix source, not retry)

**Graceful Degradation:**
- If PlantUML unavailable, skip (optional renderer)
- If validation fails, return structured errors (don't crash)

---

## 6. Performance Characteristics

### Rendering Performance

**Small diagrams (<50 elements):**
- D2: ~100-200ms
- Mermaid: ~300-500ms
- Structurizr: ~500ms (Java startup)

**Large diagrams (>200 elements):**
- D2: ~2-5 seconds
- Mermaid: ~5-10 seconds
- Structurizr: ~10-20 seconds (workspace export)

**Bottlenecks:**
- CLI startup time (especially Java for Structurizr/PlantUML)
- Layout algorithm complexity (ELK > Dagre > TALA)
- Image encoding (SVG fast, PNG/PDF slower)

**Optimization Strategies:**
- Stream I/O (io.Reader/Writer, not files)
- Concurrent rendering (multiple diagrams in parallel)
- Caching (future: cache rendered outputs)

### Validation Performance

**C4 Compliance:**
- Complexity: O(n) where n = elements + relationships
- Typical: <10ms for diagrams with <100 elements
- Memory: O(n) - single pass over elements

**Syntax Validation:**
- Complexity: Same as rendering (calls CLI)
- Typical: 100-500ms (CLI startup overhead)
- Memory: Stream-based (constant)

---

## 7. Testing Strategy

### Unit Tests

**C4 Model Tests (`c4model/validator_test.go`):**
```go
func TestValidator_ContextDiagram(t *testing.T) {
    tests := []struct {
        name    string
        diagram *Diagram
        wantErr bool
        wantScore int
    }{
        {"valid context", validContext, false, 100},
        {"missing person", missingPerson, false, 90},
        {"wrong element", wrongElement, false, 90},
    }
    // Table-driven tests
}
```

**Coverage Target:** ≥80% for business logic

**Test Categories:**
- Valid diagrams (all levels)
- Missing required elements
- Invalid element types
- Invalid relationships
- Edge cases (empty diagrams, orphans)

### Integration Tests

**Renderer Tests (`renderer/renderer_test.go`):**
```go
func TestD2Renderer_Render(t *testing.T) {
    // Requires d2 CLI installed
    if !commandExists("d2") {
        t.Skip("d2 not installed")
    }

    renderer := &D2Renderer{}
    // Test actual rendering
}
```

**Coverage Target:** ≥60% (requires CLI tools)

### CLI Adapter Tests

**Python Tests (`skills/render-diagrams/cli-adapters/test_cli_adapters.py`):**
- Test help output consistency
- Validate type hints (mypy strict mode)
- Cross-CLI interface compatibility

---

## 8. Security Considerations

### Command Injection Prevention

**Risk:** Malicious input passed to CLI tools

**Mitigation:**
```go
// BAD: Shell injection possible
cmd := exec.Command("sh", "-c", "d2 " + userInput)

// GOOD: Args array prevents injection
cmd := exec.Command("d2", userInput, outputFile)
```

**All CLI calls use exec.Command with args array, never shell strings.**

### Path Traversal Prevention

**Risk:** User-supplied paths escape intended directories

**Mitigation:**
```go
// Validate paths before execution
if filepath.IsAbs(userPath) || strings.Contains(userPath, "..") {
    return errors.New("invalid path")
}
```

**Skills layer validates paths before calling library.**

### Resource Limits

**Risk:** Large diagrams cause DoS

**Mitigation:**
- Context timeouts (30s default, configurable)
- No recursion limits (C4 is max 4 levels deep)
- Stream-based I/O (memory bounded)

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
err := renderer.Render(ctx, source, dest, opts)
```

---

## 9. Deployment Architecture

### Build Process

**Go Binaries:**
```bash
# Each module builds independently
cd renderer && go build -o render-diagrams ./...
cd c4model && go build -o c4-validator ./...
```

**Python CLI Adapters:**
- No build required (interpreted)
- Type checking: `mypy --strict *.py`
- Linting: `ruff check *.py`

### Installation

**System Requirements:**
- Go 1.21+ (for library)
- Python 3.10+ (for CLI adapters)
- Node.js 18+ (for Mermaid CLI)
- Java 17+ (for Structurizr CLI)

**CLI Tool Installation:**
```bash
# D2
go install oss.terrastruct.com/d2@latest

# Mermaid
npm install -g @mermaid-js/mermaid-cli

# Structurizr
wget https://github.com/structurizr/cli/releases/download/v2025.11.09/structurizr-cli.zip
```

**Library Installation:**
```bash
# Import in Go code
import "github.com/engram/plugins/spec-review-marketplace/lib/diagram/renderer"

# Dependencies auto-downloaded
go mod download
```

---

## 10. Monitoring & Observability

### Metrics to Track

**Rendering Metrics:**
- Render duration (p50, p95, p99)
- Success rate (%)
- Error types (CLI not found, syntax error, timeout)
- Output size distribution

**Validation Metrics:**
- C4 compliance scores (avg, median)
- Violation types (element, relationship, structure)
- Strict mode failures (%)

### Logging Strategy

**Log Levels:**
- ERROR: CLI execution failures, validation errors
- WARN: Deprecated features, suboptimal configurations
- INFO: Render start/complete, validation results
- DEBUG: CLI stdout/stderr, detailed validation

**Structured Logging:**
```go
log.Info("diagram rendered",
    "format", "d2",
    "layout", "elk",
    "duration_ms", duration.Milliseconds(),
    "output_format", "svg",
)
```

---

## 11. Future Enhancements

### Planned (Phase 2-3)

1. **Native D2 Integration:**
   - Import `oss.terrastruct.com/d2` library
   - Remove CLI dependency for D2
   - Performance: 10-100x faster

2. **Diagram Generation:**
   - CodebaseAnalyzer (extract architecture from code)
   - C4ModelBuilder (map code → C4 elements)
   - Template system (microservices, monolith patterns)

3. **Visual Regression:**
   - Baseline storage (git or cloud)
   - Pixel-diff comparison
   - Percy/Chromatic integration

### Deferred (Phase 7+)

1. **MCP Integration:**
   - Eraser MCP server
   - Draw.io AWS MCP server
   - Interactive diagram editing

2. **Advanced C4 Features:**
   - Multiple workspaces
   - Deployment diagrams
   - Dynamic views

---

## 12. Decision Records

**Key architectural decisions documented in:**
- ADR-001: Polyglot Architecture (Go + Python + TypeScript)
- ADR-002: CLI Wrapper Pattern (exec vs native libraries)
- ADR-003: Separate Go Modules (independent versioning)
- ADR-004: C4 Model Enforcement (semantic validation)
- ADR-005: Registry Pattern (renderer lookup)

See: `DECISION_LOG.md`

---

## 13. Related Documents

- **SPEC.md** - Product specification (marketplace-wide)
- **ROADMAP.md** - Implementation phases
- **DECISION_LOG.md** - All design decisions
- **RETRO_LOG.md** - Learnings and retrospectives
- **lib/diagram/README.md** - Quick start guide

---

## Appendix A: Interface Reference

### Renderer Interface

```go
type Renderer interface {
    // Render diagram from source to destination
    Render(ctx context.Context, source io.Reader, dest io.Writer, opts *RenderOptions) error

    // Validate diagram syntax without rendering
    Validate(ctx context.Context, source io.Reader) error

    // Supported output formats (svg, png, pdf)
    SupportedFormats() []OutputFormat

    // Supported layout engines (elk, dagre, tala, dot)
    SupportedEngines() []LayoutEngine

    // Format identifier (d2, mermaid, structurizr, plantuml)
    Format() Format
}
```

### C4 Validator Interface

```go
type Validator struct{}

func (v *Validator) Validate(diagram *Diagram, opts *ValidationOptions) *ValidationResult

type ValidationResult struct {
    Score    int              // 0-100
    Errors   []ValidationError
    Warnings []ValidationWarning
    Passed   bool             // Score >= 70
}
```

---

**Version History:**

| Version | Date | Changes | Rationale |
|---------|------|---------|-----------|
| 1.0.0 | 2026-03-12 | Initial architecture document | Phase 1 completion |
