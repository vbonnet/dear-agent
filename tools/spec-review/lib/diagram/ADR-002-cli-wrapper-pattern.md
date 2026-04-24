# ADR-002: CLI Wrapper Pattern (exec vs Native Libraries)

**Status:** Accepted
**Date:** 2026-03-12
**Deciders:** Claude Sonnet 4.5, User
**Phase:** Phase 1 (Foundation & Tool Integration)

---

## Context

We need to integrate 4 diagram tools into the Go library:
1. **D2** - Has official Go library (`oss.terrastruct.com/d2`)
2. **Structurizr** - Java-native (CLI available via structurizr.sh)
3. **Mermaid** - JavaScript-native (CLI available via mmdc)
4. **PlantUML** - Java-native (CLI via java -jar plantuml.jar)

We must decide: Use native library integration where available, or use CLI wrappers for all tools?

## Decision

We will use a **hybrid approach**:

1. **D2: Native library integration (Phase 2+)**
   - Import `oss.terrastruct.com/d2` Go package
   - Use Go API directly (no CLI execution)
   - Benefits: 10-100x faster, no subprocess overhead, type-safe API

2. **Structurizr, Mermaid, PlantUML: CLI wrappers**
   - Execute CLI tools via `os/exec.Command()`
   - Stream I/O via stdin/stdout
   - Benefits: No Java/Node.js dependencies in Go code, simpler

**Phase 1 (MVP): All tools use CLI wrappers** (including D2)
- Reason: Faster to implement, validates architecture
- D2 native integration deferred to Phase 2 (optimization)

## Rationale

### Why CLI Wrappers?

**1. Dependency isolation:**
```go
// No Go dependencies on Java/Node.js runtimes
// Just execute CLI as subprocess
cmd := exec.Command("mmdc", "-i", inputFile, "-o", outputFile)
```

**2. Consistent interface:**
- All renderers have same `Render()` signature
- Implementation details hidden (CLI vs native)
- Easy to swap (CLI → native without breaking API)

**3. Simpler Go modules:**
```go
// renderer/go.mod - Zero dependencies
module github.com/engram/plugins/spec-review-marketplace/lib/diagram/renderer
go 1.21
// No external dependencies needed for CLI wrappers
```

**4. Proven pattern:**
- Git commands via exec (standard practice)
- Docker CLI via exec (industry standard)
- No reinventing wheel (tools already have stable CLIs)

### Why Native D2 (Future)?

**Performance comparison:**
```
CLI wrapper:
- Process spawn: ~10-50ms
- Pipe I/O: ~5-10ms
- Total overhead: ~20-60ms per render

Native library:
- Function call: <1ms
- Direct memory access: No I/O overhead
- Total: ~1-5ms per render

For large projects (100+ diagrams): 2-6 seconds vs 100-600ms
```

**Type safety:**
```go
// CLI wrapper (stringly-typed)
cmd := exec.Command("d2", "--layout", layoutEngine, input, output)
// Errors only at runtime

// Native library (compile-time safe)
opts := d2.RenderOptions{
    Layout: d2.LayoutELK,  // Enum, checked at compile time
}
err := d2.Render(source, dest, &opts)
```

**API stability:**
- Official library has semantic versioning
- CLI flags can change between versions
- Library API more stable (breaking changes require major version)

## Consequences

### Positive

**Faster implementation (Phase 1):**
- CLI wrappers: ~2 hours per renderer
- Native integration: ~8 hours per renderer
- 75% time saved in MVP

**Lower coupling:**
- No runtime dependencies on Java/Node.js (just CLI tools)
- Go binaries remain portable
- Easier testing (mock subprocess, not library internals)

**Flexibility:**
- Can upgrade CLI tools independently
- Can switch to native libraries incrementally
- Users can use any compatible CLI version

### Negative

**Performance overhead:**
- Process spawn per render (~20-60ms)
- Not suitable for high-throughput rendering
- Mitigated: Most use cases <10 renders/session

**Error handling complexity:**
```go
// Must parse stderr for error messages
stderr := &bytes.Buffer{}
cmd.Stderr = stderr
if err := cmd.Run(); err != nil {
    return fmt.Errorf("d2 failed: %w (stderr: %s)", err, stderr.String())
}
```

**Testing requires CLI tools:**
- Can't run tests without d2, mmdc, structurizr.sh installed
- CI/CD must install all tools
- Mitigated: Tests skip if tool missing (`t.Skip("d2 not installed")`)

## Alternatives Considered

### Alternative 1: Native Libraries for All

**Approach:**
- D2: `oss.terrastruct.com/d2`
- Mermaid: Embed Node.js via go-mermaid wrapper
- Structurizr: JNI bindings to Java SDK
- PlantUML: JNI bindings

**Rejected because:**
- No mature Go libraries for Mermaid/Structurizr/PlantUML
- JNI bindings fragile (memory management, crashes)
- Embedding Node.js runtime complex (>100MB overhead)
- Not worth effort for Phase 1

### Alternative 2: All CLI Wrappers (No Native)

**Approach:** Never use native D2 library, stick with CLI

**Rejected because:**
- D2 native library exists and is official
- Performance matters for large projects (100+ diagrams)
- Type safety valuable (compile-time checks vs runtime)
- Easy migration path (CLI → native transparent to users)

### Alternative 3: gRPC/RPC Services

**Approach:** Run diagram tools as gRPC services, call via RPC

**Rejected because:**
- Massive complexity (service lifecycle, deployment)
- No benefit over CLI wrappers (same subprocess overhead)
- Harder to debug (network calls vs subprocess)
- Over-engineering for use case

## Implementation

### CLI Wrapper Template

```go
type D2Renderer struct{}

func (r *D2Renderer) Render(ctx context.Context, source io.Reader, dest io.Writer, opts *RenderOptions) error {
    // 1. Create command
    args := []string{"-", "-"}  // stdin → stdout
    if opts.Layout != "" {
        args = append(args, "--layout", string(opts.Layout))
    }
    cmd := exec.CommandContext(ctx, "d2", args...)

    // 2. Wire I/O
    cmd.Stdin = source
    cmd.Stdout = dest
    stderr := &bytes.Buffer{}
    cmd.Stderr = stderr

    // 3. Execute
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("d2 render failed: %w (stderr: %s)", err, stderr.String())
    }

    return nil
}
```

### Error Handling

**Exit codes:**
- 0: Success
- 1: Syntax error (invalid diagram)
- 2: Runtime error (layout engine failure)
- 127: Command not found (tool not installed)

**Parsing stderr:**
```go
// Example D2 error: "input.d2:5:10: invalid syntax"
if strings.Contains(stderr.String(), "invalid syntax") {
    return ErrInvalidSyntax
}
```

### Testing

**Integration tests:**
```go
func TestD2Renderer_Render(t *testing.T) {
    if !commandExists("d2") {
        t.Skip("d2 not installed")
    }

    renderer := &D2Renderer{}
    source := strings.NewReader("x -> y")
    dest := &bytes.Buffer{}

    err := renderer.Render(context.Background(), source, dest, &RenderOptions{
        Format: OutputFormatSVG,
        Layout: LayoutEngineELK,
    })

    if err != nil {
        t.Fatalf("render failed: %v", err)
    }

    if !strings.Contains(dest.String(), "<svg") {
        t.Error("output not SVG")
    }
}
```

## Migration Path (Phase 1 → Phase 2)

**Step 1:** Implement native D2 renderer
```go
type D2NativeRenderer struct{}

func (r *D2NativeRenderer) Render(...) error {
    // Use oss.terrastruct.com/d2 package
    return d2.Render(source, dest, opts)
}
```

**Step 2:** Benchmark (CLI vs native)
```
BenchmarkD2_CLI-8       50   20ms/op
BenchmarkD2_Native-8    10000   1ms/op
```

**Step 3:** Switch default (update registry)
```go
var renderers = map[Format]Renderer{
    FormatD2: &D2NativeRenderer{},  // Changed from D2Renderer
}
```

**Step 4:** Keep CLI wrapper (fallback)
- Use native by default
- CLI wrapper if native library unavailable
- Configurable via environment variable

## Validation

**Phase 1 metrics (CLI wrappers):**
- ✅ All 4 renderers implemented (D2, Mermaid, Structurizr, PlantUML)
- ✅ Tests pass (100% success rate)
- ✅ Render time: <5 seconds for typical diagrams
- ✅ Error handling robust (stderr parsing, exit codes)

**Phase 2 targets (D2 native):**
- Target: 10x performance improvement
- Target: Type-safe API (compile-time errors)
- Target: Zero subprocess overhead

## Related

- **ADR-001** - Polyglot Architecture
- **ADR-003** - Separate Go Modules
- **Decision D001** - Go for D2 Integration
- **ROADMAP.md** - Phase 2: Native D2 integration

---

**Change Log:**
- 2026-03-12: Initial ADR created (Phase 1 completion)
