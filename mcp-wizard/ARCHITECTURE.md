# Architecture: MCP Wizard

**Version:** 1.0
**Status:** Active
**Last Updated:** 2026-02-11

## Table of Contents

1. [System Overview](#system-overview)
2. [Architecture Diagram](#architecture-diagram)
3. [Component Design](#component-design)
4. [Data Flow](#data-flow)
5. [Security Model](#security-model)
6. [Plugin Architecture](#plugin-architecture)
7. [Testing Strategy](#testing-strategy)
8. [Deployment](#deployment)

## System Overview

MCP Wizard is a Node.js CLI tool built with TypeScript that automates the configuration of Model Context Protocol (MCP) servers for AI coding assistants. The architecture follows a modular design with clear separation between setup orchestration, MCP-specific plugins, and cross-cutting concerns like error handling and validation.

### Key Design Principles

1. **Modularity**: Each component has a single responsibility
2. **Extensibility**: Plugin architecture for adding new MCP servers
3. **Security-First**: OAuth best practices, file permissions, credential protection
4. **User Experience**: Progressive disclosure, actionable errors, visual feedback
5. **Reliability**: Comprehensive validation, error recovery, state persistence

### Technology Stack

- **Runtime**: Node.js ≥18.0.0
- **Language**: TypeScript 5.0+
- **CLI Framework**: Commander.js 12.0
- **Interactive Prompts**: Inquirer 9.2
- **OAuth**: googleapis 148.0
- **Testing**: Jest 29.0
- **Progress Indicators**: ora 7.0
- **Terminal Colors**: chalk 5.3

## Architecture Diagram

### High-Level System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         USER MACHINE                            │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                     mcp-wizard CLI                       │  │
│  │                 (Node.js, TypeScript)                    │  │
│  │                                                           │  │
│  │  ┌────────────────────────────────────────────────────┐  │  │
│  │  │           Command Layer (src/commands/)            │  │  │
│  │  │  • setup.ts      • health.ts                       │  │  │
│  │  │  • status.ts     • doctor.ts                       │  │  │
│  │  │  • validate.ts   • config.ts                       │  │  │
│  │  └────────────────────┬───────────────────────────────┘  │  │
│  │                       │                                   │  │
│  │  ┌────────────────────┴───────────────────────────────┐  │  │
│  │  │        Core Infrastructure (src/lib/)             │  │  │
│  │  │  • validators/    • verifiers/                     │  │  │
│  │  │  • config/        • health-checks.ts               │  │  │
│  │  │  • health-cache.ts                                 │  │  │
│  │  └────────────────────┬───────────────────────────────┘  │  │
│  │                       │                                   │  │
│  │  ┌────────────────────┴───────────────────────────────┐  │  │
│  │  │        MCP Plugin System (src/mcps/)              │  │  │
│  │  │  • google-docs/   • atlassian/                     │  │  │
│  │  │  • github/        • sequential-thinking/           │  │  │
│  │  │  • playwright/                                     │  │  │
│  │  └────────────────────┬───────────────────────────────┘  │  │
│  │                       │                                   │  │
│  │  ┌────────────────────┴───────────────────────────────┐  │  │
│  │  │     Cross-Cutting Concerns (src/ui/, src/errors/) │  │  │
│  │  │  • Progress indicators (ora spinners)              │  │  │
│  │  │  • Error handling (SetupError with actionable msgs)│ │  │
│  │  │  • Logging (pino)                                  │  │  │
│  │  └────────────────────────────────────────────────────┘  │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │                 File System State                        │  │
│  │                                                           │  │
│  │  ~/.config/mcp-wizard/                                   │  │
│  │    └── config.json (user config)                        │  │
│  │                                                           │  │
│  │  ~/.config/claude-code/mcp.json (Claude Code config)    │  │
│  │  ~/.cursor/mcp.json (Cursor config)                     │  │
│  │  ~/.cline/mcp.json (Cline config)                       │  │
│  │  ~/.codeium/windsurf/mcp.json (Windsurf config)         │  │
│  │                                                           │  │
│  │  ~/mcp-servers/                                          │  │
│  │    └── google-docs-mcp/                                 │  │
│  │        ├── credentials.json [600]                       │  │
│  │        ├── token.json [600]                             │  │
│  │        └── dist/server.js                               │  │
│  └──────────────────────────────────────────────────────────┘  │
│                                                                 │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         │ HTTPS (OAuth flow, API calls)
                         │
                         ▼
        ┌────────────────────────────────────────────┐
        │         External Services                  │
        │                                             │
        │  ┌──────────────────────────────────────┐  │
        │  │    Google Cloud Platform             │  │
        │  │  • OAuth authorization server        │  │
        │  │  • Google Docs API                   │  │
        │  │  • Google Drive API                  │  │
        │  └──────────────────────────────────────┘  │
        │                                             │
        │  ┌──────────────────────────────────────┐  │
        │  │    Atlassian (via mcp-remote)        │  │
        │  │  • Jira API                          │  │
        │  │  • Confluence API                    │  │
        │  └──────────────────────────────────────┘  │
        │                                             │
        │  ┌──────────────────────────────────────┐  │
        │  │    GitHub                            │  │
        │  │  • GitHub API                        │  │
        │  │  • OAuth (VS Code 1.101+)            │  │
        │  └──────────────────────────────────────┘  │
        └─────────────────────────────────────────────┘
```

## Component Design

### 1. Command Layer (src/commands/)

Entry points for CLI commands, orchestrating the setup workflow.

**Key Files:**
- `setup.ts`: Main setup wizard orchestration
- `health.ts`: Fast health check command
- `doctor.ts`: Comprehensive diagnostics command
- `status.ts`: Show current MCP configuration status
- `validate.ts`: Validate MCP setup
- `config.ts`: Configuration management

**Responsibilities:**
- Parse CLI arguments and flags
- Orchestrate multi-step workflows
- Call validators, installers, configurers in correct order
- Handle user input via inquirer prompts
- Display results and error messages

### 2. Validators (src/validators/)

Pre-flight checks to ensure system requirements are met.

**Components:**
- `prerequisites.ts`: Validates Node.js version, gcloud CLI, authentication
- `config-validator.ts`: Validates config file schema
- `path-validator.ts`: Path sanitization and security checks

**Key Features:**
- Parallel execution for speed (3.7x faster than sequential)
- Timeout handling (prevents hangs on external commands)
- Security: Uses `execFile` not `shell` (prevents command injection)
- Actionable error messages with fix instructions

**Example:**
```typescript
interface PrerequisiteCheck {
  name: string;
  status: 'passed' | 'failed' | 'warning';
  message: string;
  fix?: string;
  helpLink?: string;
}

async function checkNodeVersion(): Promise<PrerequisiteCheck> {
  const version = process.version;
  const major = parseInt(version.slice(1).split('.')[0]);

  if (major >= 18) {
    return { name: 'Node.js', status: 'passed', message: `v${version}` };
  } else {
    return {
      name: 'Node.js',
      status: 'failed',
      message: `v${version} (requires ≥18.0.0)`,
      fix: 'Install Node.js 18+: https://nodejs.org/',
      helpLink: 'https://github.com/nvm-sh/nvm#installing-and-updating'
    };
  }
}
```

### 3. Verifiers (src/verifiers/)

Post-setup verification to ensure MCP servers are configured correctly.

**Components:**
- `mcp-verifier.ts`: Verifies MCP servers are registered via `claude mcp list`
- `config-verifier.ts`: Validates written config files

**Key Features:**
- Parses both JSON and text output formats
- Non-blocking (wizard continues if verification fails)
- Provides detailed failure information for troubleshooting

### 4. Config Management (src/config/)

Configuration detection, merging, and writing across multiple locations.

**Components:**
- `config-detector.ts`: Detects config file locations (legacy vs new paths)
- `config-merger.ts`: Merges new MCP configs with existing settings
- `config-writer.ts`: Writes configs with backup/rollback

**Key Features:**
- Hierarchical configuration (env vars > project config > user config)
- Path sanitization (rejects ".." traversal)
- Backup before modification (600 permissions)
- Rollback on write failure
- Merge with existing MCPs (preserves user settings)

**Config Location Detection:**
```typescript
interface ConfigLocation {
  path: string;
  exists: boolean;
  legacy: boolean;
}

async function detectConfigLocation(): Promise<ConfigLocation> {
  const newPath = path.join(os.homedir(), '.config', 'claude-code', 'mcp.json');
  const legacyPath = path.join(os.homedir(), '.claude.json');

  if (await pathExists(newPath)) {
    return { path: newPath, exists: true, legacy: false };
  } else if (await pathExists(legacyPath)) {
    return { path: legacyPath, exists: true, legacy: true };
  } else {
    return { path: newPath, exists: false, legacy: false };
  }
}
```

### 5. Health Checks (src/lib/)

Monitoring system for MCP setup health and diagnostics.

**Components:**
- `health-checks.ts`: Core health check implementations (509 lines)
- `health-cache.ts`: 5-minute TTL cache (88 lines)

**Health Check Types:**

1. **Token Health**: OAuth token validity and TTL
   - Healthy: Valid token, >5min TTL
   - Degraded: Valid token, 1-5min TTL
   - Unhealthy: Expired or missing token

2. **MCP Processes**: Process existence check via `pgrep`
   - Healthy: All configured MCPs running
   - Degraded: Some MCPs running
   - Unhealthy: No MCPs running

3. **Network Connectivity**: HEAD requests to OAuth/API endpoints
   - Healthy: All endpoints reachable
   - Degraded: 1-2 endpoints unreachable
   - Unhealthy: 3+ endpoints unreachable

4. **Intent Analyzer**: Keyword matching accuracy
   - Healthy: ≥70% avg confidence, 0 mismatches
   - Degraded: ≥50% avg confidence, ≤1 mismatch
   - Unhealthy: <50% avg confidence or 2+ mismatches

5. **Configuration**: Config file schema and completeness
   - Validates required fields
   - Checks MCP server configurations
   - Reports missing/invalid settings

**Caching Strategy:**
```typescript
interface CachedHealthResult {
  result: HealthCheckResult;
  timestamp: number;
  ttl: number; // milliseconds
}

class HealthCache {
  private cache = new Map<string, CachedHealthResult>();
  private readonly TTL = 5 * 60 * 1000; // 5 minutes

  get(key: string, force: boolean): HealthCheckResult | null {
    if (force) return null;

    const cached = this.cache.get(key);
    if (!cached) return null;

    const age = Date.now() - cached.timestamp;
    return age < this.TTL ? cached.result : null;
  }

  set(key: string, result: HealthCheckResult): void {
    this.cache.set(key, {
      result,
      timestamp: Date.now(),
      ttl: this.TTL
    });
  }
}
```

### 6. MCP Plugin System (src/mcps/)

Extensible plugin architecture for MCP-specific setup logic.

**Plugin Interface:**
```typescript
interface McpPlugin {
  // Metadata
  name: string;
  displayName: string;
  description: string;
  requiresAuth: boolean;

  // Lifecycle hooks
  detect(): Promise<DetectionResult>;
  install?(): Promise<void>;
  configure(): Promise<ConfigSection>;
  authenticate?(): Promise<void>;
  verify(): Promise<VerificationResult>;
  repair?(): Promise<void>;
}

interface DetectionResult {
  installed: boolean;
  built: boolean;
  authenticated: boolean;
  version?: string;
}

interface ConfigSection {
  [mcpName: string]: {
    command: string;
    args: string[];
    env?: Record<string, string>;
  };
}
```

**Implemented Plugins:**

#### Google Docs Plugin (src/mcps/google-docs/)
- Full OAuth wizard with browser automation
- GCP Console credential creation guide
- Token storage with secure file permissions
- Token refresh support via googleapis

#### Atlassian Plugin (src/mcps/atlassian/)
- Auto-configuration via mcp-remote
- OAuth handled remotely (no local credentials)
- Jira and Confluence access

#### GitHub Plugin (src/mcps/github/)
- PAT or OAuth authentication (VS Code 1.101+)
- GitHub Enterprise Server support
- Feature selection (repos, issues, PRs, Actions, security)

#### Sequential Thinking Plugin (src/mcps/sequential-thinking/)
- Zero-config installation via npx
- Thought process logging (configurable)
- No authentication required

#### Playwright Plugin (src/mcps/playwright/)
- Browser automation support
- Zero-config installation via npx
- Chromium auto-download (~300MB on first use)
- No authentication required

### 7. Error Handling (src/errors/)

Custom error classes with actionable error messages.

**SetupError Class:**
```typescript
class SetupError extends Error {
  constructor(
    public problem: string,
    public fix: string,
    public helpLink?: string
  ) {
    super(`${problem}\n\nFix: ${fix}${helpLink ? `\nHelp: ${helpLink}` : ''}`);
    this.name = 'SetupError';
  }

  toJSON() {
    return {
      error: this.name,
      problem: this.problem,
      fix: this.fix,
      helpLink: this.helpLink
    };
  }
}
```

**Usage:**
```typescript
if (!gcloudInstalled) {
  throw new SetupError(
    'gcloud CLI not found',
    'Install gcloud CLI: https://cloud.google.com/sdk/docs/install',
    '#vida-dev Slack channel'
  );
}
```

### 8. UI Components (src/ui/)

Visual feedback and progress indicators for better user experience.

**Components:**
- `progress.ts`: Ora spinner wrappers
- `formatters.ts`: Output formatting utilities

**Progress Indicator Usage:**
```typescript
import ora from 'ora';

async function installMcp(mcpName: string) {
  const spinner = ora(`Installing ${mcpName}...`).start();

  try {
    await npmInstall(mcpName);
    spinner.succeed(`${mcpName} installed successfully`);
  } catch (error) {
    spinner.fail(`${mcpName} installation failed`);
    throw error;
  }
}
```

## Data Flow

### Setup Workflow Data Flow

```
1. USER INITIATES SETUP
   │
   ├─→ Parse CLI args (--mcps, --agents, --resume)
   │
   └─→ Load previous state (if --resume)

2. PREREQUISITES VALIDATION
   │
   ├─→ Check Node.js version (parallel)
   ├─→ Check gcloud CLI (parallel)
   ├─→ Check gcloud auth (parallel)
   └─→ Check Claude Code (parallel)
   │
   └─→ If failures: Display actionable errors, exit

3. MCP SELECTION
   │
   ├─→ If --mcps provided: Use CLI args
   └─→ Else: Show interactive prompt
   │
   └─→ Selected MCPs stored in state

4. AGENT SELECTION
   │
   ├─→ If --agents provided: Use CLI args
   └─→ Else: Show interactive prompt
   │
   └─→ Selected agents stored in state

5. MCP INSTALLATION & AUTH (for each selected MCP)
   │
   ├─→ Google Docs:
   │   ├─→ Check if credentials.json exists
   │   ├─→ If not: Guide user through GCP Console
   │   ├─→ Run OAuth flow (browser redirect)
   │   ├─→ Exchange auth code for tokens
   │   └─→ Save tokens with 600 permissions
   │
   ├─→ Atlassian:
   │   └─→ Auto-configure (OAuth via mcp-remote)
   │
   ├─→ GitHub:
   │   ├─→ PAT or OAuth selection
   │   ├─→ Feature selection prompt
   │   └─→ Validate and store credentials
   │
   ├─→ Sequential Thinking:
   │   └─→ Auto-install via npx (no auth)
   │
   └─→ Playwright:
       └─→ Auto-install via npx (no auth)

6. CONFIG GENERATION
   │
   ├─→ For each MCP: Generate config section
   ├─→ Merge with existing config (if any)
   └─→ Validate final config

7. CONFIG WRITING (for each selected agent)
   │
   ├─→ Detect config location (new vs legacy)
   ├─→ Backup existing config (if present)
   ├─→ Write new config with 600 permissions
   ├─→ Verify write success
   └─→ If failure: Rollback from backup

8. CHEZMOI INTEGRATION (if detected)
   │
   ├─→ Detect chezmoi management
   ├─→ Prompt user: "Apply via chezmoi?"
   ├─→ If yes:
   │   ├─→ Create template in chezmoi source
   │   ├─→ Show diff (optional)
   │   └─→ Run chezmoi apply
   └─→ If no or failure: Show manual instructions

9. VERIFICATION (non-blocking)
   │
   ├─→ Run `claude mcp list`
   ├─→ Verify expected MCPs appear
   └─→ Display verification results

10. COMPLETE
    │
    ├─→ Display success summary
    ├─→ Show next steps
    └─→ Exit with code 0
```

### OAuth Token Lifecycle

```
1. INITIAL AUTHENTICATION
   │
   ├─→ Load credentials.json
   ├─→ Generate auth URL with scopes
   ├─→ Open browser to auth URL
   ├─→ User signs in and grants permissions
   ├─→ Browser redirects with auth code
   ├─→ User copies code, pastes in terminal
   ├─→ Exchange code for access + refresh tokens
   └─→ Save tokens to token.json (600 permissions)

2. TOKEN USAGE (by MCP server)
   │
   ├─→ Load token.json
   ├─→ Check access token expiry
   ├─→ If valid: Use access token for API calls
   └─→ If expired:
       ├─→ Use refresh token to get new access token
       ├─→ Update token.json with new access token
       └─→ Continue with API calls

3. TOKEN HEALTH MONITORING
   │
   ├─→ Check token.json exists
   ├─→ Validate token structure
   ├─→ Calculate TTL (time until expiration)
   └─→ Report status:
       ├─→ Healthy: >5min TTL
       ├─→ Degraded: 1-5min TTL
       └─→ Unhealthy: Expired or missing

4. TOKEN REVOCATION (user-initiated)
   │
   ├─→ User runs: mcp-wizard auth revoke
   ├─→ Delete token.json
   ├─→ Show instructions: Revoke in Google Account settings
   └─→ User confirms revocation complete
```

## Security Model

### Threat Model (STRIDE Analysis)

See [ADR: MCP Wizard Production Enhancements](docs/adr/mcp-wizard-production-enhancements.md) for comprehensive STRIDE threat analysis.

**Key Threats & Mitigations:**

1. **Spoofing (S1)**: Token theft
   - Mitigation: File permissions 600, revocation docs

2. **Tampering (T3)**: MCP config tampering
   - Mitigation: Path validation, checksum validation (v2)

3. **Information Disclosure (I1, I2)**: Credentials in git
   - Mitigation: .gitignore creation, git detection, warnings

4. **Information Disclosure (I3)**: Token logging
   - Mitigation: Token redaction, code review

5. **Elevation of Privilege (E2)**: System file modification
   - Mitigation: Path validation (all writes in ~/)

### Security Best Practices

**Token Storage:**
- Location: `~/mcp-servers/google-docs-mcp/token.json`
- Permissions: 600 (owner read/write only)
- No encryption at rest (rely on OS full-disk encryption)
- Never logged or exposed in error messages

**Credential Protection:**
- .gitignore creation during setup
- Git tracking detection with warnings
- Path sanitization to prevent traversal
- No credentials in environment variables

**Command Execution:**
- Use `execFile` not `shell` (prevents injection)
- Timeout handling on all external commands
- Input validation and sanitization

**File Operations:**
- Backup before modification
- Rollback on write failure
- Permission enforcement (600 for sensitive files)
- Path validation (reject ".." traversal)

## Plugin Architecture

### Adding a New MCP Plugin

**Step 1: Create plugin file**
```typescript
// src/mcps/my-mcp/index.ts
import { McpPlugin, DetectionResult, ConfigSection } from '../types';

export class MyMcpPlugin implements McpPlugin {
  name = 'my-mcp';
  displayName = 'My MCP';
  description = 'Access My Service';
  requiresAuth = true;

  async detect(): Promise<DetectionResult> {
    // Check if MCP is already installed
    const mcpDir = path.join(os.homedir(), 'mcp-servers', 'my-mcp');
    return {
      installed: await pathExists(mcpDir),
      built: await pathExists(path.join(mcpDir, 'dist', 'server.js')),
      authenticated: await pathExists(path.join(mcpDir, 'token.json'))
    };
  }

  async install(): Promise<void> {
    // Clone and build MCP server
    await cloneRepo('https://github.com/org/my-mcp.git');
    await npmInstall();
    await npmBuild();
  }

  async configure(): Promise<ConfigSection> {
    // Generate MCP config section
    return {
      MyMCP: {
        command: 'node',
        args: [path.join(os.homedir(), 'mcp-servers/my-mcp/dist/server.js')],
        env: {
          TOKEN_PATH: path.join(os.homedir(), 'mcp-servers/my-mcp/token.json')
        }
      }
    };
  }

  async authenticate(): Promise<void> {
    // Run OAuth or API key setup
    const token = await runOAuthFlow();
    await saveToken(token);
  }

  async verify(): Promise<VerificationResult> {
    // Test MCP server works
    const mcpList = await execCommand('claude mcp list');
    return {
      success: mcpList.includes('MyMCP'),
      message: 'MCP registered successfully'
    };
  }
}
```

**Step 2: Register plugin**
```typescript
// src/mcps/index.ts
import { MyMcpPlugin } from './my-mcp';

export const PLUGINS = [
  new GoogleDocsMcpPlugin(),
  new AtlassianMcpPlugin(),
  new GitHubMcpPlugin(),
  new MyMcpPlugin(), // Add here
];
```

**Step 3: Add tests**
```typescript
// src/mcps/my-mcp/__tests__/index.test.ts
describe('MyMcpPlugin', () => {
  test('detects existing installation', async () => {
    const plugin = new MyMcpPlugin();
    const result = await plugin.detect();
    expect(result.installed).toBe(true);
  });
});
```

## Testing Strategy

### Test Levels

**Unit Tests** (`tests/unit/`)
- Component isolation
- Mock external dependencies
- Fast execution (<100ms per test)
- Coverage target: >85%

**Integration Tests** (`tests/integration/`)
- Multi-component workflows
- Real file system operations (temp directories)
- Mock external APIs
- Execution time: <5s per test

**E2E Tests** (`tests/e2e/`)
- Full user workflows
- Real external services (dev environment)
- Slower execution (acceptable)
- Manual triggers (not CI)

### Test Coverage

**Overall**: 89.68% (target: ≥70%)

**By Component:**
- `health-cache.ts`: 100%
- `health-checks.ts`: 94%
- `validators/`: 92%
- `config/`: 88%
- `mcps/`: 85%

### Test Infrastructure

**Jest Configuration:**
```javascript
// jest.config.js
module.exports = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  collectCoverageFrom: [
    'src/**/*.ts',
    '!src/**/__tests__/**',
    '!src/types.ts'
  ],
  coverageThreshold: {
    global: {
      branches: 70,
      functions: 70,
      lines: 70,
      statements: 70
    }
  },
  testMatch: [
    '**/__tests__/**/*.test.ts',
    '**/tests/**/*.test.ts'
  ]
};
```

**Mocking Strategy:**
```typescript
// __tests__/mocks/ora.ts
export default jest.fn(() => ({
  start: jest.fn().mockReturnThis(),
  succeed: jest.fn().mockReturnThis(),
  fail: jest.fn().mockReturnThis(),
  stop: jest.fn().mockReturnThis(),
}));

// __tests__/mocks/inquirer.ts
export const prompt = jest.fn();
```

## Deployment

### Distribution

**Primary: npm Package**
```bash
# Install globally
npm install -g mcp-wizard

# Usage
mcp-wizard setup
```

**Package.json:**
```json
{
  "name": "mcp-wizard",
  "version": "0.1.0",
  "bin": {
    "mcp-wizard": "bin/mcp-wizard.js"
  },
  "files": [
    "dist/",
    "bin/",
    "README.md",
    "LICENSE"
  ]
}
```

**Build Process:**
```bash
# Build TypeScript
npm run build

# Run tests
npm test

# Package
npm pack

# Publish
npm publish
```

### Installation Methods

**Global Install** (Recommended):
```bash
npm install -g mcp-wizard
mcp-wizard setup
```

**npx** (No install):
```bash
npx mcp-wizard setup
```

**Local Development**:
```bash
git clone https://github.com/org/mcp-wizard.git
cd mcp-wizard
npm install
npm run build
npm link
mcp-wizard setup
```

### Environment Requirements

**Node.js:**
- Minimum: 18.0.0
- Recommended: 20.x LTS
- Maximum tested: 24.x

**Operating Systems:**
- Linux: Full support
- macOS: Full support
- Windows: Logic exists, untested (v2)

**Disk Space:**
- Tool: ~10MB
- Dependencies: ~50MB
- Playwright MCP (optional): ~300MB (Chromium browser)

## Appendix: File Structure

```
mcp-wizard/
├── bin/
│   └── mcp-wizard.js           # CLI entry point
├── src/
│   ├── commands/               # Command implementations
│   │   ├── setup.ts           # Main setup wizard (135 lines)
│   │   ├── health.ts          # Health check (220 lines)
│   │   ├── doctor.ts          # Diagnostics (300 lines)
│   │   ├── status.ts          # Status command
│   │   ├── validate.ts        # Validation command
│   │   └── config.ts          # Config management
│   ├── lib/                   # Core infrastructure
│   │   ├── validators/        # Prerequisite validation
│   │   ├── verifiers/         # Post-setup verification
│   │   ├── config/            # Config detection & writing
│   │   ├── health-checks.ts   # Health check logic (509 lines)
│   │   └── health-cache.ts    # Health check cache (88 lines)
│   ├── mcps/                  # MCP plugin system
│   │   ├── google-docs/       # Google Docs plugin
│   │   ├── atlassian/         # Atlassian plugin
│   │   ├── github/            # GitHub plugin
│   │   ├── sequential-thinking/ # Sequential Thinking plugin
│   │   ├── playwright/        # Playwright plugin
│   │   └── types.ts           # Plugin interface
│   ├── ui/                    # UI components
│   │   ├── progress.ts        # Progress indicators
│   │   └── formatters.ts      # Output formatting
│   ├── errors/                # Error handling
│   │   └── setup-error.ts     # SetupError class
│   └── index.ts               # Main exports
├── tests/
│   ├── unit/                  # Unit tests
│   ├── integration/           # Integration tests
│   └── e2e/                   # End-to-end tests
├── docs/
│   ├── adr/                   # Architecture Decision Records
│   ├── W0-project-charter.md  # Project charter
│   ├── D1-review-council-results.md
│   ├── D2-approach-selection.md
│   ├── D3-implementation-planning.md
│   ├── HEALTH-MONITORING.md   # Health monitoring guide
│   ├── ATLASSIAN-MCP.md       # Atlassian OAuth details
│   ├── CHEZMOI-INTEGRATION.md # Chezmoi guide
│   └── SESSIONSTART-HOOK.md   # Shell integration
├── package.json
├── tsconfig.json
├── jest.config.js
├── README.md
├── SPEC.md                    # Product specification
├── ARCHITECTURE.md            # This document
└── TROUBLESHOOTING.md         # User troubleshooting guide
```

## Appendix: Design Decisions

See [docs/adr/](docs/adr/) for detailed architecture decision records.

**Key Decisions:**

1. **Node.js over Bash**: Better OAuth libraries, JSON handling, cross-platform
2. **TypeScript**: Type safety, better IDE support, maintainability
3. **Plugin Architecture**: Extensibility for future MCP servers
4. **Manual OAuth Setup**: No programmatic GCP API available, enhanced guide acceptable
5. **Per-User OAuth**: Better security than shared service accounts
6. **File Permissions 600**: Industry standard for credential files
7. **No Encryption at Rest**: Rely on OS full-disk encryption
8. **5-Minute Health Cache**: Balance freshness with performance
9. **Chezmoi Detection**: Never auto-edit templates, show user what to add
10. **Parallel Validation**: 3.7x speedup over sequential checks

## Appendix: Performance Targets

| Operation | Target | Actual |
|-----------|--------|--------|
| Prerequisites check | <5s | 3.2s |
| Health check (cached) | <100ms | ~50ms |
| Health check (uncached) | <5s | 2-4s |
| Total setup time | <15min | 10-12min |
| Test suite | <30s | ~20s |
| Build time | <10s | ~5s |

## Appendix: Version History

- **v0.1.0** (Beta): Initial release with core features
- **v0.2.0** (Planned): GitHub MCP, repair command, enhanced errors
- **v1.0.0** (Planned GA): Windows support, comprehensive testing, production-ready
