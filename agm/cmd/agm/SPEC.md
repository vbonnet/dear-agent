# AGM CLI - Technical Specification

**Version:** 2.0
**Status:** Production (Phase 6 Complete - Dolt-Only Architecture, YAML Backend Removed)
**Last Updated:** 2026-03-18

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
- **List Output Format:**
  - **Terminal width-based layouts:**
    - Minimal (<80 cols): NAME(20), UUID(8), WORKSPACE(8), AGENT(6), ACTIVITY(10)
    - Compact (80-99 cols): NAME(24), UUID(8), WORKSPACE(9), AGENT(6), PROJECT(20), ACTIVITY(10)
    - Full (≥100 cols): NAME, UUID, WORKSPACE, AGENT, PROJECT, ACTIVITY (dynamic widths)
  - **UUID column:** Shows first segment of Claude UUID (before first "-"), or "-" if no UUID
  - **Column headers:** Displayed above each status group (ACTIVE, STOPPED, STALE, ARCHIVED)
  - **Column alignment:**
    - Most columns: Left-aligned
    - ACTIVITY column: Right-aligned to make "ago" line up vertically
  - **Sort order:**
    - ACTIVE sessions: Sort by attachment status (● attached first, ◐ detached second), then alphabetically by name
    - All other groups: Sort alphabetically by name
  - **Status symbols:**
    - ● (filled circle): Active session with attached clients
    - ◐ (half-filled circle): Active session with no attached clients
    - ○ (empty circle): Stopped session
    - ⊗ (circled X): Stale session
    - ◉ (double circle): Archived session

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

#### FR11: Message Sending and Queueing
- **ID:** FR11
- **Priority:** P0 (Critical)
- **Description:** CLI MUST support non-disruptive message sending to running sessions
- **Commands:**
  - `agm session send [session-name] --prompt "message"` - Send message (queued by default)
  - `agm session send [session-name] --prompt-file /path/to/file` - Send from file
  - `agm session send [session-name] --interrupt --prompt "urgent"` - Interrupt immediately
  - `agm session send [session-name] --sender [name] --prompt "msg"` - Specify sender
  - `agm session send [session-name] --reply-to [message-id] --prompt "reply"` - Thread messages
- **Behavior:**
  - **Default (non-interrupt):** Queue messages for later delivery when session becomes READY
  - **Interrupt mode:** Send immediately via tmux, interrupting ongoing work (legacy behavior)
  - **State-based routing:**
    - READY state → Queue message (non-disruptive)
    - THINKING state → Queue message (wait for READY)
    - PERMISSION_PROMPT state → Queue message (wait for READY)
    - COMPACTING state → Reject with error (never interrupt compaction)
    - OFFLINE state → Reject with error
  - **Daemon integration:** Queued messages delivered automatically when daemon running
  - **Daemon offline warning:** Clear feedback when messages queued without daemon
- **Message Metadata:**
  - Unique message ID: `{timestamp}-{sender}-{seq}`
  - Sender attribution (auto-detected from AGM session or --sender flag)
  - Reply-to threading for conversation context
  - Audit trail logged to `~/.agm/logs/messages/`
- **Safety Guarantees:**
  - Compaction protection: NEVER interrupt during COMPACTING state (critical safety requirement)
  - Rate limiting: 10 messages per minute per sender
  - Message size limit: 10KB for prompt files
  - Sender name validation: alphanumeric, dash, underscore only
- **User Feedback:**
  - Queue confirmation: "⏳ Queued to 'session' (session READY) [ID: ...]"
  - Daemon warning: "⚠️ Message queued but delivery daemon is NOT running!"
  - Interrupt confirmation: "✓ Sent to 'session' from 'sender' ... [via: tmux]"
  - Compaction error: "❌ Cannot send to 'session' (session COMPACTING)"
- **Test Coverage:**
  - Unit tests: Flag validation, message formatting, sender validation
  - Integration tests: Queue behavior, interrupt mode, compaction protection, daemon warnings
  - Backward compatibility: Existing scripts continue to work

#### FR12: Bulk Session Operations
- **ID:** FR12
- **Priority:** P1 (High)
- **Description:** CLI MUST support bulk operations on multiple sessions for post-reboot recovery
- **Commands:**
  - `agm sessions resume-all` - Resume all stopped sessions
  - `agm sessions resume-all --workspace-filter [name]` - Resume sessions in specific workspace
  - `agm sessions resume-all --include-archived` - Also resume archived sessions
  - `agm sessions resume-all --dry-run` - Preview without executing
  - `agm sessions resume-all --detached` - Resume without attaching (default: true)
  - `agm sessions resume-all --continue-on-error` - Keep going if sessions fail (default: true)
- **Behavior:**
  - **Sequential Resume:** Resume sessions one at a time with 500ms delays (avoid tmux overload)
  - **Status-Based Filtering:** Only resume "stopped" sessions (skip active, error on archived unless --include-archived)
  - **Progress Indicators:** Charmbracelet Bubbles spinner + progress bar for visual feedback
  - **Error Collection:** Continue through failures (unless --continue-on-error=false), report all errors at end
  - **Workspace Filtering:** Filter by manifest.Workspace field (case-sensitive exact match)
  - **Detached Mode:** Resume without attaching to sessions (allows bulk operations without interruption)
- **Use Cases:**
  - **Post-Reboot Recovery:** `agm sessions resume-all` restores all stopped sessions after machine restart
  - **Workspace Isolation:** `agm sessions resume-all --workspace-filter=alpha` resumes only alpha workspace sessions
  - **Preview Changes:** `agm sessions resume-all --dry-run` shows which sessions would be resumed
- **Integration:**
  - **Orchestrator Coordination:** Writes `.agm/resume-timestamp` file for orchestrator v2 detection (see ADR-010)
  - **Boot Automation:** Works with systemd service (`agm-resume-boot.service`) for automatic boot recovery
  - **Admin Commands:** Future `agm admin enable-auto-resume` and `disable-auto-resume` for opt-in boot automation
- **Performance:**
  - **Batch Status Computation:** Uses `session.ComputeStatusBatch()` for efficient stopped session detection
  - **Sequential Processing:** 500ms delays prevent tmux server overload
  - **Progress Feedback:** Real-time updates during long operations (20+ sessions)
- **Safety Guarantees:**
  - **No Archived by Default:** Archived sessions are skipped unless explicitly included with --include-archived
  - **Health Checks:** Each session validated before resume attempt (worktree exists, manifest valid)
  - **Error Isolation:** One failed session doesn't stop remaining resumes (unless --continue-on-error=false)
  - **Summary Reporting:** Clear success/failure counts with error details at completion
- **Error Handling:**
  - No stopped sessions → "No stopped sessions found. All sessions are active or archived."
  - Manifest read errors → Skip corrupted manifest with warning, continue with next
  - Resume failures → Collect errors, continue (default), show summary at end
  - Tmux unavailable → Clear error message with remediation steps
- **Test Coverage:**
  - Unit tests: `cmd/agm/resume_all_test.go` - filterNonArchived(), filterByWorkspace(), edge cases
  - Integration tests: `test/integration/lifecycle/resume_all_test.go` - basic functionality, workspace filtering, archived handling, error scenarios, large scale (50 sessions)
  - BDD scenarios: (future) End-to-end resume-all flows
- **References:**
  - Implementation: `cmd/agm/resume_all.go`
  - Architecture Decision: `docs/adr/ADR-010-orchestrator-resume-detection.md`
  - Systemd Service: `systemd/agm-resume-boot.service`

#### FR13: Send Command Reorganization
- **ID:** FR13
- **Priority:** P0 (Critical)
- **Description:** CLI MUST provide unified `agm send` command group for all communication operations
- **Commands:**
  - `agm send msg [recipient] --prompt "..."` - Send messages (replaces `agm session send`)
  - `agm send reject [session] --reason "..."` - Reject permission prompts (replaces `agm session reject`)
  - `agm send approve [session] --reason "..."` - Approve permission prompts (NEW)
- **Backward Compatibility:**
  - Old commands still work: `agm session send`, `agm session reject`
  - Gradual migration path for users
  - Deprecation warnings in help text (future)
- **Benefits:**
  - Logical grouping of communication commands
  - Improved command discoverability
  - Future extensibility for additional send operations

#### FR14: Multi-Recipient Support
- **ID:** FR14
- **Priority:** P1 (High)
- **Description:** CLI MUST support sending messages to multiple recipients simultaneously
- **Syntax:**
  - Positional: `agm send msg session1,session2,session3 --prompt "..."`
  - Explicit flag: `agm send msg --to session1,session2 --prompt "..."`
  - Glob patterns: `agm send msg "*research*" --prompt "..."`
  - Workspace filtering: `agm send msg --workspace oss --prompt "..."`
- **Features:**
  - **Parallel delivery:** Worker pool with max 5 concurrent deliveries
  - **Recipient resolution:** Comma-separated lists, glob pattern expansion, workspace filtering
  - **Result aggregation:** Color-coded success/failure report for each recipient
  - **Error isolation:** One recipient failure doesn't block others
  - **Rate limiting:** Per-sender (not per-recipient), 10 messages/minute
- **Performance:**
  - 2.5x faster than sequential delivery (measured with 5 recipients)
  - Worker pool prevents tmux server overload
  - Buffered channels for efficient job distribution
- **Flags:**
  - `--to <recipients>` - Explicit recipient list (alternative to positional)
  - `--workspace <name>` - Filter sessions by workspace
- **Use Cases:**
  - Broadcast notifications: "Deploy complete" to all backend sessions
  - Status checks: "Health check" to all test sessions
  - Coordinated workflows: "Pause work" to multiple sessions

#### FR15: Permission Approval Command
- **ID:** FR15
- **Priority:** P1 (High)
- **Description:** CLI MUST provide symmetric approve command to complement reject
- **Command:**
  - `agm send approve [session] --reason "..." --auto-continue`
- **Features:**
  - **Automated approval:** Navigates to "Yes" option (usually default)
  - **Optional reasoning:** Add approval reason as additional instructions
  - **Auto-continue:** Automatically continue after approval (bypasses "Continue" prompt)
  - **Prompt detection:** Handles 2-option and 3-option permission prompts
  - **Smart extraction:** Extracts "## Standard Prompt (Recommended)" from markdown files
- **Flags:**
  - `--reason <text>` - Approval reason (optional)
  - `--reason-file <path>` - File containing approval reason
  - `--auto-continue` - Automatically continue after approval
- **Workflow:**
  1. Detect prompt type (2-option or 3-option)
  2. Navigate to "Yes" option if needed
  3. If `--reason` provided: Tab → reason text → Enter
  4. If `--auto-continue`: Send additional Enter
  5. Otherwise: Send Enter to approve
- **Symmetry with Reject:**
  - Same flags: `--reason`, `--reason-file`
  - Same markdown extraction logic
  - Opposite navigation (Yes instead of No)

#### FR16: Session Output Capture
- **ID:** FR16
- **Priority:** P1 (High)
- **Description:** CLI MUST expose tmux capture-pane functionality as public commands
- **Command:**
  - `agm capture [session] [flags]`
- **Modes:**
  - **Visible content** (default): Capture visible pane content
  - **Full history** (`--history`): Capture full scrollback buffer
  - **Tail** (`--tail N`): Capture last N lines only
- **Output Formats:**
  - **Text** (default): Line-by-line output
  - **JSON** (`--json`): Structured output with metadata
  - **YAML** (`--yaml`): YAML format with metadata
- **Features:**
  - **Regex filtering** (`--filter`): Filter lines matching pattern
  - **Line limiting** (`--lines N`): Limit output to N lines
  - **Metadata** (JSON/YAML): Includes session name, timestamp, line count
- **Flags:**
  - `--lines <N>` - Limit output to N lines
  - `--history` - Capture full scrollback history
  - `--tail <N>` - Capture last N lines only
  - `--json` - Output in JSON format
  - `--yaml` - Output in YAML format
  - `--filter <regex>` - Filter lines matching regex
- **Use Cases:**
  - **Debugging:** Capture error output for analysis
  - **Monitoring:** Extract specific log patterns
  - **Automation:** Parse structured output in scripts
  - **Multi-session coordination:** Capture responses from multiple sessions

#### FR17: Claude Code Skills
- **ID:** FR17
- **Priority:** P2 (Medium)
- **Description:** CLI MUST provide Claude Code skills for workflow automation
- **Installation:**
  - `make install-skills` - Install to `~/.claude/skills/agm/`
- **Skills:**
  - `/agm:new [name] [--agent TYPE] [--project PATH]` - Smart session creation
  - `/agm:send <session> --prompt "..." [--capture-response]` - Message sending with capture
  - `/agm:status [session] [--all] [--watch]` - Health monitoring
  - `/agm:resume <session> [--fuzzy] [--last]` - Intelligent resume
- **Features:**
  - **Auto-generated names:** Format `agm-YYYYMMDD-HHMMSS` if not provided
  - **Response capture:** `--capture-response` flag captures and returns output
  - **Watch mode:** Continuous status monitoring (refreshes every 2s)
  - **Fuzzy matching:** Typo-tolerant session name resolution
  - **Last session:** Resume most recently active session
- **Implementation:**
  - Simple bash scripts wrapping AGM CLI commands
  - Easy to maintain and extend
  - Well-documented with examples in `skills/README.md`

#### FR18: Test Session Isolation and Management
- **ID:** FR18
- **Priority:** P1 (High)
- **Description:** CLI MUST provide robust test session isolation and cleanup mechanisms
- **Commands:**
  - `agm new --test [session-name]` - Create isolated test session in `~/sessions-test/`
  - `agm new [session-name] --allow-test-name` - Override test pattern detection
  - `agm admin cleanup-test-sessions` - Clean up orphaned test sessions
  - `agm admin cleanup-test-sessions --dry-run` - Preview cleanup without deletion
  - `agm admin cleanup-test-sessions --pattern "regex"` - Custom pattern matching
  - `agm admin cleanup-test-sessions --message-threshold N` - Set trivial session threshold
- **Test Isolation:**
  - **--test flag behavior:**
    - Creates session in `~/sessions-test/` (isolated from production `~/.claude/sessions/`)
    - Tmux session prefixed with `agm-test-` (e.g., `agm-test-my-experiment`)
    - Not tracked in AGM database (ephemeral, no manifest created)
    - Perfect for experiments, CI/CD tests, temporary work
  - **Production workspace protection:**
    - Interactive prompt when session name contains "test" (case-insensitive)
    - PreToolUse hook blocks `test-*` patterns without `--test` flag
    - Clear error messages with remediation steps
- **Cleanup Mechanism:**
  - **Interactive multi-select:** Review and select sessions for deletion
  - **Message count analysis:** Identify trivial sessions (default threshold: 5 messages)
  - **Automatic backup:** Tarball backup to `~/.agm/backups/sessions/` before deletion
  - **Safe deletion:** Archive manifest (lifecycle: archived) then remove directory
  - **Pattern matching:** Default `^test-`, customizable via `--pattern` flag
  - **Dry-run mode:** Preview operations without making changes
- **Prevention Layers:**
  - **Layer 1 (Reactive):** Cleanup command removes existing pollution
  - **Layer 2 (Educational):** Interactive prompt teaches users `--test` flag usage
  - **Layer 3 (Preventive):** PreToolUse hook (`pretool-test-session-guard`) blocks violations
- **Override Mechanism:**
  - **--allow-test-name flag:** Bypasses test pattern detection for legitimate use cases
  - **Use cases:** Testing infrastructure work, TDD sessions, test-related documentation
  - **Guidance:** Use sparingly - most test work should use `--test` flag
- **Test Pattern Detection:**
  - **Pattern:** Case-insensitive substring match for "test"
  - **Triggers:** test-foo, TEST-FOO, Test-Bar, my-testing, contest (conservative)
  - **Does not trigger:** my-test-feature (different prefix), latest (not standalone)
  - **Rationale:** Conservative approach catches more cases, override available
- **Error Handling:**
  - **Hook failures:** Graceful degradation (allow command, log error)
  - **Missing sessions:** Clear error with suggested alternatives
  - **Backup failures:** Abort cleanup, preserve session safety
- **Test Coverage:**
  - Unit tests: Message counting, backup creation, pattern detection
  - Integration tests: Full cleanup workflow, hook integration, edge cases
  - BDD tests: User-facing scenarios (see `test/bdd/features/test_session_isolation.feature`)
  - Hook tests: 18 sub-tests covering all patterns and edge cases
- **Documentation:**
  - **User guide:** `docs/TEST-SESSION-GUIDE.md` (comprehensive examples, comparison table)
  - **ADR:** `cmd/agm/ADR-007-test-session-isolation.md` (architectural decisions)
  - **Retrospective:** `RETROSPECTIVE-TEST-SESSION-CLEANUP.md` (implementation learnings)
  - **README:** Updated with test session quick start section
  - **CHANGELOG:** v2.4 release notes with feature details
- **Related:**
  - **ADR-006:** Test Isolation Enforcement (original PreToolUse hook rationale)
  - **ADR-012:** Test Infrastructure Dolt Migration (test isolation patterns)
  - **NFR4:** Testability requirements (original `--test` flag purpose)

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
  - `--sessions-dir` - Sessions directory (default: `~/.claude/sessions`)
  - `--log-level` - Logging verbosity (debug/info/warn/error)
  - `--timeout` - Tmux command timeout
  - `--skip-health-check` - Disable health checks
  - `-C, --directory` - Working directory (like `git -C`)

## Command Structure

### Root Command
```
agm
  Shows help and available subcommands.
  Note: The 'agm [session-name]' shortcut was removed.
  Use 'agm session resume <name>' to resume a session.
```

### Command Hierarchy

```
agm
├── (no default command)     # Use subcommands below
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
├── sessions                 # Bulk session operations
│   └── resume-all [--workspace-filter] [--include-archived] [--dry-run]
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
--sessions-dir <path>        Sessions directory (default: ~/.claude/sessions)
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

### Smart Default Command (REMOVED)

The `agm [session-name]` shortcut was removed. Use explicit subcommands instead:
- `agm session resume <name>` - Resume a session
- `agm session new <name>` - Create a new session
- `agm session list` - List sessions

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

**Test Coverage**: See `cmd/agm/new_init_sequence_test.go`

**Flow**:
```
1. Claude CLI starts in tmux pane
2. Wait for Claude prompt (❯) using capture-pane polling
   - Poll interval: 500ms
   - Timeout: 30 seconds
   - Detection: containsClaudePromptPattern("❯")
3. Send /rename command:
   - Command: `/rename <session-name>`
   - Purpose: Generate Claude UUID and set session name
   - Wait 5s for command completion
4. Send /agm:agm-assoc command:
   - Command: `/agm:agm-assoc <session-name>`
   - Purpose: Associate Claude UUID with AGM manifest
   - Triggers agm session associate which creates ready-file
5. Wait for ready-file signal:
   - File: ~/.agm/ready-<session-name>
   - Timeout: 60 seconds
   - Signals: agm associate binary completed
6. Wait for skill completion:
   - Detects Claude prompt return using capture-pane polling
   - Timeout: 10 seconds
   - Ensures: skill finished all output before returning control
7. Session initialization complete
   - User attached to session
   - Commands executed successfully
   - Session ready for interaction

ERROR HANDLING:
- Trust Prompt: Pre-authorized via additionalDirectories in settings.json
  - Directory added before Claude starts
  - No prompt appears (prevented, not answered)
- Timeout (30s): If Claude never appears:
  - Warning displayed to user
  - Session remains attached (not killed)
  - User can manually run `/rename` and `/agm:agm-assoc`
- Ready-file timeout (60s): If association fails:
  - Warning displayed
  - Session usable but UUID may be missing
  - User can run `agm sync` to fix

TECHNICAL IMPLEMENTATION:
- Uses capture-pane polling (not control mode) - See ADR-0001
- Both code paths (detached and in-tmux) use identical InitSequence
- Fixed bug (2026-02-17): startClaudeInCurrentTmux now uses InitSequence.Run()
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

**Storage Backend**: Dolt database (dual-write mode during migration)

```
1. Connect to Dolt storage adapter
2. Resolve session identifier (by ID, tmux name, or manifest name)
   - Uses adapter.ResolveIdentifier() - excludes archived sessions
3. Check if already archived (error if yes)
4. Check if session is active in tmux
   - Active sessions MUST use --async flag (error if omitted)
   - Stopped sessions do NOT use --async (error if included)
5. For stopped sessions: archive immediately (no confirmation prompt)
   For active sessions with --async: spawn background reaper process
6. Update session lifecycle to "archived" in Dolt
   - Uses adapter.UpdateSession() - single database write
7. Also update YAML manifest (backward compatibility during migration)
   - Dolt is source of truth; YAML failures are warnings only
8. Print success message with restore instructions
```

**Migration Status** (v1.3 - March 2026):
- ✅ Phase 1-2 Complete: Dual-write mode (YAML + Dolt)
- ✅ `agm session new` writes to both YAML and Dolt
- ✅ `agm session list` reads from Dolt only
- ✅ `agm session archive` reads/writes Dolt, writes YAML (backward compat)
- 🚧 Phase 3-6: Command layer migration, YAML removal (in progress)

**See**: `docs/YAML-TO-DOLT-MIGRATION-PHASES-1-2.md` for migration details

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
