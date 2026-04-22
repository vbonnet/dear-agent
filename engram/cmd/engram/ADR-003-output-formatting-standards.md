# ADR-003: Output Formatting Standards

**Status**: Accepted

**Date**: 2024-01-22

**Context**: CLI output needs to be:
- Human-readable for interactive use
- Machine-parseable for automation
- Consistent across all commands
- Accessible (color optional, screen reader friendly)
- Informative without being verbose

Different users have different needs:
- Interactive users want colors, icons, progress indicators
- CI/CD systems want plain text or JSON
- Screen readers need semantic text without visual formatting

**Decision**: Implement standardized output formatting with:
1. Visual icons with semantic text fallbacks
2. Color support with `--no-color` flag
3. JSON output mode for machine consumption
4. Progress indicators for long operations
5. Quiet mode for silent success

**Output Functions**:
```go
PrintSuccess(msg string)  // ✓ [green] message
PrintError(msg string)    // ✗ [red] message
PrintWarning(msg string)  // ! [yellow] message
PrintInfo(msg string)     // ℹ [blue] message
```

**Output Modes**:
- **Default**: Colors + icons + full output
- **No Color**: Icons + plain text (via `--no-color`)
- **Quiet**: Only errors/warnings, silent on success (via `--quiet`)
- **JSON**: Structured JSON for automation (via `--json`)

**Rationale**:

1. **Accessibility**: Icons have text fallbacks, color is optional
2. **Automation**: JSON mode for scripts and CI/CD
3. **User Choice**: Users control verbosity and formatting
4. **Consistency**: All commands use same output functions
5. **Standards**: Follows UNIX conventions (quiet on success, verbose on error)

**Icons**:
- Success: ✓ (U+2713 CHECK MARK)
- Error: ✗ (U+2717 BALLOT X)
- Warning: ! (U+0021 EXCLAMATION MARK)
- Info: ℹ (U+2139 INFORMATION SOURCE)

**Colors** (via ANSI escape codes):
- Success: Green (32)
- Error: Red (31)
- Warning: Yellow (33)
- Info: Blue (34)

**Alternatives Considered**:

1. **No colors**: Less engaging for interactive use
2. **Always color**: Breaks automation, accessibility issues
3. **Emoji only**: Unicode support issues, accessibility concerns
4. **Log levels**: Too verbose for CLI output

**Consequences**:

**Positive**:
- Improved UX with visual feedback
- Automation-friendly with JSON mode
- Accessible with --no-color
- Consistent across all commands
- Easy to integrate in tests (check for icons)

**Negative**:
- Requires discipline to use consistently
- JSON schema must be maintained
- Color detection edge cases (some terminals)

**Implementation Guidelines**:

1. **Use output functions**: Never use fmt.Println for status messages
2. **Provide JSON mode**: For any command that outputs structured data
3. **Respect flags**: Check `--no-color`, `--quiet`, `--json` flags
4. **Progress for >1s**: Use progress indicator for operations >1 second
5. **Test output**: Tests should verify output format

**JSON Output Format**:
```json
{
  "query": "search term",
  "candidates": [...],
  "metadata": {
    "count": 10,
    "used_api": true
  }
}
```

**Examples**:

```go
// Interactive output
cli.PrintSuccess("Indexed 42 engrams in 150ms")
cli.PrintWarning("Plugin 'foo' disabled in config")
cli.PrintError("Failed to connect to API")

// JSON output (when --json flag set)
json.NewEncoder(os.Stdout).Encode(result)

// Progress indicator
progress := cli.NewProgress("Indexing engrams...")
progress.Start()
// ... long operation ...
progress.Complete("Indexed 42 engrams")
```

**Related Decisions**:
- ADR-002: Structured Error Handling
- ADR-001: Cobra CLI Framework
