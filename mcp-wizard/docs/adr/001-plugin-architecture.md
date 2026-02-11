# ADR 001: Plugin Architecture for MCP Server Support

**Date:** 2025-12-04
**Status:** Accepted
**Deciders:** Engineering team
**Related:** W0-project-charter.md, D2-approach-selection.md

## Context

MCP Wizard needs to support multiple MCP servers (Google Docs, Atlassian, GitHub, Sequential Thinking, Playwright) with different authentication methods, installation processes, and configuration requirements. We need an architecture that:

1. Allows easy addition of new MCP servers without modifying core wizard code
2. Supports diverse authentication patterns (OAuth, PAT, API keys, zero-config)
3. Maintains code organization and testability
4. Provides consistent user experience across different MCPs

## Decision

We will implement a **plugin architecture** where each MCP server is represented by a plugin class implementing a common interface.

### Plugin Interface

```typescript
interface McpPlugin {
  // Metadata
  name: string;                    // "google-docs", "atlassian", etc.
  displayName: string;             // "Google Docs MCP"
  description: string;             // User-facing description
  requiresAuth: boolean;           // Whether authentication is needed

  // Lifecycle hooks
  detect(): Promise<DetectionResult>;      // Check if already installed
  install?(): Promise<void>;               // Clone/build MCP server (optional)
  configure(): Promise<ConfigSection>;     // Generate MCP config JSON
  authenticate?(): Promise<void>;          // Run OAuth flow (optional)
  verify(): Promise<VerificationResult>;   // Test MCP works
  repair?(): Promise<void>;                // Fix broken setup (optional)
}

interface DetectionResult {
  installed: boolean;              // MCP server files exist
  built: boolean;                  // MCP server is built (dist/ exists)
  authenticated: boolean;          // Credentials/tokens present
  version?: string;                // Installed version (optional)
}

interface ConfigSection {
  [mcpName: string]: {
    command: string;               // Command to run MCP server
    args: string[];                // Command arguments
    env?: Record<string, string>;  // Environment variables
  };
}

interface VerificationResult {
  success: boolean;                // Verification passed
  message: string;                 // Success/failure message
  details?: any;                   // Additional details (optional)
}
```

### Plugin Registry

```typescript
// src/mcps/index.ts
import { GoogleDocsMcpPlugin } from './google-docs';
import { AtlassianMcpPlugin } from './atlassian';
import { GitHubMcpPlugin } from './github';
import { SequentialThinkingMcpPlugin } from './sequential-thinking';
import { PlaywrightMcpPlugin } from './playwright';

export const PLUGINS: McpPlugin[] = [
  new GoogleDocsMcpPlugin(),
  new AtlassianMcpPlugin(),
  new GitHubMcpPlugin(),
  new SequentialThinkingMcpPlugin(),
  new PlaywrightMcpPlugin(),
];

export function getPlugin(name: string): McpPlugin | undefined {
  return PLUGINS.find(p => p.name === name);
}
```

### Plugin Organization

```
src/mcps/
  ├── index.ts              # Plugin registry
  ├── types.ts              # Plugin interfaces
  ├── google-docs/
  │   ├── index.ts         # GoogleDocsMcpPlugin
  │   ├── oauth.ts         # OAuth flow logic
  │   └── __tests__/
  ├── atlassian/
  │   ├── index.ts         # AtlassianMcpPlugin
  │   └── __tests__/
  ├── github/
  │   ├── index.ts         # GitHubMcpPlugin
  │   ├── auth.ts          # PAT and OAuth logic
  │   └── __tests__/
  ├── sequential-thinking/
  │   ├── index.ts         # SequentialThinkingMcpPlugin
  │   └── __tests__/
  └── playwright/
      ├── index.ts         # PlaywrightMcpPlugin
      └── __tests__/
```

## Rationale

### Why Plugin Architecture?

**Extensibility:**
- New MCP servers can be added without modifying setup wizard
- Each plugin is self-contained with its own logic
- Future MCPs (Slack, Notion, Linear) can be added by community

**Maintainability:**
- Clear separation of concerns (one plugin = one MCP)
- Each plugin can be tested independently
- Changes to one MCP don't affect others

**Flexibility:**
- Plugins can implement only relevant lifecycle hooks
- Zero-config MCPs (Sequential Thinking, Playwright) skip `authenticate()`
- Remote-auth MCPs (Atlassian) skip `authenticate()` locally

**Consistency:**
- Common interface ensures consistent user experience
- Setup wizard orchestrates plugins in same way
- Error handling and progress indicators work uniformly

### Alternative: Monolithic Setup

**Rejected Approach:**
```typescript
// src/setup.ts
async function setupMcp(mcpName: string) {
  if (mcpName === 'google-docs') {
    // Google Docs specific logic (50 lines)
  } else if (mcpName === 'atlassian') {
    // Atlassian specific logic (30 lines)
  } else if (mcpName === 'github') {
    // GitHub specific logic (60 lines)
  }
  // ... 10 more MCPs = 500 lines in one file
}
```

**Why Rejected:**
- Single file becomes unmaintainable (>500 lines)
- Changes to one MCP risk breaking others
- Cannot test MCPs in isolation
- Difficult to add community-contributed MCPs

### Alternative: Configuration-Driven

**Rejected Approach:**
```json
// mcp-configs.json
{
  "google-docs": {
    "install": "git clone ...",
    "auth": "oauth",
    "config": { ... }
  }
}
```

**Why Rejected:**
- JSON lacks flexibility for complex logic (OAuth flows, error handling)
- Cannot express conditional behavior (PAT vs OAuth)
- Limited error handling capabilities
- Difficult to test without code

## Consequences

### Positive

✅ **Easy to extend**: Adding new MCP is ~100 lines in new file
✅ **Testable**: Each plugin tested independently with mocks
✅ **Maintainable**: Changes isolated to single plugin
✅ **Flexible**: Plugins implement only what they need
✅ **Consistent**: Common interface enforces uniform UX

### Negative

⚠️ **Learning curve**: Contributors need to understand plugin interface
⚠️ **Abstraction overhead**: Simple MCPs have boilerplate
⚠️ **Interface evolution**: Changes to interface affect all plugins

### Mitigations

**For learning curve:**
- Provide plugin template with comments
- Document plugin development in CONTRIBUTING.md
- Examples: SequentialThinkingMcpPlugin (simple), GoogleDocsMcpPlugin (complex)

**For abstraction overhead:**
- Optional lifecycle hooks (install, authenticate, repair)
- Base class with common utilities (future enhancement)
- Helper functions for common tasks

**For interface evolution:**
- Keep interface minimal (6 methods, 3 optional)
- Deprecate old methods before removing
- Provide migration guide for breaking changes

## Implementation Details

### Example: Google Docs Plugin

```typescript
// src/mcps/google-docs/index.ts
export class GoogleDocsMcpPlugin implements McpPlugin {
  name = 'google-docs';
  displayName = 'Google Docs MCP';
  description = 'Access Google Drive and Docs';
  requiresAuth = true;

  async detect(): Promise<DetectionResult> {
    const mcpDir = path.join(os.homedir(), 'mcp-servers', 'google-docs-mcp');
    const credentialsPath = path.join(mcpDir, 'credentials.json');
    const tokenPath = path.join(mcpDir, 'token.json');

    return {
      installed: await pathExists(mcpDir),
      built: await pathExists(path.join(mcpDir, 'dist', 'server.js')),
      authenticated: await pathExists(credentialsPath) && await pathExists(tokenPath)
    };
  }

  async install(): Promise<void> {
    const mcpDir = path.join(os.homedir(), 'mcp-servers', 'google-docs-mcp');
    await cloneRepo('https://github.com/a-bonus/google-docs-mcp.git', mcpDir);
    await npmInstall(mcpDir);
    await npmBuild(mcpDir);
  }

  async authenticate(): Promise<void> {
    // Guide user through GCP Console
    await guideOAuthSetup();
    // Run OAuth flow
    const oauth2Client = await createOAuthClient();
    const authUrl = oauth2Client.generateAuthUrl({ scope: SCOPES });
    await openBrowser(authUrl);
    const code = await promptForCode();
    const { tokens } = await oauth2Client.getToken(code);
    await saveTokens(tokens);
  }

  async configure(): Promise<ConfigSection> {
    const mcpDir = path.join(os.homedir(), 'mcp-servers', 'google-docs-mcp');
    return {
      GoogleDocs: {
        command: 'node',
        args: [path.join(mcpDir, 'dist', 'server.js')],
        env: {
          CREDENTIALS_PATH: path.join(mcpDir, 'credentials.json'),
          TOKEN_PATH: path.join(mcpDir, 'token.json')
        }
      }
    };
  }

  async verify(): Promise<VerificationResult> {
    const mcpList = await execCommand('claude mcp list');
    const success = mcpList.includes('GoogleDocs');
    return {
      success,
      message: success ? 'Google Docs MCP registered' : 'Google Docs MCP not found'
    };
  }
}
```

### Example: Sequential Thinking Plugin (Zero-Config)

```typescript
// src/mcps/sequential-thinking/index.ts
export class SequentialThinkingMcpPlugin implements McpPlugin {
  name = 'sequential-thinking';
  displayName = 'Sequential Thinking MCP';
  description = 'Enhanced reasoning with structured thinking';
  requiresAuth = false;

  async detect(): Promise<DetectionResult> {
    // Always returns not installed (npx handles it)
    return { installed: false, built: true, authenticated: true };
  }

  // No install() - npx handles it
  // No authenticate() - no auth required

  async configure(): Promise<ConfigSection> {
    return {
      SequentialThinking: {
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-sequential-thinking'],
        env: {
          DISABLE_THOUGHT_LOGGING: 'false'
        }
      }
    };
  }

  async verify(): Promise<VerificationResult> {
    const mcpList = await execCommand('claude mcp list');
    const success = mcpList.includes('SequentialThinking');
    return {
      success,
      message: success ? 'Sequential Thinking MCP registered' : 'Sequential Thinking MCP not found'
    };
  }
}
```

## Testing Strategy

### Unit Tests

Each plugin tested in isolation with mocks:

```typescript
// src/mcps/google-docs/__tests__/index.test.ts
describe('GoogleDocsMcpPlugin', () => {
  let plugin: GoogleDocsMcpPlugin;

  beforeEach(() => {
    plugin = new GoogleDocsMcpPlugin();
  });

  test('detect returns correct status for installed MCP', async () => {
    mockPathExists({ 'mcp-servers/google-docs-mcp': true });
    const result = await plugin.detect();
    expect(result.installed).toBe(true);
  });

  test('configure returns valid config section', async () => {
    const config = await plugin.configure();
    expect(config.GoogleDocs).toBeDefined();
    expect(config.GoogleDocs.command).toBe('node');
  });

  test('authenticate saves tokens with correct permissions', async () => {
    mockOAuthFlow({ tokens: { access_token: 'test' } });
    await plugin.authenticate();
    expect(fs.chmodSync).toHaveBeenCalledWith(expect.any(String), 0o600);
  });
});
```

### Integration Tests

Test plugin orchestration by setup wizard:

```typescript
// tests/integration/plugin-orchestration.test.ts
describe('Plugin Orchestration', () => {
  test('setup wizard runs all lifecycle hooks in order', async () => {
    const plugin = new GoogleDocsMcpPlugin();
    const spy = jest.spyOn(plugin, 'detect');
    const spy2 = jest.spyOn(plugin, 'install');
    const spy3 = jest.spyOn(plugin, 'configure');

    await setupWizard(['google-docs']);

    expect(spy).toHaveBeenCalledBefore(spy2);
    expect(spy2).toHaveBeenCalledBefore(spy3);
  });
});
```

## Future Enhancements

### Base Plugin Class

Provide common utilities to reduce boilerplate:

```typescript
abstract class BaseMcpPlugin implements McpPlugin {
  abstract name: string;
  abstract displayName: string;
  abstract description: string;
  abstract requiresAuth: boolean;

  // Common utilities
  protected getMcpDir(): string {
    return path.join(os.homedir(), 'mcp-servers', this.name);
  }

  protected async pathExists(filePath: string): Promise<boolean> {
    try {
      await fs.access(filePath);
      return true;
    } catch {
      return false;
    }
  }

  protected async setFilePermissions(filePath: string, mode: number): Promise<void> {
    await fs.chmod(filePath, mode);
  }

  // Default implementations
  async verify(): Promise<VerificationResult> {
    const mcpList = await execCommand('claude mcp list');
    const success = mcpList.includes(this.displayName);
    return {
      success,
      message: success ? `${this.displayName} registered` : `${this.displayName} not found`
    };
  }
}
```

### Plugin Lifecycle Hooks

Add additional lifecycle hooks for advanced scenarios:

```typescript
interface McpPlugin {
  // ... existing hooks ...

  // Optional advanced hooks
  onPreInstall?(): Promise<void>;     // Before installation
  onPostInstall?(): Promise<void>;    // After installation
  onPreAuth?(): Promise<void>;        // Before authentication
  onPostAuth?(): Promise<void>;       // After authentication
  onPreConfigure?(): Promise<void>;   // Before config generation
  onPostConfigure?(): Promise<void>;  // After config writing
}
```

### Plugin Dependencies

Allow plugins to declare dependencies on other plugins:

```typescript
interface McpPlugin {
  // ... existing fields ...
  dependencies?: string[];  // Other plugins required (e.g., ['google-docs'])
}
```

## References

- [D2: Approach Selection](../D2-approach-selection.md) - Initial plugin architecture proposal
- [D3: Implementation Planning](../D3-implementation-planning.md) - Plugin interface specification
- [ADR: Production Enhancements](mcp-wizard-production-enhancements.md) - Prerequisites validation for plugins

## Changelog

- **2025-12-04**: Initial ADR - Plugin architecture decision
- **2026-02-11**: Updated with actual implementation details and test coverage
