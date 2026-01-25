/**
 * Migration CLI - entry point for mcp-wizard migrate command
 */

import { readConfig, writeConfig, getConfigPath } from './migrate/config-manager';
import { createBackup, restoreBackup, listBackups } from './migrate/backup-manager';
import { migrateToGateway, generatePreview } from './migrate/migration-engine';
import { detectChezmoi } from './migrate/chezmoi-detector';

/**
 * Handle dry-run mode (preview migration without applying)
 */
async function handleDryRun(): Promise<void> {
  try {
    const config = readConfig();
    const preview = generatePreview(config);
    console.log(preview);
  } catch (error) {
    throw new Error(`Dry-run failed: ${(error as Error).message}`);
  }
}

/**
 * Handle rollback mode (restore from most recent backup)
 */
async function handleRollback(): Promise<void> {
  try {
    const backups = listBackups();
    if (backups.length === 0) {
      throw new Error('No backups found. Cannot restore.');
    }

    const mostRecent = backups[0];
    const timestamp = mostRecent.timestamp.toISOString().replace('T', ' ').split('.')[0];

    console.log(`🔍 Found backup: ${mostRecent.path}`);
    console.log(`   Created: ${timestamp} UTC`);
    console.log('⏮️  Restoring backup...');

    restoreBackup(mostRecent.path);

    console.log('✅ Rollback complete!\n');
    console.log('⚠️  IMPORTANT: Restart Claude Desktop to apply changes');
  } catch (error) {
    throw new Error(`Rollback failed: ${(error as Error).message}`);
  }
}

/**
 * Handle full migration (backup + migrate + write)
 */
async function handleMigration(): Promise<void> {
  try {
    const configPath = getConfigPath();
    console.log(`🔍 Detected config: ${configPath}\n`);

    // Create backup
    console.log('💾 Creating backup...');
    const backupPath = createBackup();
    const backupFilename = backupPath.split('/').pop();
    console.log(`✅ Backup created: ${backupFilename}\n`);

    // Migrate
    const currentConfig = readConfig();
    const { migratedServers, newConfig } = migrateToGateway(currentConfig);

    const serverCount = migratedServers.length;
    const serverText = serverCount === 1 ? 'server' : 'servers';

    if (serverCount === 0) {
      console.log('ℹ️  No MCP servers found in current config');
      console.log('   Creating mcp-wizard gateway entry anyway\n');
    } else {
      console.log(`🔄 Migrating ${serverCount} MCP ${serverText} to gateway:`);
      migratedServers.forEach(s => console.log(`   - ${s}`));
      console.log();
    }

    writeConfig(newConfig);
    console.log('✅ Migration complete!\n');

    // Chezmoi warning
    const chezmoiStatus = detectChezmoi();
    if (chezmoiStatus.detected && chezmoiStatus.message) {
      console.log(chezmoiStatus.message);
      console.log();
    }

    console.log('⚠️  IMPORTANT: Restart Claude Desktop to apply changes\n');
    console.log('If anything goes wrong, rollback with:');
    console.log('  mcp-wizard migrate --rollback');
  } catch (error) {
    throw new Error(`Migration failed: ${(error as Error).message}`);
  }
}

/**
 * Main migrate command handler
 *
 * @param options Command-line options
 * @param options.dryRun - Preview migration without applying changes
 * @param options.rollback - Restore from most recent backup
 */
export async function migrateCommand(options: { dryRun?: boolean; rollback?: boolean }): Promise<void> {
  try {
    if (options.rollback) {
      await handleRollback();
    } else if (options.dryRun) {
      await handleDryRun();
    } else {
      await handleMigration();
    }
  } catch (error) {
    console.error('❌ Migration error:', (error as Error).message);
    process.exit(1);
  }
}

/**
 * Register migrate command with CLI framework
 * (Integration point for existing mcp-wizard CLI)
 */
export function registerMigrateCommand(program: any): void {
  program
    .command('migrate')
    .description('Migrate Claude Desktop config to mcp-wizard gateway')
    .option('--dry-run', 'Preview migration without applying changes')
    .option('--rollback', 'Restore from most recent backup')
    .action(migrateCommand);
}
