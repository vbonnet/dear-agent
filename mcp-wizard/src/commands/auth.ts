import * as path from 'path';
import * as os from 'os';
import { oauthFlow } from '../lib/oauth';
import { sanitizeError } from '../lib/errors';

interface AuthOptions {
  verbose?: boolean;
}

export async function authCommand(mcp: string, options: AuthOptions = {}): Promise<void> {
  const homedir = os.homedir();

  try {
    // Validate MCP parameter
    if (mcp !== 'google-docs') {
      console.error(`✗ Error: Unsupported MCP "${mcp}"`);
      console.error('  Supported MCPs: google-docs');
      process.exit(1);
    }

    console.log(`\n╔════════════════════════════════════════════════════════════════╗`);
    console.log(`║   Re-authenticate: ${mcp.padEnd(43)} ║`);
    console.log(`╚════════════════════════════════════════════════════════════════╝\n`);

    // Determine paths
    const credentialsPath = path.join(homedir, 'mcp-servers/google-docs-mcp/credentials.json');
    const tokenPath = path.join(homedir, 'mcp-servers/google-docs-mcp/token.json');

    if (options.verbose) {
      console.log(`Credentials: ${credentialsPath}`);
      console.log(`Token: ${tokenPath}`);
    }

    // Check if credentials exist
    const fs = await import('fs/promises');
    try {
      await fs.access(credentialsPath);
    } catch {
      console.error(`✗ Error: credentials.json not found`);
      console.error(`  Expected: ${credentialsPath}`);
      console.error('\n  Run "mcp-wizard setup" first to configure OAuth credentials.');
      process.exit(1);
    }

    // Run OAuth flow (will overwrite existing token.json)
    console.log('Starting OAuth flow...\n');
    await oauthFlow(credentialsPath, tokenPath);

    console.log('\n✓ Re-authentication complete!');
    console.log(`  Token saved: ${tokenPath}`);
    console.log('\n  Your MCP server will use the new credentials on next restart.');

  } catch (error) {
    const sanitized = sanitizeError(error as Error);
    console.error(`\n✗ Re-authentication failed: ${sanitized.message}`);

    if (options.verbose && sanitized.stack) {
      console.error('\nStack trace:');
      console.error(sanitized.stack);
    }

    console.error('\n  Troubleshooting:');
    console.error('  - Verify credentials.json is valid');
    console.error('  - Check internet connectivity');
    console.error('  - Ensure browser can access accounts.google.com');
    console.error('  - Run with --verbose for more details');

    process.exit(1);
  }
}
