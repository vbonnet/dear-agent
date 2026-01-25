import {
  mergeMcpConfig,
  promptMcpSelection,
  generateMcpConfig,
  detectInstalledAgents,
  promptAgentSelection,
  writeMcpConfig,
  showChezmoiSnippet,
  validatePath,
  pathExists,
  McpConfig,
  AgentInfo,
} from '../../src/lib/config';
import * as configModule from '../../src/lib/config';
import * as fs from 'fs/promises';
import * as os from 'os';
import * as path from 'path';
import inquirer from 'inquirer';
import { validateMcpCommand } from '../../src/lib/command-whitelist';
import { getConfigValue } from '../../src/lib/user-config';

// Mock modules
jest.mock('fs/promises');
jest.mock('inquirer', () => ({
  prompt: jest.fn(),
}));
jest.mock('os');
jest.mock('path');
jest.mock('../../src/lib/command-whitelist');
jest.mock('../../src/lib/user-config');

// Create typed mock for inquirer
const mockPrompt = inquirer.prompt as unknown as jest.Mock;

describe('mergeMcpConfig', () => {
  it('should preserve existing MCP servers when merging', () => {
    const existingConfig: McpConfig = {
      mcpServers: {
        GoogleDocs: {
          command: 'node',
          args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
        },
        Slack: {
          command: 'npx',
          args: ['-y', '@modelcontextprotocol/server-slack'],
        },
      },
    };

    const newConfig: McpConfig = {
      mcpServers: {
        Atlassian: {
          command: 'npx',
          args: ['-y', 'mcp-remote@latest', 'https://mcp.atlassian.com/v1/sse'],
        },
      },
    };

    const merged = mergeMcpConfig(existingConfig, newConfig);

    // Should have all three MCPs
    expect(merged.mcpServers).toHaveProperty('GoogleDocs');
    expect(merged.mcpServers).toHaveProperty('Slack');
    expect(merged.mcpServers).toHaveProperty('Atlassian');
    expect(Object.keys(merged.mcpServers)).toHaveLength(3);
  });

  it('should handle empty existing config', () => {
    const existingConfig: McpConfig = {
      mcpServers: {},
    };

    const newConfig: McpConfig = {
      mcpServers: {
        GoogleDocs: {
          command: 'node',
          args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
        },
      },
    };

    const merged = mergeMcpConfig(existingConfig, newConfig);

    expect(merged.mcpServers).toHaveProperty('GoogleDocs');
    expect(Object.keys(merged.mcpServers)).toHaveLength(1);
  });

  it('should handle missing mcpServers in existing config', () => {
    const existingConfig = {} as McpConfig;

    const newConfig: McpConfig = {
      mcpServers: {
        GoogleDocs: {
          command: 'node',
          args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
        },
      },
    };

    const merged = mergeMcpConfig(existingConfig, newConfig);

    expect(merged.mcpServers).toHaveProperty('GoogleDocs');
    expect(Object.keys(merged.mcpServers)).toHaveLength(1);
  });

  it('should override existing MCP if same name in new config', () => {
    const existingConfig: McpConfig = {
      mcpServers: {
        GoogleDocs: {
          command: 'node',
          args: ['/old/path/server.js'],
        },
      },
    };

    const newConfig: McpConfig = {
      mcpServers: {
        GoogleDocs: {
          command: 'node',
          args: ['/new/path/server.js'],
        },
      },
    };

    const merged = mergeMcpConfig(existingConfig, newConfig);

    expect(merged.mcpServers.GoogleDocs.args).toEqual(['/new/path/server.js']);
  });

  describe('idempotency', () => {
    it('should preserve manually-added MCPs when setup is re-run', () => {
      // Simulate first setup run: GoogleDocs
      const initialConfig: McpConfig = {
        mcpServers: {},
      };

      const firstSetup: McpConfig = {
        mcpServers: {
          GoogleDocs: {
            command: 'node',
            args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
          },
        },
      };

      const afterFirstSetup = mergeMcpConfig(initialConfig, firstSetup);

      // User manually adds Slack MCP
      afterFirstSetup.mcpServers.Slack = {
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-slack'],
      };

      // Simulate second setup run: Atlassian
      const secondSetup: McpConfig = {
        mcpServers: {
          Atlassian: {
            command: 'npx',
            args: ['-y', 'mcp-remote@latest', 'https://mcp.atlassian.com/v1/sse'],
          },
        },
      };

      const afterSecondSetup = mergeMcpConfig(afterFirstSetup, secondSetup);

      // All three MCPs should be present
      expect(afterSecondSetup.mcpServers).toHaveProperty('GoogleDocs');
      expect(afterSecondSetup.mcpServers).toHaveProperty('Slack'); // Manually added
      expect(afterSecondSetup.mcpServers).toHaveProperty('Atlassian');
      expect(Object.keys(afterSecondSetup.mcpServers)).toHaveLength(3);
    });

    it('should preserve manually-added MCPs across multiple setup runs', () => {
      let config: McpConfig = { mcpServers: {} };

      // Setup 1: Add GoogleDocs
      config = mergeMcpConfig(config, {
        mcpServers: {
          GoogleDocs: {
            command: 'node',
            args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
          },
        },
      });

      // User manually adds GitHub MCP
      config.mcpServers.GitHub = {
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-github'],
      };

      // Setup 2: Add Atlassian
      config = mergeMcpConfig(config, {
        mcpServers: {
          Atlassian: {
            command: 'npx',
            args: ['-y', 'mcp-remote@latest', 'https://mcp.atlassian.com/v1/sse'],
          },
        },
      });

      // User manually adds another MCP
      config.mcpServers.Memory = {
        command: 'npx',
        args: ['-y', '@modelcontextprotocol/server-memory'],
      };

      // Setup 3: Re-run with GoogleDocs (should not remove other MCPs)
      config = mergeMcpConfig(config, {
        mcpServers: {
          GoogleDocs: {
            command: 'node',
            args: ['/home/user/mcp-servers/google-docs-mcp/dist/server.js'],
          },
        },
      });

      // All four MCPs should still be present
      expect(config.mcpServers).toHaveProperty('GoogleDocs');
      expect(config.mcpServers).toHaveProperty('GitHub'); // Manually added
      expect(config.mcpServers).toHaveProperty('Atlassian');
      expect(config.mcpServers).toHaveProperty('Memory'); // Manually added
      expect(Object.keys(config.mcpServers)).toHaveLength(4);
    });
  });
});

// Test setup helpers
const mockHomeDir = '/home/testuser';

describe('promptMcpSelection', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('should return selected MCPs when user selects multiple', async () => {
    mockPrompt.mockResolvedValue({
      selectedMcps: ['googledocs', 'atlassian'],
    });

    const result = await promptMcpSelection();

    expect(result).toEqual(['googledocs', 'atlassian']);
    expect(inquirer.prompt).toHaveBeenCalledWith([
      expect.objectContaining({
        type: 'checkbox',
        name: 'selectedMcps',
      }),
    ]);
  });

  it('should return single MCP when user selects one', async () => {
    mockPrompt.mockResolvedValue({
      selectedMcps: ['googledocs'],
    });

    const result = await promptMcpSelection();

    expect(result).toEqual(['googledocs']);
  });

  it('should validate at least one selection required', async () => {
    const promptCall = mockPrompt.mock.calls[0];
    // First call to setup mock
    mockPrompt.mockResolvedValue({
      selectedMcps: ['googledocs'],
    });

    await promptMcpSelection();

    const promptArgs = mockPrompt.mock.calls[0][0][0];
    const validateFn = promptArgs.validate;

    // Test validation function
    expect(validateFn([])).toBe('Please select at least one MCP server');
    expect(validateFn(['googledocs'])).toBe(true);
  });

  it('should pre-check GoogleDocs by default', async () => {
    mockPrompt.mockResolvedValue({
      selectedMcps: ['googledocs'],
    });

    await promptMcpSelection();

    const promptArgs = mockPrompt.mock.calls[0][0][0];
    const googledocsChoice = promptArgs.choices.find((c: any) => c.value === 'googledocs');

    expect(googledocsChoice.checked).toBe(true);
  });
});

describe('generateMcpConfig', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (os.homedir as jest.Mock).mockReturnValue(mockHomeDir);
    (path.join as jest.Mock).mockImplementation((...parts) => parts.join('/'));
    (getConfigValue as jest.Mock).mockReturnValue('[REDACTED_EMPLOYER]');
  });

  it('should generate config for single MCP', async () => {
    const config = await generateMcpConfig(['googledocs']);

    expect(config.mcpServers).toHaveProperty('GoogleDocs');
    expect(config.mcpServers).not.toHaveProperty('Slack');
    expect(config.mcpServers).not.toHaveProperty('Atlassian');
    expect(Object.keys(config.mcpServers)).toHaveLength(1);
  });

  it('should generate config for multiple MCPs', async () => {
    const config = await generateMcpConfig(['googledocs', 'atlassian']);

    expect(config.mcpServers).toHaveProperty('GoogleDocs');
    expect(config.mcpServers).toHaveProperty('Atlassian');
    expect(Object.keys(config.mcpServers)).toHaveLength(2);
  });

  it('should include all servers when no selection provided (backward compat)', async () => {
    const config = await generateMcpConfig();

    expect(config.mcpServers).toHaveProperty('GoogleDocs');
    expect(config.mcpServers).toHaveProperty('Glean');
    expect(config.mcpServers).toHaveProperty('Slack');
    expect(config.mcpServers).toHaveProperty('Atlassian');
    expect(Object.keys(config.mcpServers)).toHaveLength(4);
  });

  it('should filter out invalid MCP IDs', async () => {
    const config = await generateMcpConfig(['googledocs', 'invalid-mcp-id']);

    expect(config.mcpServers).toHaveProperty('GoogleDocs');
    expect(config.mcpServers).not.toHaveProperty('invalid-mcp-id');
    expect(Object.keys(config.mcpServers)).toHaveLength(1);
  });

  it('should use environment variable for glean_instance', async () => {
    (getConfigValue as jest.Mock).mockReturnValue('acme');

    const config = await generateMcpConfig(['glean']);

    expect(getConfigValue).toHaveBeenCalledWith('company.glean_instance', '[REDACTED_EMPLOYER]');
    expect(config.mcpServers.Glean.env).toBeDefined();
    expect(config.mcpServers.Glean.env?.GLEAN_INSTANCE).toBe('acme');
  });
});

describe('detectInstalledAgents', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (os.homedir as jest.Mock).mockReturnValue(mockHomeDir);
    (path.join as jest.Mock).mockImplementation((...parts) => parts.join('/'));
    (path.dirname as jest.Mock).mockImplementation((p) => {
      const parts = p.split('/');
      parts.pop();
      return parts.join('/');
    });
  });

  it('should return empty detected flags when no agents found', async () => {
    (fs.access as jest.Mock).mockRejectedValue(new Error('ENOENT'));

    const agents = await detectInstalledAgents();

    expect(agents).toHaveLength(4); // All 4 supported agents
    expect(agents.every(a => !a.detected)).toBe(true);
  });

  it('should detect one agent when directory exists', async () => {
    (fs.access as jest.Mock).mockImplementation((path: string) => {
      if (path.includes('claude-code')) return Promise.resolve();
      throw new Error('ENOENT');
    });

    const agents = await detectInstalledAgents();

    const claudeCode = agents.find(a => a.name === 'Claude Code');
    const cursor = agents.find(a => a.name === 'Cursor');

    expect(claudeCode?.detected).toBe(true);
    expect(cursor?.detected).toBe(false);
  });

  it('should detect multiple agents', async () => {
    (fs.access as jest.Mock).mockImplementation((path: string) => {
      if (path.includes('claude-code') || path.includes('cursor')) {
        return Promise.resolve();
      }
      throw new Error('ENOENT');
    });

    const agents = await detectInstalledAgents();

    const detected = agents.filter(a => a.detected);
    expect(detected).toHaveLength(2);
    expect(detected.map(a => a.name)).toContain('Claude Code');
    expect(detected.map(a => a.name)).toContain('Cursor');
  });

  it('should detect when config file exists without directory', async () => {
    (fs.access as jest.Mock).mockImplementation((path: string) => {
      if (path.includes('mcp.json')) return Promise.resolve();
      throw new Error('ENOENT');
    });

    const agents = await detectInstalledAgents();

    // At least one agent should be detected (config file path is checked)
    const detected = agents.filter(a => a.detected);
    expect(detected.length).toBeGreaterThan(0);
  });

  it('should handle permission denied gracefully', async () => {
    const permError: any = new Error('Permission denied');
    permError.code = 'EACCES';
    (fs.access as jest.Mock).mockRejectedValue(permError);

    const agents = await detectInstalledAgents();

    expect(agents).toHaveLength(4);
    expect(agents.every(a => !a.detected)).toBe(true);
  });
});

describe('promptAgentSelection', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('should present agent choices to user', async () => {
    // Mock fs.access to simulate one agent detected (Claude Code) and one not (Cursor)
    (fs.access as jest.Mock).mockImplementation((path: string) => {
      if (path.includes('claude-code')) return Promise.resolve();
      throw new Error('ENOENT');
    });

    (os.homedir as jest.Mock).mockReturnValue('/home/testuser');
    (path.join as jest.Mock).mockImplementation((...parts) => parts.join('/'));

    mockPrompt.mockResolvedValue({
      selectedAgents: ['Claude Code'],
    });

    await promptAgentSelection();

    // Verify prompt was called
    expect(mockPrompt).toHaveBeenCalled();
    const promptArgs = mockPrompt.mock.calls[0][0][0];

    // Verify prompt structure
    expect(promptArgs.type).toBe('checkbox');
    expect(promptArgs.name).toBe('selectedAgents');
    expect(promptArgs.choices).toBeDefined();
    expect(Array.isArray(promptArgs.choices)).toBe(true);

    // Verify choices contain Claude Code and Cursor
    const claudeChoice = promptArgs.choices.find((c: any) => c.value === 'Claude Code');
    const cursorChoice = promptArgs.choices.find((c: any) => c.value === 'Cursor');

    expect(claudeChoice).toBeDefined();
    expect(cursorChoice).toBeDefined();

    // Verify detected agent is pre-checked
    expect(claudeChoice?.checked).toBe(true);
    expect(cursorChoice?.checked).toBe(false);
  });

  it('should allow selection when none detected', async () => {
    jest.spyOn(configModule, 'detectInstalledAgents').mockResolvedValue([
      { name: 'Claude Code', detected: false, configPath: '.config/claude-code/mcp.json', description: 'Test' },
      { name: 'Cursor', detected: false, configPath: '.cursor/mcp.json', description: 'Test' },
    ]);

    mockPrompt.mockResolvedValue({
      selectedAgents: ['Claude Code', 'Cursor'],
    });

    const result = await promptAgentSelection();

    expect(result).toEqual(['Claude Code', 'Cursor']);
  });

  it('should validate at least one agent selected', async () => {
    jest.spyOn(configModule, 'detectInstalledAgents').mockResolvedValue([
      { name: 'Claude Code', detected: true, configPath: '.config/claude-code/mcp.json', description: 'Test' },
    ]);

    mockPrompt.mockResolvedValue({
      selectedAgents: ['Claude Code'],
    });

    await promptAgentSelection();

    const promptArgs = mockPrompt.mock.calls[0][0][0];
    const validateFn = promptArgs.validate;

    expect(validateFn([])).toBe('Please select at least one agent');
    expect(validateFn(['Claude Code'])).toBe(true);
  });
});

describe('writeMcpConfig', () => {
  const mockConfig: McpConfig = {
    mcpServers: {
      GoogleDocs: {
        command: 'node',
        args: ['/home/testuser/mcp-servers/google-docs-mcp/dist/server.js'],
      },
    },
  };

  beforeEach(() => {
    jest.clearAllMocks();
    (os.homedir as jest.Mock).mockReturnValue(mockHomeDir);
    (path.join as jest.Mock).mockImplementation((...parts) => parts.join('/'));
    (path.dirname as jest.Mock).mockImplementation((p) => {
      const parts = p.split('/');
      parts.pop();
      return parts.join('/');
    });
    (validateMcpCommand as jest.Mock).mockImplementation(() => {}); // No throw
  });

  it('should write new config when file doesn\'t exist', async () => {
    (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
    (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));
    (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

    const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();

    await writeMcpConfig(mockConfig, ['Claude Code']);

    expect(fs.mkdir).toHaveBeenCalled();
    expect(fs.writeFile).toHaveBeenCalled();
    expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('Wrote Claude Code config'));

    consoleLogSpy.mockRestore();
  });

  it('should merge with existing config', async () => {
    const existingConfig = {
      mcpServers: {
        Slack: {
          command: 'npx',
          args: ['-y', '@modelcontextprotocol/server-slack'],
        },
      },
    };

    (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
    (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(existingConfig));
    (fs.copyFile as jest.Mock).mockResolvedValue(undefined);
    (fs.writeFile as jest.Mock).mockResolvedValue(undefined);

    const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();

    await writeMcpConfig(mockConfig, ['Claude Code']);

    expect(fs.copyFile).toHaveBeenCalled(); // Backup
    expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('Backed up'));
    expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('Merged'));

    consoleLogSpy.mockRestore();
  });

  it('should throw when command validation fails', async () => {
    (validateMcpCommand as jest.Mock).mockImplementation(() => {
      throw new Error('Invalid command');
    });

    (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
    (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT'));

    const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

    await writeMcpConfig(mockConfig, ['Claude Code']);

    expect(consoleErrorSpy).toHaveBeenCalledWith(expect.stringContaining('Failed to write'));

    consoleErrorSpy.mockRestore();
  });

  it('should skip write when no agents selected', async () => {
    const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();

    await writeMcpConfig(mockConfig, []);

    expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('No agents selected'));
    expect(fs.writeFile).not.toHaveBeenCalled();

    consoleLogSpy.mockRestore();
  });
});

describe('showChezmoiSnippet', () => {
  const mockConfig: McpConfig = {
    mcpServers: {
      GoogleDocs: {
        command: 'node',
        args: ['/home/testuser/mcp-servers/google-docs-mcp/dist/server.js'],
      },
    },
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('should render snippet with config JSON', async () => {
    mockPrompt.mockResolvedValue({ confirm: '' });
    const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();

    await showChezmoiSnippet(mockConfig);

    expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('chezmoi'));
    expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('GoogleDocs'));

    consoleLogSpy.mockRestore();
  });

  it('should wait for user confirmation', async () => {
    mockPrompt.mockResolvedValue({ confirm: '' });
    const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();

    await showChezmoiSnippet(mockConfig);

    expect(inquirer.prompt).toHaveBeenCalledWith([
      expect.objectContaining({
        type: 'input',
        name: 'confirm',
      }),
    ]);

    consoleLogSpy.mockRestore();
  });
});

describe('validatePath', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (os.homedir as jest.Mock).mockReturnValue(mockHomeDir);
  });

  it('should allow valid paths in home directory', () => {
    expect(() => validatePath('/home/testuser/documents')).not.toThrow();
    expect(() => validatePath('/home/testuser/.config/test')).not.toThrow();
  });

  it('should reject path traversal (..)', () => {
    expect(() => validatePath('/home/testuser/../etc/passwd')).toThrow('contains ..');
  });

  it('should reject paths outside home directory', () => {
    expect(() => validatePath('/etc/passwd')).toThrow('outside home directory');
    expect(() => validatePath('/var/log/test')).toThrow('outside home directory');
  });
});

describe('pathExists', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('should return true when path exists', async () => {
    (fs.access as jest.Mock).mockResolvedValue(undefined);

    const result = await pathExists('/home/testuser/test.txt');

    expect(result).toBe(true);
  });

  it('should return false when path doesn\'t exist', async () => {
    (fs.access as jest.Mock).mockRejectedValue(new Error('ENOENT'));

    const result = await pathExists('/home/testuser/nonexistent.txt');

    expect(result).toBe(false);
  });

  it('should return false on permission denied', async () => {
    const permError: any = new Error('Permission denied');
    permError.code = 'EACCES';
    (fs.access as jest.Mock).mockRejectedValue(permError);

    const result = await pathExists('/home/testuser/noperm.txt');

    expect(result).toBe(false);
  });
});
