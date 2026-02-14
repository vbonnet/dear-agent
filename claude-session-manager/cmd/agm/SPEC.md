# AGM CLI - Technical Specification

**Version:** 1.0
**Status:** Implemented
**Last Updated:** 2026-02-11

## Overview

The AGM (AI/Agent Gateway Manager) CLI is a command-line interface for managing multi-agent AI sessions with tmux integration. It provides unified session management across multiple AI providers (Claude, Gemini, GPT) through a consistent, user-friendly command structure.

## Purpose

Provide a production-ready CLI that:
- Enables creation, resumption, and lifecycle management of AI agent sessions
- Integrates seamlessly with tmux for persistent terminal sessions
- Supports multiple AI agents through a unified interface
- Provides robust session discovery and fuzzy matching
- Ensures session health through validation and diagnostics
- Maintains backward compatibility with Agent Session Manager (AGM)

## Requirements

### Functional Requirements

#### FR1: Session Lifecycle Management
- **ID:** FR1
- **Priority:** P0 (Critical)
- **Description:** CLI MUST support full session lifecycle operations
- **Commands:**
  - `agm new [session-name]` - Create new session with tmux integration
  - `agm resume [identifier]` - Resume existing session by UUID/name/fuzzy match
  - `agm session list` - List all non-archived sessions
  - `agm session archive [session-name]` - Mark session as archived
  - `agm session kill [session-name]` - Terminate active session
  - `agm session unarchive [session-name]` - Restore archived session
- **Validation:** All lifecycle commands update manifest and maintain session integrity

#### FR2: Smart Session Resolution
- **ID:** FR2
- **Priority:** P0 (Critical)
- **Description:** CLI MUST intelligently resolve session identifiers
- **Resolution Strategy:**
  1. Exact name match (highest priority)
  2. UUID prefix match (partial UUID accepted)
  3. Tmux session name match
  4. Fuzzy name matching (Levenshtein distance ≥ 0.6)
  5. Interactive picker (when no args provided)
- **Behavior:**
  - No args + sessions exist → Interactive TUI picker
  - No args + no sessions → Prompt to create new session
  - Name provided + exact match → Resume immediately
  - Name provided + fuzzy matches → "Did you mean" prompt
  - Name provided + no match → Offer to create new session

#### FR3: Multi-Agent Support
- **ID:** FR3
- **Priority:** P0 (Critical)
- **Description:** CLI MUST support multiple AI agent backends
- **Implementation:**
  - Agent selection via `--agent` flag (default: claude)
  - Supported agents: claude, gemini, gpt
  - Agent availability detection (API keys, CLI installation)
  - Agent-specific command translation
  - `agm agent list` - Show all available agents with capabilities
- **Error Handling:**
  - Warning if agent unavailable (missing API key/CLI)
  - Graceful degradation for unsupported agent features
  - Clear error messages with remediation steps

#### FR4: Tmux Integration
- **ID:** FR4
- **Priority:** P0 (Critical)
- **Description:** CLI MUST integrate with tmux for session persistence
- **Behavior:**
  - Create tmux session if not exists
  - Attach to existing tmux session if available
  - Send commands to tmux panes (cd, agent CLI invocation)
  - Detect running tmux sessions
  - Support detached mode (`--detached` flag)
- **Constraints:**
  - Cannot run `agm new` from within tmux (unless `--detached`)
  - Tmux session names must match AGM session names
  - Health checks validate tmux session state

#### FR5: Session Discovery and Search
- **ID:** FR5
- **Priority:** P1 (High)
- **Description:** CLI MUST provide robust session discovery
- **Commands:**
  - `agm search [query]` - Search sessions by name/project path
  - `agm session list --all` - Include archived sessions
  - `agm session list --json` - Machine-readable output
  - `agm admin get-uuid [identifier]` - Get session UUID
  - `agm admin get-session-name [identifier]` - Get session name
- **Search Features:**
  - Fuzzy matching with similarity threshold
  - Filter by lifecycle state (active/stopped/archived)
  - Filter by current directory (project-scoped sessions)
  - Status computation (active/stopped/archived based on tmux state)

#### FR6: Health Checks and Diagnostics
- **ID:** FR6
- **Priority:** P1 (High)
- **Description:** CLI MUST provide diagnostic capabilities
- **Commands:**
  - `agm admin doctor` - Quick structural health check
  - `agm admin doctor --validate` - Deep functional validation
  - `agm admin doctor --apply-fixes` - Auto-repair common issues
  - `agm admin fix-uuid [session-name]` - Fix UUID associations
- **Checks:**
  - Structural: Duplicate sessions, orphaned directories, invalid manifests
  - Functional: Session resumability, agent availability, tmux state
  - Auto-fixes: UUID conflicts, missing manifests, stale tmux sessions

#### FR7: Backup and Migration
- **ID:** FR7
- **Priority:** P2 (Medium)
- **Description:** CLI MUST support backup and migration operations
- **Commands:**
  - `agm backup [session-name]` - Create numbered backup
  - `agm backup list` - List all backups
  - `agm backup restore [session-name] [backup-id]` - Restore from backup
  - `agm migrate to-unified-storage` - Migrate legacy sessions
- **Behavior:**
  - Automatic backups before destructive operations (archive, UUID update)
  - Numbered backups with timestamps
  - Restore from specific backup number
  - Migration wizard for AGM → AGM transition

#### FR8: Workflow Automation
- **ID:** FR8
- **Priority:** P2 (Medium)
- **Description:** CLI MUST support predefined workflows
- **Commands:**
  - `agm workflow list` - List available workflows
  - `agm new --workflow [name]` - Create session with workflow
- **Workflows:**
  - `deep_research` - Gemini-based deep research workflow
  - Custom workflows via registration system
- **Features:**
  - Auto-detect agent for workflow
  - Validate workflow prerequisites
  - Inject initial prompt

#### FR9: Interactive UI Components
- **ID:** FR9
- **Priority:** P1 (High)
- **Description:** CLI MUST provide rich interactive experiences
- **Components:**
  - Session picker (Huh TUI library)
  - "Did you mean" prompt (fuzzy match selection)
  - Create confirmation prompt
  - Multi-step session creation form
  - Spinner for long-running operations
- **Accessibility:**
  - `--no-color` flag for WCAG AA compliance
  - `--screen-reader` flag for text-only symbols
  - Keyboard navigation (arrow keys, vim bindings)

#### FR10: Session Association
- **ID:** FR10
- **Priority:** P1 (High)
- **Description:** CLI MUST support manual and automatic session association
- **Commands:**
  - `agm session associate [session-name]` - Manually associate UUID
  - `agm session associate [session-name] --uuid [uuid]` - Specify UUID
  - Auto-detection on session creation/resume
- **Detection:**
  - Hybrid detection algorithm (timestamp + tmux correlation)
  - Confidence scoring (high/medium/low)
  - Auto-detect only in high-confidence scenarios
  - Fallback to manual association

### Non-Functional Requirements

#### NFR1: Performance
- **ID:** NFR1
- **Priority:** P1 (High)
- **Description:** CLI MUST meet performance targets
- **Targets:**
  - Command startup: < 100ms (cold start)
  - Session list (100 sessions): < 200ms
  - Session picker (100 sessions): < 500ms
  - Doctor structural check: 1-5 seconds
  - Doctor functional validation: 5-30 seconds per session
- **Optimization:**
  - Batch status computation for session lists
  - Cache tmux health checks (5-second TTL)
  - Lazy loading of session manifests
  - Concurrent health checks

#### NFR2: Reliability
- **ID:** NFR2
- **Priority:** P0 (Critical)
- **Description:** CLI MUST handle errors gracefully
- **Guarantees:**
  - No panics (all errors handled with `PrintError`)
  - Automatic backups before destructive operations
  - Lock-free design (fine-grained locks per resource)
  - Idempotent operations (safe to retry)
  - Clear error messages with actionable remediation
- **Error Presentation:**
  - User-friendly error messages
  - Context-specific remediation steps
  - Debug mode for detailed stack traces (`--debug` or `AGM_DEBUG=true`)

#### NFR3: Backward Compatibility
- **ID:** NFR3
- **Priority:** P0 (Critical)
- **Description:** CLI MUST maintain AGM compatibility
- **Guarantees:**
  - Read AGM manifest v2 format
  - Write AGM manifest v3 format
  - Auto-upgrade manifests on first write
  - `csm` command symlinked to `agm` (deprecated)
  - AGM sessions discoverable in AGM
- **Migration:**
  - Wizard for AGM → AGM migration
  - Preserve all session metadata
  - Maintain UUID associations

#### NFR4: Testability
- **ID:** NFR4
- **Priority:** P1 (High)
- **Description:** CLI MUST have comprehensive test coverage
- **Requirements:**
  - Unit tests: >80% code coverage
  - Integration tests: End-to-end command execution
  - BDD tests: User-facing scenarios (Gherkin/Cucumber)
  - Test mode: `--test` flag for isolated testing
  - Mock tmux client for unit tests
- **Test Infrastructure:**
  - `~/sessions-test/` directory for test mode
  - Injected dependencies (`ExecuteWithDeps`)
  - Deterministic test fixtures

#### NFR5: Usability
- **ID:** NFR5
- **Priority:** P1 (High)
- **Description:** CLI MUST be intuitive and self-documenting
- **Features:**
  - Comprehensive help text for all commands
  - Examples in command descriptions
  - Tab completion (bash/zsh)
  - Colorized output (with `--no-color` fallback)
  - Progress indicators for long operations
- **Documentation:**
  - Inline examples in `--help` output
  - Error messages include remediation steps
  - Validation errors show expected format

#### NFR6: Configuration Management
- **ID:** NFR6
- **Priority:** P2 (Medium)
- **Description:** CLI MUST support flexible configuration
- **Configuration Sources:**
  1. Command-line flags (highest priority)
  2. Environment variables (AGM_DEBUG, ANTHROPIC_API_KEY, etc.)
  3. Config file (`~/.config/agm/config.yaml`)
  4. Smart defaults (lowest priority)
- **Configurable Options:**
  - `--sessions-dir` - Sessions directory (default: `~/sessions`)
  - `--log-level` - Logging verbosity (debug/info/warn/error)
  - `--timeout` - Tmux command timeout
  - `--skip-health-check` - Disable health checks
  - `-C, --directory` - Working directory (like `git -C`)

## Command Structure

### Root Command
```
agm [session-name]
  Smart session picker/creator:
  - No args + sessions exist → Interactive picker
  - No args + no sessions → Create prompt
  - Name provided + match → Resume
  - Name provided + fuzzy matches → "Did you mean"
  - Name provided + no match → Create prompt
```

### Command Hierarchy

```
agm
├── [session-name]           # Smart resume/create (default command)
├── new [session-name]       # Create new session
├── resume [identifier]      # Resume existing session
├── session                  # Session lifecycle management
│   ├── new [session-name]
│   ├── resume [identifier]
│   ├── list [--all] [--json]
│   ├── archive [session-name]
│   ├── unarchive [session-name]
│   ├── kill [session-name]
│   └── associate [session-name]
├── agent                    # Agent management
│   └── list [--json]
├── search [query]           # Search sessions by name/project
├── workflow                 # Workflow automation
│   └── list
├── backup                   # Backup management
│   ├── [session-name]
│   ├── list
│   └── restore [session-name] [backup-id]
├── logs                     # Log management
│   ├── [session-name]
│   ├── clean [session-name]
│   ├── stats [session-name]
│   ├── thread [session-name] [thread-id]
│   └── query [session-name] [query]
├── send [session-name] [message]  # Send message to session
├── admin                    # Administrative commands
│   ├── doctor [--validate] [--apply-fixes]
│   ├── fix-uuid [session-name]
│   ├── get-uuid [identifier]
│   ├── get-session-name [identifier]
│   ├── clean                # Clean up stale sessions
│   ├── unlock               # Remove stale locks
│   └── test                 # Test infrastructure
├── migrate                  # Migration utilities
│   └── to-unified-storage
├── sync                     # Sync session metadata
└── version                  # Show version info
```

### Global Flags

```
-C, --directory <path>       Working directory (default: current)
--config <path>              Config file (default: ~/.config/agm/config.yaml)
--sessions-dir <path>        Sessions directory (default: ~/sessions)
--log-level <level>          Log level (debug/info/warn/error)
--debug                      Enable debug logging (shorthand for --log-level debug)
--timeout <duration>         Tmux command timeout
--skip-health-check          Skip health checks
--no-color                   Disable colored output
--screen-reader              Use text symbols (accessibility)
```

## Data Structures

### Command Context
```go
type CommandContext struct {
    Config          *config.Config
    SessionsDir     string
    ProjectDir      string      // From -C flag or cwd
    HealthChecker   *tmux.HealthChecker
    UIConfig        *ui.Config
}
```

### Session Creation Options
```go
type NewSessionOptions struct {
    SessionName   string
    AgentName     string      // claude/gemini/gpt
    WorkflowName  string      // Optional workflow
    ProjectID     string      // Optional project identifier
    Prompt        string      // Initial prompt
    PromptFile    string      // Initial prompt from file
    Detached      bool        // Create without attaching
}
```

## Command Execution Flow

### Smart Default Command (agm [session-name])

```
1. Parse args and flags
2. Load configuration (config file + flags)
3. Initialize UI config
4. Set project directory (from -C or cwd)
5. List all sessions
6. Filter to sessions matching current directory

IF no session name provided:
  IF no matching sessions:
    → Prompt to create new session
  IF one matching session:
    → Resume directly
  IF multiple matching sessions:
    → Show interactive picker

IF session name provided:
  → Try exact match (resume if found)
  → Try fuzzy matching (show "did you mean" prompt)
  → No match → Offer to create new session
```

### New Session Flow (agm new [session-name])

```
1. FAIL FAST: Cannot run from within tmux (unless --detached)
2. Determine session name:
   - From args
   - From interactive form
   - From current tmux session (if inside tmux)
3. Validate agent availability
   - Check API keys
   - Check CLI installation
   - Warn if unavailable (non-blocking)
4. Check for workflow
   - Auto-detect agent from workflow
   - Inject workflow prompt
5. Check session name uniqueness
6. Generate session ID (UUID)
7. Create manifest
8. Create/attach tmux session
9. Start agent CLI in tmux pane
10. Associate Claude UUID (if Claude agent)
11. Print success message
```

### Session Initialization Sequence (Automatic)

**Critical User Journey**: After `agm session new --agent=claude` creates a tmux session and starts Claude, the InitSequence automatically executes initialization commands without user intervention.

**Test Coverage**: See `test/bdd/features/session_initialization.feature`

**Flow**:
```
1. Claude CLI starts in tmux pane
2. Wait for Claude prompt (❯) using capture-pane polling
   - Poll interval: 500ms
   - Timeout: 30 seconds
   - Detection: containsClaudePromptPattern("❯")
3. Send /rename command:
   - Command: `/rename <session-name>`
   - Purpose: Set Claude session name to match tmux session
   - Wait for command completion
4. Send /agm:agm-assoc command:
   - Command: `/agm:agm-assoc`
   - Purpose: Associate session with AGM
   - Creates ready-file: ~/.agm/ready-<session-name>
5. Session initialization complete
   - User attached to session
   - Commands executed successfully
   - Session ready for interaction

ERROR HANDLING:
- Trust Prompt: If Claude shows "Do you want to allow access?" prompt:
  - Initialization WAITS for user input (captured by tmux pane)
  - User answers prompt manually
  - Initialization continues after answer
- Timeout (30s): If Claude never appears:
  - Warning displayed to user
  - Session remains attached (not killed)
  - User can manually run `/rename` and `/agm:agm-assoc`
- Network Interruption: Retries with exponential backoff

TECHNICAL IMPLEMENTATION:
- Uses capture-pane polling (not control mode)
- Proven approach from prompt_detector.go:WaitForClaudePrompt()
- See ADR-0001 for architectural decision rationale
```

**Expected Behavior** (from BDD scenarios):
- ✓ Successful initialization completes within 90 seconds
- ✓ Session renamed to match tmux session name
- ✓ Session associated with AGM (ready-file created)
- ✓ Timeout handled gracefully (session remains accessible)
- ✓ Trust prompts handled via user input (no auto-answering)
- ✓ Parallel session creation works without race conditions

**Reference**:
- BDD Tests: `test/bdd/features/session_initialization.feature`
- Implementation: `internal/tmux/init_sequence.go`
- Architecture Decision: `docs/adr/0001-init-sequence-capture-pane.md`

### Resume Session Flow (agm resume [identifier])

```
1. Resolve identifier:
   - Exact session name match
   - UUID prefix match
   - Tmux session name match
   - Fuzzy name match
   - Interactive picker (if no identifier)
2. Read manifest
3. Check lifecycle (error if archived)
4. Validate agent availability (warn if unavailable)
5. Check session health:
   - Worktree exists
   - Agent directories present
   - Tmux session state
6. Create/attach tmux session
7. Send cd command to worktree
8. Send agent resume command (with UUID/conversation_id)
9. Update manifest timestamp
10. Attach to tmux session
```

### Archive Session Flow (agm session archive [session-name])

```
1. Resolve session identifier
2. Read manifest
3. Check if already archived (error if yes)
4. Check if session is active in tmux (warn if yes)
5. Prompt for confirmation (unless --force)
6. Create automatic backup
7. Update manifest lifecycle to "archived"
8. Write manifest
9. Print success message
```

### Doctor Health Check Flow (agm admin doctor)

```
STRUCTURAL CHECKS (always performed):
1. Check Claude installation (history.jsonl exists)
2. Check tmux installation (tmux version)
3. List all manifests
4. Check for duplicate sessions (old vs new naming)
5. Check for UUID conflicts (multiple sessions same UUID)
6. Check for empty UUIDs
7. Check for orphaned directories
8. Check for invalid manifests

IF --validate flag:
  FUNCTIONAL CHECKS (per session):
  1. Attempt to resume session
  2. Classify resume errors
  3. Suggest fixes

  IF --apply-fixes flag:
    4. Auto-repair common issues:
       - Fix UUID conflicts
       - Remove orphaned directories
       - Repair invalid manifests
```

## Error Handling

### Error Categories

| Category | Handling | User Experience |
|----------|----------|-----------------|
| **Configuration Errors** | Print error + remediation steps | Exit code 1, clear instructions |
| **Session Not Found** | Fuzzy match suggestions or create prompt | Non-blocking, helpful suggestions |
| **Agent Unavailable** | Warning message + continue | Non-blocking, session still created |
| **Tmux Errors** | Detailed error + tmux diagnostics | Exit code 1, remediation steps |
| **Manifest Errors** | Backup + attempt repair | Auto-fix or manual intervention |
| **Lock Conflicts** | Retry or abort | Clear message about concurrent access |

### Error Presentation Format

```
❌ Error: Failed to resume session

  Session 'my-session' could not be resumed because Claude UUID is missing.

  To fix this:
    • Run: agm session associate my-session
    • Or manually add UUID to manifest: ~/sessions/session-abc123/manifest.yaml
    • Then try resuming again
```

## UI/UX Patterns

### Interactive Session Picker

```
┌─────────────────────────────────────────────────┐
│ Select a session to resume:                     │
│                                                  │
│ > my-project (active)     Updated: 2 mins ago  │
│   feature-auth (stopped)  Updated: 1 hour ago  │
│   bugfix-123 (active)     Updated: 5 hours ago │
│                                                  │
│ [↑/↓: Navigate | Enter: Select | q: Quit]       │
└─────────────────────────────────────────────────┘
```

### Fuzzy Match "Did You Mean" Prompt

```
Session 'my-proj' not found.

Did you mean one of these?
  1. my-project
  2. my-project-v2
  3. new-project
  4. Create new session "my-proj"

Choice [1-4]:
```

### Progress Spinner

```
⠋ Creating session 'my-project'...
⠙ Initializing tmux session...
⠹ Starting Claude CLI...
⠸ Associating session UUID...
✓ Session created successfully!
```

## Security Considerations

### API Key Handling
- Never log API keys
- Read from environment variables only
- Validate presence (not value) in availability checks
- No API keys stored in manifests or config files

### File System Security
- Manifests stored with 0600 permissions
- Session directories with 0700 permissions
- Backup files inherit source permissions
- No sensitive data in logs

### Tmux Socket Security
- Use default tmux socket permissions
- Validate tmux session ownership
- Prevent unauthorized session access

## Versioning

### Version Information

```go
var (
    Version   = "3.0.0"
    GitCommit = "abc1234"
    BuildDate = "2026-02-11"
)
```

Printed on every command execution:
```
agm 3.0.0 (/usr/local/bin/agm)
```

### Compatibility Matrix

| AGM Version | Manifest Version | AGM Compatible | Agents Supported |
|-------------|------------------|----------------|------------------|
| 3.0.0 | v3 (writes), v2 (reads) | Yes | claude, gemini, gpt |
| 2.x.x | v2 | Yes | claude only |
| 1.x.x | v1 | N/A | claude only |

## Dependencies

### External Libraries
- `github.com/spf13/cobra` - CLI framework
- `github.com/charmbracelet/huh` - Interactive TUI components
- `github.com/google/uuid` - UUID generation
- `gopkg.in/yaml.v3` - YAML parsing (manifests)

### Internal Packages
- `internal/agent` - Agent abstraction
- `internal/manifest` - Manifest schema
- `internal/tmux` - Tmux integration
- `internal/session` - Session management
- `internal/discovery` - Session discovery
- `internal/detection` - UUID detection
- `internal/ui` - UI components
- `internal/fuzzy` - Fuzzy matching
- `internal/config` - Configuration management

## Acceptance Criteria

### V1.0 Completion Checklist
- [x] All session lifecycle commands implemented
- [x] Smart session resolution with fuzzy matching
- [x] Multi-agent support (claude, gemini, gpt)
- [x] Tmux integration with detached mode
- [x] Interactive TUI components
- [x] Session discovery and search
- [x] Health checks and diagnostics
- [x] Backup and restore operations
- [x] Workflow automation support
- [x] Session association (manual + auto-detect)
- [x] Backward compatibility with AGM
- [x] Comprehensive error handling
- [x] Accessibility features
- [x] Tab completion
- [x] Configuration management
- [x] Test infrastructure

## References

- [AGM Architecture](ARCHITECTURE.md)
- [Agent Interface](../../internal/agent/interface.go)
- [Manifest Schema](../../internal/manifest/manifest.go)
- [Cobra CLI Framework](https://github.com/spf13/cobra)
- [Huh TUI Library](https://github.com/charmbracelet/huh)
