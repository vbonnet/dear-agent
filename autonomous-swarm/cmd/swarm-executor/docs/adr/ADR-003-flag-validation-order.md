# ADR-003: Flag Validation Order and Help Accessibility

## Status

**Accepted** - Implemented in swarm-executor v0.1.0

## Context

swarm-executor accepts multiple command-line flags, some required (--queue, --bead-id,
--session) and some optional (--version, --help). We need to determine the order of
operations for flag parsing, validation, and special flag handling.

### Requirements

1. **Help Accessibility**: Users should be able to get help even with invalid flags
2. **Version Accessibility**: Version output should not require valid configuration
3. **Early Failure**: Invalid flags should be detected before expensive operations
4. **Clear Error Messages**: Missing flags should show what's required
5. **Consistent Behavior**: Predictable ordering across all execution paths

### Constraints

- Go's flag package parses all flags before accessing any values
- Special flags (--help, --version) should take precedence over normal execution
- Required flag validation must happen before business logic
- Header output (version + path) should appear in most scenarios

## Decision

Implement a **staged flag handling order** with clear precedence rules:

```
Stage 1: Parse all flags
  └─► flag.Parse()

Stage 2: Handle --version (highest priority)
  └─► if *showVersion:
        Print version to stdout
        Exit(0)

Stage 3: Print header (skip if --version processed)
  └─► fmt.Fprintf(stderr, "swarm-executor %s (%s)\n", Version, executable)

Stage 4: Handle --help (second priority)
  └─► if *showHelp:
        Print usage to stderr
        Exit(0)

Stage 5: Validate required flags
  └─► if missing required flags:
        Print error + usage to stderr
        Exit(1)

Stage 6: Execute bead (normal flow)
  └─► exitCode := executeBead(...)
      Exit(exitCode)
```

### Implementation

```go
func main() {
    // Stage 1: Parse flags
    var (
        queuePath   = flag.String("queue", "", "Path to TASK-QUEUE.yaml file (required)")
        beadID      = flag.String("bead-id", "", "Bead ID to execute (required)")
        sessionName = flag.String("session", "", "CSM session name (required)")
        showVersion = flag.Bool("version", false, "Show version and exit")
        showHelp    = flag.Bool("help", false, "Show help and exit")
    )
    flag.Parse()

    // Stage 2: Handle --version (early exit, no header)
    if *showVersion {
        fmt.Printf("swarm-executor version %s\n", Version)
        os.Exit(0)
    }

    // Stage 3: Print header to stderr
    executable, err := os.Executable()
    if err != nil {
        executable = "unknown"
    }
    fmt.Fprintf(os.Stderr, "swarm-executor %s (%s)\n", Version, executable)

    // Stage 4: Handle --help (after header, before validation)
    if *showHelp {
        printUsage()
        os.Exit(0)
    }

    // Stage 5: Validate required flags
    if *queuePath == "" || *beadID == "" || *sessionName == "" {
        fmt.Fprintf(os.Stderr, "Error: Missing required flags\n\n")
        printUsage()
        os.Exit(1)
    }

    // Stage 6: Execute bead
    exitCode := executeBead(*queuePath, *beadID, *sessionName)
    os.Exit(exitCode)
}
```

## Consequences

### Positive

**Help Without Valid Flags**:
```bash
# User can get help even with no flags
$ swarm-executor --help
swarm-executor 0.1.0 (/usr/local/bin/swarm-executor)
swarm-executor - Autonomous bead execution harness
[... usage text ...]

# Or with invalid flags
$ swarm-executor --queue invalid.yaml --help
swarm-executor 0.1.0 (/usr/local/bin/swarm-executor)
swarm-executor - Autonomous bead execution harness
[... usage text ...]
```

**Version Without Dependencies**:
```bash
# Version works even without CSM installed
$ swarm-executor --version
swarm-executor version 0.1.0

# No header printed - clean output for parsing
```

**Clear Error Messages**:
```bash
# Missing flags show error + usage
$ swarm-executor --queue Q.yaml
swarm-executor 0.1.0 (/usr/local/bin/swarm-executor)
Error: Missing required flags

swarm-executor - Autonomous bead execution harness
[... usage text ...]
```

**Consistent Header Behavior**:
- Header printed in ALL scenarios except --version
- Users always see what version is running
- Helps with debugging (confirms correct binary)

**Predictable Precedence**:
- --version always wins (even with --help)
- --help always accessible (even with invalid flags)
- Validation only after special flags handled

### Negative

**Header Before --help**:
- Header clutters --help output slightly
- Mitigated: Header is one line, informative
- Trade-off: Consistency vs minimal output

**Flag Parsing Always Happens**:
- Invalid flag syntax fails before --help processed
- Example: `swarm-executor --invalid --help` → error
- Mitigated: Go's flag package behavior, common across all Go CLIs

**No Quiet Mode**:
- Header always printed (except --version)
- Cannot suppress header without code change
- Future: Add --quiet flag if needed

## Alternatives Considered

### Alternative 1: Validation Before Special Flags

**Approach**: Validate required flags, then check --help/--version

```go
// Validate first
if *queuePath == "" || *beadID == "" || *sessionName == "" {
    if !*showHelp && !*showVersion {
        fmt.Fprintf(os.Stderr, "Error: Missing required flags\n")
        os.Exit(1)
    }
}

// Then handle special flags
if *showVersion { ... }
if *showHelp { ... }
```

**Pros**:
- Catches missing flags early
- Simpler logic (fewer conditions)

**Cons**:
- Users cannot get help if flags invalid
- Poor UX: "Error: missing flags" without showing what's required
- **REJECTED**: Help should always be accessible

### Alternative 2: No Header for Special Flags

**Approach**: Skip header when --help or --version provided

```go
if !*showVersion && !*showHelp {
    fmt.Fprintf(os.Stderr, "swarm-executor %s (%s)\n", Version, executable)
}
```

**Pros**:
- Cleaner --help output
- Minimal --version output

**Cons**:
- Inconsistent behavior (header sometimes appears)
- Harder to debug (users don't see version with --help)
- **REJECTED**: Consistency more important than minimal output

### Alternative 3: Combined --help/--version Flag

**Approach**: Single flag for help text that includes version

```bash
$ swarm-executor --help
swarm-executor version 0.1.0
Build: abc123 (2024-01-01)

Usage:
  ...
```

**Pros**:
- One flag for all informational output
- Version always visible with help

**Cons**:
- Cannot get version alone (for parsing)
- Breaks Unix convention (--version is standard)
- **REJECTED**: Less flexible, breaks conventions

### Alternative 4: Deferred Validation

**Approach**: Validate flags at usage time, not at startup

```go
func executeBead(queuePath, beadID, sessionName string) int {
    if queuePath == "" {
        return fmt.Errorf("queue path required")
    }
    // ... continue execution ...
}
```

**Pros**:
- Simpler main() function
- Validation closer to usage

**Cons**:
- Late failure (after header, after component initialization)
- Harder to provide usage text on validation failure
- **REJECTED**: Fail-fast principle violated

## Implementation Notes

### Header Formatting

```go
// Header includes version and binary path
executable, err := os.Executable()
if err != nil {
    executable = "unknown"  // Fallback if path unavailable
}
fmt.Fprintf(os.Stderr, "swarm-executor %s (%s)\n", Version, executable)

// Example output:
// swarm-executor 0.1.0 (/usr/local/bin/swarm-executor)
```

**Rationale**:
- Version confirms correct binary version
- Path helps debug which binary is actually running (useful if multiple versions installed)
- stderr ensures doesn't interfere with stdout pipelines

### Version Output Format

```go
// Minimal output for parseability
fmt.Printf("swarm-executor version %s\n", Version)

// NOT: Includes build info, headers, decorations
// Reason: Scripts parse version output
```

**Parsing Example**:
```bash
# Extract version number
VERSION=$(swarm-executor --version | awk '{print $3}')
# VERSION="0.1.0"
```

### Usage Text Structure

```go
func printUsage() {
    fmt.Fprintf(os.Stderr, `swarm-executor - Autonomous bead execution harness

Usage:
  swarm-executor --queue <path> --bead-id <id> --session <name>
  swarm-executor --version
  swarm-executor --help

Flags:
  --queue <path>    Path to TASK-QUEUE.yaml file (required)
  --bead-id <id>    Bead ID to execute (required)
  --session <name>  CSM session name for execution (required)
  --version         Show version and exit
  --help            Show this help and exit

Exit Codes:
  0  Success - Bead executed successfully
  1  Error - Execution failed (see stderr for details)
  2  Escalation - Bead requires human intervention

Examples:
  # Execute a bead
  swarm-executor --queue ./TASK-QUEUE.yaml --bead-id bead-1 --session session-1

  # Show version
  swarm-executor --version
`)
}
```

### Testing Flag Validation Order

```go
func TestFlagValidationOrder(t *testing.T) {
    tests := []struct {
        name       string
        args       []string
        wantExit   int
        wantStdout string
        wantStderr string
    }{
        {
            name:       "version flag only",
            args:       []string{"--version"},
            wantExit:   0,
            wantStdout: "swarm-executor version",
            wantStderr: "",  // No header
        },
        {
            name:       "help flag only",
            args:       []string{"--help"},
            wantExit:   0,
            wantStdout: "",
            wantStderr: "swarm-executor 0.1.0",  // Header present
        },
        {
            name:       "help with invalid flags",
            args:       []string{"--queue", "missing.yaml", "--help"},
            wantExit:   0,
            wantStdout: "",
            wantStderr: "Usage:",  // Help shown, no error
        },
        {
            name:       "missing required flags",
            args:       []string{"--queue", "Q.yaml"},
            wantExit:   1,
            wantStdout: "",
            wantStderr: "Error: Missing required flags",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Run binary with args
            // Verify exit code, stdout, stderr
        })
    }
}
```

## Edge Cases

### Empty String vs Unset Flag

```go
// These are equivalent (both empty string)
$ swarm-executor --queue ""
$ swarm-executor --queue

// Validation catches both
if *queuePath == "" {
    // Error: missing required flag
}
```

### Multiple Calls to Same Flag

```go
// Go's flag package: last value wins
$ swarm-executor --queue A.yaml --queue B.yaml
// *queuePath == "B.yaml"
```

### Flag After Arguments

```go
// Works (flags parsed from anywhere in args)
$ swarm-executor --queue Q.yaml --bead-id B --session S
$ swarm-executor --bead-id B --queue Q.yaml --session S

// Both equivalent
```

### Unknown Flags

```go
// Go's flag package rejects unknown flags
$ swarm-executor --unknown-flag
flag provided but not defined: -unknown-flag
Usage of swarm-executor:
  ...
exit status 2
```

**Note**: Exit code 2 here is from flag package, not our escalation code. This is
acceptable - flag parsing errors are distinct from execution escalations.

## Future Enhancements

### Quiet Mode

```bash
# Suppress header output
$ swarm-executor --quiet --queue Q.yaml --bead-id B --session S
# (no header printed to stderr)
```

Implementation:
```go
var quietMode = flag.Bool("quiet", false, "Suppress header output")

if !*quietMode {
    fmt.Fprintf(os.Stderr, "swarm-executor %s (%s)\n", Version, executable)
}
```

### Verbose Mode

```bash
# Show debug information
$ swarm-executor --verbose --queue Q.yaml --bead-id B --session S
swarm-executor 0.1.0 (/usr/local/bin/swarm-executor)
[DEBUG] Initializing telemetry: /path/to/EXECUTION-LOG.jsonl
[DEBUG] Loading queue: /path/to/TASK-QUEUE.yaml
...
```

### Configuration File Support

```bash
# Load defaults from config
$ swarm-executor --config swarm.yaml --bead-id B --session S
# (queue path read from config file)
```

**Flag Precedence**:
1. Command-line flags (highest)
2. Environment variables
3. Config file
4. Built-in defaults (lowest)

## Related Decisions

- **ADR-001: Exit Code Design** - Exit codes used for special flags vs errors
- **ADR-002: Telemetry File Location** - Header shows version before telemetry init

## References

- [Go flag package documentation](https://pkg.go.dev/flag)
- [POSIX Utility Conventions](https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap12.html)
- [GNU Coding Standards: --help and --version](https://www.gnu.org/prep/standards/html_node/_002d_002dhelp.html)

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-02-11 | Initial decision record | Backfill Documentation |
