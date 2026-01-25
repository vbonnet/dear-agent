/**
 * Doctor Command - Comprehensive Diagnostics for MCP Wizard
 *
 * Runs detailed diagnostics beyond health checks and provides actionable recommendations.
 * Supports --silent (exit code only), --json (machine-readable), --force (bypass cache).
 *
 * Part of Phase 4-v2 Health and Doctor Commands (oss-n1nq.4-v2)
 *
 * @module commands/doctor
 */

import ora from 'ora';
import chalk from 'chalk';
import { runAllHealthChecks, validateConfiguration } from '../lib/health-checks';

/**
 * Diagnostic report interface
 */
export interface DiagnosticReport {
  findings: DiagnosticFinding[];
  recommendations: string[];
}

/**
 * Individual diagnostic finding
 */
export interface DiagnosticFinding {
  component: string;
  status: 'ok' | 'warning' | 'error';
  message: string;
  details?: string;
}

/**
 * Doctor command options
 */
export interface DoctorOptions {
  /** Suppress output, exit code only */
  silent?: boolean;

  /** Output JSON instead of human-readable */
  json?: boolean;

  /** Bypass cache, run fresh checks */
  force?: boolean;
}

/**
 * Run comprehensive system diagnostics
 *
 * @param options - Doctor options
 * @returns Diagnostic report with findings and recommendations
 */
export async function runDiagnostics(options: DoctorOptions = {}): Promise<DiagnosticReport> {
  const findings: DiagnosticFinding[] = [];
  const recommendations: string[] = [];

  // Run all health checks with detailed information
  const healthResults = await runAllHealthChecks({ force: options.force });

  // Run configuration validation
  const configResult = await validateConfiguration();

  // Process health check results
  healthResults.forEach((result) => {
    let status: 'ok' | 'warning' | 'error';
    if (result.status === 'healthy') {
      status = 'ok';
    } else if (result.status === 'degraded') {
      status = 'warning';
    } else {
      status = 'error';
    }

    findings.push({
      component: result.name,
      status,
      message: result.message,
      details: result.details ? JSON.stringify(result.details, null, 2) : undefined,
    });

    // Add recommendations based on status
    if (result.status === 'unhealthy') {
      recommendations.push(...getRecommendations(result.name, result));
    } else if (result.status === 'degraded') {
      recommendations.push(...getRecommendations(result.name, result));
    }
  });

  // Process configuration result
  let configStatus: 'ok' | 'warning' | 'error';
  if (configResult.status === 'healthy') {
    configStatus = 'ok';
  } else if (configResult.status === 'degraded') {
    configStatus = 'warning';
  } else {
    configStatus = 'error';
  }

  findings.push({
    component: configResult.name,
    status: configStatus,
    message: configResult.message,
    details: configResult.details ? JSON.stringify(configResult.details, null, 2) : undefined,
  });

  if (configResult.status !== 'healthy') {
    recommendations.push(...getRecommendations(configResult.name, configResult));
  }

  // Remove duplicate recommendations
  const uniqueRecommendations = Array.from(new Set(recommendations));

  return {
    findings,
    recommendations: uniqueRecommendations,
  };
}

/**
 * Get actionable recommendations for a health check result
 *
 * @param checkName - Name of the health check
 * @param result - Health check result
 * @returns Array of recommendations
 */
function getRecommendations(checkName: string, result: any): string[] {
  const recs: string[] = [];

  switch (checkName) {
    case 'Token Health':
      if (result.status === 'unhealthy') {
        const expiryTime = result.details?.expiresAt
          ? new Date(result.details.expiresAt).toISOString()
          : 'unknown time';
        recs.push(`Token expired at ${expiryTime}. Run: \`mcp-wizard setup --mcps=googledocs\``);
      } else if (result.status === 'degraded') {
        const expiryTime = result.details?.expiresAt
          ? new Date(result.details.expiresAt).toISOString()
          : 'soon';
        const ttlMinutes = result.details?.ttlMinutes ?? '?';
        recs.push(`Token expires at ${expiryTime} (in ${ttlMinutes} minutes). Re-authenticate proactively: \`mcp-wizard setup --mcps=googledocs\``);
      }
      break;

    case 'MCP Processes':
      if (result.status === 'unhealthy' || result.status === 'degraded') {
        const processes = result.details?.processes || [];
        const down = processes.filter((p: any) => !p.alive);
        down.forEach((p: any) => {
          recs.push(`MCP server '${p.name}' not running. Restart it with Claude Code or check config: ~/.config/mcp-wizard/config.json`);
        });
        if (down.length === 0 && result.status !== 'healthy') {
          recs.push('Some MCP processes are down. Restart them with Claude Code.');
        }
      }
      break;

    case 'Network Connectivity':
      if (result.status === 'unhealthy' || result.status === 'degraded') {
        const endpoints = result.details?.endpoints || [];
        const unreachable = endpoints.filter((e: any) => !e.reachable);
        unreachable.forEach((e: any) => {
          const errorInfo = e.error || 'timeout after 3s';
          recs.push(`Endpoint unreachable: ${e.endpoint} (${errorInfo}). Check VPN/firewall settings.`);
        });
        if (unreachable.length === 0 && result.status !== 'healthy') {
          recs.push('Some network endpoints unreachable. Check internet connection.');
        }
      }
      break;

    case 'Intent Analyzer':
      if (result.status === 'unhealthy') {
        recs.push('Intent analyzer is failing. This may indicate corrupted configuration.');
        recs.push('Try reinstalling mcp-wizard: `npm install -g mcp-wizard`');
      } else if (result.status === 'degraded') {
        recs.push('Intent analyzer accuracy is below optimal. Consider updating to latest version.');
      }
      break;

    case 'Configuration':
      if (result.status === 'unhealthy') {
        const configPath = result.details?.path || '~/.config/mcp-wizard/config.json';
        const errors = result.details?.errors || [];
        if (errors.length > 0) {
          recs.push(`Config validation failed (${errors.length} errors). Fix issues in ${configPath} or run: \`mcp-wizard setup\` to recreate`);
        } else {
          recs.push(`Config file missing or invalid at ${configPath}. Run: \`mcp-wizard setup\` to create`);
        }
      } else if (result.status === 'degraded') {
        const configPath = result.details?.path || '~/.config/mcp-wizard/config.json';
        recs.push(`Config is incomplete. Add missing settings in ${configPath} or run: \`mcp-wizard config init\``);
      }
      break;
  }

  return recs;
}

/**
 * Format diagnostic report for display
 *
 * @param report Diagnostic report object
 * @returns Formatted string for console output
 */
export function formatDiagnosticReport(report: DiagnosticReport): string {
  const lines: string[] = [];

  lines.push(chalk.bold('System Diagnostics Report'));
  lines.push('========================');
  lines.push('');

  if (report.findings.length === 0) {
    lines.push(chalk.green('No issues found.'));
  } else {
    lines.push(chalk.bold('Findings:'));
    report.findings.forEach((finding) => {
      const statusColor = finding.status === 'ok' ? chalk.green :
                          finding.status === 'warning' ? chalk.yellow : chalk.red;
      const icon = finding.status === 'ok' ? '✓' :
                   finding.status === 'warning' ? '⚠' : '✗';

      lines.push(statusColor(`  ${icon} [${finding.status.toUpperCase()}] ${finding.component}: ${finding.message}`));
      if (finding.details) {
        // Format details with indentation
        const detailLines = finding.details.split('\n');
        detailLines.forEach((line) => {
          lines.push(chalk.dim(`      ${line}`));
        });
      }
    });
  }

  if (report.recommendations.length > 0) {
    lines.push('');
    lines.push(chalk.bold('Recommendations:'));
    report.recommendations.forEach((rec, index) => {
      lines.push(chalk.cyan(`  ${index + 1}. ${rec}`));
    });
  }

  return lines.join('\n');
}

/**
 * Run doctor command
 *
 * Exit codes:
 * - 0: All checks healthy
 * - 1: One or more warnings
 * - 2: One or more errors
 *
 * @param options - Command options
 *
 * @example
 * await doctor({ silent: false, json: false, force: false });
 */
export async function doctor(options: DoctorOptions = {}): Promise<void> {
  let spinner: any;

  // Show spinner unless silent or JSON mode
  if (!options.silent && !options.json) {
    spinner = ora('Running diagnostics...').start();
  }

  try {
    // Run diagnostics
    const report = await runDiagnostics(options);

    if (spinner) {
      spinner.stop();
    }

    // Determine overall status
    const hasErrors = report.findings.some((f) => f.status === 'error');
    const hasWarnings = report.findings.some((f) => f.status === 'warning');
    let overallStatus: 'healthy' | 'warning' | 'error';

    if (hasErrors) {
      overallStatus = 'error';
    } else if (hasWarnings) {
      overallStatus = 'warning';
    } else {
      overallStatus = 'healthy';
    }

    // Output results
    if (options.json) {
      // JSON output
      const output = {
        overall_status: overallStatus,
        findings: report.findings,
        recommendations: report.recommendations,
        timestamp: new Date().toISOString(),
      };
      console.log(JSON.stringify(output, null, 2));
    } else if (!options.silent) {
      // Human-readable output
      console.log(formatDiagnosticReport(report));

      console.log();
      const statusColor = overallStatus === 'healthy' ? chalk.green :
                          overallStatus === 'warning' ? chalk.yellow : chalk.red;
      console.log(`Overall Status: ${statusColor(overallStatus.toUpperCase())}`);
    }

    // Exit with appropriate code
    const exitCode = overallStatus === 'healthy' ? 0 : overallStatus === 'warning' ? 1 : 2;
    process.exit(exitCode);
  } catch (error: any) {
    if (spinner) {
      spinner.fail('Diagnostics failed');
    }

    if (!options.silent) {
      console.error(chalk.red(`Error: ${error.message}`));
    }

    process.exit(2); // Critical failure
  }
}
