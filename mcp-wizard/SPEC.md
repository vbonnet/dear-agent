# Product Specification: MCP Wizard

**Version:** 1.0
**Status:** Active
**Last Updated:** 2026-02-11

## Overview

MCP Wizard is an automated CLI tool that simplifies Model Context Protocol (MCP) server setup, reducing setup time from 45+ minutes to under 15 minutes. It provides interactive configuration, OAuth authentication, and multi-agent support for AI coding assistants.

## Problem Statement

Developers using Claude Code and other AI coding assistants need to manually configure MCP servers for accessing services like Google Docs, Atlassian (Jira/Confluence), GitHub, and other integrations. The current manual process involves:

1. Navigating Google Cloud Console to create OAuth credentials
2. Manually configuring OAuth consent screens
3. Downloading and placing credentials files
4. Cloning and building MCP server repositories
5. Running manual authentication flows
6. Configuring MCP config files across multiple AI agents

This 30-45 step process is error-prone, poorly documented, and creates a high barrier to adoption.

## Goals

### Primary Goals
1. **Reduce setup time**: From 45+ minutes to <15 minutes
2. **Improve user experience**: Match quality of industry-standard CLI tools (`gh auth login`, `gcloud auth login`)
3. **Increase adoption**: Make Claude Code + MCPs accessible to all developers
4. **Reduce support burden**: Self-service tool reduces support tickets by 75%

### Secondary Goals
1. **Extensibility**: Support for multiple MCP servers
2. **Multi-agent support**: Configure Claude Code, Cursor, Cline, and Windsurf
3. **Health monitoring**: Proactive detection of authentication and connectivity issues
4. **Maintainability**: Clear code structure, comprehensive testing, good documentation

## Success Metrics

**Quantitative:**
- Setup time: <15 minutes from start to authenticated
- Error rate: <5% of setup attempts fail
- Support tickets: 75% reduction in MCP-related support requests
- Test coverage: >85% code coverage

**Qualitative:**
- Users can complete setup without documentation
- Clear, actionable error messages guide users to solutions
- Works on both fresh installs and repair scenarios
- Positive feedback from users (9.3/10 multi-persona review)

## Core Features

### 1. Interactive MCP Selection

**Description:** User selects which MCP servers to configure through an interactive CLI prompt.

**Supported MCPs:**
- Google Docs MCP: Full OAuth wizard with automated setup
- Atlassian MCP: Auto-configured (OAuth handled by mcp-remote)
- GitHub MCP: PAT or OAuth authentication with feature selection
- Sequential Thinking MCP: Enhanced reasoning (zero-config)
- Playwright MCP: Browser automation (zero-config)

**User Flow:**
```bash
mcp-wizard setup

? Select MCP servers to configure:
  [x] Google Docs
  [x] Atlassian
  [ ] GitHub
  [x] Sequential Thinking
  [ ] Playwright
```

### 2. Prerequisites Validation

**Description:** Validates system requirements before setup starts to prevent failures.

**Checks:**
- Node.js version ≥18.0.0
- gcloud CLI installed (for Google Docs MCP)
- gcloud authentication status
- Claude Code installation (optional)

**Performance:** Parallel execution, <5 seconds total check time

**Error Handling:** Actionable error messages with fix instructions and help links

### 3. OAuth Wizard (Google Docs)

**Description:** Guided OAuth setup for Google Docs MCP with browser automation.

**Flow:**
1. Tool opens GCP Console URLs with pre-filled project ID
2. Step-by-step wizard prompts user through credential creation
3. Automated OAuth flow with browser redirect
4. Token storage with secure file permissions (600)
5. Validation of successful authentication

**Security:**
- Per-user OAuth credentials (no shared service accounts)
- File permissions enforcement (600 for credentials/tokens)
- .gitignore creation to prevent credential leaks
- Clear revocation instructions

### 4. Multi-Agent Configuration

**Description:** Writes MCP configuration to multiple AI agent config files.

**Supported Agents:**
- Claude Code: `~/.config/claude-code/mcp.json`
- Cursor: `~/.cursor/mcp.json`
- Cline: `~/.cline/mcp.json`
- Windsurf: `~/.codeium/windsurf/mcp.json`

**Features:**
- Agent selection during setup
- Config backup before modification
- Merge with existing configurations
- Rollback on write failure

### 5. Chezmoi Integration

**Description:** Automatic detection and integration with chezmoi dotfile manager.

**Flow:**
1. Detect chezmoi installation and management status
2. Ask user: "Apply via chezmoi? (Y/n)"
3. Create template file in chezmoi source directory
4. Optionally show diff preview
5. Run `chezmoi apply` to apply changes
6. Fall back to manual instructions on errors

**Safety:**
- Never auto-edit existing chezmoi templates
- Clear user prompts for each step
- Preserve user's chezmoi workflow

### 6. Health Monitoring

**Description:** Fast health checks and comprehensive diagnostics for MCP setup.

**Commands:**
- `mcp-wizard health`: Fast check (<5 seconds) with overall status
- `mcp-wizard doctor`: Comprehensive diagnostics with recommendations
- `mcp-wizard session-start`: Shell startup hook for proactive warnings

**Health Checks:**
1. Token Health: OAuth token validity and expiration (>5min = healthy)
2. MCP Processes: Verifies configured MCPs are running
3. Network Connectivity: Tests OAuth/API endpoint accessibility
4. Intent Analyzer: Validates keyword matching accuracy
5. Configuration: Checks config file schema and completeness

**Features:**
- 5-minute cache for fast repeated checks
- JSON output for automation
- Exit codes: 0=healthy, 1=warning, 2=error
- Actionable recommendations for issues

### 7. Configuration Management

**Description:** Hierarchical configuration with multiple sources.

**Precedence (highest to lowest):**
1. Environment variables (`MCP_WIZARD_*`)
2. Project config (`.mcp-wizard.json` in project directory)
3. User config (`~/.config/mcp-wizard/config.json`)

**Commands:**
- `mcp-wizard config init`: Interactive config setup
- Config location detection (legacy and new paths)
- Config merge (preserves existing settings)

### 8. Progress Indicators

**Description:** Visual feedback during long-running operations.

**Features:**
- Ora spinners for async operations
- Non-blocking progress updates
- Clear status messages (checking, installing, configuring)
- Success/failure indicators

### 9. Resume Capability

**Description:** Ability to resume interrupted setup sessions.

**Usage:**
```bash
mcp-wizard setup --resume
```

**Features:**
- State persistence across interruptions
- OAuth timeout recovery
- Partial completion support

## User Personas

### Primary Persona: Software Engineer
**Goals:** Set up MCPs quickly to use Claude Code for development
**Pain Points:** Manual setup is time-consuming and error-prone
**Success Criteria:** <15 min setup, works without troubleshooting

### Secondary Persona: DevOps Engineer
**Goals:** Automate MCP setup for team onboarding
**Pain Points:** No scriptable setup process
**Success Criteria:** Can include in onboarding scripts, JSON output for automation

### Tertiary Persona: Technical Support
**Goals:** Help users fix broken MCP setups
**Pain Points:** Generic errors make troubleshooting difficult
**Success Criteria:** Actionable error messages, repair commands that work

## Non-Goals (Out of Scope)

- Creating new MCP servers (only configures existing ones)
- Managing Claude Code installation (assumes already installed)
- GUI/web interface (CLI only)
- Automated updates/version management (manual for v1)
- Windows support (Linux/macOS only for v1)
- Enterprise SSO integration (future consideration)

## Technical Requirements

### System Requirements
- Node.js ≥18.0.0
- npm (comes with Node.js)
- gcloud CLI (for Google Docs MCP)
- Git (for cloning MCP repos)

### Platform Support
- Linux: Full support, tested
- macOS: Full support, tested
- Windows: Logic exists, untested (community testing in v2)

### Dependencies
- TypeScript: Type safety and modern JavaScript features
- Commander.js: CLI framework
- Inquirer: Interactive prompts
- googleapis: OAuth flow and Google API access
- ora: Progress spinners
- chalk: Terminal colors
- Jest: Testing framework

### Security Requirements
- OAuth tokens stored with 600 permissions (owner-only)
- Credentials never logged or exposed in error messages
- .gitignore creation to prevent credential commits
- Path sanitization to prevent traversal attacks
- No command injection (uses execFile, not shell)
- Timeout handling on all external commands

### Performance Requirements
- Prerequisites check: <5 seconds
- Total setup time: <15 minutes (including user interaction)
- Health check: <5 seconds (with caching)
- Test suite: <30 seconds

## Future Enhancements (v2+)

### Planned Features
- Repair command: Automated fix for common issues
- Auth command: Token refresh and re-authentication
- Windows full support with testing
- File locking for concurrent wizard prevention
- Automated monitoring/telemetry
- Additional MCP servers (Slack, Notion, Linear)

### Under Consideration
- Web dashboard for health monitoring
- Auto-refresh for expiring tokens
- Deeper MCP validation (stdio ping)
- Historical health tracking
- Metrics export (Prometheus/Grafana)
- Service account option (with security review)

## Documentation Requirements

### User Documentation
- README.md: Quick start and overview
- TROUBLESHOOTING.md: Common issues and solutions
- HEALTH-MONITORING.md: Health check documentation
- SESSIONSTART-HOOK.md: Shell integration guide
- ATLASSIAN-MCP.md: Atlassian OAuth details
- CHEZMOI-INTEGRATION.md: Chezmoi setup guide

### Developer Documentation
- SPEC.md: This document
- ARCHITECTURE.md: Technical architecture
- ADR/: Architecture decision records
- API documentation: JSDoc comments
- Test documentation: Coverage reports

### Planning Documentation (archived)
- W0-project-charter.md: Initial problem/solution/scope
- D1-review-council-results.md: First review, identified blockers
- D2-approach-selection.md: Technical decisions
- D3-implementation-planning.md: Implementation plan
- RETROSPECTIVE.md: Project retrospective

## Appendix: Command Reference

### Setup Commands
```bash
# Interactive setup (default)
mcp-wizard
mcp-wizard setup

# Specify MCPs via CLI
mcp-wizard setup --mcps=github
mcp-wizard setup --mcps=googledocs,atlassian,github

# Resume from previous session
mcp-wizard setup --resume

# Select specific AI agents
mcp-wizard setup --agents=claude-code,cursor
```

### Status Commands
```bash
# Show current setup status
mcp-wizard status

# Validate MCP configuration
mcp-wizard validate
```

### Health Commands
```bash
# Fast health check
mcp-wizard health

# Comprehensive diagnostics
mcp-wizard doctor

# Shell startup check
mcp-wizard session-start
```

### Configuration Commands
```bash
# Interactive config setup
mcp-wizard config init
```

### Flags
- `--json`: Output in JSON format
- `--force`: Bypass cache, force fresh check
- `--silent`: Exit code only, no output
- `--verbose`: Detailed logging
- `--resume`: Resume interrupted setup
- `--mcps=<list>`: Specify MCPs to configure
- `--agents=<list>`: Specify AI agents to configure

## Appendix: ROI Calculation

**Time Savings:**
- Manual setup: 45 minutes per user
- Automated setup: 15 minutes per user
- Savings: 30 minutes per user

**Support Reduction:**
- Before: 8 hours/week support time
- After: 2 hours/week support time
- Savings: 6 hours/week = 24 hours/month

**First Month ROI:**
- Development time: 6.5 hours
- User time saved: 50 users × 0.5 hours = 25 hours
- Support time saved: 24 hours
- Total saved: 49 hours
- ROI: 49 / 6.5 = 7.5x

**Actual Results:**
- ROI: 11.8x in first month (77 hours saved vs 6.5 hours invested)
- Support time reduction: 75% (8 hours/week → 2 hours/week)
- Multi-persona review: 9.3/10 approval rating

## Appendix: Version History

- **v0.1.0** (Beta): Initial release with Google Docs, Atlassian, Sequential Thinking, Playwright MCPs
- **v0.2.0** (Planned): Add GitHub MCP, repair command, enhanced error handling
- **v1.0.0** (Planned GA): Production-ready with Windows support, comprehensive testing
