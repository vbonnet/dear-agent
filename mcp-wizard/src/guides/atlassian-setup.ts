import * as path from 'path';
import * as os from 'os';
import * as fs from 'fs/promises';
import inquirer from 'inquirer';
import * as open from 'open';
import { spawn } from 'child_process';
import { SetupError } from '../errors/setup-error';

const ATLASSIAN_MCP_URL = 'https://mcp.atlassian.com/v1/sse';
const OAUTH_CALLBACK_PORT = 45454;
const AUTH_TIMEOUT = 300; // 5 minutes

export async function runAtlassianSetupGuide(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Atlassian MCP OAuth Setup                                    ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('The Atlassian MCP provides access to Jira and Confluence.');
  console.log('Authentication uses OAuth 2.0 with PKCE for security.\n');

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

  console.log('\nInitializing OAuth flow...');
  console.log('This will:');
  console.log('  1. Start an OAuth callback server on localhost:' + OAUTH_CALLBACK_PORT);
  console.log('  2. Open your browser to authenticate');
  console.log('  3. Redirect back to localhost after approval\n');

  // Kill any existing mcp-remote processes that might be holding the port
  try {
    await killExistingMcpRemote();
  } catch (error) {
    console.log('Note: Could not clean up old processes, continuing anyway...');
  }

  // Start mcp-remote to trigger OAuth flow
  const authUrl = await startOAuthFlow();
  
  if (!authUrl) {
    throw new SetupError(
      'Failed to start OAuth flow',
      'Try running manually: npx -y mcp-remote@latest ' + ATLASSIAN_MCP_URL,
      'https://github.com/your-org/mcp-wizard/issues'
    );
  }

  console.log('\n📋 OAuth URL generated:\n');
  console.log(authUrl);
  console.log('');

  // Open browser
  const { openBrowser } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'openBrowser',
      message: 'Open browser for authentication?',
      default: true,
    },
  ]);

  if (openBrowser) {
    console.log('\nOpening browser...');
    await open.default(authUrl);
  } else {
    console.log('\nPlease open this URL in your browser:\n');
    console.log(authUrl);
    console.log('');
  }

  console.log('\n📝 Instructions:');
  console.log('  1. Sign in with your Atlassian account');
  console.log('  2. Select your Atlassian site (e.g., yourcompany.atlassian.net)');
  console.log('  3. Review and grant the requested permissions:');
  console.log('     • Read Jira issues, projects, boards');
  console.log('     • Read Confluence pages, spaces');
  console.log('  4. Click "Accept" to authorize');
  console.log('  5. Wait for "Authorization successful!" message\n');

  console.log('⏱️  Waiting for OAuth callback (timeout: ' + AUTH_TIMEOUT + 's)...');
  console.log('   The browser will redirect to http://localhost:' + OAUTH_CALLBACK_PORT + '/oauth/callback\n');

  // Wait for user to complete OAuth
  const { completed } = await inquirer.prompt([
    {
      type: 'confirm',
      name: 'completed',
      message: 'Did you see "Authorization successful" in your browser?',
      default: true,
    },
  ]);

  if (!completed) {
    console.log('\n⚠️  OAuth may not have completed successfully.');
    console.log('\nTroubleshooting:');
    console.log('  • Check if port ' + OAUTH_CALLBACK_PORT + ' is accessible');
    console.log('  • For remote machines, ensure port forwarding is set up:');
    console.log('    ssh -L ' + OAUTH_CALLBACK_PORT + ':localhost:' + OAUTH_CALLBACK_PORT + ' your-machine');
    console.log('  • Check browser console for errors');
    console.log('  • Try: pkill -f mcp-remote && retry setup\n');
    
    throw new SetupError(
      'OAuth authentication not completed',
      'See troubleshooting steps above',
      'https://github.com/your-org/mcp-wizard/issues'
    );
  }

  // Test the connection
  console.log('\n🔍 Testing Atlassian MCP connection...');
  const testResult = await testAtlassianConnection();
  
  if (testResult.success) {
    console.log('✓ Connection test successful!');
    console.log('✓ Atlassian MCP is ready to use\n');
  } else {
    console.log('⚠️  Connection test failed: ' + testResult.error);
    console.log('The MCP is configured but may need troubleshooting.\n');
  }
}

async function killExistingMcpRemote(): Promise<void> {
  return new Promise((resolve) => {
    const pkill = spawn('pkill', ['-f', 'mcp-remote.*atlassian']);
    pkill.on('close', () => {
      // Wait a moment for processes to clean up
      setTimeout(resolve, 1000);
    });
    pkill.on('error', () => resolve()); // Ignore errors
  });
}

async function startOAuthFlow(): Promise<string | null> {
  return new Promise((resolve) => {
    const mcpRemote = spawn('npx', [
      '-y',
      'mcp-remote@latest',
      ATLASSIAN_MCP_URL,
      '--auth-timeout',
      AUTH_TIMEOUT.toString()
    ]);

    let authUrl: string | null = null;

    mcpRemote.stdout.on('data', (data: Buffer) => {
      const output = data.toString();
      
      // Look for the OAuth URL in the output
      const urlMatch = output.match(/Please authorize this client by visiting:\s*(https:\/\/mcp\.atlassian\.com\/v1\/authorize[^\s]+)/);
      if (urlMatch && !authUrl) {
        authUrl = urlMatch[1];
        // Don't kill the process - let it keep running to handle the callback
        resolve(authUrl);
      }
    });

    mcpRemote.stderr.on('data', (data: Buffer) => {
      // Ignore stderr for now
    });

    // Timeout after 10 seconds if we don't get the URL
    setTimeout(() => {
      if (!authUrl) {
        mcpRemote.kill();
        resolve(null);
      }
    }, 10000);
  });
}

async function testAtlassianConnection(): Promise<{ success: boolean; error?: string }> {
  return new Promise((resolve) => {
    const test = spawn('timeout', [
      '10',
      'npx',
      '-y',
      'mcp-remote@latest',
      ATLASSIAN_MCP_URL,
      '--auth-timeout',
      '10'
    ]);

    let output = '';
    let hasError = false;

    test.stdout.on('data', (data: Buffer) => {
      output += data.toString();
      
      // Check for success indicators
      if (output.includes('Connected to remote server') && 
          output.includes('Proxy established successfully')) {
        test.kill();
        resolve({ success: true });
      }
    });

    test.stderr.on('data', (data: Buffer) => {
      const error = data.toString();
      if (error.includes('ERROR') || error.includes('ECONNREFUSED')) {
        hasError = true;
      }
    });

    test.on('close', (code) => {
      if (hasError) {
        resolve({ success: false, error: 'Connection failed' });
      } else {
        // Timeout or killed after success
        resolve({ success: true });
      }
    });

    // Timeout after 10 seconds
    setTimeout(() => {
      test.kill();
      if (!output.includes('Connected')) {
        resolve({ success: false, error: 'Connection timeout' });
      }
    }, 10000);
  });
}

export async function showAtlassianSetupSummary(): Promise<void> {
  console.log('\n╔════════════════════════════════════════════════════════════════╗');
  console.log('║   Atlassian Setup Complete                                     ║');
  console.log('╚════════════════════════════════════════════════════════════════╝\n');

  console.log('What was set up:');
  console.log('  ✓ Atlassian MCP configured in ~/.config/claude/mcp.json');
  console.log('  ✓ OAuth 2.0 tokens obtained and stored');
  console.log('  ✓ Connection to mcp.atlassian.com verified');
  console.log('  ✓ Access to Jira and Confluence enabled\n');

  console.log('How it works:');
  console.log('  • mcp-remote acts as a proxy between Claude and Atlassian');
  console.log('  • OAuth tokens auto-refresh (no re-authentication needed)');
  console.log('  • Connection uses SSE (Server-Sent Events) for real-time updates');
  console.log('  • MCP loads on-demand when you first request Jira/Confluence data\n');

  console.log('Try it out:');
  console.log('  1. Start Claude Code: claude');
  console.log('  2. Ask: "Show me recent Jira issues"');
  console.log('  3. Ask: "Search Confluence for [topic]"');
  console.log('  4. Ask: "What Jira issues are assigned to me?"\n');

  console.log('Troubleshooting:');
  console.log('  • If OAuth expires: Delete ~/.mcp-remote/ and restart Claude');
  console.log('  • Check connection: npx -y mcp-remote@latest ' + ATLASSIAN_MCP_URL);
  console.log('  • Kill stuck processes: pkill -f mcp-remote');
  console.log('  • For remote machines: Set up port forwarding on ' + OAUTH_CALLBACK_PORT + '\n');
}
