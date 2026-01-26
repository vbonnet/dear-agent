import * as path from 'path';
import * as os from 'os';
import * as fs from 'fs/promises';
import inquirer from 'inquirer';
import { SetupError } from '../errors/setup-error';

const GITHUB_TOKEN_URL = 'https://github.com/settings/tokens/new';
const REQUIRED_SCOPES = ['repo', 'read:org'];
const OPTIONAL_SCOPES = ['read:packages', 'workflow'];

export async function runGitHubSetupGuide(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   GitHub MCP Setup                                             ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('The GitHub MCP provides access to repositories, issues, pull requests,');
  console.log('GitHub Actions, and code security features.\n');

  // Step 1: Authentication setup
  await setupGitHubAuth();

  // Step 2: GitHub.com vs Enterprise
  const { isEnterprise } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'isEnterprise',
      message: 'Are you using GitHub Enterprise Server (not GitHub.com)?',
      default: false,
    },
  ]);

  if (isEnterprise) {
    await setupEnterpriseUrl();
  }

  // Step 3: Feature selection
  await selectToolsets();

  console.log('\n✓ GitHub MCP setup complete!');
  console.log('Restart Claude Code (or reload window) to use GitHub MCP.\n');
}

async function setupGitHubAuth(): Promise<void> {
  console.log('─── Step 1: GitHub Authentication ───\n');

  // Check if VS Code for OAuth support (future enhancement)
  const vscodeVersion = process.env.VSCODE_VERSION || '';
  const supportsOAuth = checkOAuthSupport(vscodeVersion);

  if (supportsOAuth) {
    console.log('✓ VS Code 1.101+ detected - OAuth available\n');

    const { authMethod } = await inquirer.prompt([
      {
        type: 'list',
        name: 'authMethod',
        message: 'Choose authentication method:',
        choices: [
          { name: 'Personal Access Token (PAT) - Recommended', value: 'pat' },
          { name: 'OAuth via VS Code (Experimental)', value: 'oauth' },
        ],
        default: 'pat',
      },
    ]);

    if (authMethod === 'oauth') {
      console.log('\n⚠ OAuth flow not yet implemented - falling back to PAT\n');
    }
  }

  // PAT flow (primary method)
  await setupPAT();
}

async function setupPAT(): Promise<void> {
  console.log('GitHub Personal Access Token (PAT) is required.\n');

  const { hasPAT } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'hasPAT',
      message: 'Do you have a GitHub Personal Access Token?',
      default: false,
    },
  ]);

  if (!hasPAT) {
    console.log('\nTo create a Personal Access Token:');
    console.log(`1. Visit: ${GITHUB_TOKEN_URL}`);
    console.log('2. Click "Generate new token (classic)"');
    console.log('3. Select scopes:');
    console.log(`   • ${REQUIRED_SCOPES.join(', ')} (required)`);
    console.log(`   • ${OPTIONAL_SCOPES.join(', ')} (optional)`);
    console.log('4. Click "Generate token"');
    console.log('5. Copy the token (you won\'t see it again!)\n');

    const { readyToProceed } = await inquirer.prompt([
      {
        type: 'confirm',
        name: 'readyToProceed',
        message: 'Press Enter when you have your token...',
        default: true,
      },
    ]);

    if (!readyToProceed) {
      throw new SetupError(
        'GitHub PAT required',
        'Generate a token and re-run setup',
        GITHUB_TOKEN_URL
      );
    }
  }

  // Prompt for token
  const { token } = await inquirer.prompt([
    {
      type: 'password',
      name: 'token',
      message: 'Enter your GitHub Personal Access Token:',
      mask: '*',
      validate: (input: string) => {
        if (!input || input.length < 20) {
          return 'Token appears invalid (too short)';
        }
        if (!input.startsWith('ghp_') && !input.startsWith('github_pat_')) {
          return 'Token should start with "ghp_" or "github_pat_"';
        }
        return true;
      },
    },
  ]);

  // Store token
  console.log('\nStoring token...');
  const tokenDir = path.join(os.homedir(), 'mcp-servers/github-mcp');
  const tokenPath = path.join(tokenDir, '.github-token');

  await fs.mkdir(tokenDir, { recursive: true });
  await fs.writeFile(tokenPath, token, { mode: 0o600 }); // Owner read/write only

  console.log(`✓ Token stored securely in ${tokenPath}`);
  console.log('⚠  Keep this token safe - it grants access to your GitHub repositories\n');
}

async function setupEnterpriseUrl(): Promise<void> {
  console.log('\n─── Step 2: GitHub Enterprise Server ───\n');

  const { enterpriseUrl } = await inquirer.prompt([
    {
      type: 'input',
      name: 'enterpriseUrl',
      message: 'Enter your GitHub Enterprise Server URL:',
      default: 'https://github.company.com',
      validate: (input: string) => {
        if (!input.startsWith('https://')) {
          return 'Enterprise URL must start with https://';
        }

        try {
          new URL(input);
          return true;
        } catch {
          return 'Invalid URL format';
        }
      },
    },
  ]);

  // Store enterprise URL for config generation
  const configDir = path.join(os.homedir(), 'mcp-servers/github-mcp');
  const urlPath = path.join(configDir, '.github-enterprise-url');

  await fs.mkdir(configDir, { recursive: true });
  await fs.writeFile(urlPath, enterpriseUrl.replace(/\/+$/, ''));

  console.log(`✓ Enterprise URL configured: ${enterpriseUrl}\n`);
}

async function selectToolsets(): Promise<void> {
  console.log('─── Step 3: Feature Selection ───\n');

  const { toolsets } = await inquirer.prompt([
    {
      type: 'checkbox',
      name: 'toolsets',
      message: 'Select GitHub features to enable:',
      choices: [
        { name: 'Repositories (file search, navigation)', value: 'repos', checked: true },
        { name: 'Issues (search, create, comment)', value: 'issues', checked: true },
        { name: 'Pull Requests (review, status, comment)', value: 'pull_requests', checked: false },
        { name: 'GitHub Actions (workflow monitoring)', value: 'actions', checked: false },
        { name: 'Code Security (vulnerability scanning)', value: 'code_security', checked: false },
      ],
      validate: (answer: string[]) => {
        if (answer.length === 0) {
          return 'Please select at least one feature';
        }
        return true;
      },
    },
  ]);

  // Store toolsets for config generation
  const configDir = path.join(os.homedir(), 'mcp-servers/github-mcp');
  const toolsetsPath = path.join(configDir, '.github-toolsets');

  await fs.mkdir(configDir, { recursive: true });
  await fs.writeFile(toolsetsPath, toolsets.join(','));

  console.log(`\n✓ Features configured: ${toolsets.join(', ')}\n`);
}

function checkOAuthSupport(vscodeVersion: string): boolean {
  if (!vscodeVersion) return false;

  const [major, minor] = vscodeVersion.split('.').map(Number);
  return major > 1 || (major === 1 && minor >= 101);
}

export async function showGitHubSetupSummary(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   GitHub MCP Setup Complete                                    ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('What was set up:');
  console.log('  ✓ GitHub Personal Access Token configured');
  console.log('  ✓ MCP server configured');
  console.log('  ✓ Features selected\n');

  console.log('Next steps:');
  console.log('  1. Restart Claude Code (or reload window)');
  console.log('  2. Try: "Search GitHub for issues mentioning bug"');
  console.log('  3. Try: "Show me recent pull requests in [repo]"\n');
}
