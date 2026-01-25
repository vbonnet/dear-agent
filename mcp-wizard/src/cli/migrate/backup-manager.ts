/**
 * Backup Manager - handles timestamped backups and restoration
 */

import * as fs from 'fs';
import * as path from 'path';
import { BackupInfo, BackupError } from './types';
import { getConfigPath } from './config-manager';

/**
 * Format timestamp for backup filename: YYYY-MM-DD-HH-MM-SS
 */
function formatTimestamp(date: Date): string {
  return date.toISOString()
    .replace(/:/g, '-')
    .replace(/\..+/, '')
    .replace('T', '-');
}

/**
 * Parse timestamp from backup filename
 */
function parseTimestamp(filename: string): Date | null {
  const match = filename.match(/\.backup-(\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2})$/);
  if (!match) return null;

  const timestamp = match[1];
  const [year, month, day, hour, minute, second] = timestamp.split('-').map(Number);

  return new Date(Date.UTC(year, month - 1, day, hour, minute, second));
}

/**
 * Verify backup file is valid JSON
 */
export function verifyBackup(backupPath: string): boolean {
  try {
    if (!fs.existsSync(backupPath)) {
      return false;
    }

    const content = fs.readFileSync(backupPath, 'utf8');
    JSON.parse(content);
    return true;
  } catch {
    return false;
  }
}

/**
 * Create timestamped backup of current config
 */
export function createBackup(): string {
  const configPath = getConfigPath();

  try {
    if (!fs.existsSync(configPath)) {
      throw new BackupError(`Config file not found at ${configPath}. Cannot create backup.`);
    }

    const timestamp = formatTimestamp(new Date());
    const backupPath = `${configPath}.backup-${timestamp}`;

    // Copy config to backup location
    fs.copyFileSync(configPath, backupPath);

    // Verify backup was created correctly
    if (!verifyBackup(backupPath)) {
      throw new BackupError('Backup verification failed. Backup may be corrupted.');
    }

    return backupPath;
  } catch (error) {
    if (error instanceof BackupError) {
      throw error;
    }

    if ((error as NodeJS.ErrnoException).code === 'EACCES') {
      throw new BackupError(`Cannot write backup file. Check file permissions for ${configPath}`);
    }

    if ((error as NodeJS.ErrnoException).code === 'ENOSPC') {
      throw new BackupError('Insufficient disk space. Cannot create backup.');
    }

    throw new BackupError(`Failed to create backup: ${(error as Error).message}`);
  }
}

/**
 * List all available backups, sorted by timestamp (most recent first)
 */
export function listBackups(): BackupInfo[] {
  const configPath = getConfigPath();
  const configDir = path.dirname(configPath);
  const configFilename = path.basename(configPath);

  try {
    if (!fs.existsSync(configDir)) {
      return [];
    }

    const files = fs.readdirSync(configDir);
    const backups: BackupInfo[] = [];

    for (const file of files) {
      if (file.startsWith(configFilename + '.backup-')) {
        const backupPath = path.join(configDir, file);
        const timestamp = parseTimestamp(file);

        if (timestamp && verifyBackup(backupPath)) {
          backups.push({ path: backupPath, timestamp });
        }
      }
    }

    // Sort by timestamp (most recent first)
    backups.sort((a, b) => b.timestamp.getTime() - a.timestamp.getTime());

    return backups;
  } catch {
    return [];
  }
}

/**
 * Restore config from backup (most recent if not specified)
 */
export function restoreBackup(backupPath?: string): void {
  const configPath = getConfigPath();
  const tempPath = `${configPath}.tmp`;

  try {
    // Find most recent backup if not specified
    if (!backupPath) {
      const backups = listBackups();
      if (backups.length === 0) {
        throw new BackupError('No backups found. Cannot restore.');
      }
      backupPath = backups[0].path;
    }

    // Verify backup exists and is valid
    if (!verifyBackup(backupPath)) {
      throw new BackupError(`Invalid backup: ${backupPath}. Cannot restore.`);
    }

    // Copy backup to temp file
    fs.copyFileSync(backupPath, tempPath);

    // Atomic rename to final location
    fs.renameSync(tempPath, configPath);
  } catch (error) {
    // Cleanup temp file on error
    if (fs.existsSync(tempPath)) {
      try {
        fs.unlinkSync(tempPath);
      } catch {
        // Ignore cleanup errors
      }
    }

    if (error instanceof BackupError) {
      throw error;
    }

    if ((error as NodeJS.ErrnoException).code === 'EACCES') {
      throw new BackupError(`Cannot write to config directory. Check file permissions for ${configPath}`);
    }

    throw new BackupError(`Failed to restore backup: ${(error as Error).message}`);
  }
}
