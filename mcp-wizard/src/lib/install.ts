import * as path from 'path';
import * as os from 'os';
import { promises as fs } from 'fs';
import { exec } from 'child_process';
import { promisify } from 'util';
import { pathExists } from './detect';
import { retryWithBackoff } from './errors';

const execAsync = promisify(exec);

export async function installMcpServers(): Promise<void> {
  const installPath = path.join(os.homedir(), 'mcp-servers/google-docs-mcp');

  // Check if already installed
  if (await pathExists(path.join(installPath, 'dist/server.js'))) {
    console.log('✓ Google Docs MCP already installed');
    return;
  }

  console.log('Installing Google Docs MCP...');

  // Create parent directory
  await fs.mkdir(path.dirname(installPath), { recursive: true });

  // Clone repository (with retry)
  await retryWithBackoff(
    () =>
      execAsync(`git clone https://github.com/a-bonus/google-docs-mcp.git ${installPath}`),
    1,
    5000
  );

  // Install dependencies (with retry)
  console.log('Running npm install... (this may take a minute)');
  await retryWithBackoff(
    () => execAsync('npm install', { cwd: installPath }),
    1,
    5000
  );

  // Build
  console.log('Building MCP server...');
  await execAsync('npm run build', { cwd: installPath });

  // Verify build succeeded
  if (!(await pathExists(path.join(installPath, 'dist/server.js')))) {
    throw new Error('MCP build failed (dist/server.js not found)');
  }

  console.log('✓ Google Docs MCP installed successfully');
}
