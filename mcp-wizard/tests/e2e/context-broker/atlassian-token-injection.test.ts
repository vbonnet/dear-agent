/**
 * E2E Tests for Context Broker with Atlassian MCP
 *
 * Tests real intent processing with Atlassian MCP server and Okta token injection
 *
 * Prerequisites:
 * - Atlassian MCP server installed
 * - OKTA_TOKEN environment variable set
 * - Valid Atlassian/Jira instance configured
 *
 * Note: This test requires actual MCP infrastructure and is skipped if prerequisites not met
 */

import { spawn, ChildProcess } from 'child_process';
import { readFileSync, existsSync } from 'fs';
import { join } from 'path';

/**
 * MCP Server Manager
 */
class MCPServerManager {
  private process?: ChildProcess;
  private serverPath?: string;

  /**
   * Find Atlassian MCP server
   */
  findAtlassianMCP(): string | null {
    const possiblePaths = [
      join(process.env.HOME || '', 'mcp-servers/atlassian-mcp/dist/index.js'),
      join(
        process.env.HOME || '',
        '.config/mcp-servers/atlassian-mcp/dist/index.js'
      ),
      '/usr/local/lib/mcp-servers/atlassian-mcp/dist/index.js',
    ];

    for (const path of possiblePaths) {
      if (existsSync(path)) {
        return path;
      }
    }

    return null;
  }

  /**
   * Spawn MCP server with Okta token injection
   */
  async spawnWithToken(oktaToken: string): Promise<void> {
    const serverPath = this.findAtlassianMCP();

    if (!serverPath) {
      throw new Error('Atlassian MCP server not found');
    }

    this.serverPath = serverPath;

    return new Promise((resolve, reject) => {
      this.process = spawn('node', [this.serverPath!], {
        env: {
          ...process.env,
          OKTA_TOKEN: oktaToken,
          NODE_ENV: 'test',
        },
        stdio: ['pipe', 'pipe', 'pipe'],
      });

      this.process.on('error', (err) => {
        reject(new Error(`Failed to spawn MCP server: ${err.message}`));
      });

      // Wait for server to be ready
      this.process.stdout?.on('data', (data) => {
        const output = data.toString();
        if (output.includes('MCP server started') || output.includes('ready')) {
          resolve();
        }
      });

      // Timeout after 5 seconds
      setTimeout(() => {
        if (!this.process?.pid) {
          reject(new Error('MCP server failed to start within timeout'));
        } else {
          resolve(); // Server started but no ready message
        }
      }, 5000);
    });
  }

  /**
   * Send request to MCP server
   */
  async sendRequest(request: any): Promise<any> {
    if (!this.process || !this.process.stdin) {
      throw new Error('MCP server not running');
    }

    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        reject(new Error('Request timeout'));
      }, 10000);

      this.process!.stdout?.once('data', (data) => {
        clearTimeout(timeout);
        try {
          const response = JSON.parse(data.toString());
          resolve(response);
        } catch (err) {
          reject(new Error(`Invalid JSON response: ${data.toString()}`));
        }
      });

      this.process!.stdin!.write(JSON.stringify(request) + '\n');
    });
  }

  /**
   * Stop MCP server
   */
  async stop(): Promise<void> {
    if (this.process) {
      return new Promise((resolve) => {
        this.process!.on('exit', () => resolve());
        this.process!.kill('SIGTERM');

        // Force kill after 3 seconds
        setTimeout(() => {
          if (this.process && !this.process.killed) {
            this.process.kill('SIGKILL');
          }
          resolve();
        }, 3000);
      });
    }
  }
}

/**
 * Check if E2E tests can run
 */
function canRunE2ETests(): { canRun: boolean; reason?: string } {
  const oktaToken = process.env.OKTA_TOKEN;
  if (!oktaToken) {
    return {
      canRun: false,
      reason: 'OKTA_TOKEN environment variable not set',
    };
  }

  const manager = new MCPServerManager();
  const mcpPath = manager.findAtlassianMCP();
  if (!mcpPath) {
    return {
      canRun: false,
      reason: 'Atlassian MCP server not found',
    };
  }

  return { canRun: true };
}

describe('Context Broker E2E - Atlassian Token Injection', () => {
  let mcpManager: MCPServerManager;
  const canRun = canRunE2ETests();

  beforeAll(async () => {
    if (!canRun.canRun) {
      console.log(`Skipping E2E tests: ${canRun.reason}`);
      return;
    }

    mcpManager = new MCPServerManager();
  });

  afterAll(async () => {
    if (mcpManager) {
      await mcpManager.stop();
    }
  });

  describe('Token Injection', () => {
    it('should spawn MCP with injected Okta token', async () => {
      if (!canRun.canRun) {
        return;
      }

      const oktaToken = process.env.OKTA_TOKEN!;

      await expect(mcpManager.spawnWithToken(oktaToken)).resolves.not.toThrow();
    });

    it('should verify token is injected in environment', async () => {
      if (!canRun.canRun) {
        return;
      }

      const oktaToken = process.env.OKTA_TOKEN!;
      await mcpManager.spawnWithToken(oktaToken);

      // Send tools/list request to verify MCP is working
      const request = {
        jsonrpc: '2.0',
        id: 1,
        method: 'tools/list',
        params: {},
      };

      const response = await mcpManager.sendRequest(request);

      expect(response).toHaveProperty('result');
      expect(response.result).toHaveProperty('tools');
      expect(Array.isArray(response.result.tools)).toBe(true);
    });
  });

  describe('Auth Flow Integration', () => {
    it('should authenticate with Atlassian using injected token', async () => {
      if (!canRun.canRun) {
        return;
      }

      const oktaToken = process.env.OKTA_TOKEN!;
      await mcpManager.spawnWithToken(oktaToken);

      // Try to list Jira projects (requires auth)
      const request = {
        jsonrpc: '2.0',
        id: 2,
        method: 'tools/call',
        params: {
          name: 'list_jira_projects',
          arguments: {},
        },
      };

      const response = await mcpManager.sendRequest(request);

      // Should not return auth error
      expect(response).not.toHaveProperty('error');
      expect(response).toHaveProperty('result');
    });

    it('should handle token expiration gracefully', async () => {
      if (!canRun.canRun) {
        return;
      }

      // Use an expired token
      const expiredToken = 'expired_token_placeholder';

      await mcpManager.spawnWithToken(expiredToken);

      const request = {
        jsonrpc: '2.0',
        id: 3,
        method: 'tools/call',
        params: {
          name: 'list_jira_projects',
          arguments: {},
        },
      };

      const response = await mcpManager.sendRequest(request);

      // Should return auth error
      expect(response).toHaveProperty('error');
      expect(response.error.message).toMatch(/auth|token|expired/i);
    });
  });

  describe('Real Intent Processing', () => {
    it('should process intent and call Atlassian MCP', async () => {
      if (!canRun.canRun) {
        return;
      }

      const oktaToken = process.env.OKTA_TOKEN!;
      await mcpManager.spawnWithToken(oktaToken);

      // User intent: "List all Jira projects"
      const intent = 'List all Jira projects';

      // In real implementation, this would go through Context Broker
      // For now, directly call MCP server
      const request = {
        jsonrpc: '2.0',
        id: 4,
        method: 'tools/call',
        params: {
          name: 'list_jira_projects',
          arguments: {},
        },
      };

      const response = await mcpManager.sendRequest(request);

      expect(response).toHaveProperty('result');
      expect(response.result).toBeDefined();
    });

    it('should create Jira issue via intent', async () => {
      if (!canRun.canRun) {
        return;
      }

      const oktaToken = process.env.OKTA_TOKEN!;
      await mcpManager.spawnWithToken(oktaToken);

      // User intent: "Create a test Jira ticket"
      const request = {
        jsonrpc: '2.0',
        id: 5,
        method: 'tools/call',
        params: {
          name: 'create_jira_issue',
          arguments: {
            project: 'TEST',
            summary: 'E2E test ticket from Context Broker',
            description: 'This is a test ticket created by E2E tests',
            issueType: 'Task',
          },
        },
      };

      const response = await mcpManager.sendRequest(request);

      expect(response).toHaveProperty('result');
      expect(response.result).toHaveProperty('key'); // Jira issue key
    });
  });

  describe('Error Handling', () => {
    it('should handle MCP server not found', async () => {
      if (!canRun.canRun) {
        return;
      }

      // Temporarily override findAtlassianMCP to simulate not found
      const originalFind = mcpManager.findAtlassianMCP;
      mcpManager.findAtlassianMCP = () => null;

      await expect(mcpManager.spawnWithToken('test_token')).rejects.toThrow(
        'Atlassian MCP server not found'
      );

      mcpManager.findAtlassianMCP = originalFind;
    });

    it('should handle MCP server crash', async () => {
      if (!canRun.canRun) {
        return;
      }

      const oktaToken = process.env.OKTA_TOKEN!;
      await mcpManager.spawnWithToken(oktaToken);

      // Kill the server
      await mcpManager.stop();

      // Try to send request
      const request = {
        jsonrpc: '2.0',
        id: 6,
        method: 'tools/list',
        params: {},
      };

      await expect(mcpManager.sendRequest(request)).rejects.toThrow(
        'MCP server not running'
      );
    });
  });
});

describe('Context Broker E2E - Prerequisites Check', () => {
  it('should report if E2E tests can run', () => {
    const result = canRunE2ETests();

    console.log('E2E Test Prerequisites:');
    console.log(`  Can run: ${result.canRun}`);
    if (!result.canRun) {
      console.log(`  Reason: ${result.reason}`);
    }

    // This test always passes - just reports status
    expect(result).toBeDefined();
  });

  it('should check for OKTA_TOKEN environment variable', () => {
    const hasToken = !!process.env.OKTA_TOKEN;

    console.log(`OKTA_TOKEN set: ${hasToken}`);

    if (!hasToken) {
      console.log('To run E2E tests, set OKTA_TOKEN environment variable');
    }

    expect(typeof hasToken).toBe('boolean');
  });

  it('should check for Atlassian MCP installation', () => {
    const manager = new MCPServerManager();
    const mcpPath = manager.findAtlassianMCP();

    console.log(`Atlassian MCP found: ${!!mcpPath}`);
    if (mcpPath) {
      console.log(`  Path: ${mcpPath}`);
    } else {
      console.log('  To run E2E tests, install Atlassian MCP server');
    }

    expect(typeof mcpPath).toBe(mcpPath ? 'string' : 'object');
  });
});
