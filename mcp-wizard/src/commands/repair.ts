import * as path from 'path';
import * as os from 'os';
import * as fs from 'fs/promises';
import { installMcpServers } from '../lib/install';
import { generateMcpConfig, writeMcpConfig } from '../lib/config';
import { sanitizeError } from '../lib/errors';

interface RepairOptions {
  verbose?: boolean;
  dryRun?: boolean;
  force?: boolean;
}

export async function repairCommand(mcp: string | undefined, options: RepairOptions = {}): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   MCP Setup Repair                                             ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  const homedir = os.homedir();

  try {
    // Determine repair scope
    if (mcp && mcp !== 'google-docs' && mcp !== 'all') {
      console.error(`✗ Error: Unsupported MCP "${mcp}"`);
      console.error('  Supported values: google-docs, all');
      process.exit(1);
    }

    const repairAll = !mcp || mcp === 'all';

    if (options.dryRun) {
      console.log('[DRY RUN MODE] - No changes will be made\n');
    }

    // Repair 1: MCP directory structure
    if (repairAll) {
      await repairMcpDirectory(homedir, options);
    }

    // Repair 2: Google Docs MCP installation
    if (repairAll || mcp === 'google-docs') {
      await repairGoogleDocsMcp(homedir, options);
    }

    // Repair 3: MCP configuration
    if (repairAll) {
      await repairMcpConfig(homedir, options);
    }

    // Repair 4: File permissions (token.json)
    if (repairAll || mcp === 'google-docs') {
      await repairTokenPermissions(homedir, options);
    }

    console.log('\n✓ Repair complete!');
    console.log('\n  Run "mcp-wizard validate" to verify setup health.');

  } catch (error) {
    const sanitized = sanitizeError(error as Error);
    console.error(`\n✗ Repair failed: ${sanitized.message}`);

    if (options.verbose && sanitized.stack) {
      console.error('\nStack trace:');
      console.error(sanitized.stack);
    }

    console.error('\n  If repair fails repeatedly, try:');
    console.error('  - Run "mcp-wizard setup" to start fresh');
    console.error('  - Check file permissions in ~/mcp-servers/');
    console.error('  - Run with --verbose for more details');

    process.exit(1);
  }
}

async function repairMcpDirectory(homedir: string, options: RepairOptions): Promise<void> {
  const mcpDir = path.join(homedir, 'mcp-servers');

  try {
    const stat = await fs.stat(mcpDir);

    if (!stat.isDirectory()) {
      console.log('⚠ Warning: ~/mcp-servers exists but is not a directory');

      if (options.force) {
        if (!options.dryRun) {
          console.log('  Removing file and creating directory...');
          await fs.unlink(mcpDir);
          await fs.mkdir(mcpDir, { recursive: true });
          console.log('✓ Created ~/mcp-servers directory');
        } else {
          console.log('  [DRY RUN] Would remove file and create directory');
        }
      } else {
        console.log('  Run with --force to remove file and create directory');
      }
    } else {
      if (options.verbose) {
        console.log('✓ MCP directory exists and is valid');
      }
    }
  } catch (error) {
    if (error && typeof error === 'object' && 'code' in error && (error as NodeJS.ErrnoException).code === 'ENOENT') {
      console.log('⚠ ~/mcp-servers not found');

      if (!options.dryRun) {
        console.log('  Creating directory...');
        await fs.mkdir(mcpDir, { recursive: true });
        console.log('✓ Created ~/mcp-servers directory');
      } else {
        console.log('  [DRY RUN] Would create directory');
      }
    } else {
      throw error;
    }
  }
}

async function repairGoogleDocsMcp(homedir: string, options: RepairOptions): Promise<void> {
  const mcpPath = path.join(homedir, 'mcp-servers/google-docs-mcp');
  const serverPath = path.join(mcpPath, 'dist/server.js');

  let needsRepair = false;

  try {
    // Check if dist/server.js exists
    await fs.access(serverPath);

    // Check if package.json exists
    const packagePath = path.join(mcpPath, 'package.json');
    await fs.access(packagePath);

    if (options.verbose) {
      console.log('✓ Google Docs MCP appears installed');
    }

    // If --force, rebuild anyway
    if (options.force) {
      console.log('⚠ --force specified, rebuilding Google Docs MCP...');
      needsRepair = true;
    }
  } catch {
    console.log('⚠ Google Docs MCP not found or incomplete');
    needsRepair = true;
  }

  if (needsRepair) {
    if (!options.dryRun) {
      console.log('  Installing Google Docs MCP...');
      await installMcpServers();
      console.log('✓ Google Docs MCP installed and built');
    } else {
      console.log('  [DRY RUN] Would install and build Google Docs MCP');
    }
  }
}

async function repairMcpConfig(homedir: string, options: RepairOptions): Promise<void> {
  const configPath = path.join(homedir, '.config/claude-code/mcp.json');

  let needsRepair = false;

  try {
    const content = await fs.readFile(configPath, 'utf8');
    const config = JSON.parse(content);

    // Check if GoogleDocs server is configured
    if (!config.mcpServers || !config.mcpServers.GoogleDocs) {
      console.log('⚠ MCP config missing GoogleDocs server');
      needsRepair = true;
    } else {
      const googleDocs = config.mcpServers.GoogleDocs;

      if (!googleDocs.command || !googleDocs.args || !googleDocs.env) {
        console.log('⚠ MCP config incomplete for GoogleDocs');
        needsRepair = true;
      } else if (options.verbose) {
        console.log('✓ MCP config appears valid');
      }
    }

    // If --force, regenerate anyway
    if (options.force && !needsRepair) {
      console.log('⚠ --force specified, regenerating MCP config...');
      needsRepair = true;
    }
  } catch (error) {
    if (error && typeof error === 'object' && 'code' in error && (error as NodeJS.ErrnoException).code === 'ENOENT') {
      console.log('⚠ MCP config file not found');
      needsRepair = true;
    } else {
      console.log('⚠ MCP config invalid JSON');
      needsRepair = true;
    }
  }

  if (needsRepair) {
    if (!options.dryRun) {
      console.log('  Regenerating MCP config...');
      const config = await generateMcpConfig();
      await writeMcpConfig(config);
      console.log('✓ MCP config regenerated');
    } else {
      console.log('  [DRY RUN] Would regenerate MCP config');
    }
  }
}

async function repairTokenPermissions(homedir: string, options: RepairOptions): Promise<void> {
  const tokenPath = path.join(homedir, 'mcp-servers/google-docs-mcp/token.json');

  try {
    const stat = await fs.stat(tokenPath);
    const mode = stat.mode & 0o777;

    if (mode !== 0o600) {
      console.log(`⚠ token.json has insecure permissions (${mode.toString(8)}, should be 600)`);

      if (!options.dryRun) {
        console.log('  Fixing permissions...');
        await fs.chmod(tokenPath, 0o600);
        console.log('✓ Set token.json permissions to 600');
      } else {
        console.log('  [DRY RUN] Would fix permissions to 600');
      }
    } else if (options.verbose) {
      console.log('✓ token.json has correct permissions (600)');
    }
  } catch (error) {
    if (error && typeof error === 'object' && 'code' in error && (error as NodeJS.ErrnoException).code === 'ENOENT') {
      // token.json doesn't exist - not an error for repair (might not be authenticated yet)
      if (options.verbose) {
        console.log('ℹ token.json not found (run "mcp-wizard auth google-docs" to authenticate)');
      }
    } else {
      throw error;
    }
  }
}
