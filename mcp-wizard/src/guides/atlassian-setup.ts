import * as path from 'path';
import * as os from 'os';
import * as fs from 'fs/promises';
import inquirer from 'inquirer';
import * as open from 'open';
import { SetupError } from '../errors/setup-error';

const ATLASSIAN_MCP_URL = 'https://mcp.atlassian.com/v1/sse';
const ATLASSIAN_AUTH_URL = 'https://mcp.atlassian.com/v1/authorize';

export async function runAtlassianSetupGuide(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Atlassian MCP OAuth Setup                                    ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('The Atlassian MCP provides access to Jira and Confluence.');
  console.log('Authentication requires OAuth through your browser.\n');

  const { proceed } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'proceed',
      message: 'Ready to authenticate with Atlassian?',
      default: true,
    },
  ]);

  if (!proceed) {
    throw new SetupError(
      'Atlassian OAuth setup cancelled',
      'Re-run setup and select "y" when prompted',
      'https://github.com/your-org/mcp-wizard/issues'
    );
  }

  console.log('\nAuthenticating with Atlassian...');
  console.log('This will open a browser window for you to sign in.\n');

  // Open browser for OAuth
  const { openBrowser } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'openBrowser',
      message: 'Open browser for Atlassian authentication?',
      default: true,
    },
  ]);

  if (openBrowser) {
    console.log('\nOpening browser...');
    // The mcp-remote package handles OAuth internally
    // We just need to guide the user through the process
    await open.default(ATLASSIAN_AUTH_URL);
  } else {
    console.log(`\nManually navigate to:\n${ATLASSIAN_AUTH_URL}\n`);
  }

  console.log('\nInstructions:');
  console.log('  1. Sign in with your Atlassian account');
  console.log('  2. Select the Atlassian site (Jira/Confluence instance)');
  console.log('  3. Review and grant the requested permissions');
  console.log('  4. Wait for the "Authentication successful" message\n');

  console.log('Note: The mcp-remote package will automatically handle the OAuth flow.');
  console.log('When you use the Atlassian MCP for the first time, it will complete authentication.\n');

  const { understood } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'understood',
      message: 'Press Enter to continue...',
      default: true,
    },
  ]);

  console.log('\n✓ Atlassian MCP configured');
  console.log('Authentication will complete on first use.\n');
}

export async function showAtlassianSetupSummary(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Atlassian Setup Complete                                     ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('What was set up:');
  console.log('  ✓ Atlassian MCP configured');
  console.log('  ✓ OAuth authentication prepared');
  console.log('  ℹ  Authentication will complete on first MCP use\n');

  console.log('Next steps:');
  console.log('  1. Restart Claude Code (or reload window)');
  console.log('  2. Try: "Search Jira for recent issues"');
  console.log('  3. Try: "Find Confluence pages about [topic]"\n');
}
