# Table Package Specification

## Overview

The `table` package provides a unified API for rendering styled tables in terminal applications using `charmbracelet/lipgloss`. It replaces disparate `text/tabwriter` implementations across CLI commands with a consistent, visually appealing table rendering system.

## Requirements

### Functional Requirements

**FR-1**: Table Creation
- Must support creating tables with column headers
- Must support adding rows dynamically
- Must support variable column widths (auto-calculated)

**FR-2**: Output Formats
- Must support styled terminal output (default)
- Must support Markdown table format
- Must support CSV format
- Must support custom writer (for testing/piping)

**FR-3**: Styling
- Must support default styled appearance (colors, borders)
- Must support minimal style (plain text, no colors)
- Must support custom styles via Style struct
- Must support alternating row colors

**FR-4**: Column Handling
- Must auto-calculate column widths based on content
- Must pad columns to align data
- Must handle empty cells gracefully
- Must handle mismatched row lengths (pad or truncate)

### Non-Functional Requirements

**NFR-1**: Performance
- Zero allocations in render path (where possible)
- Fast column width calculation
- Efficient string building

**NFR-2**: Usability
- Fluent API with method chaining
- Clear error messages
- Easy migration from tabwriter

**NFR-3**: Compatibility
- Terminal width detection (future)
- ANSI color support detection (future)
- Works on Linux, macOS, Windows

## API Specification

### Types

```go
// Style defines visual appearance
type Style struct {
    HeaderStyle lipgloss.Style
    RowStyle    lipgloss.Style
    AltRowStyle lipgloss.Style
    BorderStyle lipgloss.Style
}

// Table represents a formatted table
type Table struct {
    headers      []string
    rows         [][]string
    style        Style
    columnWidths []int
    writer       io.Writer
}
```

### Functions

**Constructor**:
```go
func New(headers []string) *Table
```
Creates a new table with specified headers. Uses DefaultStyle().

**Style Factories**:
```go
func DefaultStyle() Style
func MinimalStyle() Style
```
Return predefined styles (colored vs plain).

**Methods**:
```go
func (t *Table) WithStyle(style Style) *Table
func (t *Table) WithWriter(w io.Writer) *Table
func (t *Table) AddRow(columns ...string) *Table
func (t *Table) Render() string
func (t *Table) RenderMarkdown() string
func (t *Table) RenderCSV() string
func (t *Table) Print() error
```

All methods return `*Table` for chaining (except Render* which return strings, Print which returns error).

## Usage Examples

### Basic Usage
```go
tbl := table.New([]string{"Name", "Age", "City"}).
    AddRow("Alice", "30", "NYC").
    AddRow("Bob", "25", "SF")

tbl.Print()  // Styled output to stdout
```

### Multiple Formats
```go
tbl := table.New([]string{"Command", "Count"}).
    AddRow("engram dev", "150").
    AddRow("engram build", "45")

fmt.Print(tbl.Render())         // Styled terminal
fmt.Print(tbl.RenderMarkdown()) // Markdown table
fmt.Print(tbl.RenderCSV())      // CSV format
```

### Custom Styling
```go
tbl := table.New([]string{"Name", "Status"}).
    WithStyle(table.MinimalStyle()).
    AddRow("service-1", "running")

tbl.Print()  // Plain text, no colors
```

### Custom Writer
```go
var buf bytes.Buffer
tbl := table.New([]string{"Col1", "Col2"}).
    WithWriter(&buf).
    AddRow("A", "B")

tbl.Print()  // Output to buf, not stdout
```

## Migration from tabwriter

**Before** (text/tabwriter):
```go
w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
defer w.Flush()

fmt.Fprintln(w, "COMMAND\tCOUNT\tAVG TIME")
fmt.Fprintln(w, "-------\t-----\t--------")

for _, row := range rows {
    fmt.Fprintf(w, "%s\t%d\t%s\n", row.cmd, row.count, row.avg)
}
```

**After** (pkg/table):
```go
tbl := table.New([]string{"COMMAND", "COUNT", "AVG TIME"})

for _, row := range rows {
    tbl.AddRow(row.cmd, fmt.Sprintf("%d", row.count), row.avg)
}

tbl.Print()
```

**Benefits**:
- ~20 LOC reduction per table
- Automatic styling (colors, borders)
- Multiple output formats (markdown, CSV)
- No manual separator rows needed

## Design Decisions

### ADR-002: Table Formatting Enhancement
See `core/internal/ADR-002-table-formatting-lipgloss.md` for comprehensive rationale.

**Key decisions**:
1. Adopt lipgloss (not custom markdown builder)
2. Create unified `pkg/table` package
3. Migrate 4 CLI commands as proof-of-concept
4. Support 3 output formats (styled, markdown, CSV)

## Test Coverage

### Unit Tests (17 tests, 97.6% coverage)
- Basic construction and row addition
- Column padding and truncation
- Multiple output formats
- CSV escaping (commas, quotes, newlines)
- Custom writer support
- Style variants
- Edge cases (empty tables, single columns)
- Chained method calls

### Integration Tests
Covered by CLI command tests:
- `core/cmd/engram/cmd/analytics_usage_test.go`
- `core/cmd/engram/cmd/telemetry_test.go`
- `core/cmd/engram-benchmark/main_test.go`

## Dependencies

**Direct**:
- `github.com/charmbracelet/lipgloss v1.1.0` (styling)

**Transitive** (managed by go.mod):
- `github.com/lucasb-eyer/go-colorful`
- `github.com/muesli/termenv`
- `github.com/charmbracelet/x/ansi`

All dependencies from trusted source (Charmbracelet ecosystem, 8,900+ stars).

## Future Enhancements

**Not in scope for Phase 6** (documented for future work):

1. **Column alignment**: Left/center/right per column
2. **Cell wrapping**: Multi-line cells for long content
3. **Terminal width detection**: Auto-wrap to terminal width
4. **Sortable tables**: Interactive CLI with arrow keys
5. **Pagination**: For very large result sets
6. **Compact style**: Dense tables (single-line borders)
7. **Export formats**: JSON, YAML output

## Version History

- **v1.0.0** (2026-03-20): Initial release
  - Phase 6 Task 6.4: Table Formatting Enhancement
  - 3 output formats (styled, markdown, CSV)
  - 2 built-in styles (default, minimal)
  - 97.6% test coverage
  - 4 CLI commands migrated

## References

- ADR-002: Table Formatting Enhancement
- Lipgloss docs: https://github.com/charmbracelet/lipgloss
- Migrated commands: analytics_usage.go, telemetry.go, main.go (benchmark), list.go (invariants)
