# MCP Setup Wizard

Automated setup tool for MCP (Model Context Protocol) servers.

## Quick Start

```bash
# Install globally
npm install -g mcp-wizard

# Run setup wizard (interactive MCP selection - default command)
mcp-wizard

# Or specify MCPs via CLI
mcp-wizard setup --mcps=googledocs,atlassian

# Resume from previous session
mcp-wizard setup --resume

# Select specific AI agents
mcp-wizard setup --agents=claude-code,cursor

# Check system health
mcp-wizard health

# Get detailed diagnostics
mcp-wizard doctor

# Check MCP health at shell startup (add to ~/.bashrc or ~/.zshrc)
mcp-wizard session-start
```

## What This Tool Does

- ✅ **Interactive MCP selection**: Choose which MCPs to configure
- ✅ **Google Docs MCP**: Automated install + OAuth wizard
- ✅ **GitHub MCP**: Automated setup with PAT or OAuth (VS Code 1.101+)
- ✅ **Atlassian MCP**: Auto-configured (OAuth on first use)
- ✅ **Sequential Thinking MCP**: Enhanced reasoning with structured thinking (no auth required)
- ✅ **Prerequisites validation**: Checks Node.js, gcloud CLI, Claude Code
- ✅ **Multi-agent support**: Claude Code, Cursor, Cline, Windsurf
- ✅ **Progress tracking**: Visual feedback during setup
- ✅ **Resume capability**: Restart from where you left off
- ✅ **Chezmoi automation**: Automated template creation and apply

## Prerequisites

- **Node.js:** Version 18.0.0 or later
- **Configuration:** Company-specific settings (see Configuration section below)

## Configuration

MCP Wizard uses hierarchical configuration with the following precedence:

**1. Environment Variables** (highest priority)
```bash
export MCP_WIZARD_COMPANY_NAME="Acme Corp"
export MCP_WIZARD_GLEAN_INSTANCE="acme"
export MCP_WIZARD_OKTA_DOMAIN="acme.okta.com"
```

**2. Project Config** (`.mcp-wizard.json` in project directory)
```json
{
  "company": {
    "name": "Acme Corp",
    "glean_instance": "acme",
    "okta_domain": "acme.okta.com"
  }
}
```

**3. User Config** (`~/.config/mcp-wizard/config.json`)
```json
{
  "company": {
    "name": "Acme Corp",
    "glean_instance": "acme",
    "okta_domain": "acme.okta.com"
  }
}
```

### First-Time Setup

Run interactive config setup:
```bash
mcp-wizard config init
```

This will prompt for:
- Company name
- Glean instance (lowercase, no spaces)
- Okta domain (e.g., `company.okta.com`)

### Project-Specific Config

Create `.mcp-wizard.json` in your project root:
```bash
cat > .mcp-wizard.json <<'EOF'
{
  "company": {
    "name": "Acme Corp",
    "glean_instance": "acme",
    "okta_domain": "acme.okta.com"
  }
}
EOF
```

**Note:** `.mcp-wizard.json` is gitignored by default to prevent company data leaks.

### Environment Variable Overrides

For CI/CD or temporary overrides:
```bash
MCP_WIZARD_GLEAN_INSTANCE=staging mcp-wizard setup
```

## Status

**Current Version:** Beta (PR #369 + #370)

**Ready for Testing:**
- ✅ Prerequisites validation with actionable errors
- ✅ Interactive MCP selection (GoogleDocs, Atlassian, Sequential Thinking)
- ✅ Google Docs OAuth wizard (GCP Console guide)
- ✅ Atlassian auto-configuration (mcp-remote OAuth)
- ✅ Sequential Thinking MCP (zero-config, no authentication)
- ✅ Multi-agent config (Claude Code, Cursor, Cline, Windsurf)
- ✅ Setup verification (non-blocking)
- ✅ 45 unit tests, 89% coverage

**Available Commands:**
- ✅ `setup` - Interactive MCP setup wizard
- ✅ `status` - Show current setup status
- ✅ `validate` - Validate MCP configuration and health
- ✅ `health` - Fast health check (<5 seconds)
- ✅ `doctor` - Comprehensive diagnostics with recommendations
- ✅ `session-start` - Check MCP auth status at shell startup (see docs/SESSIONSTART-HOOK.md)

**Not Yet Available:**
- ⏸️ Glean MCP (requires Glean admin token)
- ⏸️ Slack MCP (requires workspace admin)
- 🚧 `repair` command (coming soon)
- 🚧 `auth` command (coming soon)

## Health Monitoring

MCP Wizard includes comprehensive health monitoring to ensure your setup is working correctly.

### Quick Health Check

```bash
# Fast check (<5 seconds)
mcp-wizard health

# Output example:
# ✓ Token Health: Google token valid (expires in 45 minutes)
# ✓ MCP Processes: 2/2 processes alive
# ✓ Network Connectivity: All 3 endpoints reachable
# ✓ Intent Analyzer: Confidence 87%, 0 mismatches
# Overall: Healthy
```

### Comprehensive Diagnostics

```bash
# Detailed diagnostics with recommendations
mcp-wizard doctor

# Output includes:
# - All health check details
# - Configuration validation
# - Actionable fix recommendations
```

### What Gets Checked

1. **Token Health** - Google OAuth token validity and expiration
2. **MCP Processes** - Verifies configured MCPs are running
3. **Network Connectivity** - Tests OAuth/API endpoint accessibility
4. **Intent Analyzer** - Validates keyword matching accuracy
5. **Configuration** - Checks config file schema and completeness

### Features

- **5-minute cache** - Fast repeated checks (bypass with `--force`)
- **JSON output** - Machine-readable format (`--json`)
- **Exit codes** - 0=healthy, 1=warning, 2=error
- **Silent mode** - Exit code only (`--silent`)

See [docs/HEALTH-MONITORING.md](docs/HEALTH-MONITORING.md) for detailed documentation.

## Shell Integration (Session Startup)

Check MCP health automatically when you start a new terminal session:

```bash
# Add to ~/.bashrc or ~/.zshrc
mcp-wizard session-start
```

**What it does:**
- ✅ Checks Okta token health at shell startup
- ✅ Shows proactive warnings if tokens need refresh
- ✅ <500ms execution (uses health check cache)
- ✅ Silent when healthy (no output clutter)

**Example output:**

```bash
# All healthy (green)
✓ MCP Health: 4 MCPs authenticated

# Token expiring soon (yellow)
⚠ MCP Health: Token expiring soon (15m)
Run `mcp-wizard auth` to refresh

# Token expired (red)
✗ MCP Health: Token expired or invalid
Run `mcp-wizard auth` to re-authenticate
```

**Options:**
- `--verbose` - Show detailed status (expiration times, MCP list)
- `--auto-refresh` - Automatically refresh expired tokens (uses Device Flow)

**Silent mode** (suppress warnings):

```bash
# Only show errors, suppress warnings
mcp-wizard session-start 2>/dev/null || true
```

See [docs/SESSIONSTART-HOOK.md](docs/SESSIONSTART-HOOK.md) for complete documentation.

## Supported MCP Servers

### Sequential Thinking MCP

The Sequential Thinking MCP enhances AI reasoning by breaking down complex problems into structured, step-by-step thinking.

**Features:**
- No authentication required
- Automatic installation via npx
- Thought process logging enabled by default
- Compatible with all AI agents (Claude Code, Cursor, Cline, Windsurf)

**When to use:**
- Debugging complex issues
- Planning multi-step implementations
- Breaking down architectural decisions
- Analyzing trade-offs and alternatives

**Performance note:**
Thought logging may add ~50-100ms latency per AI response. If performance is critical, disable logging by manually editing your MCP config:

```json
{
  "mcpServers": {
    "SequentialThinking": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-sequential-thinking"],
      "env": {
        "DISABLE_THOUGHT_LOGGING": "true"
      }
    }
  }
}
```

**Example prompts:**
- "Break down this problem step-by-step"
- "Plan the implementation for [feature] using structured thinking"
- "Analyze the trade-offs between [option A] and [option B]"

### Google Docs MCP

Automated installation and OAuth setup for Google Docs and Drive access.

### Atlassian MCP

Auto-configured access to Jira and Confluence with automatic OAuth on first use.

## Project Documentation

All planning and implementation documents are in `docs/`:

- **W0-project-charter.md** - Initial problem/solution/scope
- **D1-review-council-results.md** - First review, identified blockers
- **D2-approach-selection.md** - Technical decisions
- **D2-REVIEW-COUNCIL.md** - Second review, 10 conditions
- **D3-implementation-planning.md** - Comprehensive implementation plan
- **D3-REVIEW-COUNCIL.md** - Third review, unanimous approval
- **D4-week1-critical-conditions.md** - All 4 CRITICAL conditions
- **D4-week1-summary.md** - Week 1 completion report
- **RETROSPECTIVE.md** - Living retrospective

## Development

```bash
# Install dependencies
npm install

# Build
npm run build

# Run tests
npm test

# Run tests with coverage
npm run test:coverage

# Lint
npm run lint

# Format
npm run format
```

## Architecture

This is a Node.js CLI tool using:
- **TypeScript** for type safety
- **Commander.js** for CLI framework
- **googleapis** for OAuth flow
- **Jest** for testing

See `docs/D3-implementation-planning.md` for detailed architecture.

## Beta Goals

- **Setup Time:** <15 minutes for GoogleDocs + Atlassian
- **Success Rate:** >95% of setups complete without support
- **User Experience:** Self-service with clear error messages

## Chezmoi Support

If you use chezmoi to manage your dotfiles, the setup wizard will automatically:
1. Detect chezmoi installation
2. Ask: "Apply via chezmoi? (Y/n)"
3. Create template file in your chezmoi source directory
4. Optionally show diff preview: "Show diff before applying? (y/N)"
5. Run `chezmoi apply <file>` to apply changes
6. Fall back to manual instructions on any errors

**Automated flow**:
```bash
mcp-wizard setup
# ... setup steps ...
? Apply via chezmoi? (Y/n) Yes
? Show diff before applying? (y/N) No
─── Applying configuration via chezmoi ───
  ✓ Wrote chezmoi template: ~/.local/share/chezmoi/dot_config/claude-code/private_mcp.json.tmpl
✓ Applied MCP config via chezmoi
```

**What if chezmoi apply fails?**

The wizard automatically falls back to manual instructions:
```
✗ Chezmoi apply failed: permission denied
  Falling back to manual instructions...

ℹ️ Manual Steps:
  1. Add this to: ~/.local/share/chezmoi/dot_config/claude-code/private_mcp.json.tmpl
  2. Run: chezmoi apply
```

See `docs/CHEZMOI-INTEGRATION.md` for technical details.

## GitHub MCP

The GitHub MCP provides access to GitHub repositories, issues, pull requests, GitHub Actions, and code security features.

### Authentication Methods

**Personal Access Token (PAT)** - Primary method, works everywhere:
1. Visit https://github.com/settings/tokens/new
2. Click "Generate new token (classic)"
3. Select scopes:
   - `repo` (full repository access) - **Required**
   - `read:org` (read organization data) - **Required**
   - `read:packages` (read package data) - Optional
   - `workflow` (GitHub Actions access) - Optional
4. Generate and copy the token
5. Run `mcp-wizard setup` and select GitHub MCP

**OAuth (VS Code 1.101+)** - Enhanced method (experimental):
- Requires VS Code version 1.101 or later (released Nov 2024)
- Browser-based authentication (no manual token generation)
- Automatic token refresh
- Falls back to PAT if unavailable

### GitHub Enterprise Server

To use with GitHub Enterprise Server:
1. Select GitHub MCP during setup
2. When prompted, choose "GitHub Enterprise Server"
3. Enter your enterprise URL (e.g., `https://github.company.com`)
4. Complete authentication (PAT or OAuth)

### Feature Selection

During setup, you can select which GitHub features to enable:
- **Repositories** - File search, navigation (recommended)
- **Issues** - Search, create, comment (recommended)
- **Pull Requests** - Review, status, comment
- **GitHub Actions** - Workflow monitoring
- **Code Security** - Vulnerability scanning

### Troubleshooting

**Invalid PAT Scope Error:**
```
❌ Missing required scopes: repo, read:org
```
Regenerate your token at https://github.com/settings/tokens with the correct scopes.

**OAuth Not Available:**
```
⚠ OAuth requires VS Code 1.101+
```
Upgrade VS Code or use PAT authentication instead.

**Enterprise Connectivity Issues:**
Make sure your enterprise URL uses HTTPS and is accessible from your network.

## Documentation

- **Main Guide:** `TROUBLESHOOTING.md` - Common issues & solutions
- **Atlassian OAuth:** `docs/ATLASSIAN-MCP.md` - How automatic OAuth works
- **Chezmoi Integration:** `docs/CHEZMOI-INTEGRATION.md` - Automated chezmoi setup
- **ADR:** `docs/adr/mcp-wizard-production-enhancements.md` - Design decisions

## License

MIT License - See LICENSE file for details
