# AGMUX Patterns Guide

## Overview

Standard UX patterns for AGMcommand output, ensuring consistency, accessibility, and actionability across all commands.

---

## Error Messages

### Format

```
[SYMBOL] [Problem]

[Cause]

Try:
[Solution with examples]
```

### Example

```
❌ session not found: my-session

Could not resolve identifier to a session

Try:
  • List sessions: agmlist --all
  • Check sessions directory: ~/sessions
  • Import orphaned sessions: agmsync
```

### API

**Use helper functions for common errors:**

```go
// Session-related errors
ui.PrintSessionNotFoundError(identifier, cfg.SessionsDir)
ui.PrintArchivedSessionError(sessionID)
ui.PrintActiveSessionError(sessionName, tmuxName)

// Manifest errors
ui.PrintManifestReadError(err, manifestPath)
ui.PrintManifestWriteError(err)

// Infrastructure errors
ui.PrintTmuxNotFoundError()
ui.PrintClaudeNotFoundError()

// For custom errors with solutions
ui.PrintError(err, cause, solution)
```

### Guidelines

1. **Always provide actionable solutions** - Never use empty string for solution parameter
2. **Include concrete commands** - Show exact commands user can run
3. **Provide context** - Explain what went wrong and why
4. **Multiple options** - Offer 2-3 different paths to resolution when possible

---

## Success Messages

### Format

```
✓ [Action completed] [optional detail]
```

### API

```go
// Simple success
ui.PrintSuccess("Session created")

// Success with additional detail
ui.PrintSuccessWithDetail("Session created", "UUID: abc123")

// Progress step (multi-step operations)
ui.PrintProgressStep(1, 3, "Creating tmux session")
ui.PrintProgressStep(2, 3, "Starting Claude")
ui.PrintProgressStep(3, 3, "Associating session")
```

### Guidelines

1. **Be concise** - State what was accomplished in 1-5 words
2. **Use present tense** - "Session created" not "Creating session"
3. **Include key details** - UUID, session name, file path when relevant

---

## Warning Messages

### Format

```
⚠ [Warning message]
```

### API

```go
ui.PrintWarning("Claude is taking longer than expected")
```

### Guidelines

1. **Use plain Unicode symbol** - `⚠` not emoji variant `⚠️`
2. **Non-fatal issues** - Warnings for recoverable situations
3. **Informational** - Can include hints but not required to have solutions

---

## Accessibility

### NO_COLOR Support

**Environment Variable (legacy):**
```bash
NO_COLOR=1 agmlist
```

**Flag (recommended):**
```bash
agmlist --no-color
agmdoctor --no-color
```

Disables all ANSI color codes for users who:
- Cannot distinguish colors
- Use terminals without color support
- Need plain text output for automation

### Screen Reader Support

**Environment Variable (legacy):**
```bash
AGM_SCREEN_READER=1 agmdoctor
```

**Flag (recommended):**
```bash
agmdoctor --screen-reader
agmlist --screen-reader
```

**Symbol Conversion:**
- `✓` → `[SUCCESS]`
- `❌` → `[ERROR]`
- `⚠` → `[WARNING]`
- `○` → `[INFO]`

### Implementation

```go
// Flags take precedence over environment variables
cfg := ui.GetGlobalConfig()
symbol := "✓"

// Check flag first
if cfg.UI.ScreenReader {
    symbol = ui.ScreenReaderText(symbol)
} else if os.Getenv("AGM_SCREEN_READER") != "" {
    // Also check env var for compatibility
    symbol = ui.ScreenReaderText(symbol)
}

fmt.Printf("%s %s\n", ui.Green(symbol), message)
```

---

## Spinner Messages

### Format

Use present continuous form for active operations:

```go
spinner.New().
    Title("Creating session...").
    Accessible(true).
    Action(func() {
        // ... operation
    }).
    Run()
```

### Guidelines

1. **Present continuous** - "Creating..." not "Create" or "Created"
2. **Set Accessible(true)** - Ensures screen reader compatibility
3. **Keep short** - 2-4 words maximum

---

## Interactive Prompts

### Confirmation

```go
var confirmed bool
err := huh.NewConfirm().
    Title("Archive this session?").
    Affirmative("Yes").
    Negative("No").
    Value(&confirmed).
    Run()
if err != nil {
    // Always provide solution for prompt failures
    ui.PrintError(err,
        "Failed to read confirmation prompt",
        "  • Use --force flag to skip confirmation: agmarchive session-name --force\n"+
            "  • Check terminal is interactive (TTY)")
    return err
}
```

### Input

```go
var sessionName string
err := huh.NewInput().
    Title("Enter session name:").
    Value(&sessionName).
    Validate(func(s string) error {
        if s == "" {
            return fmt.Errorf("session name cannot be empty")
        }
        return nil
    }).
    Run()
if err != nil {
    ui.PrintError(err,
        "Failed to read session name",
        "  • Provide name as argument: agmnew <session-name>\n"+
            "  • Check terminal is interactive (TTY)")
    return err
}
```

### Selection

```go
var choice string
err := huh.NewSelect[string]().
    Title("Choose an option:").
    Options(
        huh.NewOption("Option 1", "1"),
        huh.NewOption("Option 2", "2"),
    ).
    Value(&choice).
    Run()
// ... error handling with actionable solutions
```

---

## WCAG AA Compliance

AGMmeets WCAG AA accessibility standards through:

### 1. Color Independence
- ✅ All information conveyed by color is also available through text/symbols
- ✅ NO_COLOR environment variable supported
- ✅ `--no-color` flag for explicit color disabling

### 2. Screen Reader Support
- ✅ Unicode symbols converted to text labels
- ✅ AGM_SCREEN_READER environment variable supported
- ✅ `--screen-reader` flag for explicit screen reader mode

### 3. Terminal Compatibility
- ✅ Automatic TTY detection - colors disabled in non-TTY contexts
- ✅ Works in CI/CD environments (no color, no interactive prompts)

### 4. Error Clarity
- ✅ Error messages include problem, cause, and solution
- ✅ Actionable commands provided for all error states
- ✅ No ambiguous error messages

---

## Code Organization

### UI Layer Structure

```
internal/ui/
├── colors.go          # Color functions (Red, Green, Yellow, Blue, Bold)
├── config.go          # Global configuration (NoColor, ScreenReader flags)
├── table.go           # Print functions (PrintError, PrintSuccess, PrintWarning)
├── errors.go          # Common error helpers
└── accessibility_test.go  # Tests for accessibility features
```

### Global Config Pattern

```go
// main.go - Set config from flags
uiCfg := ui.LoadConfig()
if noColor {
    uiCfg.UI.NoColor = true
}
if screenReader {
    uiCfg.UI.ScreenReader = true
}
ui.SetGlobalConfig(uiCfg)

// Anywhere in codebase - Access config
cfg := ui.GetGlobalConfig()
if cfg.UI.NoColor {
    // Skip color codes
}
```

---

## Migration Checklist

When updating existing commands:

- [ ] Replace direct `fmt.Printf` for errors with `ui.PrintError`
- [ ] Ensure all `PrintError` calls have non-empty cause and solution
- [ ] Use helper functions for common errors (session not found, tmux not found, etc.)
- [ ] Replace emoji warning symbols `⚠️` with plain Unicode `⚠`
- [ ] Success messages use `ui.PrintSuccess()` or `ui.PrintSuccessWithDetail()`
- [ ] Spinner titles use present continuous form ("Creating..." not "Create")
- [ ] Interactive prompt errors provide actionable solutions
- [ ] Test with `--no-color` and `--screen-reader` flags

---

## Testing

### Manual Testing

```bash
# Test NO_COLOR support
agmlist --no-color
agmdoctor --no-color

# Test screen reader mode
agmdoctor --screen-reader
agmlist --screen-reader

# Test both together
agmdoctor --no-color --screen-reader

# Test backward compatibility
NO_COLOR=1 agmlist
AGM_SCREEN_READER=1 agmdoctor
```

### Automated Tests

See `internal/ui/accessibility_test.go` for examples:

```go
func TestNoColorFlag(t *testing.T) {
    cfg := ui.DefaultConfig()
    cfg.UI.NoColor = true
    ui.SetGlobalConfig(cfg)

    result := ui.Red("test")
    if result != "test" {
        t.Error("NoColor flag should disable color output")
    }
}
```

---

## Examples

### Good Error Handling

```go
// ✅ GOOD - Actionable solution
m, manifestPath, err := session.ResolveIdentifier(sessionName, sessionsDir)
if err != nil {
    ui.PrintSessionNotFoundError(sessionName, sessionsDir)
    return err
}

// ✅ GOOD - Custom error with detailed solution
if err := manifest.Write(manifestPath, m); err != nil {
    ui.PrintError(err,
        "Failed to write manifest",
        "  • Check disk space: df -h\n"+
            "  • Verify permissions: ls -la "+manifestPath+"\n"+
            "  • Check file is not locked: lsof "+manifestPath)
    return err
}
```

### Bad Error Handling

```go
// ❌ BAD - Empty solution
if err != nil {
    ui.PrintError(err, "Failed to read manifest", "")
    return err
}

// ❌ BAD - Direct fmt.Printf
if err != nil {
    fmt.Printf("Error: %v\n", err)
    return err
}

// ❌ BAD - Emoji warning symbol
fmt.Println("⚠️ Warning: Session may still be initializing")
```

---

**Last Updated:** 2026-01-12
**Status:** Production Ready
**WCAG AA Compliance:** ✅ Verified
