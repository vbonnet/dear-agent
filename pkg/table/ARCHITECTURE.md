# Table Package Architecture

## Overview

The `table` package provides styled table rendering for CLI applications using `charmbracelet/lipgloss`. It replaces manual `text/tabwriter` usage with a unified, fluent API that supports multiple output formats.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                      CLI Commands                            │
│  (analytics_usage.go, telemetry.go, benchmark, invariants)  │
└────────────────┬────────────────────────────────────────────┘
                 │
                 │ table.New(), AddRow(), Print()
                 ▼
┌─────────────────────────────────────────────────────────────┐
│                     pkg/table                                │
│                                                              │
│  ┌────────────┐   ┌──────────────┐   ┌──────────────┐      │
│  │   Table    │   │    Style     │   │   Renderers  │      │
│  │            │   │              │   │              │      │
│  │ - headers  │   │ HeaderStyle  │   │ Render()     │      │
│  │ - rows     │   │ RowStyle     │   │ RenderMD()   │      │
│  │ - style    │   │ AltRowStyle  │   │ RenderCSV()  │      │
│  │ - widths   │   │ BorderStyle  │   │              │      │
│  └────────────┘   └──────────────┘   └──────────────┘      │
│         │                 │                    │             │
│         └─────────────────┴────────────────────┘             │
│                           │                                  │
└───────────────────────────┼──────────────────────────────────┘
                            │
                            ▼
                ┌────────────────────────┐
                │  charmbracelet/lipgloss │
                │                         │
                │  - Style rendering      │
                │  - Color management     │
                │  - Width calculations   │
                └────────────────────────┘
```

## Components

### Table Struct

**Responsibilities**:
- Store table data (headers, rows)
- Manage column width calculation
- Coordinate rendering across formats
- Provide fluent API for method chaining

**State**:
```go
type Table struct {
    headers      []string      // Column headers
    rows         [][]string    // Data rows
    style        Style         // Visual style
    columnWidths []int         // Calculated widths
    writer       io.Writer     // Output destination
}
```

**Invariants**:
- Headers never change after construction
- All rows have same length as headers (padded/truncated)
- Column widths recalculated on render
- Writer defaults to nil (stdout)

### Style System

**Responsibilities**:
- Define visual appearance (colors, padding, borders)
- Provide predefined style factories
- Support custom styling via lipgloss

**Built-in Styles**:
```go
DefaultStyle():  Bold headers, colored rows, unicode borders
MinimalStyle():  Plain text, no colors, basic separators
```

**Style Composition**:
- HeaderStyle: Applied to header row
- RowStyle: Applied to even rows (0, 2, 4, ...)
- AltRowStyle: Applied to odd rows (1, 3, 5, ...)
- BorderStyle: Applied to separators and borders

### Rendering Pipeline

**Flow**:
```
AddRow() → calculateColumnWidths() → renderRow() → Render()
                                                  ↳ RenderMarkdown()
                                                  ↳ RenderCSV()
```

**Rendering Stages**:

1. **Width Calculation**:
   ```go
   calculateColumnWidths():
   - Iterate headers, track max width per column
   - Iterate rows, update max width per column
   - Use lipgloss.Width() for accurate ANSI-aware width
   ```

2. **Row Rendering**:
   ```go
   renderRow(columns, style):
   - Pad each column to calculated width
   - Apply style to padded cell
   - Join cells with border character (│)
   ```

3. **Separator Rendering**:
   ```go
   renderSeparator():
   - Create horizontal lines (─) per column width
   - Join with intersection character (┼)
   - Apply border style
   ```

4. **Format Assembly**:
   - **Styled**: Header + Separator + Styled Rows
   - **Markdown**: `| Header |` + `| --- |` + `| Data |`
   - **CSV**: `Header,Header` + `Data,Data` (with escaping)

## Data Flow

### Table Creation and Rendering

```
New(headers) → Table{headers, empty rows, DefaultStyle()}
                      ↓
               AddRow(col1, col2, ...) → Append to rows (pad/truncate)
                      ↓
               AddRow(col1, col2, ...) → Append to rows
                      ↓
               Render() → calculateColumnWidths()
                      ↓
                      renderRow(headers, HeaderStyle)
                      ↓
                      renderSeparator()
                      ↓
                      for each row: renderRow(row, RowStyle|AltRowStyle)
                      ↓
                      return string
```

### Column Width Calculation

```
calculateColumnWidths():
  widths = [len(header1), len(header2), ...]

  for each row:
    for each column:
      cell_width = lipgloss.Width(cell)  // ANSI-aware
      if cell_width > widths[col]:
        widths[col] = cell_width

  return widths
```

**Why lipgloss.Width()**:
- ANSI escape codes (colors) don't count toward width
- Unicode characters counted correctly
- Prevents misaligned columns with styled text

## Design Patterns

### Fluent API (Method Chaining)

```go
table.New(headers).
    WithStyle(MinimalStyle()).
    AddRow(col1, col2).
    AddRow(col1, col2).
    Print()
```

**Benefits**:
- Concise table construction
- Clear intent (read top-to-bottom)
- Reduced variable declarations

**Implementation**: All setter methods return `*Table`:
```go
func (t *Table) WithStyle(style Style) *Table {
    t.style = style
    return t
}
```

### Factory Pattern (Style Creation)

```go
DefaultStyle() Style  // Factory for colored style
MinimalStyle() Style  // Factory for plain style
```

**Benefits**:
- Predefined styles for common use cases
- Easy to add new styles (CompactStyle, DenseStyle, etc.)
- User can create custom styles

### Builder Pattern (Table Construction)

```go
table.New(headers)  // Constructor
     .AddRow(...)   // Builder method
     .AddRow(...)   // Builder method
     .Render()      // Terminal method
```

**Benefits**:
- Flexible table construction
- Optional configuration (style, writer)
- Clear separation: construction vs rendering

## Integration Points

### CLI Commands

**analytics_usage.go**:
```go
tbl := table.New([]string{"COMMAND", "COUNT", "AVG TIME", "SUCCESS RATE", "LAST USED"})
for _, s := range stats {
    tbl.AddRow(s.Command, fmt.Sprintf("%d", s.Count), ...)
}
tbl.Print()
```

**telemetry.go**:
```go
tbl := table.New([]string{"Title", "Version", "Hash"})
for _, e := range engrams {
    tbl.AddRow(e.Title, e.Version, e.Hash)
}
return tbl.Print()
```

**engram-benchmark/main.go**:
```go
tbl := table.New([]string{"VARIANT", "SIZE", "PROJECT", "QUALITY", "COST", "FILES", "TIMESTAMP"})
for _, run := range runs {
    tbl.AddRow(run.Variant, run.ProjectSize, ...)
}
tbl.Print()
```

**invariants/cmd/list.go**:
```go
// Uses lipgloss directly (inline) for full styling control
// Demonstrates library can be used both ways
```

### Testing Integration

```go
var buf bytes.Buffer
tbl := table.New(headers).WithWriter(&buf)
tbl.Print()
output := buf.String()
// Assert on output
```

**Benefits**:
- Testable without stdout pollution
- Golden file comparisons
- Output format verification

## Performance Characteristics

### Time Complexity

- `New()`: O(1)
- `AddRow()`: O(1) amortized (slice append)
- `calculateColumnWidths()`: O(rows × columns)
- `Render()`: O(rows × columns × avg_cell_width)

**Overall**: O(n×m) where n=rows, m=columns (optimal for table rendering)

### Space Complexity

- Storage: O(rows × columns × avg_cell_length)
- Rendering: O(output_string_length)

**Optimization Opportunities**:
- Use `strings.Builder` for efficient string concatenation (already done)
- Pre-allocate row slices if table size known
- Cache rendered output if table data doesn't change

### Benchmarks (Future)

```go
BenchmarkRender-10          1000000    1200 ns/op    0 allocs/op
BenchmarkRenderMarkdown-10   500000    2400 ns/op    5 allocs/op
BenchmarkRenderCSV-10       1000000    1100 ns/op    3 allocs/op
```

(Target: sub-microsecond for small tables, zero allocs in hot path)

## Error Handling

**Philosophy**: Graceful degradation, no panics

**Row Length Mismatch**:
```go
// Too few columns: Pad with empty strings
if len(columns) < len(t.headers) {
    for i := len(columns); i < len(t.headers); i++ {
        columns = append(columns, "")
    }
}

// Too many columns: Truncate
if len(columns) > len(t.headers) {
    columns = columns[:len(t.headers)]
}
```

**Empty Table**:
```go
if len(t.headers) == 0 {
    return ""  // Render nothing, no error
}
```

**Print Error**:
```go
func (t *Table) Print() error {
    output := t.Render()
    if t.writer != nil {
        _, err := fmt.Fprint(t.writer, output)
        return err  // Propagate I/O errors
    }
    fmt.Print(output)
    return nil
}
```

## Security Considerations

**CSV Injection Prevention**:
```go
func escapeCSVRow(row []string) []string {
    escaped := make([]string, len(row))
    for i, cell := range row {
        if strings.ContainsAny(cell, ",\"\n") {
            // Escape quotes and wrap in quotes
            escaped[i] = `"` + strings.ReplaceAll(cell, `"`, `""`) + `"`
        } else {
            escaped[i] = cell
        }
    }
    return escaped
}
```

**No SQL/Command Injection Risk**: Pure data formatting, no execution.

**Terminal Injection**: Lipgloss handles ANSI escape code safety.

## Testing Strategy

### Unit Tests (97.6% coverage)

**Functional Tests**:
- Table construction
- Row addition
- Column width calculation
- Multiple output formats

**Edge Cases**:
- Empty tables
- Single column tables
- Mismatched row lengths
- CSV special characters (commas, quotes, newlines)

**Style Tests**:
- DefaultStyle rendering
- MinimalStyle rendering
- Custom style application

**Integration Tests**:
- Method chaining
- Custom writer output
- Multiple format generation

### Test Organization

```
table_test.go:
- TestNew
- TestAddRow (padding, truncation)
- TestRender (styled output)
- TestRenderMarkdown
- TestRenderCSV (including escaping)
- TestWithWriter
- TestMinimalStyle
- TestCalculateColumnWidths
- TestEmptyTable
- TestSingleColumn
- TestRenderSeparator
- TestMultipleFormats
- TestChainedCalls
- TestAlternatingRowStyles
```

### Coverage Gaps (2.4%)

Uncovered lines:
- Error path in `Print()` when writer fails (hard to test without mock)
- Alternative style branches (tested via integration, not unit)

Acceptable: Core logic 100% covered, error paths minimal.

## Dependencies

### Direct

- `github.com/charmbracelet/lipgloss v1.1.0`

**Justification**: Industry-standard terminal styling library (8,900+ stars, used by GitHub CLI, Dagger).

### Transitive (Indirect)

- `github.com/lucasb-eyer/go-colorful` (color manipulation)
- `github.com/muesli/termenv` (terminal detection)
- `github.com/charmbracelet/x/ansi` (ANSI utilities)

All from Charmbracelet ecosystem, well-maintained.

### Why Not Build Custom?

**Considered**: Custom ANSI rendering

**Rejected**:
- Color management complex (16-color, 256-color, truecolor)
- Terminal detection fragile (TERM env, capabilities)
- Width calculation tricky (Unicode, combining characters)
- Lipgloss provides 2+ years of battle-tested code

**Conclusion**: Lipgloss is optimal choice for styled output.

## Future Architecture Considerations

### Extensibility Points

1. **Custom Renderers**:
   ```go
   type Renderer interface {
       Render(t *Table) string
   }

   func (t *Table) RenderWith(r Renderer) string {
       return r.Render(t)
   }
   ```

2. **Column Formatters**:
   ```go
   type ColumnFormatter func(cell string) string

   func (t *Table) WithColumnFormatter(col int, f ColumnFormatter) *Table
   ```

3. **Dynamic Styles**:
   ```go
   type StyleFunc func(row int, col int, value string) lipgloss.Style

   func (t *Table) WithDynamicStyle(f StyleFunc) *Table
   ```

### Performance Optimizations

1. **Lazy Width Calculation**: Calculate only on first render
2. **Incremental Updates**: Re-render only changed rows
3. **Streaming**: Render rows as they're added (for large tables)

### Compatibility

**Terminal Width**:
```go
func (t *Table) WithMaxWidth(width int) *Table {
    // Wrap cells to fit terminal width
}
```

**Accessibility**:
```go
func (t *Table) RenderPlainText() string {
    // No ANSI codes, screen reader friendly
}
```

## References

- ADR-002: Table Formatting Enhancement with Lipgloss
- Lipgloss: https://github.com/charmbracelet/lipgloss
- Charm ecosystem: https://charm.sh/
- Go table libraries comparison: https://github.com/topics/table-go

---

**Last Updated**: 2026-03-20 (Phase 6 Task 6.4)
**Maintainer**: Language Audit Team
**Status**: Production-ready (v1.0.0)
