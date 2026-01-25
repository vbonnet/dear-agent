/**
 * Unit tests for command whitelist and validation
 * Tests security-critical code for preventing arbitrary command execution
 */

import {
  validateMcpCommand,
  McpServer,
  isWhitelistedPackage,
  extractPackageName,
} from '../../src/lib/command-whitelist';

describe('validateMcpCommand', () => {
  describe('Command Whitelist Validation', () => {
    test('allows npx command', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-gdocs'],
      };

      expect(() => validateMcpCommand(config)).not.toThrow();
    });

    test('blocks rm command', () => {
      const config: McpServer = {
        command: 'rm',
        args: ['-rf', '/'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command "rm" not whitelisted. Only "npx" is allowed.'
      );
    });

    test('blocks sh command', () => {
      const config: McpServer = {
        command: 'sh',
        args: ['-c', 'echo hello'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command "sh" not whitelisted. Only "npx" is allowed.'
      );
    });

    test('blocks bash command', () => {
      const config: McpServer = {
        command: 'bash',
        args: ['-c', 'echo hello'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command "bash" not whitelisted. Only "npx" is allowed.'
      );
    });

    test('blocks absolute path commands', () => {
      const config: McpServer = {
        command: '/usr/bin/node',
        args: ['script.js'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command "/usr/bin/node" not whitelisted. Only "npx" is allowed.'
      );
    });

    test('blocks node command', () => {
      const config: McpServer = {
        command: 'node',
        args: ['script.js'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command "node" not whitelisted. Only "npx" is allowed.'
      );
    });

    test('blocks python command', () => {
      const config: McpServer = {
        command: 'python',
        args: ['script.py'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command "python" not whitelisted. Only "npx" is allowed.'
      );
    });
  });

  describe('Shell Injection Detection', () => {
    test('detects semicolon command separator', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['server; rm -rf /'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Shell injection detected in args: "server; rm -rf /"'
      );
    });

    test('detects pipe operator', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['server | tee log.txt'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Shell injection detected in args: "server | tee log.txt"'
      );
    });

    test('detects ampersand background execution', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['server &'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Shell injection detected in args: "server &"'
      );
    });

    test('detects output redirection', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['server > output.txt'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Shell injection detected in args: "server > output.txt"'
      );
    });

    test('detects input redirection', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['server < input.txt'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Shell injection detected in args: "server < input.txt"'
      );
    });

    test('detects backtick command substitution', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['server `whoami`'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Shell injection detected in args: "server `whoami`"'
      );
    });

    test('detects $(...) command substitution', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['server $(whoami)'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Shell injection detected in args: "server $(whoami)"'
      );
    });

    test('detects logical OR operator', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['server || echo failed'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Shell injection detected in args: "server || echo failed"'
      );
    });

    test('detects logical AND operator', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['server && echo success'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Shell injection detected in args: "server && echo success"'
      );
    });

    test('detects newline separator', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['server\nrm -rf /'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Shell injection detected in args: "server\nrm -rf /"'
      );
    });

    test('allows safe arguments without injection', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-gdocs', '--port', '3000'],
      };

      expect(() => validateMcpCommand(config)).not.toThrow();
    });

    test('allows URLs in arguments', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['-y', 'mcp-remote@latest', 'https://mcp.atlassian.com/v1/sse'],
      };

      expect(() => validateMcpCommand(config)).not.toThrow();
    });
  });

  describe('Edge Cases and Error Handling', () => {
    test('rejects empty command', () => {
      const config: McpServer = {
        command: '',
        args: ['test'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command must be a non-empty string'
      );
    });

    test('rejects null command', () => {
      const config = {
        command: null as any,
        args: ['test'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command must be a non-empty string'
      );
    });

    test('rejects undefined command', () => {
      const config = {
        command: undefined as any,
        args: ['test'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command must be a non-empty string'
      );
    });

    test('rejects non-string command', () => {
      const config = {
        command: 123 as any,
        args: ['test'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command must be a non-empty string'
      );
    });

    test('rejects non-array args', () => {
      const config = {
        command: 'npx',
        args: 'not-an-array' as any,
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Args must be an array'
      );
    });

    test('rejects null args', () => {
      const config = {
        command: 'npx',
        args: null as any,
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Args must be an array'
      );
    });

    test('allows empty args array', () => {
      const config: McpServer = {
        command: 'npx',
        args: [],
      };

      expect(() => validateMcpCommand(config)).not.toThrow();
    });

    test('handles args with special characters in package names', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['-y', '@org/package-name'],
      };

      expect(() => validateMcpCommand(config)).not.toThrow();
    });

    test('handles multiple flags and arguments', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['-y', '--quiet', '@package/name', '--option', 'value'],
      };

      expect(() => validateMcpCommand(config)).not.toThrow();
    });
  });

  describe('Real-World MCP Server Configs', () => {
    test('validates GoogleDocs MCP server (node command - should fail)', () => {
      const config: McpServer = {
        command: 'node',
        args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command "node" not whitelisted'
      );
    });

    test('validates Atlassian MCP server (npx - should pass)', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['-y', 'mcp-remote@latest', 'https://mcp.atlassian.com/v1/sse', '--auth-timeout', '120'],
      };

      expect(() => validateMcpCommand(config)).not.toThrow();
    });

    test('validates Glean MCP server (npx - should pass)', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['-y', '@gleanwork/mcp-server'],
        env: {
          GLEAN_INSTANCE: '[REDACTED_EMPLOYER]',
          GLEAN_API_TOKEN: '/home/user/mcp-servers/glean-mcp/.glean-token',
        },
      };

      expect(() => validateMcpCommand(config)).not.toThrow();
    });

    test('validates Slack MCP server (npx - should pass)', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-slack'],
        env: {
          SLACK_BOT_TOKEN: '/home/user/mcp-servers/slack-mcp/.slack-token',
        },
      };

      expect(() => validateMcpCommand(config)).not.toThrow();
    });
  });

  describe('Error Message Quality', () => {
    test('provides clear error for blocked command', () => {
      const config: McpServer = {
        command: 'curl',
        args: ['https://evil.com'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Command "curl" not whitelisted. Only "npx" is allowed.'
      );
    });

    test('provides clear error for shell injection with full args', () => {
      const config: McpServer = {
        command: 'npx',
        args: ['malicious; rm -rf /'],
      };

      expect(() => validateMcpCommand(config)).toThrow(
        'Shell injection detected in args: "malicious; rm -rf /"'
      );
    });

    test('includes actual command in error message', () => {
      const config: McpServer = {
        command: 'evil-command',
        args: ['arg1'],
      };

      try {
        validateMcpCommand(config);
        fail('Should have thrown error');
      } catch (error: any) {
        expect(error.message).toContain('evil-command');
        expect(error.message).toContain('not whitelisted');
      }
    });
  });
});

describe('extractPackageName', () => {
  describe('Scoped Packages', () => {
    test('extracts scoped package without version', () => {
      const args = ['-y', '@modelcontextprotocol/server-gdocs'];
      expect(extractPackageName(args)).toBe('@modelcontextprotocol/server-gdocs');
    });

    test('extracts scoped package with version', () => {
      const args = ['-y', '@gleanwork/mcp-server@1.0.0'];
      expect(extractPackageName(args)).toBe('@gleanwork/mcp-server');
    });

    test('extracts scoped package with @latest', () => {
      const args = ['@modelcontextprotocol/server-slack@latest'];
      expect(extractPackageName(args)).toBe('@modelcontextprotocol/server-slack');
    });
  });

  describe('Regular Packages', () => {
    test('extracts package without version', () => {
      const args = ['-y', 'mcp-remote'];
      expect(extractPackageName(args)).toBe('mcp-remote');
    });

    test('extracts package with version', () => {
      const args = ['mcp-remote@1.2.3'];
      expect(extractPackageName(args)).toBe('mcp-remote');
    });

    test('extracts package with @latest', () => {
      const args = ['-y', 'mcp-remote@latest', 'https://mcp.atlassian.com/v1/sse'];
      expect(extractPackageName(args)).toBe('mcp-remote');
    });
  });

  describe('Edge Cases', () => {
    test('returns null for empty args', () => {
      expect(extractPackageName([])).toBeNull();
    });

    test('returns null for args with only flags', () => {
      const args = ['-y', '--quiet', '--verbose'];
      expect(extractPackageName(args)).toBeNull();
    });

    test('skips flags and returns first non-flag arg', () => {
      const args = ['-y', '--quiet', '@modelcontextprotocol/server-gdocs'];
      expect(extractPackageName(args)).toBe('@modelcontextprotocol/server-gdocs');
    });

    test('handles scoped package as first arg', () => {
      const args = ['@gleanwork/mcp-server'];
      expect(extractPackageName(args)).toBe('@gleanwork/mcp-server');
    });
  });
});

describe('isWhitelistedPackage', () => {
  describe('Allowed Packages', () => {
    test('allows @modelcontextprotocol/server-gdocs', () => {
      expect(isWhitelistedPackage('@modelcontextprotocol/server-gdocs')).toBe(true);
    });

    test('allows @modelcontextprotocol/server-github', () => {
      expect(isWhitelistedPackage('@modelcontextprotocol/server-github')).toBe(true);
    });

    test('allows @modelcontextprotocol/server-gitlab', () => {
      expect(isWhitelistedPackage('@modelcontextprotocol/server-gitlab')).toBe(true);
    });

    test('allows @modelcontextprotocol/server-slack', () => {
      expect(isWhitelistedPackage('@modelcontextprotocol/server-slack')).toBe(true);
    });

    test('allows @gleanwork/mcp-server', () => {
      expect(isWhitelistedPackage('@gleanwork/mcp-server')).toBe(true);
    });

    test('allows mcp-remote', () => {
      expect(isWhitelistedPackage('mcp-remote')).toBe(true);
    });
  });

  describe('Blocked Packages', () => {
    test('blocks unknown packages', () => {
      expect(isWhitelistedPackage('@evil/malware')).toBe(false);
    });

    test('blocks packages not in whitelist', () => {
      expect(isWhitelistedPackage('some-random-package')).toBe(false);
    });

    test('blocks empty string', () => {
      expect(isWhitelistedPackage('')).toBe(false);
    });

    test('blocks similar but different package names', () => {
      expect(isWhitelistedPackage('@modelcontextprotocol/server-evil')).toBe(false);
    });
  });
});
