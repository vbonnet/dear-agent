import * as fs from 'fs/promises';
import { google } from 'googleapis';
import * as open from 'open';
import inquirer from 'inquirer';
import { retryWithBackoff, promptWithTimeout } from './errors';
import { storeOktaToken, migrateTokensToKeychain } from './token-storage';

interface Credentials {
  installed: {
    client_id: string;
    client_secret: string;
    redirect_uris: string[];
  };
}

interface TokenData {
  type: string;
  client_id: string;
  client_secret: string;
  refresh_token: string;
}

export async function oauthFlow(credentialsPath: string, tokenPath: string): Promise<void> {
  // 1. Load credentials
  const credentials = await loadCredentials(credentialsPath);
  await validateCredentials(credentials);

  // 2. Create OAuth2 client
  const oauth2Client = new google.auth.OAuth2(
    credentials.installed.client_id,
    credentials.installed.client_secret,
    credentials.installed.redirect_uris[0]
  );

  // 3. Generate auth URL
  const authUrl = oauth2Client.generateAuthUrl({
    access_type: 'offline',
    scope: [
      'https://www.googleapis.com/auth/documents.readonly',
      'https://www.googleapis.com/auth/drive.readonly',
    ],
  });

  // 4. Open browser
  console.log('Opening browser for authentication...');
  await open.default(authUrl);

  // 5. Prompt for auth code (with timeout)
  const authCode = await promptWithTimeout(
    'Enter the authorization code: ',
    5 * 60 * 1000, // 5 min timeout
    async (message: string) => {
      const answers = await inquirer.prompt([
        {
          type: 'input',
          name: 'code',
          message,
        },
      ]);
      return answers.code;
    }
  );

  // 6. Exchange code for tokens (with retry)
  const { tokens } = await retryWithBackoff(
    () => oauth2Client.getToken(authCode),
    3,
    1000
  );

  // 7. Save tokens with 600 permissions
  await saveToken(tokens, tokenPath, credentials);
  console.log('✓ Authentication successful!');
}

export async function loadCredentials(credentialsPath: string): Promise<Credentials> {
  try {
    const content = await fs.readFile(credentialsPath, 'utf-8');
    return JSON.parse(content);
  } catch (error) {
    throw new Error(`Failed to load credentials from ${credentialsPath}: ${error}`);
  }
}

export async function validateCredentials(creds: any): Promise<void> {
  if (!creds.installed) {
    throw new Error('Missing "installed" section (expected Desktop app credentials)');
  }
  if (!creds.installed.client_id) {
    throw new Error('Missing client_id');
  }
  if (!creds.installed.client_id.endsWith('.apps.googleusercontent.com')) {
    throw new Error('client_id must end with .apps.googleusercontent.com');
  }
  if (!creds.installed.client_secret) {
    throw new Error('Missing client_secret');
  }
  if (!creds.installed.redirect_uris || creds.installed.redirect_uris.length === 0) {
    throw new Error('Missing redirect_uris');
  }
}

export async function saveToken(
  tokens: any,
  tokenPath: string,
  credentials: Credentials
): Promise<void> {
  const tokenData: TokenData = {
    type: 'authorized_user',
    client_id: credentials.installed.client_id,
    client_secret: credentials.installed.client_secret,
    refresh_token: tokens.refresh_token,
  };

  try {
    // Attempt to migrate existing plaintext tokens (one-time operation)
    const migrated = await migrateTokensToKeychain();
    if (migrated) {
      console.log('✓ Migrated existing token to OS keychain');
    }

    // Store new token in OS keychain
    await storeOktaToken(tokenData);
    console.log('✓ Token stored securely in OS keychain');

  } catch (error: any) {
    // Fail-fast: Do not fallback to plaintext for security reasons
    console.error('Failed to store token in keychain:', error.message);
    throw error;
  }
}

export async function checkGitTracking(filePath: string): Promise<{
  tracked: boolean;
  warning: string | null;
}> {
  const { exec } = require('child_process');
  const { promisify } = require('util');
  const execAsync = promisify(exec);

  try {
    const { stdout } = await execAsync(`git ls-files ${filePath}`);
    const tracked = stdout.trim().length > 0;

    if (tracked) {
      return {
        tracked: true,
        warning: `WARNING: ${filePath} is tracked in git! This sensitive file should be in .gitignore.`,
      };
    }

    return { tracked: false, warning: null };
  } catch (error) {
    // Not in a git repo or git not available - that's fine
    return { tracked: false, warning: null };
  }
}

export async function ensureGitignore(filePath: string, gitignorePath: string): Promise<void> {
  try {
    // Check if .gitignore exists
    let gitignoreContent = '';
    try {
      gitignoreContent = await fs.readFile(gitignorePath, 'utf-8');
    } catch (error) {
      // .gitignore doesn't exist, that's fine
    }

    // Check if filePath is already in .gitignore
    const lines = gitignoreContent.split('\n');
    const fileName = filePath.split('/').pop() || filePath;

    if (!lines.some(line => line.trim() === fileName || line.trim() === filePath)) {
      // Add to .gitignore
      await fs.appendFile(gitignorePath, `\n${fileName}\n`);
      console.log(`✓ Added ${fileName} to .gitignore`);
    }
  } catch (error) {
    console.warn(`Could not update .gitignore: ${error}`);
  }
}
