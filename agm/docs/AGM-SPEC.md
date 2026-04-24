# AGM (AI/Agent Gateway Manager): Product Specification

**Status**: Living Document
**Created**: 2026-02-11
**Owner**: Foundation Engineering
**Current Release**: AGM v3.0
**Target Release**: AGM v3.1+

---

## Executive Summary

AGM (AI/Agent Gateway Manager) is a unified session management CLI for multiple AI agents (Claude, Gemini, GPT). It provides consistent session lifecycle management, command translation, and workflow automation across heterogeneous AI providers through a single, intuitive interface.

**Core Value Proposition**: One CLI to manage all your AI agent sessions, regardless of provider.

**Evolution**: AGM evolved from AGM (Agent Session Manager) to support multi-agent workflows, maintaining backward compatibility while adding extensibility for new AI providers.

---

## Table of Contents

- [Problem Statement](#problem-statement)
- [Goals & Non-Goals](#goals--non-goals)
- [User Personas](#user-personas)
- [Core Features](#core-features)
- [Technical Architecture](#technical-architecture)
- [User Experience](#user-experience)
- [Acceptance Criteria](#acceptance-criteria)
- [Success Metrics](#success-metrics)
- [Risks & Mitigations](#risks--mitigations)
- [Roadmap](#roadmap)
- [References](#references)

---

## Problem Statement

### Current State

AI agents from different providers (Claude, Gemini, GPT) have:
- Different CLI interfaces and command structures
- Inconsistent session persistence models
- No unified session management across providers
- Provider-specific environment configurations
- Fragmented conversation history
- Manual context switching between agents

### User Pain Points

1. **Context Switching Overhead**
   - Users must learn multiple CLIs and their quirks
   - No consistent way to resume sessions across providers
   - Lost context when switching between agents mid-task

2. **Session Management Complexity**
   - Manual tmux session creation and management
   - No persistent mapping between tmux and agent sessions
   - Difficult to track which sessions are active/archived
   - No centralized session inventory

3. **Environment Configuration Burden**
   - Each agent requires specific environment variables
   - Conflicts between provider settings (e.g., Vertex AI vs API key mode)
   - Cryptic error messages when misconfigured
   - Hours of debugging environment issues

4. **Workflow Fragmentation**
   - No way to automate multi-step agent workflows
   - Manual coordination of multi-agent tasks
   - No session-to-session communication primitives

### Why This Matters

**Productivity Impact**: Developers spend 2-4 hours/week on session management overhead
**Adoption Barrier**: Environment configuration prevents 30%+ of users from trying new agents
**Quality Impact**: Context loss from poor session management degrades agent effectiveness

**Market Opportunity**: As AI agents proliferate, need for unified management grows exponentially

---

## Goals & Non-Goals

### Goals (v3.0 - Shipped)

✅ **G1: Multi-Agent Session Management**
- Create, resume, archive, and delete sessions for multiple agents
- Unified CLI interface across Claude, Gemini, and GPT (future)
- Persistent session manifests with agent metadata

✅ **G2: Command Translation Layer**
- Abstract agent-specific commands (rename, set-directory, hooks)
- Graceful degradation when agent doesn't support feature
- Consistent user experience regardless of underlying agent

✅ **G3: Environment Validation**
- Detect misconfigurations before session creation
- Provide actionable error messages with fix guidance
- Generate configuration templates (.envrc, .bashrc)

✅ **G4: Backward Compatibility**
- AGM sessions migrate seamlessly to AGM
- Existing tmux workflows continue working
- Zero-downtime migration path

✅ **G5: Accessibility**
- WCAG AA compliance for output formatting
- Screen reader friendly (--screen-reader flag)
- High contrast mode (--no-color flag)

### Goals (v3.1 - Planned)

🔜 **G6: Workflow Automation**
- Deep research workflow (multi-query, source aggregation)
- Code review workflow (multi-file analysis)
- Architecture design workflow (iterative planning)

🔜 **G7: Unified Storage**
- Migrate to ~/sessions/ unified storage
- Per-session conversation history
- Backup and restore for session directories

🔜 **G8: Agent Routing**
- AGENTS.md configuration for automatic agent selection
- Keyword-based routing (research → Gemini, code → Claude)
- Project-level and global-level routing rules

### Non-Goals

❌ **NG1: Environment Variable Management**
- AGM validates but does not manage environment variables
- Delegates to direnv, mise, shell RC files
- Security: Does not store API keys in config files

❌ **NG2: Tool Installation**
- AGM does not install agent CLIs or dependencies
- Delegates to package managers (npm, pip, brew)
- Provides detection and guidance only

❌ **NG3: Real-time Collaboration**
- No multi-user session sharing (deferred to v4.0+)
- No cloud sync in v3.x (future consideration)

❌ **NG4: Agent Provider Management**
- Does not manage account creation or API key generation
- Users responsible for provisioning agent access
- AGM guides configuration, does not automate it

❌ **NG5: Custom Agent Integration**
- v3.x supports Claude, Gemini, GPT only
- Custom agent support deferred to v4.0+

---

## User Personas

### P1: Solo Developer (Primary)

**Background**: Individual contributor working on multiple projects

**Needs**:
- Quick session creation and resumption
- Context persistence across terminal sessions
- Easy switching between projects
- Minimal configuration overhead

**Pain Points**:
- Loses context when terminal crashes
- Forgets which sessions exist
- Tedious tmux session management

**AGM Value**:
- `agm new my-project` creates session instantly
- `agm list` shows all active sessions
- `agm resume my-project` restores context
- Automatic tmux integration

**Success Metric**: Time to create session < 10 seconds

---

### P2: Multi-Agent User (Secondary)

**Background**: Uses different agents for different tasks (Claude for code, Gemini for research)

**Needs**:
- Consistent CLI across agents
- Environment validation for each agent
- Agent selection guidance
- Multi-agent workflow support

**Pain Points**:
- Learning curve for each agent CLI
- Environment configuration errors
- No guidance on which agent to use
- Manual coordination between agents

**AGM Value**:
- Single CLI for all agents
- `agm doctor gemini` validates environment
- Agent comparison documentation
- Unified session management

**Success Metric**: Setup time for new agent < 5 minutes

---

### P3: Team Lead (Tertiary)

**Background**: Manages team's AI agent usage, provides guidance

**Needs**:
- Standardized agent configuration
- Team-wide best practices
- Troubleshooting support
- Usage observability

**Pain Points**:
- Each team member configures differently
- Support burden for environment issues
- No visibility into agent usage
- Inconsistent workflows

**AGM Value**:
- Standardized setup via `agm doctor`
- Configuration templates team can share
- Diagnostic tools (`agm doctor --validate`)
- Message logging for audit

**Success Metric**: Support requests < 1/week for environment issues

---

## Core Features

### F1: Session Lifecycle Management

**Description**: Create, resume, kill, archive sessions with unified interface

**Commands**:
```bash
agm new my-session                    # Create new session
agm new --harness gemini-cli research-task  # Create with specific agent
agm resume my-session                 # Resume existing session
agm list                              # List all sessions
agm list --status active              # Filter by status
agm archive my-session                # Archive session
agm unarchive my-session              # Restore from archive
agm kill my-session                   # Terminate session
```

**Key Behaviors**:
- **Smart defaults**: Auto-selects Claude Code if no `--harness` specified
- **Fuzzy matching**: `agm resume res` finds "research-task"
- **Status computation**: Active (tmux running), Stopped (tmux killed), Archived (explicit)
- **Interactive picker**: No args → shows picker of matching sessions

**Implementation**: See [Architecture](ARCHITECTURE.md#session-lifecycle)

---

### F2: Command Translation

**Description**: Abstract agent-specific commands into unified interface

**Supported Commands**:

| Command | Claude | Gemini | GPT |
|---------|--------|--------|-----|
| Rename session | `/rename` slash command | API call (UpdateConversationTitle) | Planned |
| Set directory | `/agm-assoc` slash command | Metadata update | Planned |
| Run hook | tmux send-keys | Limited support | Planned |

**Usage**:
```bash
# Rename works across all agents
agm session rename my-session new-name

# Set working directory
agm session set-directory my-session ~/projects/myapp

# Execute hooks (agent-specific)
agm session run-hook my-session pre-start
```

**Graceful Degradation**:
- If agent doesn't support command → Updates manifest only
- Warns user about limited support
- Provides fallback guidance

**Implementation**: See [Command Translation Design](COMMAND-TRANSLATION-DESIGN.md)

---

### F3: Environment Validation

**Description**: Validate agent environment before session creation

**Commands**:
```bash
agm doctor gemini                        # Validate Gemini environment
agm doctor gemini --fix                  # Interactive setup wizard
agm doctor gemini --generate-envrc       # Generate .envrc template
agm doctor gemini --generate-bashrc      # Generate .bashrc snippet
agm doctor --all                         # Validate all agents
agm doctor --validate                    # Non-interactive mode (CI/CD)
```

**Checks**:
- **Command availability**: Is agent CLI installed?
- **API keys**: Are required environment variables set?
- **Conflicts**: Do variables conflict (e.g., Vertex AI vs API key mode)?
- **Source analysis**: Where are variables set (precedence issues)?

**Error Messages**:
- **What's wrong**: Clear description of validation failure
- **Why it matters**: Explain impact (not just technical detail)
- **How to fix**: Actionable steps with examples
- **Template generation**: Ready-to-use configuration

**Implementation**: See [Environment Management Spec](agm-environment-management-spec.md)

---

### F4: Workflow Automation

**Description**: Pre-built multi-step workflows for common tasks (Experimental in v3.0)

**Workflows**:

**Deep Research** (Gemini):
```bash
agm workflow deep-research my-topic \
  --project-id my-project \
  --queries "query1" "query2" "query3"
```

**Behavior**:
1. Creates Gemini session
2. Sends multiple research queries
3. Aggregates responses
4. Generates summary
5. Archives artifacts

**Future Workflows** (v3.1):
- `agm workflow code-review --files src/**.go`
- `agm workflow architect --requirements requirements.md`
- `agm workflow debug --error-log errors.txt`

**Implementation**: See `internal/workflow/`

---

### F5: Message Sending & Logging

**Description**: Send messages to sessions, log for audit

**Commands**:
```bash
agm session send my-session "Analyze this code"
agm session send my-session --file prompt.txt
agm session send my-session --reject          # Reject last response
agm logs list                         # List message logs
agm logs show my-session              # Show session messages
agm logs clean --older-than 90        # Cleanup old logs
```

**Message Attribution**:
- All messages tagged with sender metadata
- Timestamp (ISO8601 UTC)
- Message ID (unique identifier)
- Reply-to chain (threading support)

**Log Format** (JSONL):
```json
{
  "message_id": "1738612345678-agm-send-001",
  "timestamp": "2026-02-11T10:30:00Z",
  "sender": "agm-send",
  "recipient": "my-session",
  "message": "Analyze this code",
  "reply_to": null
}
```

**Retention**: 90 days default (configurable)

**Implementation**: See `internal/messages/`

---

### F6: Agent Management

**Description**: List, compare, and select agents

**Commands**:
```bash
agm agent list                    # List all agents
agm agent list --available        # Filter to available agents
agm agent compare claude gemini   # Side-by-side comparison
agm agent info gemini             # Show agent details
```

**Output**:
```
Agent: gemini
Status: Available
CLI: ~/.npm-global/bin/gemini
API Key: Set (GEMINI_API_KEY)
Version: 0.2.1
Capabilities: API-based, Real-time search, Multimodal
```

**Agent Selection Guidance**:
- Use Claude for: Code generation, debugging, refactoring
- Use Gemini for: Research, analysis, multimodal tasks
- Use GPT for: Creative writing, brainstorming (future)

**Implementation**: See `internal/agent/`

---

### F7: Health Checks & Diagnostics

**Description**: Validate system health, detect issues, offer fixes

**Commands**:
```bash
agm doctor                           # General health check
agm doctor --validate                # All checks (exit 0 if healthy)
agm doctor --fix                     # Interactive fix wizard
agm admin fix-uuid my-session        # Fix UUID association
agm admin clean-orphans              # Remove orphaned sessions
agm admin validate-manifests         # Check manifest integrity
```

**Checks**:
- Tmux availability and version
- Agent CLI installations
- Environment variables
- Session health (manifest valid, tmux exists)
- UUID associations
- Lock file staleness
- Duplicate sessions

**Fixes**:
- UUID detection and association
- Orphaned session cleanup
- Manifest repair from backups
- Lock file cleanup

**Implementation**: See `cmd/agm/doctor.go`

---

### F8: Backup & Restore

**Description**: Automatic manifest backups, manual restore

**Behavior**:
- **Auto-backup**: Every manifest write creates numbered backup
- **Backup location**: `~/sessions/<session>/.backups/manifest.{1,2,3}`
- **Retention**: 3 backups per session (FIFO)
- **Rotation**: Oldest backup deleted when creating 4th

**Commands**:
```bash
agm backup list my-session              # List backups
agm backup restore my-session 1         # Restore from backup #1
agm backup create my-session            # Manual backup
agm backup clean my-session --keep 5    # Adjust retention
```

**Use Cases**:
- Recover from manifest corruption
- Rollback after failed migration
- Audit trail for session changes

**Implementation**: See `internal/backup/`

---

### F9: Git Auto-Commit

**Description**: Automatic git commits for manifest changes when sessions directory is in a git repository

**Behavior**:
- **Auto-detect**: Walks up directory tree to find git repository
- **Selective commit**: Only commits the modified manifest file
- **Non-invasive**: Works correctly with other staged/unstaged files in repo
- **Graceful**: No-op in non-git directories (no error)
- **Descriptive messages**: Format: `agm: <operation> session '<name>'`

**Supported Operations**:
- `create` - New session creation
- `archive` - Session archival
- `unarchive` - Session restoration
- `associate` - UUID association
- `resume` - Session resume (timestamp update)
- `sync` - Manifest sync from tmux
- `create-child` - Child session creation

**Example Commit History**:
```bash
$ git log --oneline
abc1234 agm: create session 'my-coding-session'
def5678 agm: associate session 'my-coding-session'
789abc0 agm: archive session 'old-project'
```

**Use Cases**:
- Track session lifecycle in version control
- Audit trail of session operations
- Collaborate on session configurations
- Rollback manifest changes via git

**Error Handling**:
- Git commit failures logged as warnings (don't fail operation)
- Manifest always written successfully before attempting commit
- Missing git binary handled gracefully

**Implementation**: See `internal/git/git.go`

**Test Coverage**: 8 unit tests covering all scenarios (git/non-git repos, staged files, etc.)

---

### F10: Migration & Compatibility

**Description**: Migrate AGM sessions to AGM, maintain compatibility

**Commands**:
```bash
agm migrate --validate                  # Check migration readiness
agm migrate --dry-run                   # Preview changes
agm migrate                             # Execute migration
agm migrate --rollback                  # Revert migration
```

**Migration Phases**:
1. **Validation**: Check AGM sessions exist, no conflicts
2. **Backup**: Create backups of all AGM manifests
3. **Migration**: Convert manifests to AGM format (add agent field)
4. **Verification**: Validate new manifests
5. **Cleanup**: Archive old manifests (optional)

**Backward Compatibility**:
- AGM reads v2 manifests (AGM format)
- AGM writes v3 manifests (AGM format)
- `csm` symlink points to `agm` binary
- All `csm` commands work unchanged

**Implementation**: See [Migration Guide](AGM-MIGRATION-GUIDE.md)

---

### F10: Accessibility Features

**Description**: WCAG AA compliant, screen reader friendly

**Flags**:
```bash
agm list --no-color              # Disable color output
agm list --screen-reader         # Text symbols instead of Unicode
agm new my-session --no-color --screen-reader
```

**Global Flags** (work on all commands):
- `--no-color`: Disables ANSI color codes (high contrast mode)
- `--screen-reader`: Replaces Unicode symbols with ASCII text
- `--output simple`: Plain text output (no tables/boxes)

**Features**:
- High contrast output (WCAG AA 4.5:1 ratio)
- Screen reader friendly table output
- Keyboard-only TUI navigation
- Clear error messages (no icons-only)

**Implementation**: See [Accessibility](ACCESSIBILITY.md)

---

## Technical Architecture

### High-Level Design

```
┌──────────────────────────────────────────────────────┐
│              AGM CLI (Cobra)                          │
│  Unified command interface for all agents             │
└─────────────┬────────────────────────────────────────┘
              │
    ┌─────────┼─────────┐
    │         │         │
    ▼         ▼         ▼
┌────────┬────────┬────────┐
│ Claude │ Gemini │  GPT   │ Agent Adapters
│Adapter │Adapter │Adapter │ (implement Agent interface)
└────┬───┴────┬───┴────┬───┘
     │        │        │
     └────────┴────────┘
              │
    ┌─────────┼─────────┐
    │         │         │
    ▼         ▼         ▼
┌────────┬────────┬────────┐
│ Tmux   │Manifest│Messages│ Core Services
│Manager │ Store  │ Logger │
└────────┴────────┴────────┘
```

### Component Layers

**Layer 1: CLI Layer** (`cmd/agm/`)
- Command parsing (Cobra)
- Flag validation
- User interaction (Huh TUI)
- Output formatting (table, JSON, simple)
- Error presentation

**Layer 2: Business Logic** (`internal/`)
- Session lifecycle management
- Agent routing and selection
- Command translation
- Environment validation
- Workflow orchestration

**Layer 3: Agent Abstraction** (`internal/agent/`)
- Agent interface definition
- Agent-specific adapters (Claude, Gemini, GPT)
- Command translator per agent
- Capability detection

**Layer 4: Integration** (`internal/tmux/`, `internal/manifest/`)
- Tmux control mode integration
- Manifest schema (v2, v3)
- Lock management (global, per-session)
- Message logging (JSONL)

### Data Flow: Session Creation

```
1. User: agm new --harness gemini-cli research-task
        │
2. CLI: Parse command, validate flags
        │
3. Environment Validator: Check GEMINI_API_KEY, conflicts
        │ (Fail fast if invalid)
        │
4. Session Manager: Create manifest
        │ - Generate UUID
        │ - Set agent="gemini"
        │ - Set lifecycle="active"
        │ - Write to ~/sessions/research-task/manifest.yaml
        │
5. Tmux Manager: Create tmux session
        │ - tmux new-session -d -s research-task
        │
6. Agent Adapter: Start Gemini CLI
        │ - Send initialization prompt (if configured)
        │
7. UUID Detector: Monitor history.jsonl
        │ - Detect Gemini conversation UUID
        │ - Update manifest with UUID
        │
8. CLI: Attach user to session (if not --detached)
        │ - tmux attach-session -t research-task
```

### Storage Architecture

```
~/sessions/                          # Unified storage (v3+)
├── my-session/
│   ├── manifest.yaml                # Session manifest
│   ├── .backups/                    # Auto-backups
│   │   ├── manifest.1
│   │   ├── manifest.2
│   │   └── manifest.3
│   └── .lock                        # Session lock file
│
~/.claude/                           # Claude-specific
├── history.jsonl                    # Global history
└── session-env/
    └── <uuid>/
        └── manifest.json            # Per-session cache
│
~/.config/agm/                       # User config
├── config.yaml                      # Global settings
└── agents/                          # Agent requirements
    ├── claude.yaml
    ├── gemini.yaml
    └── gpt.yaml
│
~/.agm/logs/messages/                # Message logs
    ├── 2026-02-01.jsonl
    ├── 2026-02-02.jsonl
    └── 2026-02-03.jsonl
```

### Manifest Schema (v3)

```yaml
version: "3.0"
session_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
tmux_session_name: "my-session"
agent: "gemini"  # Required: claude, gemini, gpt
lifecycle: "active"  # active, stopped, archived
context:
  project: "~/projects/myapp"
  tags: ["feature", "backend"]
  workflow: "deep-research"  # Optional workflow name
metadata:
  created_at: "2026-02-11T10:00:00Z"
  updated_at: "2026-02-11T14:30:00Z"
  created_by: "agm"
  version: "3.0.0"
agent_metadata:
  # Agent-specific fields (flexible schema)
  gemini:
    conversation_id: "xyz-789"
    model: "gemini-2.0-flash-thinking-exp"
  claude:
    uuid: "abc-123"
    version: "0.7.1"
```

**Key Changes from v2**:
- Added `agent` field (required)
- Added `context.workflow` (optional)
- Renamed `claude` to `agent_metadata.claude`
- Added `agent_metadata.gemini` section

**Migration**: AGM reads v2, writes v3 on first update

---

## User Experience

### UX Principles

1. **Fail Fast, Fail Clear**
   - Validate inputs before side effects
   - Clear error messages with fix guidance
   - Exit codes: 0 (success), 1 (error), 2 (user error), 3 (not found)

2. **Progressive Disclosure**
   - Simple defaults for common cases
   - Advanced flags for power users
   - Interactive prompts when context missing

3. **Consistent Patterns**
   - All commands follow `agm <noun> <verb> [args] [flags]`
   - Global flags work on all commands
   - Output format consistent (table, JSON, simple)

4. **Accessibility First**
   - High contrast output default
   - Screen reader mode available
   - Keyboard navigation in TUIs
   - Clear, descriptive error messages

5. **Smart Defaults**
   - Agent defaults to Claude (most common)
   - Interactive mode when not in CI
   - Auto-create vs explicit create based on context

### Common Workflows

**Workflow 1: Quick Start**
```bash
# First time setup
agm doctor --validate              # Check health
agm new my-project                 # Create session (prompts for details)

# Daily usage
agm resume my-project              # Resume session
# ... work in session ...
# Ctrl+B, D to detach
```

**Workflow 2: Multi-Agent Research**
```bash
# Use Gemini for research
agm new --harness gemini-cli research-task
# ... gather information ...
# Ctrl+B, D

# Use Claude for code
agm new --harness claude-code implement-feature
# ... write code based on research ...
```

**Workflow 3: Environment Setup (New Agent)**
```bash
# Try to create session
agm new --harness gemini-cli test
# ❌ Environment validation failed

# Fix environment
agm doctor gemini --fix            # Interactive wizard
# ... follow prompts ...
# ✅ Environment validated

# Retry session creation
agm new --harness gemini-cli test
# ✅ Session created
```

**Workflow 4: Session Management**
```bash
# List all sessions
agm list

# Filter by status
agm list --status active

# Archive old sessions
agm archive old-session

# Cleanup orphaned sessions
agm admin clean-orphans
```

---

## Acceptance Criteria

### AC1: Multi-Agent Session Creation

- [ ] `agm new --harness claude-code my-session` creates Claude session
- [ ] `agm new --harness gemini-cli my-session` creates Gemini session
- [ ] `agm new --harness codex-cli my-session` creates GPT session (future)
- [ ] `agm new my-session` defaults to Claude
- [ ] Session manifest includes agent field
- [ ] Tmux session created and active
- [ ] User attached to session (if not --detached)

### AC2: Command Translation

- [ ] `agm session rename` works for Claude (slash command)
- [ ] `agm session rename` works for Gemini (API call)
- [ ] `agm session set-directory` works for Claude
- [ ] `agm session set-directory` works for Gemini
- [ ] Graceful degradation if command not supported
- [ ] Manifest updated regardless of agent support

### AC3: Environment Validation

- [ ] `agm doctor gemini` detects missing GEMINI_API_KEY
- [ ] `agm doctor gemini` detects GOOGLE_GENAI_USE_VERTEXAI=true conflict
- [ ] `agm doctor gemini --generate-envrc` produces valid .envrc
- [ ] `agm doctor gemini --fix` interactively configures environment
- [ ] `agm new --harness gemini-cli` validates environment before creation
- [ ] Clear error messages with fix guidance

### AC4: Backward Compatibility

- [ ] AGM reads v2 manifests (AGM format)
- [ ] `csm` command symlinks to `agm`
- [ ] All `csm` commands work unchanged
- [ ] Migration wizard converts sessions correctly
- [ ] Rollback restores v2 manifests

### AC5: Workflow Automation

- [ ] `agm workflow deep-research` creates session
- [ ] Workflow sends multiple queries
- [ ] Workflow aggregates responses
- [ ] Workflow generates summary artifact
- [ ] Workflow archives on completion

### AC6: Message Logging

- [ ] `agm send` logs to JSONL with metadata
- [ ] Message ID is unique and grep-friendly
- [ ] Timestamp is ISO8601 UTC
- [ ] Log file rotates daily
- [ ] `agm logs clean --older-than 90` removes old logs

### AC7: Health Checks

- [ ] `agm doctor --validate` exits 0 if healthy
- [ ] `agm doctor --validate` exits 1 if issues found
- [ ] Detects tmux not installed
- [ ] Detects agent CLI not installed
- [ ] Detects missing API keys
- [ ] Offers fix suggestions

### AC8: Accessibility

- [ ] `--no-color` flag disables ANSI colors
- [ ] `--screen-reader` flag uses ASCII symbols
- [ ] TUI supports keyboard-only navigation
- [ ] Error messages are descriptive (no icons-only)
- [ ] Output meets WCAG AA contrast ratio (4.5:1)

---

## Success Metrics

### Leading Indicators (Measure First)

**User Efficiency**:
- **Time to create session**: Target < 10 seconds (baseline: 30 seconds manual tmux)
- **Time to setup new agent**: Target < 5 minutes (baseline: 2-4 hours debugging)
- **Session context loss**: Target < 5% sessions (baseline: 20% lost due to crashes)

**User Experience**:
- **Error message clarity**: >80% users understand error without external help (survey)
- **Environment setup success**: >90% users successfully configure new agent on first try
- **Feature discoverability**: >70% users discover key features within first week

### Lagging Indicators (Measure After Launch)

**Adoption**:
- **Active users**: Target 100+ weekly active users within 3 months
- **Multi-agent adoption**: >30% users use 2+ agents within 1 month
- **Session creation rate**: Target 500+ sessions/week across all users

**Quality**:
- **Support requests**: <5% users submit environment-related issues
- **Bug reports**: <10 critical bugs/month
- **Session corruption rate**: <1% sessions require manifest repair

**Engagement**:
- **Session resumption rate**: >60% sessions resumed at least once
- **Workflow usage**: >20% users try workflow automation
- **Command diversity**: >40% users use 5+ different commands

### Qualitative Indicators (User Feedback)

**User Satisfaction**:
- "AGM made multi-agent workflows easy": >4/5 stars
- "I trust AGM with my sessions": >80% agree
- "I would recommend AGM": >70% NPS score

**Product-Market Fit**:
- "Very disappointed if AGM went away": >40% (Sean Ellis test)
- "AGM is my primary AI session manager": >60%

---

## Risks & Mitigations

### Risk 1: Agent CLI Breaking Changes

**Risk**: Agent providers (Claude, Gemini, GPT) change CLI interface, breaking AGM integration

**Likelihood**: Medium (agents evolve rapidly)
**Impact**: High (AGM stops working for that agent)

**Mitigation**:
- **Version detection**: Check agent CLI version on startup
- **Graceful degradation**: Warn if unsupported version, offer basic functionality
- **Integration tests**: CI tests against latest agent CLIs (daily)
- **Community monitoring**: Track agent release notes
- **Adapter versioning**: Support multiple versions per agent

**Contingency**: If breaking change occurs:
1. Release patch within 24 hours (hot-fix)
2. Document workaround in FAQ
3. Notify users via release notes

---

### Risk 2: Environment Configuration Complexity

**Risk**: Users struggle with environment setup despite `agm doctor` guidance

**Likelihood**: Medium (environment issues are inherently complex)
**Impact**: Medium (prevents new agent adoption, increases support burden)

**Mitigation**:
- **Step-by-step wizard**: Interactive `--fix` mode walks through setup
- **Template generation**: Ready-to-use .envrc and .bashrc snippets
- **Clear error messages**: Explain "why" not just "what"
- **Video tutorials**: Visual guides for common setups
- **Community support**: Discord/GitHub Discussions for peer help

**Metrics**:
- Track `agm doctor --fix` success rate
- Monitor support requests mentioning environment
- Survey users on setup difficulty (1-5 scale)

**Success Target**: >90% setup success rate, <5% environment-related support requests

---

### Risk 3: Manifest Corruption

**Risk**: Manifest files become corrupted (disk issues, bugs, race conditions)

**Likelihood**: Low (careful file handling, locks)
**Impact**: High (session data loss)

**Mitigation**:
- **Auto-backup**: Every write creates numbered backup (3 retained)
- **Validation**: Manifest schema validation on read
- **Repair**: `agm admin validate-manifests --fix` auto-repairs
- **Lock files**: Prevent concurrent writes
- **Atomic writes**: Write to temp file, atomic rename

**Recovery Path**:
1. Detect corruption on read
2. Offer restore from backup
3. If backups corrupt, offer manual JSON editing
4. Last resort: Recreate manifest from tmux session name + agent detection

---

### Risk 4: Workflow Scalability

**Risk**: Workflow automation doesn't scale to complex multi-step tasks

**Likelihood**: Medium (v3.0 workflows are experimental)
**Impact**: Low (workflows are optional feature)

**Mitigation**:
- **Incremental scope**: Start with simple workflows (deep-research)
- **Community feedback**: Gather use cases before expanding
- **Plugin architecture**: Allow custom workflows (v4.0+)
- **Clear limitations**: Document what workflows can/cannot do

**Decision Point**: If workflows don't gain traction (< 20% usage by month 3):
- Re-evaluate use cases
- Consider deprecating in favor of external automation (e.g., scripts)

---

### Risk 5: Backward Compatibility Burden

**Risk**: Supporting AGM manifests (v2) slows development, increases complexity

**Likelihood**: High (v2 support is indefinite commitment)
**Impact**: Medium (code complexity, testing burden)

**Mitigation**:
- **Migration deadline**: Set sunset date for v2 support (e.g., AGM v5.0)
- **Transparent migration**: `agm migrate` makes upgrade explicit
- **Version detection**: Auto-migrate on first write (transparent to users)
- **Deprecation warnings**: Warn users still on v2 manifests

**Trade-off Accepted**: Complexity cost justified by user goodwill (zero-downtime migration)

---

## Roadmap

### v3.0 (Shipped - 2026-02-04)

✅ Multi-agent session management (Claude, Gemini)
✅ Command translation layer
✅ Environment validation (Gemini)
✅ Backward compatibility (AGM migration)
✅ Accessibility features (WCAG AA)
✅ Message logging system
✅ Health checks and diagnostics
✅ Workflow automation (deep-research, experimental)

---

### v3.1 (Planned - Q2 2026)

🔜 **Unified Storage Migration**
- Migrate to ~/sessions/ unified structure
- Per-session conversation history
- Backup/restore for session directories

🔜 **Agent Routing (AGENTS.md)**
- Keyword-based agent selection
- Project-level and global-level routing
- Auto-select agent based on session name

🔜 **Enhanced Workflows**
- Code review workflow (multi-file analysis)
- Architecture design workflow (iterative planning)
- Debug workflow (error log analysis)

🔜 **GPT Support**
- GPT agent adapter
- Environment validation for OpenAI
- Command translation for GPT commands

🔜 **Improved Diagnostics**
- Session health scoring
- Performance benchmarks
- Resource usage tracking

---

### v3.2 (Planned - Q3 2026)

🔜 **Multi-Conversation Support**
- Multiple conversations per session
- Conversation switching within session
- Shared context between conversations

🔜 **Advanced Logging**
- Conversation history search
- Semantic search (Vertex AI embeddings)
- Export conversations (markdown, PDF)

🔜 **Workflow Marketplace**
- Community-contributed workflows
- Workflow discovery and installation
- Workflow versioning

---

### v4.0 (Future - Q4 2026+)

🔜 **Cloud Sync**
- Session sync across machines
- Conflict resolution
- Encrypted storage

🔜 **Web UI** (Optional)
- Browser-based session management
- Visual session timeline
- Conversation browser

🔜 **Plugin System**
- Custom agent adapters
- Custom workflows
- Extension marketplace

🔜 **Real-time Collaboration**
- Multi-user sessions
- Live cursor tracking
- Comment/annotation system

---

## Appendix

### A1: Command Reference

See [AGM Command Reference](AGM-COMMAND-REFERENCE.md) for complete CLI documentation.

### A2: Architecture Deep Dive

See [Architecture](ARCHITECTURE.md) for detailed technical architecture.

### A3: Migration Guide

See [Migration Guide](AGM-MIGRATION-GUIDE.md) for AGM to AGM migration steps.

### A4: Agent Comparison

See [Agent Comparison](AGENT-COMPARISON.md) for choosing the right agent.

### A5: Troubleshooting

See [Troubleshooting](TROUBLESHOOTING.md) for common issues and solutions.

---

## References

- **AGM Documentation**: Original Agent Session Manager docs
- **Agent Provider Docs**:
  - Claude CLI: https://docs.anthropic.com/claude/docs/cli
  - Gemini CLI: https://ai.google.dev/gemini-api/docs
  - OpenAI CLI: https://platform.openai.com/docs/cli
- **Related Projects**:
  - direnv: https://direnv.net/
  - tmux: https://github.com/tmux/tmux
  - Cobra CLI: https://github.com/spf13/cobra

---

**Maintained by**: Foundation Engineering
**License**: MIT
**Repository**: https://github.com/vbonnet/dear-agent
**Contact**: File issues on GitHub

---

**End of Specification**
