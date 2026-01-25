/**
 * MCP Configuration Discovery
 *
 * Discovers MCP configurations from multiple locations:
 * - ~/.config/mcp-wizard/config.json (mcp-wizard config)
 * - ~/.claude/.mcp.json (Claude Code config)
 *
 * Merges configs and filters for Okta-authenticated MCPs only.
 *
 * Part of Phase 4-v2 SessionStart Hook implementation (oss-n1nq.5-v2)
 *
 * @module config-discovery
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import { loadConfig } from './user-config';

/**
 * MCP configuration schema
 */
export interface MCPConfig {
  name: string;           // Display name (e.g., "GoogleDocs")
  serviceName: string;    // Keychain service name (e.g., "googledocs")
  oktaDomain: string;     // e.g., "company.okta.com"
  clientId: string;       // Okta client ID
  scopes: string[];       // OAuth scopes
  auth: string;           // Auth type (e.g., "okta")
}

/**
 * Read MCP configs from mcp-wizard config file
 *
 * Reads from ~/.config/mcp-wizard/config.json and extracts
 * downstream MCPs from broker configuration.
 *
 * @returns Array of MCP configurations
 */
export async function readWizardConfig(): Promise<MCPConfig[]> {
  try {
    const config = loadConfig() as any; // Uses existing user-config.ts

    // Extract downstream_mcps from broker config
    if (!config.broker?.downstream_mcps) {
      return [];
    }

    const mcps: MCPConfig[] = [];

    for (const [serviceName, mcpDef] of Object.entries(config.broker.downstream_mcps as Record<string, any>)) {
      // Only include MCPs with Okta auth
      if ((mcpDef as any).auth !== 'okta') {
        continue;
      }

      mcps.push({
        name: capitalize(serviceName),
        serviceName,
        oktaDomain: config.company.okta_domain,
        clientId: config.company.okta_client_id || '',
        scopes: config.company.okta_scopes || ['openid', 'profile', 'email'],
        auth: (mcpDef as any).auth,
      });
    }

    return mcps;

  } catch (error) {
    // Graceful fallback: return empty array
    return [];
  }
}

/**
 * Read MCP configs from Claude Code config file
 *
 * Reads from ~/.claude/.mcp.json and follows mcp-wizard-broker
 * configuration to find MCP definitions.
 *
 * @returns Array of MCP configurations
 */
export async function readClaudeConfig(): Promise<MCPConfig[]> {
  try {
    const claudePath = path.join(os.homedir(), '.claude/.mcp.json');

    // Check if file exists
    try {
      await fs.access(claudePath);
    } catch {
      return []; // File doesn't exist
    }

    // Read and parse
    const content = await fs.readFile(claudePath, 'utf8');
    const claudeConfig = JSON.parse(content);

    // Look for mcp-wizard-broker
    if (!claudeConfig.mcpServers?.['mcp-wizard-broker']) {
      return []; // No broker configured
    }

    const brokerConfig = claudeConfig.mcpServers['mcp-wizard-broker'];

    // If broker has MCP_WIZARD_CONFIG env var, read that config
    const wizardConfigPath = brokerConfig.env?.MCP_WIZARD_CONFIG;

    if (wizardConfigPath) {
      // Resolve ${HOME} in path
      const resolvedPath = wizardConfigPath.replace('${HOME}', os.homedir());

      // Read wizard config from specified path
      const wizardContent = await fs.readFile(resolvedPath, 'utf8');
      const wizardConfig = JSON.parse(wizardContent);

      // Extract MCPs from wizard config
      return extractMCPsFromWizardConfig(wizardConfig);
    }

    // No wizard config path found
    return [];

  } catch (error) {
    // Graceful fallback: invalid JSON, missing file, etc.
    if (error instanceof SyntaxError) {
      console.warn(`⚠️  Invalid JSON in Claude Code config: ${error.message}`);
    }
    return [];
  }
}

/**
 * Extract MCPs from wizard config object
 *
 * Helper function to parse wizard config format.
 *
 * @param config - Wizard config object
 * @returns Array of MCP configurations
 */
function extractMCPsFromWizardConfig(config: any): MCPConfig[] {
  if (!config.broker?.downstream_mcps) {
    return [];
  }

  const mcps: MCPConfig[] = [];

  for (const [serviceName, mcpDef] of Object.entries(config.broker.downstream_mcps as Record<string, any>)) {
    if (mcpDef.auth === 'okta') {
      mcps.push({
        name: capitalize(serviceName),
        serviceName,
        oktaDomain: config.company.okta_domain,
        clientId: config.company.okta_client_id || '',
        scopes: config.company.okta_scopes || ['openid', 'profile', 'email'],
        auth: mcpDef.auth,
      });
    }
  }

  return mcps;
}

/**
 * Merge MCP configs and deduplicate by serviceName
 *
 * If same serviceName appears in multiple sources, prefer first occurrence
 * (wizard config takes priority over Claude config if both provided).
 *
 * @param configs - Array of MCP config arrays to merge
 * @returns Merged and deduplicated array
 */
export function mergeMCPs(configs: MCPConfig[]): MCPConfig[] {
  const seen = new Set<string>();
  const merged: MCPConfig[] = [];

  for (const mcp of configs) {
    if (!seen.has(mcp.serviceName)) {
      seen.add(mcp.serviceName);
      merged.push(mcp);
    }
  }

  return merged;
}

/**
 * Discover all Okta-authenticated MCPs
 *
 * Reads from both wizard and Claude Code configs, merges, and filters
 * for Okta-authenticated MCPs only.
 *
 * Gracefully handles missing configs (returns empty array).
 *
 * @returns Array of Okta-authenticated MCP configurations
 *
 * @example
 * const mcps = await discoverMCPs();
 * // Returns: [
 * //   { name: "GoogleDocs", serviceName: "googledocs", ... },
 * //   { name: "Atlassian", serviceName: "atlassian", ... },
 * // ]
 */
export async function discoverMCPs(): Promise<MCPConfig[]> {
  try {
    // Read from both sources
    const wizardMCPs = await readWizardConfig();
    const claudeMCPs = await readClaudeConfig();

    // Merge (wizard takes priority)
    const allMCPs = mergeMCPs([...wizardMCPs, ...claudeMCPs]);

    // Filter for Okta only (should already be filtered, but double-check)
    return allMCPs.filter(mcp => mcp.auth === 'okta');

  } catch (error) {
    // Graceful fallback: no configs found
    console.warn('⚠️  No MCP configuration found');
    console.warn('  Run \'mcp-wizard setup\' to configure MCPs');
    return [];
  }
}

/**
 * Capitalize first letter of string
 *
 * @param str - String to capitalize
 * @returns Capitalized string
 */
function capitalize(str: string): string {
  return str.charAt(0).toUpperCase() + str.slice(1);
}
