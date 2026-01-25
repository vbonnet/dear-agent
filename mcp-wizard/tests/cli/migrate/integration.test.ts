/**
 * Integration tests for migration wizard
 * Tests complete workflows end-to-end
 */

import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import { getConfigPath, readConfig, writeConfig } from '../../../src/cli/migrate/config-manager';
import { createBackup, restoreBackup, listBackups } from '../../../src/cli/migrate/backup-manager';
import { migrateToGateway, generatePreview } from '../../../src/cli/migrate/migration-engine';
import { detectChezmoi } from '../../../src/cli/migrate/chezmoi-detector';
import { FIXTURES } from './__helpers__/fixtures';

// Mock modules for integration tests
jest.mock('fs');
jest.mock('os');
jest.mock('child_process');

describe('Migration Integration Tests', () => {
  const mockConfigPath = '/Users/testuser/Library/Application Support/Claude/claude_desktop_config.json';

  beforeEach(() => {
    jest.clearAllMocks();
    (os.platform as jest.Mock).mockReturnValue('darwin');
    (os.homedir as jest.Mock).mockReturnValue('/Users/testuser');
    (fs.existsSync as jest.Mock).mockReturnValue(true);
    (fs.writeFileSync as jest.Mock).mockImplementation(() => {});
    (fs.renameSync as jest.Mock).mockImplementation(() => {});
    (fs.copyFileSync as jest.Mock).mockImplementation(() => {});
  });

  describe('dry-run workflow', () => {
    test('should preview changes without modifying files', () => {
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      const config = readConfig();
      const preview = generatePreview(config);

      expect(preview).toContain('DRY RUN');
      expect(preview).toContain('googledocs');
      expect(preview).toContain('slack');
      expect(fs.writeFileSync).not.toHaveBeenCalled();
    });

    test('should show accurate server count in preview', () => {
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      const config = readConfig();
      const preview = generatePreview(config);

      expect(preview).toContain('3 servers');
      expect(preview).toContain('All 3 MCPs available');
    });
  });

  describe('migrate workflow', () => {
    test('should complete full migration successfully', () => {
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      // Step 1: Read current config
      const config = readConfig();
      expect(config.mcpServers).toBeDefined();

      // Step 2: Create backup
      const backupPath = createBackup();
      expect(backupPath).toMatch(/\.backup-/);
      expect(fs.copyFileSync).toHaveBeenCalled();

      // Step 3: Migrate config
      const result = migrateToGateway(config);
      expect(result.migratedServers).toHaveLength(3);

      // Step 4: Write new config
      writeConfig(result.newConfig);
      expect(fs.writeFileSync).toHaveBeenCalled();
    });

    test('should preserve existing MCP servers during migration', () => {
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      const config = readConfig();
      const result = migrateToGateway(config);

      expect(result.migratedServers).toContain('googledocs');
      expect(result.migratedServers).toContain('slack');
      expect(result.migratedServers).toContain('filesystem');
    });

    test('should handle config with environment variables', () => {
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.configWithEnvVars));

      const config = readConfig();
      const result = migrateToGateway(config);

      expect(result.envVars).toHaveProperty('GOOGLE_DRIVE_TOKEN');
      expect(result.envVars).toHaveProperty('SLACK_BOT_TOKEN');
      expect(result.newConfig.mcpServers!['mcp-wizard'].env).toEqual(result.envVars);
    });
  });

  describe('rollback workflow', () => {
    test('should restore from backup successfully', () => {
      const backupPath = `${mockConfigPath}.backup-2024-01-01-12-00-00`;
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      restoreBackup(backupPath);

      expect(fs.copyFileSync).toHaveBeenCalledWith(backupPath, expect.stringContaining('.tmp'));
      expect(fs.renameSync).toHaveBeenCalled();
    });

    test('should restore from most recent backup if not specified', () => {
      const configDir = path.dirname(mockConfigPath);
      const configFilename = path.basename(mockConfigPath);
      (fs.readdirSync as jest.Mock).mockReturnValue([
        `${configFilename}.backup-2024-01-02-12-00-00`,
        `${configFilename}.backup-2024-01-01-10-00-00`,
      ]);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      restoreBackup();

      expect(fs.copyFileSync).toHaveBeenCalledWith(
        path.join(configDir, `${configFilename}.backup-2024-01-02-12-00-00`),
        expect.stringContaining('.tmp')
      );
    });
  });

  describe('chezmoi user workflow', () => {
    test('should detect chezmoi and provide warning', () => {
      require('child_process').execSync = jest.fn(() => Buffer.from('/usr/bin/chezmoi'));

      const status = detectChezmoi();

      expect(status.detected).toBe(true);
      expect(status.message).toContain('CHEZMOI DETECTED');
      expect(status.message).toContain('chezmoi edit');
    });

    test('should not interfere with migration if chezmoi not detected', () => {
      require('child_process').execSync = jest.fn(() => {
        throw new Error('not found');
      });

      const status = detectChezmoi();

      expect(status.detected).toBe(false);
      expect(status.message).toBeUndefined();
    });
  });

  describe('error recovery', () => {
    test('should handle read config failure gracefully', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(false);

      expect(() => readConfig()).toThrow('Claude Desktop config not found');
    });

    test('should cleanup temp file on write failure', () => {
      (fs.writeFileSync as jest.Mock).mockImplementation(() => {
        throw new Error('Disk full');
      });
      (fs.existsSync as jest.Mock).mockReturnValue(true);

      expect(() => writeConfig(FIXTURES.validMcpConfig)).toThrow();
      expect(fs.unlinkSync).toHaveBeenCalled();
    });
  });

  describe('multiple backups', () => {
    test('should list all backups chronologically', () => {
      const configDir = path.dirname(mockConfigPath);
      const configFilename = path.basename(mockConfigPath);
      (fs.readdirSync as jest.Mock).mockReturnValue([
        `${configFilename}.backup-2024-01-01-10-00-00`,
        `${configFilename}.backup-2024-01-02-12-00-00`,
        `${configFilename}.backup-2024-01-01-15-00-00`,
      ]);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      const backups = listBackups();

      expect(backups).toHaveLength(3);
      expect(backups[0].path).toContain('2024-01-02'); // Most recent first
    });
  });
});
