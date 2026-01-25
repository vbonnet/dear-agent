/**
 * TypeScript types for mcp-wizard migration wizard
 */

/**
 * Claude Desktop configuration structure
 */
export interface ClaudeConfig {
  mcpServers?: Record<string, MCPServerConfig>;
  [key: string]: unknown; // Allow other config sections
}

/**
 * MCP server configuration entry
 */
export interface MCPServerConfig {
  command: string;
  args?: string[];
  env?: Record<string, string>;
}

/**
 * Backup file information
 */
export interface BackupInfo {
  path: string;
  timestamp: Date;
}

/**
 * Migration operation result
 */
export interface MigrationResult {
  migratedServers: string[];
  newConfig: ClaudeConfig;
  envVars: Record<string, string>;
}

/**
 * Chezmoi detection result
 */
export interface ChezmoiStatus {
  detected: boolean;
  message?: string;
}

/**
 * Configuration-related error
 */
export class ConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'ConfigError';
  }
}

/**
 * Backup-related error
 */
export class BackupError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'BackupError';
  }
}
