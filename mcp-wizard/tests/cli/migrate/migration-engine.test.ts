/**
 * Unit tests for migration-engine module
 */

import {
  migrateToGateway,
  generatePreview,
} from '../../../src/cli/migrate/migration-engine';
import { FIXTURES } from './__helpers__/fixtures';

// Mock getConfigPath
jest.mock('../../../src/cli/migrate/config-manager', () => ({
  getConfigPath: jest.fn(() => '/Users/testuser/Library/Application Support/Claude/claude_desktop_config.json'),
}));

describe('MigrationEngine', () => {
  describe('migrateToGateway', () => {
    test('should convert multiple MCP servers to single gateway', () => {
      const result = migrateToGateway(FIXTURES.validMcpConfig);

      expect(result.migratedServers).toEqual(['googledocs', 'slack', 'filesystem']);
      expect(result.newConfig.mcpServers).toHaveProperty('mcp-wizard');
      expect(result.newConfig.mcpServers!['mcp-wizard'].command).toBe('mcp-wizard');
      expect(result.newConfig.mcpServers!['mcp-wizard'].args).toEqual(['serve']);
    });

    test('should preserve non-MCP config sections', () => {
      const result = migrateToGateway(FIXTURES.configWithNonMcpSections);

      expect(result.newConfig.otherSettings).toEqual({ theme: 'dark', fontSize: 14 });
      expect(result.newConfig.customData).toBe('preserve this');
    });

    test('should merge environment variables from all servers', () => {
      const result = migrateToGateway(FIXTURES.configWithEnvVars);

      expect(result.envVars).toEqual({
        GOOGLE_DRIVE_TOKEN: 'test-token-123',
        GOOGLE_DRIVE_REFRESH: 'refresh-token-456',
        SLACK_BOT_TOKEN: 'xoxb-test-token',
      });
      expect(result.newConfig.mcpServers!['mcp-wizard'].env).toEqual(result.envVars);
    });

    test('should handle empty config', () => {
      const result = migrateToGateway(FIXTURES.emptyConfig);

      expect(result.migratedServers).toEqual([]);
      expect(result.envVars).toEqual({});
      expect(result.newConfig.mcpServers).toHaveProperty('mcp-wizard');
    });

    test('should handle config with no env vars', () => {
      const result = migrateToGateway(FIXTURES.validMcpConfig);

      expect(result.newConfig.mcpServers!['mcp-wizard'].env).toBeUndefined();
    });

    test('should handle single server config', () => {
      const result = migrateToGateway(FIXTURES.singleServerConfig);

      expect(result.migratedServers).toEqual(['filesystem']);
      expect(result.newConfig.mcpServers).toHaveProperty('mcp-wizard');
    });

    test('should be idempotent (migrating already-migrated config)', () => {
      const result = migrateToGateway(FIXTURES.migratedConfig);

      expect(result.migratedServers).toEqual(['mcp-wizard']);
      expect(result.newConfig.mcpServers).toHaveProperty('mcp-wizard');
    });
  });

  describe('generatePreview', () => {
    test('should generate preview showing current MCPs', () => {
      const preview = generatePreview(FIXTURES.validMcpConfig);

      expect(preview).toContain('Detected config:');
      expect(preview).toContain('Current MCPs (3 servers)');
      expect(preview).toContain('googledocs');
      expect(preview).toContain('slack');
      expect(preview).toContain('filesystem');
    });

    test('should show single gateway after migration', () => {
      const preview = generatePreview(FIXTURES.validMcpConfig);

      expect(preview).toContain('After migration:');
      expect(preview).toContain('Single mcp-wizard gateway');
      expect(preview).toContain('All 3 MCPs available through gateway');
    });

    test('should show environment variables if present', () => {
      const preview = generatePreview(FIXTURES.configWithEnvVars);

      expect(preview).toContain('Environment variables migrated:');
      expect(preview).toContain('GOOGLE_DRIVE_TOKEN');
      expect(preview).toContain('SLACK_BOT_TOKEN');
    });

    test('should indicate dry run', () => {
      const preview = generatePreview(FIXTURES.validMcpConfig);

      expect(preview).toContain('DRY RUN');
      expect(preview).toContain('No changes applied');
      expect(preview).toContain('Run without --dry-run');
    });

    test('should handle empty config', () => {
      const preview = generatePreview(FIXTURES.emptyConfig);

      expect(preview).toContain('Current MCPs (0 servers)');
      expect(preview).toContain('(none)');
    });

    test('should handle single server', () => {
      const preview = generatePreview(FIXTURES.singleServerConfig);

      expect(preview).toContain('Current MCPs (1 server)'); // Singular "server"
      expect(preview).toContain('All 1 MCP available'); // Singular "MCP"
    });
  });
});
