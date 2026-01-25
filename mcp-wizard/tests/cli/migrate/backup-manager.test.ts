/**
 * Unit tests for backup-manager module
 */

import * as fs from 'fs';
import * as path from 'path';
import {
  createBackup,
  listBackups,
  restoreBackup,
  verifyBackup,
} from '../../../src/cli/migrate/backup-manager';
import { BackupError } from '../../../src/cli/migrate/types';
import { FIXTURES, createFsError, mockBackupFilename } from './__helpers__/fixtures';

// Mock modules
jest.mock('fs');
jest.mock('../../../src/cli/migrate/config-manager');

describe('BackupManager', () => {
  const mockConfigPath = '/Users/testuser/Library/Application Support/Claude/claude_desktop_config.json';

  beforeEach(() => {
    jest.clearAllMocks();
    // Mock getConfigPath
    require('../../../src/cli/migrate/config-manager').getConfigPath = jest.fn(() => mockConfigPath);
  });

  describe('verifyBackup', () => {
    test('should return true for valid JSON backup', () => {
      const backupPath = `${mockConfigPath}.backup-2024-01-01-12-00-00`;
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      const result = verifyBackup(backupPath);

      expect(result).toBe(true);
      expect(fs.readFileSync).toHaveBeenCalledWith(backupPath, 'utf8');
    });

    test('should return false for missing backup file', () => {
      const backupPath = `${mockConfigPath}.backup-2024-01-01-12-00-00`;
      (fs.existsSync as jest.Mock).mockReturnValue(false);

      const result = verifyBackup(backupPath);

      expect(result).toBe(false);
    });

    test('should return false for malformed JSON backup', () => {
      const backupPath = `${mockConfigPath}.backup-2024-01-01-12-00-00`;
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(FIXTURES.malformedJson);

      const result = verifyBackup(backupPath);

      expect(result).toBe(false);
    });

    test('should return false when read fails', () => {
      const backupPath = `${mockConfigPath}.backup-2024-01-01-12-00-00`;
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockImplementation(() => {
        throw new Error('Read failed');
      });

      const result = verifyBackup(backupPath);

      expect(result).toBe(false);
    });
  });

  describe('createBackup', () => {
    beforeEach(() => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.copyFileSync as jest.Mock).mockImplementation(() => {});
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));
    });

    test('should create backup with timestamp', () => {
      const backupPath = createBackup();

      expect(backupPath).toMatch(/\.backup-\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2}$/);
      expect(fs.copyFileSync).toHaveBeenCalledWith(mockConfigPath, backupPath);
    });

    test('should verify backup after creation', () => {
      createBackup();

      // Verify readFileSync was called for verification
      expect(fs.readFileSync).toHaveBeenCalled();
    });

    test('should throw BackupError if config not found', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(false);

      expect(() => createBackup()).toThrow(BackupError);
      expect(() => createBackup()).toThrow('Config file not found');
    });

    test('should throw BackupError if verification fails', () => {
      // Mock copyFileSync to succeed, but readFileSync returns invalid JSON on verification
      (fs.readFileSync as jest.Mock).mockReturnValue(FIXTURES.malformedJson);

      expect(() => createBackup()).toThrow(BackupError);
    });

    test('should handle permission denied error', () => {
      (fs.copyFileSync as jest.Mock).mockImplementation(() => {
        throw createFsError('EACCES', 'Permission denied');
      });

      expect(() => createBackup()).toThrow(BackupError);
      expect(() => createBackup()).toThrow('Check file permissions');
    });

    test('should handle disk full error', () => {
      (fs.copyFileSync as jest.Mock).mockImplementation(() => {
        throw createFsError('ENOSPC', 'No space left on device');
      });

      expect(() => createBackup()).toThrow(BackupError);
      expect(() => createBackup()).toThrow('Insufficient disk space');
    });
  });

  describe('listBackups', () => {
    const configDir = path.dirname(mockConfigPath);
    const configFilename = path.basename(mockConfigPath);

    test('should list backups sorted by timestamp (most recent first)', () => {
      const files = [
        `${configFilename}.backup-2024-01-01-10-00-00`,
        `${configFilename}.backup-2024-01-02-12-00-00`,
        `${configFilename}.backup-2024-01-01-15-00-00`,
      ];

      (fs.existsSync as jest.Mock).mockImplementation((path) => {
        // Config directory exists, and all backup files exist
        return true;
      });
      (fs.readdirSync as jest.Mock).mockReturnValue(files);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      const backups = listBackups();

      expect(backups).toHaveLength(3);
      // Most recent first (2024-01-02)
      expect(backups[0].path).toContain('2024-01-02');
      expect(backups[1].path).toContain('2024-01-01-15');
      expect(backups[2].path).toContain('2024-01-01-10');
    });

    test('should return empty array if config directory does not exist', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(false);

      const backups = listBackups();

      expect(backups).toEqual([]);
    });

    test('should filter out non-backup files', () => {
      const files = [
        `${configFilename}.backup-2024-01-01-10-00-00`,
        'other-file.txt',
        'config.json',
        `${configFilename}.old`,
      ];

      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readdirSync as jest.Mock).mockReturnValue(files);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      const backups = listBackups();

      expect(backups).toHaveLength(1);
      expect(backups[0].path).toContain('backup-2024-01-01');
    });

    test('should filter out corrupt backups', () => {
      const files = [
        `${configFilename}.backup-2024-01-01-10-00-00`,
        `${configFilename}.backup-2024-01-02-12-00-00`,
      ];

      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readdirSync as jest.Mock).mockReturnValue(files);
      (fs.readFileSync as jest.Mock)
        .mockReturnValueOnce(JSON.stringify(FIXTURES.validMcpConfig)) // First backup valid
        .mockReturnValueOnce(FIXTURES.malformedJson); // Second backup corrupt

      const backups = listBackups();

      expect(backups).toHaveLength(1);
      expect(backups[0].path).toContain('2024-01-01');
    });

    test('should return empty array on read error', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readdirSync as jest.Mock).mockImplementation(() => {
        throw new Error('Read failed');
      });

      const backups = listBackups();

      expect(backups).toEqual([]);
    });
  });

  describe('restoreBackup', () => {
    const tempPath = `${mockConfigPath}.tmp`;

    beforeEach(() => {
      (fs.existsSync as jest.Mock).mockReturnValue(false);
      (fs.copyFileSync as jest.Mock).mockImplementation(() => {});
      (fs.renameSync as jest.Mock).mockImplementation(() => {});
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));
    });

    test('should restore from specified backup path', () => {
      const backupPath = `${mockConfigPath}.backup-2024-01-01-12-00-00`;
      (fs.existsSync as jest.Mock).mockReturnValue(true);

      restoreBackup(backupPath);

      expect(fs.copyFileSync).toHaveBeenCalledWith(backupPath, tempPath);
      expect(fs.renameSync).toHaveBeenCalledWith(tempPath, mockConfigPath);
    });

    test('should restore from most recent backup if path not specified', () => {
      const configDir = path.dirname(mockConfigPath);
      const configFilename = path.basename(mockConfigPath);
      const files = [
        `${configFilename}.backup-2024-01-02-12-00-00`, // Most recent
        `${configFilename}.backup-2024-01-01-10-00-00`,
      ];

      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readdirSync as jest.Mock).mockReturnValue(files);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(FIXTURES.validMcpConfig));

      restoreBackup();

      // Should copy the most recent backup (2024-01-02)
      expect(fs.copyFileSync).toHaveBeenCalledWith(
        path.join(configDir, files[0]),
        tempPath
      );
    });

    test('should throw BackupError if no backups found', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(false);

      expect(() => restoreBackup()).toThrow(BackupError);
      expect(() => restoreBackup()).toThrow('No backups found');
    });

    test('should throw BackupError for invalid backup', () => {
      const backupPath = `${mockConfigPath}.backup-2024-01-01-12-00-00`;
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(FIXTURES.malformedJson);

      expect(() => restoreBackup(backupPath)).toThrow(BackupError);
      expect(() => restoreBackup(backupPath)).toThrow('Invalid backup');
    });

    test('should cleanup temp file on restore failure', () => {
      const backupPath = `${mockConfigPath}.backup-2024-01-01-12-00-00`;
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.renameSync as jest.Mock).mockImplementation(() => {
        throw new Error('Rename failed');
      });

      expect(() => restoreBackup(backupPath)).toThrow(BackupError);
      expect(fs.unlinkSync).toHaveBeenCalledWith(tempPath);
    });

    test('should handle permission denied error', () => {
      const backupPath = `${mockConfigPath}.backup-2024-01-01-12-00-00`;
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.copyFileSync as jest.Mock).mockImplementation(() => {
        throw createFsError('EACCES', 'Permission denied');
      });

      expect(() => restoreBackup(backupPath)).toThrow(BackupError);
      expect(() => restoreBackup(backupPath)).toThrow('Check file permissions');
    });
  });
});
