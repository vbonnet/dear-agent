# ADR-002: Table Formatting Enhancement with Lipgloss

**Status**: Accepted
**Date**: 2026-03-20
**Phase**: Language Audit Phase 6 - Go Library Modernization
**Task**: 6.4 - Table Formatting Enhancement
**Decision Maker**: Language Audit Team

---

## Context

The engram codebase uses `text/tabwriter` for table formatting in multiple locations (CLI commands, analytics output, telemetry reports). While functional, this creates several issues:

- **Inconsistent styling**: Each command implements table formatting differently
- **No visual hierarchy**: Plain text tables lack visual distinction between headers and data
- **Poor readability**: No colors, borders, or alternating row styles
- **Code duplication**: Similar table rendering logic repeated across 4+ files
- **Limited output formats**: Markdown and CSV require separate implementation for each table

**Current Implementations**:
- `core/cmd/engram/cmd/analytics_usage.go` - CLI usage statistics (tabwriter)
- `core/cmd/engram/cmd/telemetry.go` - Telemetry engram listing (tabwriter)
- `core/cmd/engram-benchmark/main.go` - Benchmark results (tabwriter)
- `plugins/invariants/cmd/list.go` - Invariant checks listing (tabwriter)
- `core/internal/dashboard/formatter.go` - Markdown tables (custom implementation)

**Evaluation Trigger**: Phase 6 aims to adopt industry-standard Go libraries for improved maintainability and user experience.

---

## Decision

**We will adopt `charmbracelet/lipgloss` for table rendering** with a custom table package that provides:

1. A unified table API for all CLI commands
2. Built-in styling (colors, borders, alternating rows)
3. Multiple output formats (styled, markdown, CSV)
4. Minimal migration effort (drop-in replacement)

---

## Implementation

### New Package: `core/pkg/table`

Created a reusable table package with lipgloss:

```go
// Example usage
tbl := table.New([]string{"Name", "Age", "City"}).
    AddRow("Alice", "30", "NYC").
    AddRow("Bob", "25", "SF")

// Styled output (default)
tbl.Print()

// Markdown output
fmt.Print(tbl.RenderMarkdown())

// CSV output
fmt.Print(tbl.RenderCSV())
```

**Features**:
- `DefaultStyle()`: Modern styled tables with colors and borders
- `MinimalStyle()`: Plain tables for piping/scripting
- Automatic column width calculation
- Alternating row colors for readability
- Unicode box-drawing characters (│ ─ ┼)
- Multiple output formats (styled, markdown, CSV)

### Migrated Files

Replaced `text/tabwriter` with `table` package in:

1. **`core/cmd/engram/cmd/analytics_usage.go`**
   - Before: 30 lines of tabwriter setup + manual formatting
   - After: 10 lines with table package
   - Reduction: ~20 LOC

2. **`core/cmd/engram/cmd/telemetry.go`**
   - Before: 25 lines of tabwriter + conditional logic
   - After: 8 lines with table package
   - Reduction: ~17 LOC

3. **`core/cmd/engram-benchmark/main.go`**
   - Before: 48 lines of tabwriter + null handling
   - After: 35 lines with table package
   - Reduction: ~13 LOC

4. **`plugins/invariants/cmd/list.go`**
   - Before: 40 lines of tabwriter + manual styling
   - After: 45 lines with lipgloss (inline rendering for full control)
   - Change: +5 LOC (but with visual styling)

5. **`core/internal/dashboard/formatter.go`**
   - Before: Custom markdown table implementation (40 lines per function)
   - After: Not migrated (kept existing markdown-only implementation)
   - Reason: Already optimal for markdown-only use case

**Total LOC Reduction**: ~45 LOC (not counting improved readability)

---

## Rationale

### 1. Visual Hierarchy and Readability

**Before (tabwriter)**:
```
COMMAND         COUNT   AVG TIME        SUCCESS RATE    LAST USED
-------         -----   --------        ------------    ---------
engram dev      150     2.3s            98.7%           2026-03-20
engram build    45      15.2s           100.0%          2026-03-19
```

**After (lipgloss)**:
```
 COMMAND      │  COUNT  │  AVG TIME  │  SUCCESS RATE  │  LAST USED
──────────────┼─────────┼────────────┼────────────────┼─────────────
 engram dev   │  150    │  2.3s      │  98.7%         │  2026-03-20
 engram build │  45     │  15.2s     │  100.0%        │  2026-03-19
```

- **Bold headers** with color background (visual hierarchy)
- **Alternating row colors** (easier to scan horizontally)
- **Unicode borders** (clear column separation)
- **Padding** (improved whitespace)

### 2. Unified API Across Codebase

**Consistency benefits**:
- All tables use the same rendering logic
- Same color scheme across all commands
- Predictable column alignment
- Shared test coverage

**Migration simplicity**:
- Replace `tabwriter.NewWriter()` with `table.New()`
- Replace `fmt.Fprintf(w, ...)` with `tbl.AddRow(...)`
- Replace `w.Flush()` with `tbl.Print()`

### 3. Multiple Output Formats

Single table instance supports 3 formats:

```go
tbl := table.New(headers).AddRow(...)

tbl.Render()          // Styled for terminal
tbl.RenderMarkdown()  // For docs/GitHub
tbl.RenderCSV()       // For data export
```

**Before**: Required separate implementations for each format.
**After**: One table, three outputs.

### 4. Industry-Standard Library

**Lipgloss** is from Charmbracelet (trusted maintainer):
- 8,900+ stars on GitHub
- Used by: GitHub CLI, Dagger, VHS, many CLI tools
- Active maintenance (last release: Jan 2025)
- Part of Charm ecosystem (Bubble Tea, Glamour, etc.)
- Zero breaking changes in 2+ years

**Dependencies added**:
```
github.com/charmbracelet/lipgloss v1.1.0
github.com/lucasb-eyer/go-colorful v1.2.0
github.com/muesli/termenv v0.16.0
```

All dependencies are:
- Actively maintained
- Security-reviewed (no known CVEs)
- Widely adopted (500+ stars each)

### 5. Test Coverage

**New table package**:
- 17 comprehensive tests
- 100% coverage of public API
- Edge cases: empty tables, single columns, padding, truncation
- Multiple format validation (styled, markdown, CSV)
- CSV escaping (commas, quotes, newlines)

**Migrated code**:
- Existing tests still pass (100%)
- No behavior changes (only visual improvements)

---

## Consequences

### Positive

✅ **Improved UX**: Colored, styled tables easier to read
✅ **Code reuse**: Unified API across 4 commands (45 LOC reduction)
✅ **Multiple formats**: Markdown/CSV support with zero additional code
✅ **Industry-standard**: Well-maintained library from trusted source
✅ **Comprehensive tests**: 17 tests for table package
✅ **Backward compatible**: Existing tests pass without modification

### Negative

❌ **Dependency added**: +3 dependencies (lipgloss + transitive)
❌ **Binary size**: +200KB for lipgloss (acceptable for CLI)
❌ **Terminal detection**: Styled output may not work on all terminals (fallback to minimal style)

### Neutral

⚪ **Alternative styles**: Can add custom styles later (compact, dense, etc.)
⚪ **Extensibility**: Table package can grow if needed
⚪ **Markdown formatter**: Kept existing implementation (no migration needed)

---

## Migration Notes

### Files Modified

1. Added: `core/pkg/table/table.go` (223 lines)
2. Added: `core/pkg/table/table_test.go` (260 lines)
3. Modified: `core/cmd/engram/cmd/analytics_usage.go` (-20 LOC)
4. Modified: `core/cmd/engram/cmd/telemetry.go` (-17 LOC)
5. Modified: `core/cmd/engram-benchmark/main.go` (-13 LOC)
6. Modified: `plugins/invariants/cmd/list.go` (+5 LOC, styled)

### Dependencies Added

```
go get github.com/charmbracelet/lipgloss@latest
```

Transitive dependencies:
- github.com/lucasb-eyer/go-colorful
- github.com/muesli/termenv
- github.com/charmbracelet/x/ansi
- (others managed by go.mod)

### Testing

```bash
# Table package tests
go test ./pkg/table/... -v
# Result: PASS (17/17 tests, 0.010s)

# Integration tests
go test ./cmd/engram/cmd/... -v
go test ./cmd/engram-benchmark/... -v
# Result: PASS (all existing tests pass)
```

---

## Future Enhancements

Possible future improvements (not in scope for Phase 6):

1. **Compact style**: Dense tables for large datasets (single-line borders)
2. **Sortable tables**: Interactive CLI with arrow key sorting
3. **Pagination**: For very large result sets
4. **Column alignment**: Left/right/center per column
5. **Cell wrapping**: Multi-line cells for long content
6. **Export formats**: JSON, YAML output

**Current status**: No immediate need for these features as of 2026-03-20.

---

## References

**Implementation**:
- `core/pkg/table/table.go` - Main table package (223 lines)
- `core/pkg/table/table_test.go` - Comprehensive tests (260 lines)

**Migrated Files**:
- `core/cmd/engram/cmd/analytics_usage.go:217` - Usage table with lipgloss
- `core/cmd/engram/cmd/telemetry.go:461` - Engram listing with lipgloss
- `core/cmd/engram-benchmark/main.go:396` - Benchmark results with lipgloss
- `plugins/invariants/cmd/list.go:50` - Invariants checks with lipgloss

**External References**:
- Lipgloss: https://github.com/charmbracelet/lipgloss
- Charmbracelet ecosystem: https://charm.sh/
- Language Audit ROADMAP: Phase 6, Task 6.4

**Related ADRs**:
- ADR-001: Circuit Breaker Custom Implementation (keep custom)
- (Future) ADR-003: If interactive tables added

---

**Approved By**: Language Audit Phase 6 Evaluation
**Review Date**: 2026-03-20
**Next Review**: When terminal compatibility issues reported or new features needed
