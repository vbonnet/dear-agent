import { PlatformAdapter, MCPServer } from './PlatformAdapter';
import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';

/**
 * Gemini CLI platform adapter
 *
 * File-only approach (no CLI commands available)
 * Config location: ~/.gemini/settings.json
 * Format: Root-level mcpServers object
 * Field differences: httpUrl (not url) for HTTP servers
 */
export class GeminiAdapter implements PlatformAdapter {
  async hasCLI(): Promise<boolean> {
    // Gemini has no CLI commands for MCP configuration
    return false;
  }

  async configure(servers: MCPServer[]): Promise<void> {
    return this.configureViaFile(servers);
  }

  private async configureViaFile(servers: MCPServer[]): Promise<void> {
    const configPath = path.join(os.homedir(), '.gemini/settings.json');
    const config = await this.readOrCreateConfig(configPath);

    // Ensure mcpServers object exists
    if (!config.mcpServers) config.mcpServers = {};

    // Add each server
    for (const server of servers) {
      config.mcpServers[server.name] = this.transformServer(server);
      console.log(`  ✓ Added ${server.name} to Gemini config`);
    }

    // Ensure directory exists
    await fs.mkdir(path.dirname(configPath), { recursive: true });

    // Write back to file
    await fs.writeFile(configPath, JSON.stringify(config, null, 2));
    console.log(`  ✓ Wrote Gemini config: ${configPath}`);
  }

  private async readOrCreateConfig(configPath: string): Promise<any> {
    try {
      const content = await fs.readFile(configPath, 'utf8');
      return JSON.parse(content);
    } catch {
      // File doesn't exist or invalid JSON, create new config
      return { mcpServers: {} };
    }
  }

  private transformServer(server: MCPServer): any {
    if (server.type === 'stdio') {
      return {
        command: server.command,
        args: server.args || [],
        env: server.env || {},
      };
    } else if (server.type === 'http') {
      // Note: Gemini uses httpUrl instead of url
      return {
        httpUrl: server.url,
        headers: server.headers || {},
      };
    } else if (server.type === 'sse') {
      return {
        url: server.url,
      };
    }

    throw new Error(`Unsupported server type: ${server.type}`);
  }
}
