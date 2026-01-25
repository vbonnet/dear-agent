/**
 * Session-Start Hook Command
 *
 * Checks MCP authentication status at shell session startup.
 * Provides proactive feedback about token health and supports
 * automatic token refresh for seamless developer experience.
 *
 * Part of Phase 4-v2 SessionStart Hook Integration (oss-n1nq.5-v2)
 *
 * @module commands/session-start
 */

import ora from 'ora';
import chalk from 'chalk';
import { discoverMCPs, type MCPConfig } from '../lib/config-discovery';
import { runAllHealthChecks, type HealthCheckResult } from '../lib/health-checks';
import { loadConfig } from '../lib/user-config';
import { authenticate, type AuthConfig } from '../lib/auth';

/**
 * Session-start command options
 */
export interface SessionStartOptions {
  /** Show detailed health check results */
  verbose?: boolean;
  /** Automatically refresh expired tokens */
  autoRefresh?: boolean;
}

/**
 * Main session-start command entry point
 *
 * Workflow:
 * 1. Discover MCPs from config files
 * 2. Run health checks (with caching)
 * 3. Format and display output
 * 4. Auto-refresh tokens if enabled and needed
 * 5. Exit with appropriate code
 *
 * @param options - Command options
 */
export async function sessionStart(options: SessionStartOptions): Promise<void> {
  // 1. Discover MCPs
  const mcps = await discoverMCPs();

  if (mcps.length === 0) {
    console.log(chalk.yellow('⚠️  No MCP configuration found. Run `mcp-wizard setup` to configure MCPs.'));
    process.exit(0); // Exit 0, not an error
  }

  // 2. Run health checks with spinner
  const spinner = ora('Checking MCP health...').start();

  let healthResults: HealthCheckResult[];
  try {
    healthResults = await runAllHealthChecks({ force: false });
    spinner.stop();
  } catch (error: any) {
    spinner.fail(chalk.red('Health check failed'));
    console.error(chalk.gray(`Error: ${error.message}`));
    process.exit(1);
  }

  // 3. Analyze token health (covers all Okta MCPs)
  const tokenHealth = healthResults.find(r => r.name === 'Token Health');

  // 4. Format and display output
  formatHealthStatus(mcps, tokenHealth, options.verbose || false);

  // 5. Auto-refresh if enabled and token unhealthy
  if (options.autoRefresh && tokenHealth?.status === 'unhealthy') {
    const refreshSuccess = await handleAutoRefresh(mcps);

    // Exit 0 if refresh succeeded, 1 if failed
    process.exit(refreshSuccess ? 0 : 1);
  }

  // 6. Exit with appropriate code
  // Exit 0 for healthy or degraded (warning), Exit 1 for unhealthy (error)
  const exitCode = tokenHealth?.status === 'unhealthy' ? 1 : 0;
  process.exit(exitCode);
}

/**
 * Format and display health status
 *
 * Two modes:
 * - Verbose: Show detailed token info, MCP list, expiration
 * - Default: Single-line summary with color coding
 *
 * @param mcps - Discovered MCP configurations
 * @param tokenHealth - Token health check result
 * @param verbose - Whether to show detailed output
 */
function formatHealthStatus(
  mcps: MCPConfig[],
  tokenHealth: HealthCheckResult | undefined,
  verbose: boolean
): void {
  // Verbose mode: Show detailed status
  if (verbose && tokenHealth) {
    console.log(chalk.bold('\nMCP Health Status:'));
    console.log(`Token: ${formatTokenStatus(tokenHealth)}`);
    console.log(`MCPs configured: ${mcps.map(m => m.name).join(', ')}`);
    console.log(`Expiration: ${formatExpiration(tokenHealth)}`);
    return;
  }

  // Default mode: Single summary line with color coding
  const status = tokenHealth?.status || 'unknown';

  if (status === 'healthy') {
    // Green checkmark for healthy
    console.log(chalk.green(`✓ MCP Health: ${mcps.length} MCPs authenticated`));
  } else if (status === 'degraded') {
    // Yellow warning for degraded (expiring soon)
    console.log(chalk.yellow(`⚠ MCP Health: Token expiring soon (${formatTTL(tokenHealth)})`));
    console.log(chalk.yellow(`Run \`mcp-wizard auth\` to refresh`));
  } else {
    // Red X for unhealthy (expired or invalid)
    console.log(chalk.red(`✗ MCP Health: Token expired or invalid`));
    console.log(chalk.yellow(`Run \`mcp-wizard auth\` to re-authenticate`));
  }
}

/**
 * Handle automatic token refresh
 *
 * Attempts to refresh the Okta token non-interactively using Device Flow.
 * If successful, re-checks health and shows updated status.
 * If failed, shows error and manual remediation.
 *
 * @param mcps - Discovered MCP configurations
 * @returns true if refresh succeeded, false otherwise
 */
async function handleAutoRefresh(mcps: MCPConfig[]): Promise<boolean> {
  const spinner = ora('Refreshing tokens...').start();

  try {
    // Load config to get Okta settings
    const config = loadConfig() as any;

    if (!config.company?.okta_domain || !config.company?.okta_client_id) {
      spinner.fail(chalk.red('✗ Missing Okta configuration'));
      console.error(chalk.yellow(`Run \`mcp-wizard setup\` to configure Okta settings`));
      return false;
    }

    // Build auth config
    const authConfig: AuthConfig = {
      oktaDomain: config.company.okta_domain,
      clientId: config.company.okta_client_id,
      scopes: config.company.okta_scopes || ['openid', 'profile', 'email'],
    };

    // Attempt to refresh token via authenticate (handles Device Flow automatically)
    await authenticate(authConfig);
    spinner.succeed(chalk.green('✓ Token refreshed successfully'));

    // Re-check health to confirm
    const newHealth = await runAllHealthChecks({ force: true });
    const newTokenHealth = newHealth.find(r => r.name === 'Token Health');

    if (newTokenHealth?.status === 'healthy') {
      console.log(chalk.green(`✓ MCP Health: ${mcps.length} MCPs authenticated`));
      return true;
    } else {
      // Refresh succeeded but token still unhealthy (unexpected)
      console.log(chalk.yellow(`⚠ Token refresh succeeded but health check still shows issues`));
      console.log(chalk.yellow(`Run \`mcp-wizard auth\` to re-authenticate manually`));
      return false;
    }
  } catch (error: any) {
    // Refresh failed - show error and manual remediation
    spinner.fail(chalk.red('✗ Token refresh failed'));
    console.error(chalk.yellow(`Run \`mcp-wizard auth\` to re-authenticate manually`));
    console.error(chalk.gray(`Error: ${error.message}`));
    return false;
  }
}

/**
 * Format token status with color-coded symbol
 *
 * @param tokenHealth - Token health check result
 * @returns Formatted status string (e.g., "✓ authenticated")
 */
function formatTokenStatus(tokenHealth: HealthCheckResult): string {
  let symbol: string;

  if (tokenHealth.status === 'healthy') {
    symbol = chalk.green('✓');
  } else if (tokenHealth.status === 'degraded') {
    symbol = chalk.yellow('⚠');
  } else {
    symbol = chalk.red('✗');
  }

  return `${symbol} ${tokenHealth.message}`;
}

/**
 * Format token expiration time
 *
 * Converts TTL (time-to-live in seconds) to human-readable format.
 * Examples: "2h 15m", "45m", "Expired"
 *
 * @param tokenHealth - Token health check result
 * @returns Human-readable expiration time
 */
function formatExpiration(tokenHealth: HealthCheckResult): string {
  const ttl = tokenHealth.details?.ttl; // Seconds until expiration

  if (!ttl || ttl < 0) {
    return 'Expired';
  }

  const hours = Math.floor(ttl / 3600);
  const minutes = Math.floor((ttl % 3600) / 60);

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  } else {
    return `${minutes}m`;
  }
}

/**
 * Format TTL for warning messages (shorter format)
 *
 * @param tokenHealth - Token health check result
 * @returns Short TTL string (e.g., "15m", "2h 30m")
 */
function formatTTL(tokenHealth: HealthCheckResult | undefined): string {
  if (!tokenHealth?.details?.ttl) {
    return 'unknown';
  }

  const ttl = tokenHealth.details.ttl;
  const minutes = Math.floor(ttl / 60);

  if (minutes < 60) {
    return `${minutes}m`;
  } else {
    const hours = Math.floor(minutes / 60);
    const remainingMinutes = minutes % 60;
    return `${hours}h ${remainingMinutes}m`;
  }
}
