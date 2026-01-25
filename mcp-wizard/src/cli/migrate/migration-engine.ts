/**
 * Migration Engine - converts individual MCP configs to unified gateway format
 */

import { ClaudeConfig, MigrationResult } from './types';
import { getConfigPath } from './config-manager';

/**
 * Migrate config from individual MCPs to unified gateway
 */
export function migrateToGateway(currentConfig: ClaudeConfig): MigrationResult {
  const mcpServers = currentConfig.mcpServers || {};
  const migratedServers: string[] = [];
  const envVars: Record<string, string> = {};

  // Extract all MCP server entries
  for (const [name, config] of Object.entries(mcpServers)) {
    migratedServers.push(name);

    // Merge environment variables
    if (config.env) {
      Object.assign(envVars, config.env);
    }
  }

  // Create new config with single gateway entry
  const newConfig: ClaudeConfig = {
    ...currentConfig, // Preserve non-MCP sections
    mcpServers: {
      'mcp-wizard': {
        command: 'mcp-wizard',
        args: ['serve'],
        env: Object.keys(envVars).length > 0 ? envVars : undefined,
      }
    }
  };

  return { migratedServers, newConfig, envVars };
}

/**
 * Generate dry-run preview showing before/after state
 */
export function generatePreview(currentConfig: ClaudeConfig): string {
  const result = migrateToGateway(currentConfig);
  const configPath = getConfigPath();

  let preview = `🔍 Detected config: ${configPath}\n\n`;

  preview += `📋 Current MCPs (${result.migratedServers.length} server${result.migratedServers.length === 1 ? '' : 's'}):\n`;
  if (result.migratedServers.length > 0) {
    preview += result.migratedServers.map(s => `  - ${s}`).join('\n') + '\n';
  } else {
    preview += '  (none)\n';
  }

  preview += `\n🔄 After migration:\n`;
  preview += `  - Single mcp-wizard gateway\n`;
  if (result.migratedServers.length > 0) {
    preview += `  - All ${result.migratedServers.length} MCP${result.migratedServers.length === 1 ? '' : 's'} available through gateway\n`;
  }

  if (Object.keys(result.envVars).length > 0) {
    preview += `\n🔐 Environment variables migrated:\n`;
    preview += Object.keys(result.envVars).map(k => `  - ${k}`).join('\n') + '\n';
  }

  preview += `\n⚠️  This is a DRY RUN. No changes applied.\n`;
  preview += `Run without --dry-run to apply migration.`;

  return preview;
}
