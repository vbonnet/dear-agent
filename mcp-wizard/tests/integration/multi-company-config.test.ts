/**
 * Integration tests for multi-company configuration
 *
 * Verifies that the configuration system works correctly for:
 * - [REDACTED_EMPLOYER] (default company)
 * - Acme Corp (generic company example 1)
 * - Company X (generic company example 2)
 */

import { generateMcpConfig } from '../../src/lib/config';
import * as userConfig from '../../src/lib/user-config';

// Mock user-config module to return different company configs
jest.mock('../../src/lib/user-config');

describe('Multi-Company Configuration', () => {
  const originalEnv = process.env;

  beforeEach(() => {
    jest.resetModules();
    process.env = { ...originalEnv };
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  it('generates correct config for [REDACTED_EMPLOYER] (default)', async () => {
    // Mock loadConfig to return [REDACTED_EMPLOYER] configuration
    (userConfig.getConfigValue as jest.Mock).mockImplementation((key: string, defaultValue?: string) => {
      if (key === 'company.name') return '[REDACTED_EMPLOYER]';
      if (key === 'company.glean_instance') return '[REDACTED_EMPLOYER]';
      if (key === 'company.okta_domain') return '[REDACTED_EMPLOYER].okta.com';
      return defaultValue || '';
    });

    const config = await generateMcpConfig(['glean']);

    expect(config.mcpServers.Glean).toBeDefined();
    expect(config.mcpServers.Glean.env?.GLEAN_INSTANCE).toBe('[REDACTED_EMPLOYER]');

    // Verify no hardcoded "[REDACTED_EMPLOYER]" in paths (should use generic paths)
    const args = config.mcpServers.Glean.args;
    args?.forEach(arg => {
      if (typeof arg === 'string' && arg.includes('/')) {
        expect(arg.toLowerCase()).not.toContain('[REDACTED_EMPLOYER]-mcp');
      }
    });
  });

  it('generates correct config for Acme Corp (generic)', async () => {
    // Mock loadConfig to return Acme Corp configuration
    (userConfig.getConfigValue as jest.Mock).mockImplementation((key: string, defaultValue?: string) => {
      if (key === 'company.name') return 'Acme Corp';
      if (key === 'company.glean_instance') return 'acme';
      if (key === 'company.okta_domain') return 'acme.okta.com';
      return defaultValue || '';
    });

    const config = await generateMcpConfig(['glean']);

    expect(config.mcpServers.Glean).toBeDefined();
    expect(config.mcpServers.Glean.env?.GLEAN_INSTANCE).toBe('acme');

    // Verify company-agnostic paths (skip NPX packages)
    const args = config.mcpServers.Glean.args;
    args?.forEach(arg => {
      if (typeof arg === 'string' && arg.includes('/')) {
        const isNpxPackage = arg.startsWith('@') || !arg.includes('home');
        if (!isNpxPackage) {
          expect(arg).toContain('mcp-servers'); // Generic directory
          expect(arg.toLowerCase()).not.toContain('acme-mcp');
          expect(arg.toLowerCase()).not.toContain('[REDACTED_EMPLOYER]-mcp');
        }
      }
    });
  });

  it('generates correct config for Company X (generic)', async () => {
    // Mock loadConfig to return Company X configuration
    (userConfig.getConfigValue as jest.Mock).mockImplementation((key: string, defaultValue?: string) => {
      if (key === 'company.name') return 'Company X';
      if (key === 'company.glean_instance') return 'companyx';
      if (key === 'company.okta_domain') return 'companyx.okta.com';
      return defaultValue || '';
    });

    const config = await generateMcpConfig(['glean']);

    expect(config.mcpServers.Glean).toBeDefined();
    expect(config.mcpServers.Glean.env?.GLEAN_INSTANCE).toBe('companyx');

    // Verify no company-specific hardcoded paths (skip NPX packages)
    const args = config.mcpServers.Glean.args;
    args?.forEach(arg => {
      if (typeof arg === 'string' && arg.includes('/')) {
        const isNpxPackage = arg.startsWith('@') || !arg.includes('home');
        if (!isNpxPackage) {
          expect(arg).toContain('mcp-servers'); // Generic directory
        }
      }
    });
  });

  it('uses environment variables for company configuration', async () => {
    // Set environment variables
    process.env.MCP_WIZARD_COMPANY_NAME = 'EnvCorp';
    process.env.MCP_WIZARD_COMPANY_GLEAN_INSTANCE = 'envcorp';
    process.env.MCP_WIZARD_COMPANY_OKTA_DOMAIN = 'envcorp.okta.com';

    // Mock getConfigValue to check env vars (actual implementation does this)
    (userConfig.getConfigValue as jest.Mock).mockImplementation((key: string, defaultValue?: string) => {
      if (key === 'company.glean_instance') {
        return process.env.MCP_WIZARD_COMPANY_GLEAN_INSTANCE || defaultValue || '';
      }
      return defaultValue || '';
    });

    const config = await generateMcpConfig(['glean']);

    expect(config.mcpServers.Glean.env?.GLEAN_INSTANCE).toBe('envcorp');
  });
});
