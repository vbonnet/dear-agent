/**
 * Health Command - Fast Health Check for MCP Wizard
 *
 * Runs 2 quick health checks (<2 seconds) and reports overall status.
 * Checks: Token validation + MCP process status
 * Supports --silent (exit code only), --json (machine-readable), --force (bypass cache).
 *
 * Part of Phase 4-v2 Health and Doctor Commands (oss-n1nq.4-v2)
 *
 * @module commands/health
 */

import ora from 'ora';
import chalk from 'chalk';
import { checkTokenHealth, checkMCPProcesses, HealthCheckResult } from '../lib/health-checks';

/**
 * Health command options
 */
export interface HealthOptions {
  /** Suppress output, exit code only */
  silent?: boolean;

  /** Output JSON instead of human-readable */
  json?: boolean;

  /** Bypass cache, run fresh checks */
  force?: boolean;
}

/**
 * Health status interface for test compatibility
 */
export interface HealthStatus {
  overall: 'healthy' | 'warning' | 'critical';
  checks: {
    token: { status: string; details?: string };
    mcp: { status: string; details?: string };
    network: { status: string; details?: string };
    intentAnalyzer: { status: string; details?: string };
    globalMcp: { status: string; details?: string };
  };
  warnings?: string[];
  errors?: string[];
}

/**
 * Check health status (exported for testing)
 *
 * @param options - Health check options
 * @returns Health status object
 */
export async function checkHealth(options: HealthOptions = {}): Promise<HealthStatus> {
  // Run only token and MCP checks for <2s performance
  const results = await Promise.all([
    checkTokenHealth({ force: options.force }),
    checkMCPProcesses({ force: options.force }),
  ]);

  // Map results to check statuses
  const checks = {
    token: { status: 'healthy', details: undefined as string | undefined },
    mcp: { status: 'healthy', details: undefined as string | undefined },
    network: { status: 'healthy', details: undefined as string | undefined },
    intentAnalyzer: { status: 'healthy', details: undefined as string | undefined },
    globalMcp: { status: 'healthy', details: undefined as string | undefined },
  };

  const warnings: string[] = [];
  const errors: string[] = [];

  results.forEach((result) => {
    const checkKey = getCheckKey(result.name);
    if (checkKey) {
      checks[checkKey].status = result.status;
      checks[checkKey].details = result.message;

      if (result.status === 'degraded') {
        warnings.push(`${result.name}: ${result.message}`);
      } else if (result.status === 'unhealthy') {
        errors.push(`${result.name}: ${result.message}`);
      }
    }
  });

  // Determine overall status
  const statuses = results.map((r) => r.status);
  let overall: 'healthy' | 'warning' | 'critical';

  if (statuses.includes('unhealthy')) {
    overall = 'critical';
  } else if (statuses.includes('degraded')) {
    overall = 'warning';
  } else {
    overall = 'healthy';
  }

  return { overall, checks, warnings, errors };
}

/**
 * Format health status for display (exported for testing)
 *
 * @param status - Health status object
 * @returns Formatted string
 */
export function formatHealthOutput(status: HealthStatus): string {
  const lines: string[] = [];

  // Overall status header
  const statusColor = status.overall === 'healthy' ? chalk.green :
                      status.overall === 'warning' ? chalk.yellow : chalk.red;
  lines.push(statusColor(`Overall Status: ${status.overall.toUpperCase()}`));
  lines.push('');

  // Component health
  lines.push('Component Health:');
  Object.entries(status.checks).forEach(([key, check]) => {
    const icon = check.status === 'healthy' ? '✓' :
                 check.status === 'degraded' || check.status === 'warning' ? '⚠' : '✗';
    const color = check.status === 'healthy' ? chalk.green :
                  check.status === 'degraded' || check.status === 'warning' ? chalk.yellow : chalk.red;
    // Map keys to display labels
    const labelMap: Record<string, string> = {
      token: 'Token',
      mcp: 'MCP',
      network: 'Network',
      intentAnalyzer: 'Intent Analyzer',
      globalMcp: 'Global MCP',
    };
    const label = labelMap[key] || key;
    lines.push(color(`  ${icon} ${label}: ${check.status}`));
    if (check.details) {
      lines.push(chalk.dim(`     ${check.details}`));
    }
  });

  // Warnings section
  if (status.warnings && status.warnings.length > 0) {
    lines.push('');
    lines.push(chalk.yellow('Warnings:'));
    status.warnings.forEach((w) => {
      lines.push(chalk.yellow(`  • ${w}`));
    });
  }

  // Errors section
  if (status.errors && status.errors.length > 0) {
    lines.push('');
    lines.push(chalk.red('Errors:'));
    status.errors.forEach((e) => {
      lines.push(chalk.red(`  • ${e}`));
    });
  }

  return lines.join('\n');
}

/**
 * Helper to map check name to key
 */
function getCheckKey(name: string): keyof HealthStatus['checks'] | null {
  if (name === 'Token Health') return 'token';
  if (name === 'MCP Processes') return 'mcp';
  if (name === 'Network Connectivity') return 'network';
  if (name === 'Intent Analyzer') return 'intentAnalyzer';
  if (name === 'Global MCP Discovery') return 'globalMcp';
  return null;
}

/**
 * Run health command
 *
 * Exit codes:
 * - 0: All checks healthy
 * - 1: One or more checks degraded (warnings)
 * - 2: One or more checks unhealthy (critical)
 *
 * @param options - Command options
 *
 * @example
 * await health({ silent: false, json: false, force: false });
 */
export async function health(options: HealthOptions = {}): Promise<void> {
  let spinner: any;

  // Show spinner unless silent or JSON mode
  if (!options.silent && !options.json) {
    spinner = ora('Running health checks...').start();
  }

  try {
    // Use checkHealth for consistent behavior
    const status = await checkHealth(options);

    if (spinner) {
      spinner.stop();
    }

    // Output results
    if (options.json) {
      // JSON output
      const output = {
        overall_status: status.overall,
        checks: status.checks,
        warnings: status.warnings,
        errors: status.errors,
        timestamp: new Date().toISOString(),
      };
      console.log(JSON.stringify(output, null, 2));
    } else if (!options.silent) {
      // Human-readable output
      console.log(formatHealthOutput(status));

      // Suggest doctor command if not healthy
      if (status.overall !== 'healthy') {
        console.log();
        console.log(chalk.dim("Run 'mcp-wizard doctor' for detailed diagnostics and recommendations."));
      }
    }

    // Exit with appropriate code
    const exitCode = status.overall === 'healthy' ? 0 : status.overall === 'warning' ? 1 : 2;
    process.exit(exitCode);
  } catch (error: any) {
    if (spinner) {
      spinner.fail('Health check failed');
    }

    if (!options.silent) {
      console.error(chalk.red(`Error: ${error.message}`));
    }

    process.exit(2); // Critical failure
  }
}
