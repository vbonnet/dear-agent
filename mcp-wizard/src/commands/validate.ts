import * as path from 'path';
import * as os from 'os';
import * as fs from 'fs/promises';
import { google } from 'googleapis';
import { sanitizeError } from '../lib/errors';

interface ValidationResult {
  name: string;
  status: 'pass' | 'fail' | 'warn';
  message: string;
  details?: string;
}

interface ValidateOptions {
  verbose?: boolean;
  fix?: boolean;
}

export async function validateCommand(options: ValidateOptions = {}): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   MCP Setup Validation                                         ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  const results: ValidationResult[] = [];
  const homedir = os.homedir();

  // Validation 1: Node.js version
  results.push(await validateNodeVersion());

  // Validation 2: MCP directory structure
  results.push(await validateMcpDirectory(homedir));

  // Validation 3: credentials.json
  results.push(await validateCredentials(homedir));

  // Validation 4: token.json
  results.push(await validateToken(homedir));

  // Validation 5: MCP config file
  results.push(await validateMcpConfig(homedir));

  // Validation 6: Google Docs MCP package
  results.push(await validateGoogleDocsMcp(homedir));

  // Validation 7: Token validity (API test)
  if (results.find(r => r.name === 'token.json')?.status === 'pass') {
    results.push(await validateTokenWorks(homedir, options.verbose));
  }

  // Print results
  console.log('Validation Results:\n');

  let passCount = 0;
  let failCount = 0;
  let warnCount = 0;

  for (const result of results) {
    const icon = result.status === 'pass' ? '✓' : result.status === 'fail' ? '✗' : '⚠';
    const color = result.status === 'pass' ? '' : result.status === 'fail' ? '' : '';

    console.log(`${icon} ${result.name}: ${result.message}`);

    if (result.details && options.verbose) {
      console.log(`  ${result.details}`);
    }

    if (result.status === 'pass') passCount++;
    else if (result.status === 'fail') failCount++;
    else warnCount++;
  }

  console.log('\n─────────────────────────────────────────────────────────────────');
  console.log(`Summary: ${passCount} passed, ${failCount} failed, ${warnCount} warnings\n`);

  if (failCount > 0) {
    console.log('Failures detected. Fix suggestions:\n');

    const failedChecks = results.filter(r => r.status === 'fail');
    for (const check of failedChecks) {
      console.log(`  • ${check.name}: ${check.message}`);
      if (check.details) {
        console.log(`    ${check.details}`);
      }
    }

    console.log('\n  Run "mcp-wizard setup --resume" to fix issues.');
    console.log('  See TROUBLESHOOTING.md for manual fixes.\n');

    process.exit(1);
  } else if (warnCount > 0) {
    console.log('✓ Setup is functional but has warnings.');
    console.log('  Review warnings above and address if needed.\n');
    process.exit(0);
  } else {
    console.log('✓ All validations passed! MCP setup is healthy.\n');
    process.exit(0);
  }
}

async function validateNodeVersion(): Promise<ValidationResult> {
  const version = process.version;
  const major = parseInt(version.slice(1).split('.')[0]);

  if (major >= 18) {
    return {
      name: 'Node.js version',
      status: 'pass',
      message: `${version} (>= 18.0.0)`,
    };
  } else {
    return {
      name: 'Node.js version',
      status: 'fail',
      message: `${version} (requires >= 18.0.0)`,
      details: 'Upgrade Node.js to v18.0.0 or higher',
    };
  }
}

async function validateMcpDirectory(homedir: string): Promise<ValidationResult> {
  const mcpDir = path.join(homedir, 'mcp-servers');

  try {
    const stat = await fs.stat(mcpDir);
    if (stat.isDirectory()) {
      return {
        name: 'MCP directory',
        status: 'pass',
        message: `${mcpDir} exists`,
      };
    } else {
      return {
        name: 'MCP directory',
        status: 'fail',
        message: `${mcpDir} is not a directory`,
        details: 'Delete the file and re-run setup',
      };
    }
  } catch {
    return {
      name: 'MCP directory',
      status: 'fail',
      message: `${mcpDir} not found`,
      details: 'Run "mcp-wizard setup" to create',
    };
  }
}

async function validateCredentials(homedir: string): Promise<ValidationResult> {
  const credentialsPath = path.join(homedir, 'mcp-servers/google-docs-mcp/credentials.json');

  try {
    const content = await fs.readFile(credentialsPath, 'utf8');
    const credentials = JSON.parse(content);

    if (!credentials.installed) {
      return {
        name: 'credentials.json',
        status: 'fail',
        message: 'Invalid format (missing "installed" section)',
        details: 'Re-download OAuth credentials from GCP Console',
      };
    }

    const { client_id, client_secret, redirect_uris } = credentials.installed;

    if (!client_id || !client_secret || !redirect_uris) {
      return {
        name: 'credentials.json',
        status: 'fail',
        message: 'Missing required fields',
        details: 'Re-download OAuth credentials from GCP Console',
      };
    }

    return {
      name: 'credentials.json',
      status: 'pass',
      message: 'Valid OAuth credentials',
      details: `client_id: ${client_id.slice(0, 20)}...`,
    };
  } catch (error) {
    const err = error as Error;
    if (error && typeof error === 'object' && 'code' in error && (error as NodeJS.ErrnoException).code === 'ENOENT') {
      return {
        name: 'credentials.json',
        status: 'fail',
        message: 'File not found',
        details: `Expected: ${credentialsPath}`,
      };
    } else {
      return {
        name: 'credentials.json',
        status: 'fail',
        message: `Invalid JSON: ${err.message}`,
        details: 'Re-download OAuth credentials from GCP Console',
      };
    }
  }
}

async function validateToken(homedir: string): Promise<ValidationResult> {
  const tokenPath = path.join(homedir, 'mcp-servers/google-docs-mcp/token.json');

  try {
    const content = await fs.readFile(tokenPath, 'utf8');
    const token = JSON.parse(content);

    if (!token.refresh_token) {
      return {
        name: 'token.json',
        status: 'fail',
        message: 'Missing refresh_token',
        details: 'Run "mcp-wizard setup --resume" to re-authenticate',
      };
    }

    // Check file permissions (should be 600)
    const stat = await fs.stat(tokenPath);
    const mode = stat.mode & 0o777;

    if (mode !== 0o600) {
      return {
        name: 'token.json',
        status: 'warn',
        message: `Insecure permissions (${mode.toString(8)}, should be 600)`,
        details: `Run: chmod 600 ${tokenPath}`,
      };
    }

    return {
      name: 'token.json',
      status: 'pass',
      message: 'Valid token with secure permissions',
    };
  } catch (error) {
    const err = error as Error;
    if (error && typeof error === 'object' && 'code' in error && (error as NodeJS.ErrnoException).code === 'ENOENT') {
      return {
        name: 'token.json',
        status: 'fail',
        message: 'File not found',
        details: 'Run "mcp-wizard setup --resume" to authenticate',
      };
    } else {
      return {
        name: 'token.json',
        status: 'fail',
        message: `Invalid JSON: ${err.message}`,
        details: 'Run "mcp-wizard setup --resume" to re-authenticate',
      };
    }
  }
}

async function validateMcpConfig(homedir: string): Promise<ValidationResult> {
  const configPath = path.join(homedir, '.config/claude-code/mcp.json');

  try {
    const content = await fs.readFile(configPath, 'utf8');
    const config = JSON.parse(content);

    if (!config.mcpServers || !config.mcpServers.GoogleDocs) {
      return {
        name: 'mcp.json',
        status: 'fail',
        message: 'Missing GoogleDocs server configuration',
        details: 'Run "mcp-wizard setup" to regenerate config',
      };
    }

    const googleDocs = config.mcpServers.GoogleDocs;

    if (!googleDocs.command || !googleDocs.args || !googleDocs.env) {
      return {
        name: 'mcp.json',
        status: 'fail',
        message: 'Incomplete GoogleDocs configuration',
        details: 'Run "mcp-wizard setup" to regenerate config',
      };
    }

    return {
      name: 'mcp.json',
      status: 'pass',
      message: 'Valid MCP configuration',
    };
  } catch (error) {
    const err = error as Error;
    if (error && typeof error === 'object' && 'code' in error && (error as NodeJS.ErrnoException).code === 'ENOENT') {
      return {
        name: 'mcp.json',
        status: 'fail',
        message: 'File not found',
        details: `Expected: ${configPath}`,
      };
    } else {
      return {
        name: 'mcp.json',
        status: 'fail',
        message: `Invalid JSON: ${err.message}`,
        details: 'Run "mcp-wizard setup" to regenerate config',
      };
    }
  }
}

async function validateGoogleDocsMcp(homedir: string): Promise<ValidationResult> {
  const mcpPath = path.join(homedir, 'mcp-servers/google-docs-mcp');
  const serverPath = path.join(mcpPath, 'dist/server.js');
  const packagePath = path.join(mcpPath, 'package.json');

  try {
    // Check dist/server.js
    await fs.access(serverPath);

    // Check package.json
    const packageContent = await fs.readFile(packagePath, 'utf8');
    const packageJson = JSON.parse(packageContent);

    return {
      name: 'Google Docs MCP',
      status: 'pass',
      message: `Installed (${packageJson.name}@${packageJson.version})`,
      details: serverPath,
    };
  } catch (error) {
    const err = error as Error;
    if (error && typeof error === 'object' && 'code' in error && (error as NodeJS.ErrnoException).code === 'ENOENT') {
      return {
        name: 'Google Docs MCP',
        status: 'fail',
        message: 'Package not built or missing',
        details: `Run: cd ${mcpPath} && npm install && npm run build`,
      };
    } else {
      return {
        name: 'Google Docs MCP',
        status: 'fail',
        message: `Error: ${err.message}`,
        details: 'Run "mcp-wizard setup" to reinstall',
      };
    }
  }
}

async function validateTokenWorks(homedir: string, verbose?: boolean): Promise<ValidationResult> {
  const tokenPath = path.join(homedir, 'mcp-servers/google-docs-mcp/token.json');
  const credentialsPath = path.join(homedir, 'mcp-servers/google-docs-mcp/credentials.json');

  try {
    if (verbose) {
      console.log('  Testing token with Google API...');
    }

    // Load credentials and token
    const credContent = await fs.readFile(credentialsPath, 'utf8');
    const credentials = JSON.parse(credContent);

    const tokenContent = await fs.readFile(tokenPath, 'utf8');
    const token = JSON.parse(tokenContent);

    // Create OAuth2 client
    const oauth2Client = new google.auth.OAuth2(
      credentials.installed.client_id,
      credentials.installed.client_secret,
      credentials.installed.redirect_uris[0]
    );

    oauth2Client.setCredentials({
      refresh_token: token.refresh_token,
    });

    // Try to refresh token (this validates it works)
    const { credentials: refreshed } = await oauth2Client.refreshAccessToken();

    if (refreshed.access_token) {
      return {
        name: 'Token API test',
        status: 'pass',
        message: 'Successfully refreshed access token',
        details: verbose ? 'Token is valid and can access Google APIs' : undefined,
      };
    } else {
      return {
        name: 'Token API test',
        status: 'fail',
        message: 'Failed to obtain access token',
        details: 'Run "mcp-wizard setup --resume" to re-authenticate',
      };
    }
  } catch (error) {
    const sanitized = sanitizeError(error as Error);
    return {
      name: 'Token API test',
      status: 'fail',
      message: `API error: ${sanitized.message}`,
      details: 'Run "mcp-wizard setup --resume" to re-authenticate',
    };
  }
}
