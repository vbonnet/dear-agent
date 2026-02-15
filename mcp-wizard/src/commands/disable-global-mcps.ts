/**
 * Disable Global MCPs Command
 *
 * Disables global MCP discovery and reverts to stdio-only MCPs.
 * Part of Task 1.3: mcp-wizard Integration for global MCP support.
 *
 * @module commands/disable-global-mcps
 */

import { loadConfig, saveConfig, getUserConfigPath } from '../lib/user-config';
import { sanitizeError } from '../lib/errors';

/**
 * Command options for disabling global MCPs
 */
export interface DisableGlobalMcpsOptions {
  /** Suppress success messages (for automation) */
  silent?: boolean;
}

/**
 * Disable global MCP discovery command
 *
 * @param options - Command options
 *
 * @example
 * await disableGlobalMcpsCommand({});
 * await disableGlobalMcpsCommand({ silent: true });
 */
export async function disableGlobalMcpsCommand(
  options: DisableGlobalMcpsOptions = {}
): Promise<void> {
  try {
    if (!options.silent) {
      console.log('Disabling global MCP discovery...\n');
    }

    const config = await loadConfig();

    // Disable global MCPs if configured
    if (config.globalMcps) {
      config.globalMcps.enabled = false;
    } else {
      // No globalMcps config exists, create disabled entry
      config.globalMcps = {
        enabled: false,
      };
    }

    await saveConfig(config);

    if (!options.silent) {
      console.log('✓ Global MCP discovery disabled');
      console.log('  Sessions will use stdio MCPs only.');
      console.log('');
      console.log(`Config saved to: ${getUserConfigPath()}`);
    }

  } catch (error) {
    const sanitized = sanitizeError(error);
    console.error(`✗ Failed to disable global MCPs: ${sanitized.message}`);

    if (sanitized.fix) {
      console.error(`\nFix: ${sanitized.fix}`);
    }

    process.exit(1);
  }
}
