import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import inquirer from 'inquirer';
import { EnvironmentInfo } from './detect';
import { validateMcpCommand } from './command-whitelist';
import { getConfigValue } from './user-config';

export interface McpServer {
  command: string;
  args: string[];
  env?: Record<string, string>;
}

export interface McpConfig {
  mcpServers: Record<string, McpServer>;
}

export interface AgentInfo {
  name: string;
  configPath: string;
  detected: boolean;
  description: string;
}

export interface McpInfo {
  id: string;
  name: string;
  description: string;
  requiresAuth: 'oauth' | 'token' | 'none';
}

export const SUPPORTED_MCPS: McpInfo[] = [
  {
    id: 'googledocs',
    name: 'GoogleDocs',
    description: 'Access Google Docs and Drive (Community-maintained)',
    requiresAuth: 'oauth',
  },
  {
    id: 'atlassian',
    name: 'Atlassian',
    description: 'Access Jira and Confluence',
    requiresAuth: 'oauth',  // mcp-remote handles OAuth automatically on first use
  },
  {
    id: 'github',
    name: 'GitHub',
    description: 'Access GitHub repos, issues, PRs, and Actions',
    requiresAuth: 'oauth',  // PAT (primary) or OAuth (VS Code 1.101+)
  },
  {
    id: 'sequentialthinking',
    name: 'Sequential Thinking',
    description: 'Enhanced reasoning with structured thinking',
    requiresAuth: 'none',
  },
  {
    id: 'playwright',
    name: 'Playwright',
    description: 'Browser automation and testing',
    requiresAuth: 'none',
  },
  // Glean requires Glean admin to provision API tokens - not available for self-service
  // {
  //   id: 'glean',
  //   name: 'Glean',
  //   description: 'Enterprise search across tools',
  //   requiresAuth: 'token',
  // },
  // Slack requires workspace admin to create app - not available for self-service
  // {
  //   id: 'slack',
  //   name: 'Slack',
  //   description: 'Search and interact with Slack',
  //   requiresAuth: 'token',
  // },
];

export const SUPPORTED_AGENTS: Omit<AgentInfo, 'detected'>[] = [
  {
    name: 'Claude Code',
    configPath: '.config/claude-code/mcp.json',
    description: 'Anthropic Claude CLI tool',
  },
  {
    name: 'Cursor',
    configPath: '.cursor/mcp.json',
    description: 'AI-powered code editor (also used by Aider)',
  },
  {
    name: 'Cline',
    configPath: '.cline/mcp.json',
    description: 'AI coding assistant',
  },
  {
    name: 'Windsurf',
    configPath: '.codeium/windsurf/mcp.json',
    description: 'Codeium AI editor',
  },
];

export async function promptMcpSelection(): Promise<string[]> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   MCP Server Selection                                         ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('Select which MCP servers to configure:\n');

  const choices = SUPPORTED_MCPS.map(mcp => ({
    name: `${mcp.name} - ${mcp.description} (requires ${mcp.requiresAuth === 'oauth' ? 'OAuth' : mcp.requiresAuth === 'token' ? 'API token' : 'no auth'})`,
    value: mcp.id,
    checked: mcp.id === 'googledocs', // Pre-check GoogleDocs as default
  }));

  const { selectedMcps } = await inquirer.prompt([
    {
      type: 'checkbox',
      name: 'selectedMcps',
      message: 'Select MCP servers (space to toggle, enter to confirm):',
      choices,
      validate: (answer: string[]) => {
        if (answer.length === 0) {
          return 'Please select at least one MCP server';
        }
        return true;
      },
    },
  ]);

  return selectedMcps;
}

export async function generateMcpConfig(
  selectedMcps?: string[],
  env?: EnvironmentInfo
): Promise<McpConfig> {
  const homedir = os.homedir();

  const allServers: Record<string, McpServer> = {
    GoogleDocs: {
      command: 'node',
      args: [path.join(homedir, 'mcp-servers/google-docs-mcp/dist/server.js')],
      // No env vars needed - the server expects credentials.json and token.json
      // in /home/user/mcp-servers/google-docs-mcp/ (hardcoded paths)
      // For service account auth, use: env: { SERVICE_ACCOUNT_PATH: '/path/to/service-account.json' }
    },
    Glean: {
      command: 'npx',
      args: ['-y', '@gleanwork/mcp-server'],
      env: {
        GLEAN_INSTANCE: getConfigValue('company.glean_instance'),
        GLEAN_API_TOKEN: path.join(homedir, 'mcp-servers/glean-mcp/.glean-token'),
      },
    },
    Slack: {
      command: 'npx',
      args: ['-y', '@modelcontextprotocol/server-slack'],
      env: {
        SLACK_BOT_TOKEN: path.join(homedir, 'mcp-servers/slack-mcp/.slack-token'),
        SLACK_TEAM_ID: path.join(homedir, 'mcp-servers/slack-mcp/.slack-team-id'),
      },
    },
    Atlassian: {
      command: 'npx',
      args: ['-y', 'mcp-remote@latest', 'https://mcp.atlassian.com/v1/sse', '--auth-timeout', '120'],
    },
    GitHub: {
      command: 'npx',
      args: ['-y', '@modelcontextprotocol/server-github'],
      env: {
        GITHUB_PERSONAL_ACCESS_TOKEN: path.join(homedir, 'mcp-servers/github-mcp/.github-token'),
      },
    },
    SequentialThinking: {
      command: 'npx',
      args: ['-y', '@modelcontextprotocol/server-sequential-thinking'],
    },
    Playwright: {
      command: 'npx',
      args: ['-y', '@microsoft/mcp-server-playwright'],
    },
  };

  // If no selection provided, include all servers (backward compatible)
  if (!selectedMcps || selectedMcps.length === 0) {
    return { mcpServers: allServers };
  }

  // Filter to only selected MCPs
  const mcpIdToName: Record<string, string> = {
    googledocs: 'GoogleDocs',
    glean: 'Glean',
    slack: 'Slack',
    atlassian: 'Atlassian',
    github: 'GitHub',
    sequentialthinking: 'SequentialThinking',
    playwright: 'Playwright',
  };

  const filteredServers: Record<string, McpServer> = {};
  for (const mcpId of selectedMcps) {
    const serverName = mcpIdToName[mcpId];
    if (serverName && allServers[serverName]) {
      filteredServers[serverName] = allServers[serverName];
    }
  }

  return { mcpServers: filteredServers };
}

export async function detectInstalledAgents(): Promise<AgentInfo[]> {
  const homedir = os.homedir();
  const agents: AgentInfo[] = [];

  for (const agent of SUPPORTED_AGENTS) {
    const configPath = path.join(homedir, agent.configPath);
    const dirPath = path.dirname(configPath);

    // Check if agent directory exists OR config file exists
    let detected = false;
    try {
      await fs.access(dirPath);
      detected = true;
    } catch {
      // Try config file directly
      try {
        await fs.access(configPath);
        detected = true;
      } catch {
        detected = false;
      }
    }

    agents.push({
      ...agent,
      detected,
    });
  }

  return agents;
}

export async function promptAgentSelection(): Promise<string[]> {
  const agents = await detectInstalledAgents();

  const detectedCount = agents.filter(a => a.detected).length;

  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   AI Agent Selection                                           ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  if (detectedCount > 0) {
    console.log(`Detected ${detectedCount} AI agent(s) on this system:\n`);
    agents.filter(a => a.detected).forEach(a => {
      console.log(`  ✓ ${a.name} - ${a.description}`);
    });
    console.log('');
  }

  const choices = agents.map(agent => ({
    name: `${agent.name} - ${agent.description}${agent.detected ? ' (detected)' : ''}`,
    value: agent.name,
    checked: agent.detected, // Pre-check detected agents
  }));

  const { selectedAgents } = await inquirer.prompt([
    {
      type: 'checkbox',
      name: 'selectedAgents',
      message: 'Select AI agents to configure (space to toggle, enter to confirm):',
      choices,
      validate: (answer: string[]) => {
        if (answer.length === 0) {
          return 'Please select at least one agent';
        }
        return true;
      },
    },
  ]);

  return selectedAgents;
}

export function mergeMcpConfig(existingConfig: McpConfig, newConfig: McpConfig): McpConfig {
  // Start with existing config
  const merged = { ...existingConfig };

  // Ensure mcpServers exists
  if (!merged.mcpServers) {
    merged.mcpServers = {};
  }

  // Merge new servers (new servers take precedence)
  for (const [name, server] of Object.entries(newConfig.mcpServers)) {
    merged.mcpServers[name] = server;
  }

  return merged;
}

export async function writeMcpConfig(config: McpConfig, selectedAgents?: string[]): Promise<void> {
  const homedir = os.homedir();

  // If no agents specified, prompt user
  const agentNames = selectedAgents || await promptAgentSelection();

  const agentsToConfig = SUPPORTED_AGENTS.filter(a => agentNames.includes(a.name));

  if (agentsToConfig.length === 0) {
    console.log('⚠ No agents selected. Skipping MCP config write.');
    return;
  }

  console.log('\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
  console.log('Writing MCP Configuration');
  console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n');

  const written: string[] = [];
  const failed: Array<{ path: string; error: string }> = [];
  const merged: string[] = [];

  for (const agent of agentsToConfig) {
    const configPath = path.join(homedir, agent.configPath);

    try {
      // Ensure directory exists
      await fs.mkdir(path.dirname(configPath), { recursive: true });

      // Read existing config and merge if it exists
      let finalConfig = config;
      try {
        const existingContent = await fs.readFile(configPath, 'utf8');
        const existingConfig = JSON.parse(existingContent);

        // Backup existing config
        await fs.copyFile(configPath, `${configPath}.backup`);
        console.log(`  ✓ Backed up existing ${agent.name} config`);

        // Merge configs (new MCPs take precedence)
        finalConfig = mergeMcpConfig(existingConfig, config);
        merged.push(agent.name);
        console.log(`  ✓ Merged with existing ${agent.name} config`);
      } catch {
        // No existing config or invalid JSON, use new config as-is
      }

      // Validate commands
      for (const [name, server] of Object.entries(finalConfig.mcpServers)) {
        try {
          validateMcpCommand(server);
        } catch (error: any) {
          throw new Error(`Invalid MCP server "${name}": ${error.message}`);
        }
      }

      // Validate paths
      for (const [name, server] of Object.entries(finalConfig.mcpServers)) {
        for (const arg of server.args || []) {
          if (typeof arg === 'string' && arg.startsWith('/')) {
            validatePath(arg);
          }
        }
      }

      // Write merged config
      await fs.writeFile(configPath, JSON.stringify(finalConfig, null, 2));
      written.push(`${agent.name} (${configPath})`);
      console.log(`  ✓ Wrote ${agent.name} config: ${configPath}`);

    } catch (error) {
      const err = error as Error;
      failed.push({ path: agent.name, error: err.message });
      console.error(`  ✗ Failed to write ${agent.name} config: ${err.message}`);
    }
  }

  console.log('\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');

  if (written.length > 0) {
    console.log(`\n✓ Successfully configured ${written.length} agent(s):\n`);
    written.forEach(w => console.log(`  • ${w}`));
  }

  if (merged.length > 0) {
    console.log(`\n✓ Merged with existing configs for ${merged.length} agent(s):\n`);
    merged.forEach(m => console.log(`  • ${m} (existing MCP servers preserved)`));
  }

  if (failed.length > 0) {
    console.log(`\n⚠ Failed to configure ${failed.length} agent(s):\n`);
    failed.forEach(f => console.log(`  • ${f.path}: ${f.error}`));
  }

  console.log('');
}

export async function showChezmoiSnippet(config: McpConfig, skipPrompt = false): Promise<void> {
  const snippet = `{{- if hasSuffix "-w" .chezmoi.hostname }}
${JSON.stringify(config, null, 2)}
{{- else }}
{ "mcpServers": {} }
{{- end }}`;

  console.log('');
  console.log('╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Chezmoi Manual Setup Required                               ║');
  console.log('╚════════════════════════════════════════════════════════════════╝');
  console.log('');
  console.log('Your MCP config is managed by chezmoi.');
  console.log('Add this to: ~/.local/share/chezmoi/dot_config/claude-code/private_mcp.json.tmpl');
  console.log('');
  console.log('─'.repeat(70));
  console.log(snippet);
  console.log('─'.repeat(70));
  console.log('');
  console.log('After adding, run: chezmoi apply --force');
  console.log('');

  if (!skipPrompt) {
    await inquirer.prompt([
      {
        type: 'input',
        name: 'confirm',
        message: 'Press Enter when done...',
        default: '',
      },
    ]);
  }
}

export function validatePath(filePath: string): void {
  const homedir = os.homedir();

  // Reject path traversal
  if (filePath.includes('..')) {
    throw new Error(`Invalid path (contains ..): ${filePath}`);
  }

  // Reject paths outside home directory
  if (!filePath.startsWith(homedir) && !filePath.startsWith('/home/')) {
    throw new Error(`Invalid path (outside home directory): ${filePath}`);
  }

  // Canonicalize and verify
  const absPath = path.resolve(filePath);
  const cleanPath = path.normalize(absPath);
  if (absPath !== cleanPath) {
    throw new Error(`Invalid path (not canonical): ${filePath}`);
  }
}

export async function pathExists(filePath: string): Promise<boolean> {
  try {
    await fs.access(filePath);
    return true;
  } catch {
    return false;
  }
}

// ============================================================================
// Downstream MCP Configuration (Meta-Server Mode)
// ============================================================================

export interface DownstreamMCPConfig {
  command: string;
  args: string[];
  env?: Record<string, string>;
}

export interface DownstreamConfig {
  downstreamMCPs: Record<string, DownstreamMCPConfig>;
}

/**
 * Load downstream MCP configuration for meta-server mode
 * Default path: ~/.config/mcp-wizard/downstream.json
 * Override with MCP_WIZARD_CONFIG_PATH env var
 */
export async function loadDownstreamConfig(): Promise<DownstreamConfig> {
  const homedir = os.homedir();
  const defaultPath = path.join(homedir, '.config/mcp-wizard/downstream.json');
  const configPath = process.env.MCP_WIZARD_CONFIG_PATH || defaultPath;

  try {
    const content = await fs.readFile(configPath, 'utf8');
    const config = JSON.parse(content) as DownstreamConfig;

    // Validate schema
    if (!config.downstreamMCPs || typeof config.downstreamMCPs !== 'object') {
      throw new Error('Invalid config: missing downstreamMCPs object');
    }

    // Validate each MCP entry
    for (const [name, mcp] of Object.entries(config.downstreamMCPs)) {
      if (!mcp.command || typeof mcp.command !== 'string') {
        throw new Error(`Invalid config for ${name}: missing command`);
      }
      if (!Array.isArray(mcp.args)) {
        throw new Error(`Invalid config for ${name}: args must be an array`);
      }
    }

    return config;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
      throw new Error(
        `Downstream config not found: ${configPath}\n` +
          `Create config file or set MCP_WIZARD_CONFIG_PATH environment variable`
      );
    }

    if (error instanceof SyntaxError) {
      throw new Error(`Invalid JSON in config file: ${configPath}\n${error.message}`);
    }

    throw error;
  }
}

/**
 * Write downstream MCP configuration
 */
export async function writeDownstreamConfig(
  config: DownstreamConfig,
  customPath?: string
): Promise<void> {
  const homedir = os.homedir();
  const defaultPath = path.join(homedir, '.config/mcp-wizard/downstream.json');
  const configPath = customPath || defaultPath;

  // Ensure directory exists
  await fs.mkdir(path.dirname(configPath), { recursive: true });

  // Write config
  await fs.writeFile(configPath, JSON.stringify(config, null, 2));
}
