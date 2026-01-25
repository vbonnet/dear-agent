/**
 * Token Injection Layer - Usage Example
 *
 * Demonstrates how to spawn MCP processes with automatic token management.
 *
 * @module examples/token-injection-example
 */

import { spawnMCPWithToken, needsTokenRefresh, TokenInjectionConfig } from '../src/lib/token-injection';

/**
 * Example: Spawn MCP server with automatic token management
 */
async function spawnMCPExample() {
  const config: TokenInjectionConfig = {
    oktaDomain: process.env.OKTA_DOMAIN || '[REDACTED_EMPLOYER].okta.com',
    clientId: process.env.OKTA_CLIENT_ID || 'your-client-id',
    scopes: ['openid', 'profile', 'email'],
  };

  console.log('Spawning MCP server with token injection...');

  try {
    // Spawn MCP process
    // Token is automatically validated, refreshed (if needed), or re-authenticated
    const mcpProcess = await spawnMCPWithToken(
      ['mcp-server-gdocs', '--port', '3000'],
      config
    );

    console.log('✓ MCP server spawned successfully');
    console.log(`  PID: ${mcpProcess.pid}`);

    // Handle MCP output
    mcpProcess.stdout?.on('data', (data) => {
      console.log(`[MCP] ${data.toString().trim()}`);
    });

    mcpProcess.stderr?.on('data', (data) => {
      console.error(`[MCP ERROR] ${data.toString().trim()}`);
    });

    mcpProcess.on('exit', (code) => {
      console.log(`MCP process exited with code ${code}`);
    });

    // Keep process running for demonstration
    await new Promise((resolve) => setTimeout(resolve, 5000));

    // Graceful shutdown
    mcpProcess.kill('SIGTERM');
    console.log('MCP process terminated');
  } catch (error: any) {
    console.error('Failed to spawn MCP:', error.message);
    process.exit(1);
  }
}

/**
 * Example: Check if token needs refresh
 */
async function checkTokenHealthExample() {
  const config: TokenInjectionConfig = {
    oktaDomain: process.env.OKTA_DOMAIN || '[REDACTED_EMPLOYER].okta.com',
    clientId: process.env.OKTA_CLIENT_ID || 'your-client-id',
    scopes: ['openid', 'profile', 'email'],
  };

  console.log('Checking token health...');

  try {
    const needsRefresh = await needsTokenRefresh(config);

    if (needsRefresh) {
      console.log('⚠ Token needs refresh or re-authentication');
      console.log('  Next spawnMCPWithToken() call will handle this automatically');
    } else {
      console.log('✓ Token is healthy');
    }
  } catch (error: any) {
    console.error('Failed to check token health:', error.message);
  }
}

/**
 * Example: Spawn multiple MCP processes
 */
async function spawnMultipleMCPsExample() {
  const config: TokenInjectionConfig = {
    oktaDomain: process.env.OKTA_DOMAIN || '[REDACTED_EMPLOYER].okta.com',
    clientId: process.env.OKTA_CLIENT_ID || 'your-client-id',
    scopes: ['openid', 'profile', 'email'],
  };

  console.log('Spawning multiple MCP processes...');

  const mcpCommands = [
    ['mcp-server-gdocs', '--port', '3000'],
    ['mcp-server-atlassian', '--port', '3001'],
    ['mcp-server-slack', '--port', '3002'],
  ];

  try {
    const processes = await Promise.all(
      mcpCommands.map((cmd) => spawnMCPWithToken(cmd, config))
    );

    console.log(`✓ Spawned ${processes.length} MCP processes`);
    processes.forEach((proc, i) => {
      console.log(`  [${i + 1}] PID: ${proc.pid} - ${mcpCommands[i][0]}`);
    });

    // Cleanup
    setTimeout(() => {
      processes.forEach((proc) => proc.kill('SIGTERM'));
      console.log('All MCP processes terminated');
    }, 5000);
  } catch (error: any) {
    console.error('Failed to spawn MCPs:', error.message);
  }
}

/**
 * Main function
 */
async function main() {
  console.log('='.repeat(60));
  console.log('Token Injection Layer - Examples');
  console.log('='.repeat(60));
  console.log();

  // Example 1: Check token health
  console.log('Example 1: Check Token Health');
  console.log('-'.repeat(60));
  await checkTokenHealthExample();
  console.log();

  // Example 2: Spawn single MCP
  console.log('Example 2: Spawn Single MCP');
  console.log('-'.repeat(60));
  await spawnMCPExample();
  console.log();

  // Example 3: Spawn multiple MCPs
  console.log('Example 3: Spawn Multiple MCPs');
  console.log('-'.repeat(60));
  await spawnMultipleMCPsExample();
  console.log();

  console.log('='.repeat(60));
  console.log('Examples complete');
  console.log('='.repeat(60));
}

// Run examples if this file is executed directly
if (require.main === module) {
  main().catch((error) => {
    console.error('Fatal error:', error);
    process.exit(1);
  });
}
