import { PlatformAdapter, MCPServer } from './PlatformAdapter';
import { exec as execCallback } from 'child_process';
import { promisify } from 'util';
import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';

const exec = promisify(execCallback);

/**
 * Claude Code platform adapter
 *
 * Hybrid approach: Try CLI first (`claude mcp add`), fallback to file writes
 * Config location: ~/.claude.json (NOT .config/claude-code/mcp.json!)
 * Format: projects[cwd].mcpServers (per-project config)
 */
export class ClaudeCodeAdapter implements PlatformAdapter {
  async hasCLI(): Promise<boolean> {
    try {
      await exec('which claude');
      return true;
    } catch {
      return false;
    }
  }

  async configure(servers: MCPServer[]): Promise<void> {
    if (await this.hasCLI()) {
      console.log('  ✓ Claude CLI detected, using `claude mcp add` commands');
      return this.configureViaCLI(servers);
    }

    console.log('  ⚠ Claude CLI not found, falling back to file write');
    console.log('    Install CLI: npm install -g @anthropic/claude-code');
    return this.configureViaFile(servers);
  }

  private async configureViaCLI(servers: MCPServer[]): Promise<void> {
    for (const server of servers) {
      const cmd = this.buildCLICommand(server);
      console.log(`    Running: ${cmd}`);

      try {
        const { stdout, stderr } = await exec(cmd);
        if (stdout) console.log(`    ${stdout.trim()}`);
        if (stderr) console.error(`    ${stderr.trim()}`);
        console.log(`  ✓ Configured ${server.name} via CLI`);
      } catch (error: any) {
        console.error(`  ✗ CLI failed for ${server.name}: ${error.message}`);
        console.log(`    Falling back to file write for ${server.name}`);
        await this.configureViaFile([server]);
      }
    }
  }

  private buildCLICommand(server: MCPServer): string {
    let cmd = `claude mcp add ${server.name}`;

    if (server.type === 'stdio') {
      cmd += ` --transport stdio`;

      // Add environment variables
      if (server.env) {
        for (const [key, value] of Object.entries(server.env)) {
          cmd += ` -e ${key}=${value}`;
        }
      }

      // Add command and args
      cmd += ` -- ${server.command} ${(server.args || []).join(' ')}`;
    } else if (server.type === 'http') {
      cmd += ` --transport http ${server.url}`;
      // TODO: headers support if claude CLI supports it
    } else if (server.type === 'sse') {
      cmd += ` --transport sse ${server.url}`;
    }

    return cmd;
  }

  private async configureViaFile(servers: MCPServer[]): Promise<void> {
    const configPath = path.join(os.homedir(), '.claude.json');
    const config = await this.readOrCreateConfig(configPath);
    const cwd = process.cwd();

    // Ensure projects structure exists
    if (!config.projects) config.projects = {};
    if (!config.projects[cwd]) config.projects[cwd] = {};
    if (!config.projects[cwd].mcpServers) config.projects[cwd].mcpServers = {};

    // Add each server to project config
    for (const server of servers) {
      config.projects[cwd].mcpServers[server.name] = this.transformServer(server);
      console.log(`  ✓ Added ${server.name} to ${configPath}`);
    }

    // Write back to file
    await fs.writeFile(configPath, JSON.stringify(config, null, 2));
    console.log(`  ✓ Wrote Claude Code config: ${configPath}`);
  }

  private async readOrCreateConfig(configPath: string): Promise<any> {
    try {
      const content = await fs.readFile(configPath, 'utf8');
      return JSON.parse(content);
    } catch {
      // File doesn't exist or invalid JSON, create new config
      return { projects: {} };
    }
  }

  private transformServer(server: MCPServer): any {
    if (server.type === 'stdio') {
      return {
        type: 'stdio',
        command: server.command,
        args: server.args || [],
        env: server.env || {},
      };
    } else if (server.type === 'http' || server.type === 'sse') {
      return {
        type: server.type,
        url: server.url,
        headers: server.headers || {},
      };
    }

    throw new Error(`Unsupported server type: ${server.type}`);
  }
}
