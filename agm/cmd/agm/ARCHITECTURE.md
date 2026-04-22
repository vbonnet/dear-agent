# AGM CLI - Architecture Documentation

**Version:** 1.0
**Last Updated:** 2026-02-11

---

## Table of Contents

- [Overview](#overview)
- [Design Principles](#design-principles)
- [System Architecture](#system-architecture)
- [Component Structure](#component-structure)
- [Command Flow Patterns](#command-flow-patterns)
- [Data Flow](#data-flow)
- [Integration Points](#integration-points)
- [Error Handling Architecture](#error-handling-architecture)
- [Testing Architecture](#testing-architecture)
- [Performance Considerations](#performance-considerations)

---

## Overview

### What is AGM CLI?

The AGM CLI (`cmd/agm/`) is the primary user interface for the AI/Agent Gateway Manager system. It provides a unified command-line experience for managing multi-agent AI sessions with tmux integration. The CLI serves as the orchestration layer that coordinates session lifecycle management, agent selection, tmux integration, and user interaction.

### Design Philosophy

1. **User-Centric Design** - Optimize for developer workflows and common use cases
2. **Smart Defaults** - Minimize required user input through intelligent detection
3. **Progressive Disclosure** - Simple commands for common tasks, advanced flags for power users
4. **Fail Fast, Recover Gracefully** - Validate early, provide clear error messages with remediation
5. **Backward Compatible** - Seamless migration from AGM to AGM
6. **Dependency Injection** - Testable design with injected dependencies

---

## Design Principles

### 1. Command Composition Over Monoliths

Each command is a self-contained Cobra command with minimal shared state. This enables:
- Independent testing per command
- Clear separation of concerns
- Easy addition of new commands
- Reduced risk of breaking changes

### 2. Smart Identifier Resolution

The CLI uses a multi-strategy resolution algorithm for session identifiers:

```
User Input → Resolution Strategies → Session Match
  "my-proj"
     ├─ Exact name match
     ├─ UUID prefix match
     ├─ Tmux session name match
     ├─ Fuzzy name match (Levenshtein ≥ 0.6)
     └─ Interactive picker (fallback)
```

This design principle eliminates user frustration from typing exact names while maintaining determinism.

### 3. Layered Error Handling

Errors are handled at three layers:
1. **Validation Layer** - Early validation before state changes
2. **Execution Layer** - Safe execution with automatic backups
3. **Presentation Layer** - User-friendly error messages with remediation steps

### 4. Configuration Cascade

Configuration follows a clear precedence hierarchy:
```
CLI Flags (highest)
  ↓
Environment Variables
  ↓
Config File (~/.config/agm/config.yaml)
  ↓
Smart Defaults (lowest)
```

This allows users to override defaults progressively without modifying files.

### 5. Dependency Injection for Testability

All external dependencies (tmux client, file system, config) are injected via `ExecuteWithDeps`:

```go
func ExecuteWithDeps(tmux session.TmuxInterface) error
```

This enables:
- Unit testing with mocks (no real tmux required)
- Integration testing with real tmux
- Behavioral testing in isolation

---

## System Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        AGM CLI (cmd/agm/)                        │
│                                                                   │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────────┐ │
│  │   Cobra     │  │    Flags     │  │   Interactive UI        │ │
│  │  Commands   │  │  Processing  │  │  (Huh TUI Library)      │ │
│  └──────┬──────┘  └──────┬───────┘  └───────────┬─────────────┘ │
│         │                 │                      │               │
│         └─────────────────┴──────────────────────┘               │
│                           │                                      │
└───────────────────────────┼──────────────────────────────────────┘
                            │
        ┌───────────────────┴───────────────────┐
        │                                       │
   ┌────▼─────┐                          ┌─────▼──────┐
   │ Business │                          │   Error    │
   │  Logic   │                          │ Handling   │
   │ (internal)                          │  (ui pkg)  │
   └────┬─────┘                          └────────────┘
        │
   ┌────┴──────────────────────────────┐
   │                                    │
┌──▼───────┐  ┌───────────┐  ┌────────▼────┐
│ Session  │  │  Manifest │  │    Tmux     │
│ Manager  │  │  Storage  │  │ Integration │
└──────────┘  └───────────┘  └─────────────┘
```

### Component Layers

#### Layer 1: Command Layer (Cobra)
- **Responsibility**: Parse commands, validate flags, invoke business logic
- **Components**: `rootCmd`, `newCmd`, `resumeCmd`, `sessionCmd`, `agentCmd`, etc.
- **Pattern**: Each command is a self-contained `*cobra.Command`

#### Layer 2: Business Logic Layer (internal packages)
- **Responsibility**: Session lifecycle, agent routing, UUID detection
- **Components**: `internal/session`, `internal/agent`, `internal/detection`
- **Pattern**: Service objects with clear interfaces

#### Layer 3: Integration Layer (tmux, manifest)
- **Responsibility**: External system integration
- **Components**: `internal/tmux`, `internal/manifest`, `internal/config`
- **Pattern**: Adapter pattern for external dependencies

#### Layer 4: Presentation Layer (ui)
- **Responsibility**: User feedback, error messages, interactive prompts
- **Components**: `internal/ui`, `internal/fuzzy`
- **Pattern**: Specialized UI components

---

## Component Structure

### File Organization

```
cmd/agm/
├── main.go                 # Entry point, rootCmd definition
├── version.go              # Version command
│
├── new.go                  # Create new session
├── resume.go               # Resume existing session
├── list.go                 # List sessions
├── search.go               # Search sessions
│
├── session.go              # Session subcommand group
├── archive.go              # Archive session
├── unarchive.go            # Unarchive session
├── kill.go                 # Terminate session
├── associate.go            # Associate session UUID
│
├── agent.go                # Agent subcommand group
├── workflow.go             # Workflow subcommand group
│
├── backup.go               # Backup operations
├── logs.go                 # Log management
├── send.go                 # Send message to session
│
├── admin.go                # Admin subcommand group
├── doctor.go               # Health diagnostics
├── fix-uuid.go             # UUID repair
├── get_uuid.go             # Get session UUID
├── get_session_name.go     # Get session name
├── clean.go                # Clean up stale sessions
├── unlock.go               # Remove stale locks
│
├── migrate.go              # Migration utilities
├── sync.go                 # Sync session metadata
├── test.go                 # Test infrastructure
│
├── helpers.go              # Shared helper functions
└── select_option.go        # TUI option selection

Test Files:
├── archive_test.go         # Archive command tests
├── autoimport_test.go      # Auto-import tests
├── header_test.go          # Header display tests
└── new_integration_test.go # Integration tests
```

### Command Registration Pattern

All commands follow a consistent registration pattern:

```go
// 1. Define command variable
var exampleCmd = &cobra.Command{
    Use:   "example [args]",
    Short: "Brief description",
    Long:  `Detailed description with examples`,
    RunE:  runExample,  // Error-returning handler
}

// 2. Initialize flags in init()
func init() {
    exampleCmd.Flags().BoolVar(&flag, "flag-name", false, "description")
    parentCmd.AddCommand(exampleCmd)
}

// 3. Implement handler function
func runExample(cmd *cobra.Command, args []string) error {
    // Validation
    // Business logic
    // UI feedback
    return nil
}
```

### Shared State Management

Minimal global state is used, limited to:

```go
var (
    cfg               *config.Config      // Loaded in PersistentPreRunE
    globalHealthCheck *tmux.HealthChecker // Initialized if enabled
    tmuxClient        session.TmuxInterface // Injected dependency
)
```

All other state is passed explicitly through function parameters.

---

## Command Flow Patterns

### Pattern 1: Smart Default Command (agm [session-name])

```go
func runDefaultCommand(cmd *cobra.Command, args []string) error {
    projectDir := cli.GetProjectDirectory()
    manifests, _ := manifest.List(cfg.SessionsDir)

    // Filter to current directory
    matchingSessions := filterByProject(manifests, projectDir)

    if len(args) == 0 {
        return handleNoArgs(matchingSessions, projectDir, uiCfg)
    }

    sessionName := args[0]
    return handleNamedSession(sessionName, manifests, matchingSessions, projectDir, uiCfg)
}

func handleNoArgs(matchingSessions, projectDir, uiCfg) error {
    if len(matchingSessions) == 0 {
        // Prompt to create new session
    } else if len(matchingSessions) == 1 {
        // Resume directly
    } else {
        // Show interactive picker
    }
}

func handleNamedSession(name, allSessions, matchingSessions, projectDir, uiCfg) error {
    // Exact match → Resume
    // Fuzzy matches → "Did you mean"
    // No match → Offer to create
}
```

### Pattern 2: Session Lifecycle Commands

All session lifecycle commands follow this pattern:

```
1. Identifier Resolution
   ↓
2. Manifest Loading
   ↓
3. Validation (lifecycle state, health checks)
   ↓
4. Confirmation (if destructive)
   ↓
5. Backup (if modifying manifest)
   ↓
6. Execute Operation
   ↓
7. Update Manifest
   ↓
8. UI Feedback
```

Example from `archive.go`:

```go
func archiveSession(cmd *cobra.Command, args []string) error {
    // 1. Resolve identifier
    sessionID, manifestPath, err := resolveSessionIdentifier(args[0])

    // 2. Load manifest
    m, err := manifest.Read(manifestPath)

    // 3. Validate state
    if m.Lifecycle == manifest.LifecycleArchived {
        return fmt.Errorf("session already archived")
    }

    // 4. Confirm (unless --force)
    if !forceArchive {
        confirmed, _ := ui.Confirm("Archive this session?")
        if !confirmed { return nil }
    }

    // 5. Backup
    backup.Create(manifestPath)

    // 6. Execute
    m.Lifecycle = manifest.LifecycleArchived

    // 7. Update manifest
    manifest.Write(manifestPath, m)

    // 8. UI feedback
    ui.PrintSuccess("Session archived successfully")
    return nil
}
```

### Pattern 3: Interactive Forms

Commands that require user input use the Huh TUI library:

```go
func showSessionCreationForm() (*SessionParams, error) {
    var params SessionParams

    form := huh.NewForm(
        huh.NewGroup(
            huh.NewInput().
                Title("Session Name").
                Value(&params.Name).
                Validate(validateSessionName),

            huh.NewSelect[string]().
                Title("Agent").
                Options(
                    huh.NewOption("Claude", "claude"),
                    huh.NewOption("Gemini", "gemini"),
                    huh.NewOption("GPT", "gpt"),
                ).
                Value(&params.Agent),
        ),
    )

    err := form.Run()
    return &params, err
}
```

### Pattern 4: Batch Operations

Commands that operate on multiple sessions use consistent batch patterns:

```go
func batchArchiveSessions(filter FilterCriteria) error {
    sessions := findSessions(filter)

    if dryRun {
        previewChanges(sessions)
        return nil
    }

    for _, session := range sessions {
        if err := archiveSession(session); err != nil {
            logError(session, err)
            continue  // Continue on error, don't abort
        }
        logSuccess(session)
    }

    printSummary(sessions)
    return nil
}
```

---

## Data Flow

### Session Creation Flow

```
User Input
  ↓
Parse Flags (agent, workflow, prompt, detached)
  ↓
Validate Environment
  ├─ Check if inside tmux (error unless --detached)
  ├─ Check agent availability (warn if unavailable)
  └─ Check session name uniqueness
  ↓
Generate Session Metadata
  ├─ session_id = UUID
  ├─ session_name = user input or prompt
  ├─ agent = flag or default
  └─ context.project = cwd or -C flag
  ↓
Create Manifest File
  ↓
Create/Attach Tmux Session
  ↓
Start Agent CLI in Tmux Pane
  ↓
Associate Agent UUID (auto-detect or manual)
  ↓
Update Manifest with UUID
  ↓
Print Success + Instructions
```

### Session Resume Flow

```
User Input (identifier)
  ↓
Resolve Identifier
  ├─ Exact name match?
  ├─ UUID prefix match?
  ├─ Tmux name match?
  ├─ Fuzzy name match?
  └─ Interactive picker
  ↓
Load Manifest
  ↓
Validate Session State
  ├─ Check lifecycle (error if archived)
  ├─ Check agent availability (warn if unavailable)
  └─ Health check (worktree exists, tmux state)
  ↓
Create/Attach Tmux Session
  ↓
Send 'cd <worktree>' to Tmux
  ↓
Send '<agent> --resume <uuid>' to Tmux
  ↓
Update Manifest Timestamp
  ↓
Attach to Tmux Session
```

### Configuration Loading Flow

```
Process Start
  ↓
PersistentPreRunE Hook
  ↓
Load Configuration
  ├─ Read config file (~/.config/agm/config.yaml)
  ├─ Apply environment variables
  └─ Apply CLI flags (highest priority)
  ↓
Initialize UI Config
  ├─ Load UI preferences
  ├─ Apply --no-color flag
  └─ Apply --screen-reader flag
  ↓
Set Global Timeout (if configured)
  ↓
Initialize Health Checker (if enabled)
  ↓
Resolve Working Directory
  ├─ From -C flag (if provided)
  └─ From current working directory
  ↓
Execute Command
```

---

## Integration Points

### Tmux Integration

The CLI integrates with tmux through the `internal/tmux` package:

```go
// Create or attach to tmux session
tmux.CreateOrAttach(sessionName)

// Send commands to tmux pane
tmux.SendKeys(sessionName, "cd /path/to/project")
tmux.SendKeys(sessionName, "claude --resume uuid")

// Check tmux session status
status := tmux.SessionExists(sessionName)

// Health check (cached)
healthy := globalHealthCheck.Check(sessionName)
```

**Lock Strategy:**
- Tmux operations use fine-grained locks via `tmux.AcquireTmuxLock()`
- No global command lock (multiple AGM commands can run concurrently)
- Locks prevent race conditions in tmux server state

### Manifest Storage Integration

The CLI reads/writes session manifests via `internal/manifest`:

```go
// List all manifests
manifests, err := manifest.List(sessionsDir)

// Read specific manifest
m, err := manifest.Read(manifestPath)

// Write manifest (atomic operation)
err := manifest.Write(manifestPath, m)

// Acquire manifest lock (for concurrent safety)
unlock, err := manifest.AcquireLock(manifestPath)
defer unlock()
```

**Versioning:**
- Reads both v2 (AGM) and v3 (AGM) manifests
- Always writes v3 format
- Automatic upgrade on first write

### Agent Integration

The CLI selects and starts agents via `internal/agent`:

```go
// Get agent by name
agent, err := agent.Get(agentName)

// Check availability (API keys, CLI installation)
available := agent.IsAvailable()

// Start agent CLI
err := agent.Start(ctx, sessionID, &agent.StartOptions{
    WorkingDir: projectDir,
    Resume:     true,
    UUID:       claudeUUID,
})

// Get command translator
translator := agent.GetTranslator()
err := translator.RenameSession(ctx, sessionID, newName)
```

### UI Integration

The CLI uses `internal/ui` for user interaction:

```go
// Error presentation
ui.PrintError(err, "Operation failed", "remediation steps")

// Success feedback
ui.PrintSuccess("Operation completed")

// Warning messages
ui.PrintWarning("Agent unavailable, session created without agent")

// Interactive picker
selected, err := ui.SessionPicker(sessions, uiCfg)

// Confirmation prompt
confirmed, err := ui.Confirm("Proceed with operation?")

// Fuzzy match prompt
choice, err := ui.DidYouMean(input, suggestions, uiCfg)

// Progress spinner
err := huh.NewSpinner().
    Title("Creating session...").
    Action(func() { /* work */ }).
    Run()
```

---

## Error Handling Architecture

### Error Classification

Errors are classified into categories for appropriate handling:

```go
type ErrorCategory int

const (
    ConfigError      ErrorCategory = iota  // Invalid configuration
    SessionNotFound                        // Session doesn't exist
    AgentUnavailable                      // Agent not configured
    TmuxError                             // Tmux operation failed
    ManifestError                         // Manifest read/write failed
    ValidationError                       // Input validation failed
)
```

### Error Presentation Strategy

All errors use the `ui.PrintError` function:

```go
func PrintError(err error, title string, remediation string) {
    fmt.Fprintf(os.Stderr, "❌ Error: %s\n\n", title)
    fmt.Fprintf(os.Stderr, "  %s\n\n", err.Error())
    if remediation != "" {
        fmt.Fprintf(os.Stderr, "  To fix this:\n%s\n", remediation)
    }
}
```

**Examples:**

```
❌ Error: Session not found

  No session found matching 'my-proj'

  To fix this:
    • Check available sessions: agm session list
    • Try fuzzy search: agm search my-proj
    • Create new session: agm new my-proj
```

### Validation Strategy

Input validation follows fail-fast principle:

```go
func validateSessionName(name string) error {
    if name == "" {
        return fmt.Errorf("session name cannot be empty")
    }
    if strings.Contains(name, "/") {
        return fmt.Errorf("session name cannot contain '/'")
    }
    if len(name) > 255 {
        return fmt.Errorf("session name too long (max 255 chars)")
    }
    return nil
}
```

Validation occurs before any state changes.

### Automatic Backup Strategy

Destructive operations create automatic backups:

```go
func updateManifest(manifestPath string, updater func(*Manifest) error) error {
    // 1. Read current manifest
    m, err := manifest.Read(manifestPath)
    if err != nil { return err }

    // 2. Create backup
    backupPath, err := backup.Create(manifestPath)
    if err != nil { return err }

    // 3. Apply changes
    if err := updater(m); err != nil {
        return err
    }

    // 4. Write updated manifest
    if err := manifest.Write(manifestPath, m); err != nil {
        // Restore from backup on write failure
        backup.Restore(backupPath, manifestPath)
        return err
    }

    return nil
}
```

---

## Testing Architecture

### Test Layers

```
┌─────────────────────────────────────────┐
│   BDD Tests (test/bdd/)                 │
│   User-facing scenarios (Gherkin)       │
└───────────────┬─────────────────────────┘
                │
┌───────────────▼─────────────────────────┐
│   Integration Tests (*_integration_test)│
│   End-to-end command execution          │
└───────────────┬─────────────────────────┘
                │
┌───────────────▼─────────────────────────┐
│   Unit Tests (*_test.go)                │
│   Component-level testing with mocks    │
└─────────────────────────────────────────┘
```

### Unit Test Pattern

```go
func TestArchiveSession(t *testing.T) {
    // Setup
    tmuxMock := &mockTmuxClient{}
    tempDir := t.TempDir()

    // Create test session
    m := &manifest.Manifest{
        Name: "test-session",
        Lifecycle: "",
    }
    manifestPath := filepath.Join(tempDir, "manifest.yaml")
    manifest.Write(manifestPath, m)

    // Execute
    err := archiveSessionInternal(manifestPath, true /* force */)

    // Assert
    assert.NoError(t, err)

    updated, _ := manifest.Read(manifestPath)
    assert.Equal(t, manifest.LifecycleArchived, updated.Lifecycle)
}
```

### Integration Test Pattern

```go
func TestNewSessionEndToEnd(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Setup real environment
    testSessionsDir := filepath.Join(os.TempDir(), "agm-test-sessions")
    defer os.RemoveAll(testSessionsDir)

    // Execute real command
    cmd := exec.Command("agm", "new", "test-session",
        "--sessions-dir", testSessionsDir,
        "--detached",
    )
    output, err := cmd.CombinedOutput()

    // Assert
    assert.NoError(t, err)
    assert.Contains(t, string(output), "Session created successfully")

    // Verify manifest created
    manifests, _ := manifest.List(testSessionsDir)
    assert.Len(t, manifests, 1)
}
```

### Test Mode Support

The CLI supports a `--test` mode for isolated testing:

```go
var testMode bool

func getSessionsDir() string {
    if testMode {
        return filepath.Join(os.UserHomeDir(), "sessions-test")
    }
    return cfg.SessionsDir
}
```

This prevents test commands from affecting production sessions.

### Dependency Injection for Testing

All external dependencies are injected via `ExecuteWithDeps`:

```go
// Production
func main() {
    if err := ExecuteWithDeps(session.NewRealTmux()); err != nil {
        os.Exit(1)
    }
}

// Testing
func TestCommand(t *testing.T) {
    mockTmux := &mockTmuxClient{}
    ExecuteWithDeps(mockTmux)
}
```

---

## Performance Considerations

### Batch Status Computation

When listing sessions, status is computed in batch for efficiency:

```go
// SLOW: O(n) tmux calls
for _, m := range manifests {
    status := session.ComputeStatus(m, tmuxClient)
}

// FAST: Single tmux call
statuses := session.ComputeStatusBatch(manifests, tmuxClient)
```

### Health Check Caching

Tmux health checks are cached with a 5-second TTL:

```go
type HealthChecker struct {
    cache     map[string]*CacheEntry
    mu        sync.RWMutex
    cacheTTL  time.Duration  // 5 seconds
}

func (hc *HealthChecker) Check(sessionName string) bool {
    // Check cache first
    if cached, ok := hc.getFromCache(sessionName); ok {
        return cached
    }

    // Perform actual health check
    healthy := tmux.SessionExists(sessionName)
    hc.setCache(sessionName, healthy)
    return healthy
}
```

### Lazy Manifest Loading

Manifests are loaded on-demand, not all at once:

```go
// List manifests (metadata only)
manifests, _ := manifest.List(sessionsDir)  // Fast: only reads filenames

// Load full manifest when needed
for _, m := range filteredManifests {
    fullManifest, _ := manifest.Read(m.Path)  // Lazy: only selected sessions
}
```

### Command Startup Optimization

- Configuration loaded once in `PersistentPreRunE`
- Health checker initialized once (if enabled)
- Tmux client connection reused across operations
- No unnecessary file I/O in hot paths

### Concurrent Operations

Multiple AGM commands can run concurrently:

```
Terminal 1: agm session list       (reads manifests)
Terminal 2: agm new my-session     (writes new manifest)
Terminal 3: agm resume other       (reads + updates manifest)
```

**Safety Guarantees:**
- Manifest writes are atomic (temp file + rename)
- Manifest locks prevent concurrent modifications to same file
- Tmux locks prevent concurrent tmux server state changes
- No global command lock (each command independent)

---

## Future Enhancements

### Planned Improvements

1. **Plugin System**
   - Load custom commands from `~/.config/agm/plugins/`
   - Agent plugins for custom AI providers
   - Workflow plugins for domain-specific automation

2. **Advanced Search**
   - Full-text search in session metadata
   - Semantic search (vector embeddings)
   - Tag-based organization

3. **Session Templates**
   - Predefined session configurations
   - Template marketplace/sharing
   - Project-specific templates

4. **Real-Time Monitoring**
   - Dashboard TUI for all active sessions
   - Resource usage tracking
   - Token consumption metrics

5. **Remote Session Support**
   - SSH integration for remote tmux sessions
   - Cloud-based session storage
   - Team session sharing

---

## References

- [AGM CLI Specification](SPEC.md)
- [ADR-001: CLI Command Structure](ADR-001-cli-command-structure.md)
- [ADR-002: Smart Identifier Resolution](ADR-002-smart-identifier-resolution.md)
- [ADR-003: Dependency Injection Pattern](ADR-003-dependency-injection.md)
- [Cobra CLI Framework](https://github.com/spf13/cobra)
- [Huh TUI Library](https://github.com/charmbracelet/huh)
- [AGM System Architecture](../../docs/ARCHITECTURE.md)
