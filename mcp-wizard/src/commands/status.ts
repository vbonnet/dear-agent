import * as path from 'path';
import * as os from 'os';
import { detectEnvironment, pathExists } from '../lib/detect';

export async function statusCommand(): Promise<void> {
  console.log('Checking MCP setup status...\n');

  // 1. Environment
  const env = await detectEnvironment();

  console.log('Environment:');
  console.log(`  ${env.isWorkMachine ? '✓' : '✗'} Work machine: ${env.hostname}`);
  console.log(`  ${env.nodeVersionValid ? '✓' : '✗'} Node.js: ${env.nodeVersion}`);

  if (env.chezmoiDetected) {
    console.log(`  ${env.chezmoiManagesConfig ? '✓' : 'ℹ'} Chezmoi: ${
      env.chezmoiManagesConfig ? 'managing config' : 'installed (not managing MCP config)'
    }`);
  }

  console.log('');

  // 2. MCP Servers
  console.log('MCP Servers:');

  // Google Docs MCP
  const googleDocsMcpPath = path.join(os.homedir(), 'mcp-servers/google-docs-mcp');
  const googleDocsInstalled = await pathExists(path.join(googleDocsMcpPath, 'dist/server.js'));
  const googleDocsCredentials = await pathExists(path.join(googleDocsMcpPath, 'credentials.json'));
  const googleDocsToken = await pathExists(path.join(googleDocsMcpPath, 'token.json'));

  console.log('  Google Docs MCP:');
  console.log(`    ${googleDocsInstalled ? '✓' : '✗'} Installed: ${googleDocsMcpPath}`);

  if (googleDocsInstalled) {
    console.log(`    ${googleDocsCredentials ? '✓' : '✗'} Credentials: ${googleDocsCredentials ? 'present' : 'missing'}`);
    console.log(`    ${googleDocsToken ? '✓' : '✗'} Authenticated: ${googleDocsToken ? 'yes' : 'no'}`);
  }

  // MCP Config
  const configPath = path.join(os.homedir(), '.config/claude-code/mcp.json');
  const configExists = await pathExists(configPath);

  console.log(`    ${configExists ? '✓' : '✗'} Configured: ${configExists ? configPath : 'not configured'}`);

  console.log('');
  console.log('  Atlassian MCP:');
  console.log(`    ℹ  Remote MCP (authenticate on first use)`);

  console.log('');

  // Overall status
  const allGood = env.isWorkMachine &&
                  env.nodeVersionValid &&
                  googleDocsInstalled &&
                  googleDocsCredentials &&
                  googleDocsToken &&
                  configExists;

  if (allGood) {
    console.log('✓ Overall: All systems operational');
  } else {
    console.log('⚠ Overall: Setup incomplete');
    console.log('');
    console.log('Run `mcp-wizard setup` to complete setup');
  }
}
