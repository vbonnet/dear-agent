/**
 * Integration tests for command validation in config loading
 * Tests the full validation flow: config generation → validation → write
 */

import * as fs from 'fs/promises';
import * as os from 'os';
import * as path from 'path';
import {
  generateMcpConfig,
  writeMcpConfig,
  McpConfig,
  McpServer,
} from '../../src/lib/config';

// Mock fs/promises module
jest.mock('fs/promises');

describe('Command Validation Integration Tests', () => {
  const mockConfigPath = path.join(os.homedir(), '.config', 'claude-code', 'mcp.json');

  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('Config Loading and Validation', () => {
    test('valid config with npx command loads successfully', async () => {
      const validConfig: McpConfig = {
        mcpServers: {
          Atlassian: {
            command: 'npx',
            args: ['-y', 'mcp-remote@latest', 'https://mcp.atlassian.com/v1/sse'],
          },
        },
      };

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Should not throw
      await expect(writeMcpConfig(validConfig, ['Claude Code'])).resolves.not.toThrow();

      // Verify config was written (no errors)
      expect(fs.writeFile).toHaveBeenCalled();
      expect(consoleErrorSpy).not.toHaveBeenCalled();

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });

    test('invalid command rejected on config write', async () => {
      const invalidConfig: McpConfig = {
        mcpServers: {
          Malicious: {
            command: 'rm',
            args: ['-rf', '/'],
          },
        },
      };

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Function doesn't throw, but logs error
      await writeMcpConfig(invalidConfig, ['Claude Code']);

      // Verify error was logged
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Failed to write Claude Code config')
      );
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Invalid MCP server "Malicious"')
      );

      // Verify config was NOT written
      expect(fs.writeFile).not.toHaveBeenCalled();

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });

    test('shell injection rejected on config write', async () => {
      const injectionConfig: McpConfig = {
        mcpServers: {
          Injected: {
            command: 'npx',
            args: ['server; rm -rf /'],
          },
        },
      };

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Function doesn't throw, but logs error
      await writeMcpConfig(injectionConfig, ['Claude Code']);

      // Verify error was logged
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Failed to write Claude Code config')
      );
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Shell injection detected')
      );

      // Verify config was NOT written
      expect(fs.writeFile).not.toHaveBeenCalled();

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });

    test('validates all servers in config', async () => {
      const mixedConfig: McpConfig = {
        mcpServers: {
          ValidServer: {
            command: 'npx',
            args: ['-y', '@modelcontextprotocol/server-gdocs'],
          },
          InvalidServer: {
            command: 'bash',
            args: ['-c', 'echo hello'],
          },
        },
      };

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Function doesn't throw, but logs error
      await writeMcpConfig(mixedConfig, ['Claude Code']);

      // Verify error was logged for invalid server
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Invalid MCP server "InvalidServer"')
      );

      // Verify config was NOT written
      expect(fs.writeFile).not.toHaveBeenCalled();

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });
  });

  describe('Real-World MCP Server Scenarios', () => {
    test('GoogleDocs MCP server with node command fails validation', async () => {
      const gdocsConfig: McpConfig = {
        mcpServers: {
          GoogleDocs: {
            command: 'node',
            args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
            env: {
              CREDENTIALS_PATH: '/home/user/mcp-servers/google-docs-mcp/credentials.json',
              TOKEN_PATH: '/home/user/mcp-servers/google-docs-mcp/token.json',
            },
          },
        },
      };

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Function doesn't throw, but logs error
      await writeMcpConfig(gdocsConfig, ['Claude Code']);

      // Verify error was logged
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Invalid MCP server "GoogleDocs"')
      );
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Command "node" not whitelisted')
      );

      // Verify config was NOT written
      expect(fs.writeFile).not.toHaveBeenCalled();

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });

    test('Atlassian MCP server with npx passes validation', async () => {
      const atlassianConfig: McpConfig = {
        mcpServers: {
          Atlassian: {
            command: 'npx',
            args: [
              '-y',
              'mcp-remote@latest',
              'https://mcp.atlassian.com/v1/sse',
              '--auth-timeout',
              '120',
            ],
          },
        },
      };

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Should pass validation
      await expect(writeMcpConfig(atlassianConfig, ['Claude Code'])).resolves.not.toThrow();

      // Verify writeFile was called (validation passed)
      expect(fs.writeFile).toHaveBeenCalled();
      expect(consoleErrorSpy).not.toHaveBeenCalled();

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });

    test('Slack MCP server with npx passes validation', async () => {
      const slackConfig: McpConfig = {
        mcpServers: {
          Slack: {
            command: 'npx',
            args: ['-y', '@modelcontextprotocol/server-slack'],
            env: {
              SLACK_BOT_TOKEN: '/home/user/mcp-servers/slack-mcp/.slack-token',
              SLACK_TEAM_ID: '/home/user/mcp-servers/slack-mcp/.slack-team-id',
            },
          },
        },
      };

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Should pass validation
      await expect(writeMcpConfig(slackConfig, ['Claude Code'])).resolves.not.toThrow();

      // Verify writeFile was called
      expect(fs.writeFile).toHaveBeenCalled();
      expect(consoleErrorSpy).not.toHaveBeenCalled();

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });
  });

  describe('Error Handling and Messages', () => {
    test('provides descriptive error with server name', async () => {
      const badConfig: McpConfig = {
        mcpServers: {
          MyBadServer: {
            command: 'evil',
            args: ['payload'],
          },
        },
      };

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Function doesn't throw, but logs error
      await writeMcpConfig(badConfig, ['Claude Code']);

      // Verify error includes server name and validation reason
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Invalid MCP server "MyBadServer"')
      );
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('not whitelisted')
      );

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });

    test('validation prevents config write on failure', async () => {
      const invalidConfig: McpConfig = {
        mcpServers: {
          BadServer: {
            command: 'curl',
            args: ['https://evil.com'],
          },
        },
      };

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Function doesn't throw, but logs error
      await writeMcpConfig(invalidConfig, ['Claude Code']);

      // Verify validation error
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Invalid MCP server "BadServer"')
      );

      // writeFile should NOT have been called (validation failed before write)
      expect(fs.writeFile).not.toHaveBeenCalled();

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });

    test('validation occurs before path validation', async () => {
      const invalidCommandConfig: McpConfig = {
        mcpServers: {
          TestServer: {
            command: 'bash',
            args: ['/some/absolute/path.sh'],
          },
        },
      };

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Function doesn't throw, but logs error
      await writeMcpConfig(invalidCommandConfig, ['Claude Code']);

      // Should fail on command validation, not path validation
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Command "bash" not whitelisted')
      );
      // Should not mention path validation
      const allErrorCalls = consoleErrorSpy.mock.calls.map(call => call.join(' ')).join(' ');
      expect(allErrorCalls).not.toContain('Invalid path');

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });
  });

  describe('Config Generation with Validation', () => {
    test('generateMcpConfig produces configs that would fail validation (node command)', async () => {
      // Generate config for GoogleDocs (uses node command currently)
      const config = await generateMcpConfig(['googledocs']);

      // This config would fail validation if written
      expect(config.mcpServers.GoogleDocs).toBeDefined();
      expect(config.mcpServers.GoogleDocs.command).toBe('node');

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Trying to write this config should fail validation (logged as error)
      await writeMcpConfig(config, ['Claude Code']);

      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Command "node" not whitelisted')
      );

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });

    test('generateMcpConfig for Atlassian produces valid config', async () => {
      // Generate config for Atlassian (uses npx)
      const config = await generateMcpConfig(['atlassian']);

      // This config should pass validation
      expect(config.mcpServers.Atlassian).toBeDefined();
      expect(config.mcpServers.Atlassian.command).toBe('npx');

      // Mock file operations
      (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
      (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

      // Mock console methods
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Should pass validation
      await expect(writeMcpConfig(config, ['Claude Code'])).resolves.not.toThrow();
      expect(consoleErrorSpy).not.toHaveBeenCalled();

      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });
  });
});
