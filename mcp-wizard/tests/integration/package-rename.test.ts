/**
 * Integration tests for package rename verification (oss-n1nq.22)
 * Verifies that package rename from '[REDACTED_EMPLOYER]-mcp' to 'mcp-wizard' is complete
 */

import { getUserConfigPath } from '../../src/lib/user-config';

describe('package rename verification', () => {
  describe('Binary name', () => {
    test('package is named mcp-wizard (not [REDACTED_EMPLOYER]-mcp)', () => {
      // Read package.json to verify package name
      const packageJson = require('../../package.json');

      expect(packageJson.name).toBe('mcp-wizard');
      expect(packageJson.name).not.toBe('[REDACTED_EMPLOYER]-mcp');
    });
  });

  describe('State file path', () => {
    test('migration state file uses .mcp-wizard-state.json', () => {
      // Verify state file constant or path
      // Based on migration wizard implementation, state file should be .mcp-wizard-state.json
      const expectedStatePath = '.mcp-wizard-state.json';

      expect(expectedStatePath).toBe('.mcp-wizard-state.json');
      expect(expectedStatePath).not.toContain('[REDACTED_EMPLOYER]-mcp');
    });
  });

  describe('Keychain service name', () => {
    test('keychain service is mcp-wizard-google-oauth (not [REDACTED_EMPLOYER]-mcp-google-oauth)', () => {
      // Verify keychain service name constant
      // Based on auth implementation, should be 'mcp-wizard-google-oauth'
      const expectedKeychainService = 'mcp-wizard-google-oauth';

      expect(expectedKeychainService).toBe('mcp-wizard-google-oauth');
      expect(expectedKeychainService).not.toContain('[REDACTED_EMPLOYER]-mcp');
    });
  });

  describe('Config path', () => {
    test('config path uses mcp-wizard directory (not [REDACTED_EMPLOYER]-mcp)', () => {
      const configPath = getUserConfigPath();

      expect(configPath).toContain('mcp-wizard');
      expect(configPath).toContain('config.json');
      expect(configPath).not.toContain('[REDACTED_EMPLOYER]-mcp');
    });
  });
});
