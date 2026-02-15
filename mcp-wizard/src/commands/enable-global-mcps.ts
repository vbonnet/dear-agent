/**
 * Enable Global MCPs Command
 *
 * Enables global MCP discovery and HTTP transport for session-wide MCP availability.
 * Part of Task 1.3: mcp-wizard Integration for global MCP support.
 *
 * @module commands/enable-global-mcps
 */

import { loadConfig, saveConfig, getUserConfigPath } from '../lib/user-config';
import { sanitizeError } from '../lib/errors';

/**
 * Command options for enabling global MCPs
 */
export interface EnableGlobalMcpsOptions {
  /** Health check URL (default: http://localhost:8001/health) */
  healthUrl?: string;

  /** Discovery URL (default: http://localhost:8001/discovery) */
  discoveryUrl?: string;

  /** Temporal server URL (default: http://localhost:7233) */
  temporalUrl?: string;
}

/**
 * Enable global MCP discovery command
 *
 * @param options - Command options
 *
 * @example
 * await enableGlobalMcpsCommand({});
 * await enableGlobalMcpsCommand({ healthUrl: 'http://localhost:9000/health' });
 */
export async function enableGlobalMcpsCommand(
  options: EnableGlobalMcpsOptions = {}
): Promise<void> {
  try {
    console.log('Enabling global MCP discovery...\n');

    const config = await loadConfig();

    // Initialize or update globalMcps configuration
    if (!config.globalMcps) {
      config.globalMcps = {
        enabled: true,
        healthCheckUrl: options.healthUrl || 'http://localhost:8001/health',
        discoveryUrl: options.discoveryUrl || 'http://localhost:8001/discovery',
        temporalUrl: options.temporalUrl || 'http://localhost:7233',
      };
    } else {
      config.globalMcps.enabled = true;

      // Update URLs if provided
      if (options.healthUrl) {
        config.globalMcps.healthCheckUrl = options.healthUrl;
      }
      if (options.discoveryUrl) {
        config.globalMcps.discoveryUrl = options.discoveryUrl;
      }
      if (options.temporalUrl) {
        config.globalMcps.temporalUrl = options.temporalUrl;
      }
    }

    await saveConfig(config);

    console.log('✓ Global MCP discovery enabled\n');
    console.log('Configuration:');
    console.log(`  Health check URL: ${config.globalMcps.healthCheckUrl}`);
    console.log(`  Discovery URL:    ${config.globalMcps.discoveryUrl}`);
    console.log(`  Temporal URL:     ${config.globalMcps.temporalUrl}`);
    console.log(`  Temporal UI:      ${config.globalMcps.temporalUrl.replace('7233', '8088')}`);
    console.log('');
    console.log(`Config saved to:    ${getUserConfigPath()}`);
    console.log('');
    console.log("Run 'mcp-wizard status' to verify global MCPs.");

  } catch (error) {
    const sanitized = sanitizeError(error);
    console.error(`✗ Failed to enable global MCPs: ${sanitized.message}`);

    if (sanitized.fix) {
      console.error(`\nFix: ${sanitized.fix}`);
    }

    process.exit(1);
  }
}
