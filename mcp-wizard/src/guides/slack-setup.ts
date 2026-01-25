import * as path from 'path';
import * as os from 'os';
import * as fs from 'fs/promises';
import inquirer from 'inquirer';
import * as open from 'open';
import { SetupError } from '../errors/setup-error';

const SLACK_APP_URL = 'https://api.slack.com/apps';

export async function runSlackSetupGuide(): Promise<{ botToken: string; teamId: string }> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Slack MCP Token Setup                                        ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('The Slack MCP provides access to your Slack workspace.');
  console.log('You need to create a Slack app and get a bot token.\n');

  // Step 1: Guide user to create Slack app
  await createSlackAppStep();

  // Step 2: Guide user to add scopes
  await addSlackScopesStep();

  // Step 3: Install app to workspace
  await installSlackAppStep();

  // Step 4: Collect bot token
  const botToken = await collectBotTokenStep();

  // Step 5: Collect team ID
  const teamId = await collectTeamIdStep();

  // Step 6: Save tokens
  await saveSlackTokens(botToken, teamId);

  console.log('\n✓ Slack MCP configured!');
  return { botToken, teamId };
}

async function createSlackAppStep(): Promise<void> {
  console.log('─── Step 1: Create Slack App ───\n');

  const { openBrowser } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'openBrowser',
      message: 'Open Slack App Management page?',
      default: true,
    },
  ]);

  if (openBrowser) {
    await open.default(SLACK_APP_URL);
  } else {
    console.log(`\nManually navigate to:\n${SLACK_APP_URL}\n`);
  }

  console.log('\nInstructions:');
  console.log('  1. Click "Create New App"');
  console.log('  2. Select "From scratch"');
  console.log('  3. App Name: "Claude Code MCP"');
  console.log('  4. Select your workspace');
  console.log('  5. Click "Create App"\n');

  await waitForUserConfirmation('Press Enter when app is created...');
}

async function addSlackScopesStep(): Promise<void> {
  console.log('\n─── Step 2: Add Bot Token Scopes ───\n');

  console.log('In your Slack app settings:');
  console.log('  1. Go to "OAuth & Permissions" (left sidebar)');
  console.log('  2. Scroll to "Scopes" → "Bot Token Scopes"');
  console.log('  3. Click "Add an OAuth Scope"');
  console.log('  4. Add these scopes:');
  console.log('     • channels:history');
  console.log('     • channels:read');
  console.log('     • chat:write');
  console.log('     • groups:history');
  console.log('     • groups:read');
  console.log('     • im:history');
  console.log('     • im:read');
  console.log('     • mpim:history');
  console.log('     • mpim:read');
  console.log('     • search:read');
  console.log('     • users:read\n');

  await waitForUserConfirmation('Press Enter when scopes are added...');
}

async function installSlackAppStep(): Promise<void> {
  console.log('\n─── Step 3: Install App to Workspace ───\n');

  console.log('In your Slack app settings:');
  console.log('  1. Stay on "OAuth & Permissions" page');
  console.log('  2. Scroll to top');
  console.log('  3. Click "Install to Workspace" button');
  console.log('  4. Review permissions and click "Allow"\n');

  await waitForUserConfirmation('Press Enter when app is installed...');
}

async function collectBotTokenStep(): Promise<string> {
  console.log('\n─── Step 4: Copy Bot Token ───\n');

  console.log('In your Slack app settings:');
  console.log('  1. On "OAuth & Permissions" page');
  console.log('  2. Find "Bot User OAuth Token"');
  console.log('  3. It starts with "xoxb-"');
  console.log('  4. Click "Copy" button\n');

  const { botToken } = await inquirer.prompt([
    {
      type: 'password',
      name: 'botToken',
      message: 'Paste your Bot User OAuth Token (starts with xoxb-):',
      validate: (input: string) => {
        if (!input.startsWith('xoxb-')) {
          return 'Bot token must start with "xoxb-". Please check and try again.';
        }
        if (input.length < 20) {
          return 'Token seems too short. Please paste the full token.';
        }
        return true;
      },
    },
  ]);

  return botToken;
}

async function collectTeamIdStep(): Promise<string> {
  console.log('\n─── Step 5: Get Team ID ───\n');

  console.log('In your Slack app settings:');
  console.log('  1. Go to "Basic Information" (left sidebar)');
  console.log('  2. Scroll to "App Credentials"');
  console.log('  3. Find "Signing Secret" or "Team ID"');
  console.log('  4. Copy the Team ID (format: T0XXXXXXXXX)\n');

  const { teamId } = await inquirer.prompt([
    {
      type: 'input',
      name: 'teamId',
      message: 'Paste your Team ID (starts with T):',
      validate: (input: string) => {
        if (!input.startsWith('T')) {
          return 'Team ID must start with "T". Please check and try again.';
        }
        if (input.length < 8) {
          return 'Team ID seems too short. Please paste the full ID.';
        }
        return true;
      },
    },
  ]);

  return teamId;
}

async function saveSlackTokens(botToken: string, teamId: string): Promise<void> {
  const slackDir = path.join(os.homedir(), 'mcp-servers/slack-mcp');
  await fs.mkdir(slackDir, { recursive: true });

  const tokenPath = path.join(slackDir, '.slack-token');
  const teamIdPath = path.join(slackDir, '.slack-team-id');

  // Write with 600 permissions (owner-only)
  await fs.writeFile(tokenPath, botToken, { mode: 0o600 });
  await fs.writeFile(teamIdPath, teamId, { mode: 0o600 });

  console.log(`\n✓ Saved bot token to: ${tokenPath}`);
  console.log(`✓ Saved team ID to: ${teamIdPath}`);
}

async function waitForUserConfirmation(message: string): Promise<void> {
  await inquirer.prompt([
    {
      type: 'input',
      name: 'confirm',
      message,
      default: '',
    },
  ]);
}

export async function showSlackSetupSummary(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Slack Setup Complete                                         ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('What was set up:');
  console.log('  ✓ Slack app created');
  console.log('  ✓ Bot token scopes configured');
  console.log('  ✓ App installed to workspace');
  console.log('  ✓ Bot token saved securely');
  console.log('  ✓ Team ID saved\n');

  console.log('Next steps:');
  console.log('  1. Restart Claude Code (or reload window)');
  console.log('  2. Try: "Search Slack for messages about [topic]"');
  console.log('  3. Try: "Show recent messages in #channel-name"\n');
}
