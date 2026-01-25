import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';

export class ConfigLocationDetector {
  /**
   * Detect Claude Code config location (prefer new standard ~/.claude.json)
   * @returns Absolute path to config file
   */
  async detect(): Promise<string> {
    const newLocation = path.join(os.homedir(), '.claude.json');
    const legacyLocation = path.join(
      os.homedir(),
      '.config/claude-code/mcp.json'
    );

    // Prefer new location (check if exists)
    try {
      await fs.access(newLocation);
      return newLocation;
    } catch {
      // Check legacy location
      try {
        await fs.access(legacyLocation);
        console.warn(
          '⚠️  Using legacy config location. Consider migrating to ~/.claude.json'
        );
        return legacyLocation;
      } catch {
        // Neither exists - use new standard location
        return newLocation;
      }
    }
  }

  /**
   * Merge MCP server config into existing Claude Code config
   * @param cwd - Current working directory (project path)
   * @param mcpServerName - Name of MCP server (e.g., "googledocs")
   * @param mcpServerConfig - MCP server configuration object
   */
  async merge(
    cwd: string,
    mcpServerName: string,
    mcpServerConfig: any
  ): Promise<void> {
    // Sanitize cwd before use
    const safeCwd = this.sanitizePath(cwd);

    const configPath = await this.detect();

    // Read existing config (or create empty)
    let existingConfig: any = {};
    try {
      const content = await fs.readFile(configPath, 'utf-8');
      existingConfig = JSON.parse(content);
    } catch (error: any) {
      if (error.code === 'ENOENT') {
        // Config doesn't exist - create new
        console.log('Creating new Claude Code config at', configPath);
      } else if (error instanceof SyntaxError) {
        // Handle corrupted JSON
        console.warn('⚠️  Existing config is corrupted. Creating backup and starting fresh.');
        await this.backup(configPath);
        existingConfig = {};
      } else {
        throw error;
      }
    }

    // Backup before modifying
    const backupPath = await this.backup(configPath);

    // Merge MCP servers (preserve existing)
    const updatedConfig = {
      ...existingConfig,
      projects: {
        ...existingConfig.projects,
        [safeCwd]: {
          ...existingConfig.projects?.[safeCwd],
          mcpServers: {
            ...existingConfig.projects?.[safeCwd]?.mcpServers,
            [mcpServerName]: mcpServerConfig, // Add/update googledocs only
          },
        },
      },
    };

    // Wrap write in try/catch with rollback
    try {
      await fs.writeFile(configPath, JSON.stringify(updatedConfig, null, 2));
      console.log(`✓ Updated ${configPath}`);
    } catch (error) {
      console.error('✗ Failed to write config. Attempting rollback...');

      // Rollback: restore from backup
      if (backupPath) {
        try {
          await fs.access(backupPath);
          await fs.copyFile(backupPath, configPath);
          console.log('✓ Rollback successful. Config restored from backup.');
        } catch {
          console.error('✗ Rollback failed. Manual intervention required.');
        }
      }

      throw error;
    }
  }

  /**
   * Create backup of config file before modification
   * Prevents data loss if merge operation fails
   * @param configPath - Path to config file to backup
   * @returns Path to backup file (.claude.json.bak.TIMESTAMP), or empty string if no backup created
   * @private
   */
  private async backup(configPath: string): Promise<string> {
    try {
      await fs.access(configPath);

      // Backup exists - create timestamped backup
      const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
      const backupPath = `${configPath}.bak.${timestamp}`;

      await fs.copyFile(configPath, backupPath);

      // Set backup permissions to 600 (user-only)
      // Security: Config may contain env vars (info leak prevention)
      await fs.chmod(backupPath, 0o600);

      console.log(`✓ Backup created: ${backupPath}`);
      return backupPath;
    } catch (error) {
      // Config doesn't exist - no backup needed
      return '';
    }
  }

  /**
   * Sanitize path to prevent directory traversal attacks
   * Normalizes path and rejects any containing ".." sequences
   * @param userPath - User-provided path (potentially unsafe)
   * @returns Normalized absolute path (safe)
   * @throws Error if path contains traversal attempts
   * @private
   */
  private sanitizePath(userPath: string): string {
    const normalized = path.normalize(userPath);

    // Reject paths with traversal attempts
    if (normalized.includes('..')) {
      throw new Error(
        'Invalid path (traversal detected). Use absolute path without ".."'
      );
    }

    // Resolve to absolute path
    const resolved = path.resolve(normalized);

    return resolved;
  }
}
