/**
 * Integration tests for package rename verification
 *
 * Ensures that the package rename from company-specific to generic "mcp-wizard"
 * doesn't break functionality and that no hardcoded company names remain in logic.
 */

import { generateMcpConfig } from '../../src/lib/config';
import { getUserConfigPath } from '../../src/lib/user-config';

describe('Package Rename Verification', () => {
  it('should not hardcode "[REDACTED_EMPLOYER]" in config paths', async () => {
    const config = await generateMcpConfig(['googledocs', 'glean', 'slack', 'atlassian']);

    // Check all MCP server configurations
    Object.entries(config.mcpServers).forEach(([name, server]) => {
      // Check command arguments
      server.args?.forEach(arg => {
        if (typeof arg === 'string' && arg.includes('/')) {
          // Paths should use generic "mcp-servers", not "[REDACTED_EMPLOYER]-mcp-servers"
          expect(arg.toLowerCase()).not.toContain('[REDACTED_EMPLOYER]-mcp');
          expect(arg.toLowerCase()).not.toContain('[REDACTED_EMPLOYER]/');

          // Should use generic directory name
          if (arg.includes('mcp-servers')) {
            expect(arg).toContain('mcp-servers');
          }
        }
      });

      // Check environment variables
      if (server.env) {
        Object.values(server.env).forEach(value => {
          if (typeof value === 'string' && value.includes('/')) {
            expect(value.toLowerCase()).not.toContain('[REDACTED_EMPLOYER]-mcp');
            expect(value.toLowerCase()).not.toContain('[REDACTED_EMPLOYER]/');
          }
        });
      }
    });
  });

  it('should use package name "mcp-wizard" for config directory', () => {
    // Verify getUserConfigPath returns mcp-wizard, not [REDACTED_EMPLOYER]-wizard
    const configPath = getUserConfigPath();

    expect(configPath).toContain('mcp-wizard');
    expect(configPath.toLowerCase()).not.toContain('[REDACTED_EMPLOYER]-wizard');
    expect(configPath.toLowerCase()).not.toContain('[REDACTED_EMPLOYER]/mcp');

    // Should follow XDG spec
    expect(configPath).toMatch(/\.config\/mcp-wizard\/config\.json$/);
  });

  it('config generation works with new package name', async () => {
    // This test verifies that the config generation doesn't rely on old package names
    const config = await generateMcpConfig(['googledocs']);

    expect(config.mcpServers.GoogleDocs).toBeDefined();
    expect(config.mcpServers.GoogleDocs.command).toBe('node');

    // Verify config is valid and functional
    expect(config.mcpServers.GoogleDocs.args).toHaveLength(1);
    expect(config.mcpServers.GoogleDocs.env).toHaveProperty('CREDENTIALS_PATH');
    expect(config.mcpServers.GoogleDocs.env).toHaveProperty('TOKEN_PATH');
  });

  it('should use generic paths for all MCPs', async () => {
    const config = await generateMcpConfig(['googledocs', 'glean', 'slack']);

    // All paths should use generic "mcp-servers" directory
    Object.values(config.mcpServers).forEach(server => {
      const allValues = [
        ...(server.args || []),
        ...Object.values(server.env || {}),
      ];

      allValues.forEach(value => {
        if (typeof value === 'string' && value.includes('/') && value.includes('mcp')) {
          // Skip NPX packages (e.g., @gleanwork/mcp-server, @modelcontextprotocol/server-slack)
          const isNpxPackage = value.startsWith('@') || !value.includes('home');

          if (!isNpxPackage) {
            // Should use generic pattern: ~/mcp-servers/{server-name}
            expect(value).toMatch(/mcp-servers\/[^/]+/);
            // Should NOT use company-specific pattern
            expect(value.toLowerCase()).not.toMatch(/[REDACTED_EMPLOYER]-mcp|acme-mcp|company.*-mcp/);
          }
        }
      });
    });
  });

  it('package name is used consistently across config paths', () => {
    const configPath = getUserConfigPath();

    // Extract package name from config path
    const match = configPath.match(/\.config\/([^/]+)\/config\.json$/);
    expect(match).not.toBeNull();

    const packageName = match![1];
    expect(packageName).toBe('mcp-wizard');

    // This ensures the package name is consistent and not hardcoded differently in different places
  });
});
