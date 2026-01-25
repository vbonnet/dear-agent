import * as path from 'path';
import * as os from 'os';
import * as fs from 'fs/promises';
import inquirer from 'inquirer';
import * as open from 'open';
import { validateCredentials } from '../lib/oauth';
import { SetupError } from '../errors/setup-error';

const GCP_PROJECT = 'shared-dev-ai-pct45x';
const GCP_CONSOLE_BASE = 'https://console.cloud.google.com';

export async function runGcpSetupGuide(): Promise<string> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Google Cloud OAuth Setup                                     ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('We need to create OAuth credentials in Google Cloud Console.');
  console.log(`Project: ${GCP_PROJECT}\n`);

  // Step 1: Enable APIs
  await enableApisStep();

  // Step 2: Configure OAuth Consent Screen
  await oauthConsentStep();

  // Step 3: Create OAuth Credentials
  await createCredentialsStep();

  // Step 4: Download and validate credentials.json
  const credentialsPath = await downloadCredentialsStep();

  console.log('\n✓ GCP OAuth setup complete!');
  return credentialsPath;
}

async function enableApisStep(): Promise<void> {
  console.log('─── Step 1: Enable Required APIs ───\n');

  const apiLibraryUrl = `${GCP_CONSOLE_BASE}/apis/library?project=${GCP_PROJECT}`;

  console.log('Required APIs:');
  console.log('  • Google Docs API');
  console.log('  • Google Drive API\n');

  const { openBrowser } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'openBrowser',
      message: 'Open Google Cloud API Library?',
      default: true,
    },
  ]);

  if (openBrowser) {
    await open.default(apiLibraryUrl);
  } else {
    console.log(`\nManually navigate to:\n${apiLibraryUrl}\n`);
  }

  console.log('\nInstructions:');
  console.log('  1. Search for "Google Docs API" → Click ENABLE');
  console.log('  2. Search for "Google Drive API" → Click ENABLE');
  console.log('  3. Wait for both APIs to be enabled\n');

  await waitForUserConfirmation('Press Enter when APIs are enabled...');
}

async function oauthConsentStep(): Promise<void> {
  console.log('\n─── Step 2: Configure OAuth Consent Screen ───\n');

  const consentUrl = `${GCP_CONSOLE_BASE}/apis/credentials/consent?project=${GCP_PROJECT}`;

  const { openBrowser } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'openBrowser',
      message: 'Open OAuth Consent Screen configuration?',
      default: true,
    },
  ]);

  if (openBrowser) {
    await open.default(consentUrl);
  } else {
    console.log(`\nManually navigate to:\n${consentUrl}\n`);
  }

  console.log('\nInstructions:');
  console.log('  1. User Type: Select "Internal"');
  console.log('  2. App Information:');
  console.log('     • App name: "Claude Code MCP - Google Docs"');
  console.log('     • User support email: (your email)');
  console.log('     • Developer contact: (your email)');
  console.log('  3. Scopes: Click "ADD OR REMOVE SCOPES"');
  console.log('     • Filter: "Google Docs API"');
  console.log('       → Select: ".../auth/documents.readonly"');
  console.log('     • Filter: "Google Drive API"');
  console.log('       → Select: ".../auth/drive.readonly"');
  console.log('  4. Test Users: Add yourself');
  console.log('  5. Click "SAVE AND CONTINUE" through all steps\n');

  await waitForUserConfirmation('Press Enter when OAuth Consent Screen is configured...');
}

async function createCredentialsStep(): Promise<void> {
  console.log('\n─── Step 3: Create OAuth Credentials ───\n');

  const credentialsUrl = `${GCP_CONSOLE_BASE}/apis/credentials?project=${GCP_PROJECT}`;

  const { openBrowser } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'openBrowser',
      message: 'Open Credentials page?',
      default: true,
    },
  ]);

  if (openBrowser) {
    await open.default(credentialsUrl);
  } else {
    console.log(`\nManually navigate to:\n${credentialsUrl}\n`);
  }

  console.log('\nInstructions:');
  console.log('  1. Click "+ CREATE CREDENTIALS" (top of page)');
  console.log('  2. Select "OAuth client ID"');
  console.log('  3. Application type: "Desktop app"');
  console.log('  4. Name: "Claude Code MCP Client"');
  console.log('  5. Click "CREATE"');
  console.log('  6. In the popup, click "DOWNLOAD JSON"');
  console.log('     (Save to Downloads folder)\n');

  await waitForUserConfirmation('Press Enter after downloading credentials JSON...');
}

async function downloadCredentialsStep(): Promise<string> {
  console.log('\n─── Step 4: Install Credentials File ───\n');

  const { credentialsPath } = await inquirer.prompt([
    {
      type: 'input',
      name: 'credentialsPath',
      message: 'Enter path to downloaded credentials.json:',
      default: path.join(os.homedir(), 'Downloads/client_secret_*.json'),
      validate: async (input: string) => {
        // Expand ~ and wildcards
        const expandedPath = input.replace(/^~/, os.homedir());

        // Handle wildcards (simple glob)
        if (expandedPath.includes('*')) {
          const { glob } = await import('glob');
          const matches = glob.sync(expandedPath);
          if (matches.length === 0) {
            return 'No files match this pattern. Please check the path.';
          }
          if (matches.length > 1) {
            return `Multiple files match (${matches.length}). Please specify exact path.`;
          }
          // Use the matched file
          return true;
        }

        try {
          await fs.access(expandedPath);
          return true;
        } catch {
          return 'File not found. Please check the path.';
        }
      },
    },
  ]);

  // Expand path and resolve wildcards
  let resolvedPath = credentialsPath.replace(/^~/, os.homedir());
  if (resolvedPath.includes('*')) {
    const { glob } = await import('glob');
    const matches = glob.sync(resolvedPath);
    resolvedPath = matches[0];
  }

  // Read and validate credentials
  console.log('\nValidating credentials...');
  const content = await fs.readFile(resolvedPath, 'utf-8');
  const credentials = JSON.parse(content);

  try {
    await validateCredentials(credentials);
    console.log('✓ Valid credentials.json');
  } catch (error) {
    throw new Error(`Invalid credentials.json: ${error}`);
  }

  // Copy to MCP server directory
  const targetDir = path.join(os.homedir(), 'mcp-servers/google-docs-mcp');
  const targetPath = path.join(targetDir, 'credentials.json');

  await fs.mkdir(targetDir, { recursive: true });
  await fs.copyFile(resolvedPath, targetPath);

  console.log(`✓ Copied to ${targetPath}`);

  return targetPath;
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

export async function showGcpSetupSummary(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   GCP Setup Complete                                           ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('What was set up:');
  console.log('  ✓ Google Docs API enabled');
  console.log('  ✓ Google Drive API enabled');
  console.log('  ✓ OAuth Consent Screen configured');
  console.log('  ✓ OAuth Client ID created (Desktop app)');
  console.log('  ✓ credentials.json downloaded and installed\n');

  console.log('Next: OAuth authentication (browser will open)\n');
}

/**
 * Get the actual gcloud Application Default Credentials path
 * @returns Absolute path to ADC file
 */
export function getAdcPath(): string {
  // Linux/macOS standard location
  if (process.platform === 'win32') {
    // Windows: %APPDATA%\gcloud\application_default_credentials.json
    return path.join(
      process.env.APPDATA || path.join(os.homedir(), 'AppData', 'Roaming'),
      'gcloud',
      'application_default_credentials.json'
    );
  } else {
    // Linux/macOS: ~/.config/gcloud/application_default_credentials.json
    return path.join(
      os.homedir(),
      '.config',
      'gcloud',
      'application_default_credentials.json'
    );
  }
}

/**
 * Use gcloud Application Default Credentials instead of OAuth flow
 * @returns Path to ADC file
 */
export async function useGcloudCredentialsPath(): Promise<string> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Using gcloud CLI Credentials                                 ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('The Google Docs MCP will use your existing gcloud credentials.');
  console.log('This bypasses the need for OAuth client setup.\n');

  const adcPath = getAdcPath();

  // Validate ADC file exists
  try {
    await fs.access(adcPath);
    console.log(`✓ Found Application Default Credentials: ${adcPath}`);
  } catch (error) {
    throw new SetupError(
      'gcloud Application Default Credentials not found',
      'Run: gcloud auth application-default login',
      'https://cloud.google.com/sdk/gcloud/reference/auth/application-default/login'
    );
  }

  console.log('✓ Configured to use gcloud Application Default Credentials');
  console.log('The MCP server will authenticate as your gcloud user.');
  console.log('No additional OAuth setup needed!\n');

  return adcPath;
}
