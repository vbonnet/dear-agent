/**
 * Test fixtures for migration wizard tests
 */

import { ClaudeConfig, MCPServerConfig } from '../../../../src/cli/migrate/types';

export const FIXTURES = {
  /**
   * Valid MCP config with multiple servers
   */
  validMcpConfig: {
    mcpServers: {
      googledocs: {
        command: 'npx',
        args: ['-y', '@mcp/server-gdrive'],
      },
      slack: {
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-slack'],
      },
      filesystem: {
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-filesystem', '/Users/username/Documents'],
      },
    },
  } as ClaudeConfig,

  /**
   * Empty config (no MCP servers)
   */
  emptyConfig: {} as ClaudeConfig,

  /**
   * Config with non-MCP sections (should be preserved)
   */
  configWithNonMcpSections: {
    mcpServers: {
      googledocs: {
        command: 'npx',
        args: ['-y', '@mcp/server-gdrive'],
      },
    },
    otherSettings: {
      theme: 'dark',
      fontSize: 14,
    },
    customData: 'preserve this',
  } as ClaudeConfig,

  /**
   * Config with environment variables
   */
  configWithEnvVars: {
    mcpServers: {
      'google-drive': {
        command: 'npx',
        args: ['-y', '@mcp/server-gdrive'],
        env: {
          GOOGLE_DRIVE_TOKEN: 'test-token-123',
          GOOGLE_DRIVE_REFRESH: 'refresh-token-456',
        },
      },
      slack: {
        command: 'npx',
        args: ['-y', '@mcp/server-slack'],
        env: {
          SLACK_BOT_TOKEN: 'xoxb-test-token',
        },
      },
    },
  } as ClaudeConfig,

  /**
   * Config with single MCP server
   */
  singleServerConfig: {
    mcpServers: {
      filesystem: {
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-filesystem', '/tmp'],
      },
    },
  } as ClaudeConfig,

  /**
   * Malformed JSON string (for error testing)
   */
  malformedJson: '{ "mcpServers": { invalid json } }',

  /**
   * Already migrated config (has mcp-wizard gateway)
   */
  migratedConfig: {
    mcpServers: {
      'mcp-wizard': {
        command: 'mcp-wizard',
        args: ['serve'],
      },
    },
  } as ClaudeConfig,
};

/**
 * Generate mock backup filename with timestamp
 */
export function mockBackupFilename(timestamp: Date): string {
  const formatted = timestamp.toISOString()
    .replace(/:/g, '-')
    .replace(/\..+/, '')
    .replace('T', '-');
  return `claude_desktop_config.json.backup-${formatted}`;
}

/**
 * Create mock fs error with specific code
 */
export function createFsError(code: string, message: string): NodeJS.ErrnoException {
  const error = new Error(message) as NodeJS.ErrnoException;
  error.code = code;
  return error;
}
