/**
 * Health Checks for MCP Wizard Context Broker
 *
 * Implements 4 core health checks:
 * 1. Token Health (Google OAuth)
 * 2. MCP Process Health (existence check)
 * 3. Network Connectivity (OAuth and API endpoints)
 * 4. Intent Analyzer (keyword matching accuracy)
 *
 * Part of Phase 4-v2 Health and Doctor Commands (oss-n1nq.4-v2)
 *
 * @module health-checks
 */

import { exec } from 'child_process';
import { promisify } from 'util';
import { getOktaToken } from './token-storage';
import { checkTokenHealth as checkTokenHealthInternal } from './token-injection';
import { analyzeIntent } from './intent-analyzer';
import { getCached, setCached } from './health-cache';
import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';

const execAsync = promisify(exec);

/**
 * Health check result interface
 */
export interface HealthCheckResult {
  /** Check name (e.g., "Token Health") */
  name: string;

  /** Health status */
  status: 'healthy' | 'degraded' | 'unhealthy';

  /** Human-readable message */
  message: string;

  /** Detailed information (for doctor mode or JSON output) */
  details?: Record<string, any>;

  /** Timestamp of last check */
  last_check: Date;
}

/**
 * Health check options
 */
export interface HealthCheckOptions {
  /** Bypass cache and run fresh checks */
  force?: boolean;
}

/**
 * Run all health checks in parallel
 *
 * @param options - Health check options
 * @returns Array of health check results
 *
 * @example
 * const results = await runAllHealthChecks({ force: false });
 * // Results from all 4 checks
 */
export async function runAllHealthChecks(options: HealthCheckOptions = {}): Promise<HealthCheckResult[]> {
  const checks = [
    checkTokenHealth(options),
    checkMCPProcesses(options),
    checkNetworkConnectivity(options),
    checkIntentAnalyzer(options),
    checkGlobalMCPHealth(options),
  ];

  return await Promise.all(checks);
}

/**
 * Check 1: Token Health (Google OAuth)
 *
 * Validates Google OAuth token (stored as "Okta" but is Google).
 * Status: healthy if valid and TTL > 5min, degraded if 1-5min, unhealthy if expired.
 *
 * V1 Limitation: Only checks Google tokens (Atlassian uses mcp-remote auth)
 *
 * @param options - Health check options
 * @returns Token health check result
 */
export async function checkTokenHealth(options: HealthCheckOptions): Promise<HealthCheckResult> {
  const cached = getCached('Token Health', options.force);
  if (cached) return cached;

  try {
    const token = await getOktaToken(); // Despite name, returns Google OAuth token
    const health = checkTokenHealthInternal(token);

    let result: HealthCheckResult;

    if (health.isExpired || !health.valid) {
      result = {
        name: 'Token Health',
        status: 'unhealthy',
        message: 'Google OAuth token expired',
        details: { expiresAt: health.expiresAt, remainingTTL: health.remainingTTL },
        last_check: new Date(),
      };
    } else {
      const ttlMinutes = health.remainingTTL ? Math.floor(health.remainingTTL / 60000) : 0;

      if (ttlMinutes < 5) {
        result = {
          name: 'Token Health',
          status: 'degraded',
          message: `Google token expires in ${ttlMinutes} minutes`,
          details: { expiresAt: health.expiresAt, remainingTTL: health.remainingTTL, ttlMinutes },
          last_check: new Date(),
        };
      } else {
        result = {
          name: 'Token Health',
          status: 'healthy',
          message: `Google token valid (expires in ${ttlMinutes} minutes)`,
          details: { expiresAt: health.expiresAt, remainingTTL: health.remainingTTL, ttlMinutes },
          last_check: new Date(),
        };
      }
    }

    setCached('Token Health', result);
    return result;
  } catch (error: any) {
    const result: HealthCheckResult = {
      name: 'Token Health',
      status: 'unhealthy',
      message: 'No Google OAuth token found',
      details: { error: error.message },
      last_check: new Date(),
    };
    setCached('Token Health', result);
    return result;
  }
}

/**
 * Check 2: MCP Process Health (existence only)
 *
 * Checks if configured MCP processes are running using pgrep.
 * Status: healthy if all alive, degraded if some alive, unhealthy if none alive.
 *
 * V1 Limitation: Existence check only (no stdio ping due to process ownership constraints)
 *
 * @param options - Health check options
 * @returns MCP process health check result
 */
export async function checkMCPProcesses(options: HealthCheckOptions): Promise<HealthCheckResult> {
  const cached = getCached('MCP Processes', options.force);
  if (cached) return cached;

  try {
    // Load config to get MCP list
    const config = await loadConfig();
    const mcps = Object.entries(config.broker?.downstream_mcps || {});

    if (mcps.length === 0) {
      const result: HealthCheckResult = {
        name: 'MCP Processes',
        status: 'degraded',
        message: 'No MCP servers configured',
        details: { processes: [] },
        last_check: new Date(),
      };
      setCached('MCP Processes', result);
      return result;
    }

    // Check each process
    const results = await Promise.all(
      mcps.map(async ([name, server]: [string, any]) => {
        const pattern = server.args?.[0] || server.command;
        try {
          const { stdout } = await execAsync(`pgrep -f "${pattern}"`);
          const pid = stdout.trim();
          return { name, alive: !!pid, pid: pid ? parseInt(pid) : undefined };
        } catch {
          // pgrep returns exit code 1 if no processes found
          return { name, alive: false, pid: undefined };
        }
      })
    );

    const alive = results.filter((r) => r.alive).length;
    const total = results.length;

    let result: HealthCheckResult;
    if (alive === total) {
      result = {
        name: 'MCP Processes',
        status: 'healthy',
        message: `${alive}/${total} processes alive`,
        details: { processes: results },
        last_check: new Date(),
      };
    } else if (alive > 0) {
      result = {
        name: 'MCP Processes',
        status: 'degraded',
        message: `${alive}/${total} processes alive`,
        details: { processes: results },
        last_check: new Date(),
      };
    } else {
      result = {
        name: 'MCP Processes',
        status: 'unhealthy',
        message: 'No MCP processes running',
        details: { processes: results },
        last_check: new Date(),
      };
    }

    setCached('MCP Processes', result);
    return result;
  } catch (error: any) {
    const result: HealthCheckResult = {
      name: 'MCP Processes',
      status: 'unhealthy',
      message: `Process check failed: ${error.message}`,
      details: { error: error.message },
      last_check: new Date(),
    };
    setCached('MCP Processes', result);
    return result;
  }
}

/**
 * Check 3: Network Connectivity
 *
 * Checks connectivity to OAuth and API endpoints using HTTP HEAD requests.
 * Endpoints are dynamic based on configured MCPs.
 *
 * Status: healthy if all reachable, degraded if 1-2 unreachable, unhealthy if 3+ unreachable.
 *
 * @param options - Health check options
 * @returns Network connectivity health check result
 */
async function checkNetworkConnectivity(options: HealthCheckOptions): Promise<HealthCheckResult> {
  const cached = getCached('Network Connectivity', options.force);
  if (cached) return cached;

  try {
    // Load config to determine endpoints
    const config = await loadConfig();
    const endpoints: string[] = [];

    // Always check Okta (company auth)
    if (config.company?.okta_domain) {
      endpoints.push(`https://${config.company.okta_domain}`);
    }

    // Check configured MCPs
    if (config.broker?.downstream_mcps?.googledocs) {
      endpoints.push('https://accounts.google.com');
    }
    if (config.broker?.downstream_mcps?.atlassian) {
      endpoints.push('https://api.atlassian.com');
    }

    if (endpoints.length === 0) {
      const result: HealthCheckResult = {
        name: 'Network Connectivity',
        status: 'degraded',
        message: 'No endpoints configured to check',
        details: { endpoints: [] },
        last_check: new Date(),
      };
      setCached('Network Connectivity', result);
      return result;
    }

    // Check each endpoint with 3s timeout
    const results = await Promise.all(
      endpoints.map(async (endpoint) => {
        const startTime = Date.now();
        try {
          const controller = new AbortController();
          const timeout = setTimeout(() => controller.abort(), 3000);

          const response = await fetch(endpoint, {
            method: 'HEAD',
            signal: controller.signal,
          });

          clearTimeout(timeout);
          const responseTime = Date.now() - startTime;

          return {
            endpoint,
            reachable: response.ok || response.status < 500, // Accept 4xx as reachable
            responseTime,
            status: response.status,
          };
        } catch (error: any) {
          return {
            endpoint,
            reachable: false,
            error: error.message,
          };
        }
      })
    );

    const reachable = results.filter((r) => r.reachable).length;
    const total = results.length;
    const unreachable = total - reachable;

    let result: HealthCheckResult;
    if (unreachable === 0) {
      result = {
        name: 'Network Connectivity',
        status: 'healthy',
        message: `All ${total} endpoints reachable`,
        details: { endpoints: results },
        last_check: new Date(),
      };
    } else if (unreachable <= 2) {
      result = {
        name: 'Network Connectivity',
        status: 'degraded',
        message: `${reachable}/${total} endpoints reachable`,
        details: { endpoints: results },
        last_check: new Date(),
      };
    } else {
      result = {
        name: 'Network Connectivity',
        status: 'unhealthy',
        message: `Only ${reachable}/${total} endpoints reachable`,
        details: { endpoints: results },
        last_check: new Date(),
      };
    }

    setCached('Network Connectivity', result);
    return result;
  } catch (error: any) {
    const result: HealthCheckResult = {
      name: 'Network Connectivity',
      status: 'unhealthy',
      message: `Network check failed: ${error.message}`,
      details: { error: error.message },
      last_check: new Date(),
    };
    setCached('Network Connectivity', result);
    return result;
  }
}

/**
 * Check 4: Intent Analyzer
 *
 * Tests intent analyzer accuracy using 5 sample intents covering all actions.
 * Calculates average confidence and counts mismatches.
 *
 * Status: healthy if avg confidence ≥70% and 0 mismatches,
 *         degraded if ≥50% or 1 mismatch,
 *         unhealthy if <50% or 2+ mismatches.
 *
 * @param options - Health check options
 * @returns Intent analyzer health check result
 */
async function checkIntentAnalyzer(options: HealthCheckOptions): Promise<HealthCheckResult> {
  const cached = getCached('Intent Analyzer', options.force);
  if (cached) return cached;

  try {
    const testCases = [
      { raw: 'Create Jira ticket for bug fix', expected: 'CREATE' },
      { raw: 'Show me recent Google Docs', expected: 'READ' },
      { raw: 'Update Confluence page with notes', expected: 'UPDATE' },
      { raw: 'Delete old Slack messages', expected: 'DELETE' },
      { raw: 'Search for auth docs in Glean', expected: 'SEARCH' },
    ];

    const results = testCases.map((tc) => {
      const envelope = analyzeIntent(tc.raw);
      return {
        intent: tc.raw,
        expected: tc.expected,
        parsed: envelope.action,
        confidence: envelope.confidence,
        match: envelope.action === tc.expected,
      };
    });

    const avgConfidence = results.reduce((sum, r) => sum + r.confidence, 0) / results.length;
    const mismatches = results.filter((r) => !r.match).length;

    let status: 'healthy' | 'degraded' | 'unhealthy';
    if (avgConfidence >= 0.7 && mismatches === 0) {
      status = 'healthy';
    } else if (avgConfidence >= 0.5 && mismatches <= 1) {
      status = 'degraded';
    } else {
      status = 'unhealthy';
    }

    const result: HealthCheckResult = {
      name: 'Intent Analyzer',
      status,
      message: `Confidence ${Math.round(avgConfidence * 100)}%, ${mismatches} mismatches`,
      details: { avgConfidence, mismatches, tests: results },
      last_check: new Date(),
    };

    setCached('Intent Analyzer', result);
    return result;
  } catch (error: any) {
    const result: HealthCheckResult = {
      name: 'Intent Analyzer',
      status: 'unhealthy',
      message: `Intent analyzer check failed: ${error.message}`,
      details: { error: error.message },
      last_check: new Date(),
    };
    setCached('Intent Analyzer', result);
    return result;
  }
}

/**
 * Check 5: Global MCP Server Health
 *
 * Checks if global MCP HTTP server is enabled and healthy.
 * Makes HTTP GET request to health endpoint with 5s timeout.
 *
 * Status: healthy if enabled and responding, degraded if disabled, unhealthy if unreachable.
 *
 * @param options - Health check options
 * @returns Global MCP health check result
 */
export async function checkGlobalMCPHealth(options: HealthCheckOptions): Promise<HealthCheckResult> {
  const cached = getCached('Global MCP Discovery', options.force);
  if (cached) return cached;

  try {
    const config = await loadConfig();

    // If global MCPs not configured or disabled
    if (!config.globalMcps?.enabled) {
      const result: HealthCheckResult = {
        name: 'Global MCP Discovery',
        status: 'healthy',
        message: 'Global MCPs not enabled',
        details: { enabled: false },
        last_check: new Date(),
      };
      setCached('Global MCP Discovery', result);
      return result;
    }

    const healthUrl = config.globalMcps.healthCheckUrl || 'http://localhost:8001/health';

    // Make HTTP health check with 5s timeout
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 5000);

    try {
      const response = await fetch(healthUrl, {
        method: 'GET',
        signal: controller.signal,
      });

      clearTimeout(timeout);

      if (response.ok) {
        const data = await response.json();
        const result: HealthCheckResult = {
          name: 'Global MCP Discovery',
          status: 'healthy',
          message: `HTTP server healthy (uptime: ${data.uptime || 'unknown'}s)`,
          details: {
            enabled: true,
            healthUrl,
            uptime: data.uptime,
            sessionCount: data.sessionCount || 0,
            responseData: data,
          },
          last_check: new Date(),
        };
        setCached('Global MCP Discovery', result);
        return result;
      } else {
        const result: HealthCheckResult = {
          name: 'Global MCP Discovery',
          status: 'unhealthy',
          message: `HTTP ${response.status} ${response.statusText}`,
          details: {
            enabled: true,
            healthUrl,
            status: response.status,
            statusText: response.statusText,
          },
          last_check: new Date(),
        };
        setCached('Global MCP Discovery', result);
        return result;
      }
    } catch (error: any) {
      clearTimeout(timeout);

      const result: HealthCheckResult = {
        name: 'Global MCP Discovery',
        status: 'unhealthy',
        message: `Health check failed: ${error.message}`,
        details: {
          enabled: true,
          healthUrl,
          error: error.message,
        },
        last_check: new Date(),
      };
      setCached('Global MCP Discovery', result);
      return result;
    }
  } catch (error: any) {
    const result: HealthCheckResult = {
      name: 'Global MCP Discovery',
      status: 'unhealthy',
      message: `Config error: ${error.message}`,
      details: { error: error.message },
      last_check: new Date(),
    };
    setCached('Global MCP Discovery', result);
    return result;
  }
}

/**
 * Load configuration from ~/.config/mcp-wizard/config.json
 *
 * @returns Parsed config object
 * @throws Error if config file not found or invalid
 */
async function loadConfig(): Promise<any> {
  const configPath = path.join(os.homedir(), '.config/mcp-wizard/config.json');
  const configContent = await fs.readFile(configPath, 'utf-8');
  return JSON.parse(configContent);
}

/**
 * Validate configuration (for doctor command)
 *
 * Checks config schema, required fields, and MCP server configurations.
 *
 * @returns Configuration validation health check result
 */
export async function validateConfiguration(): Promise<HealthCheckResult> {
  try {
    const config = await loadConfig();

    // Check required fields
    const errors: string[] = [];

    if (!config.company?.name) {
      errors.push('Missing company.name');
    }
    if (!config.company?.okta_domain) {
      errors.push('Missing company.okta_domain');
    }
    if (!config.broker?.downstream_mcps || Object.keys(config.broker.downstream_mcps).length === 0) {
      errors.push('Missing broker.downstream_mcps or no MCPs configured');
    }

    // Validate MCP server configs
    if (config.broker?.downstream_mcps) {
      Object.entries(config.broker.downstream_mcps).forEach(([name, server]: [string, any]) => {
        if (!server.command) {
          errors.push(`MCP ${name}: missing command`);
        }
        if (!Array.isArray(server.args)) {
          errors.push(`MCP ${name}: missing or invalid args`);
        }
      });
    }

    if (errors.length > 0) {
      return {
        name: 'Configuration',
        status: 'unhealthy',
        message: `Config validation failed: ${errors[0]} (${errors.length} total)`,
        details: { errors, path: path.join(os.homedir(), '.config/mcp-wizard/config.json') },
        last_check: new Date(),
      };
    }

    return {
      name: 'Configuration',
      status: 'healthy',
      message: 'Config file valid',
      details: {
        path: path.join(os.homedir(), '.config/mcp-wizard/config.json'),
        company: config.company?.name,
        okta_domain: config.company?.okta_domain,
        mcp_servers: Object.keys(config.broker?.downstream_mcps || {}),
      },
      last_check: new Date(),
    };
  } catch (error: any) {
    return {
      name: 'Configuration',
      status: 'unhealthy',
      message: `Config file error: ${error.message}`,
      details: { error: error.message, path: path.join(os.homedir(), '.config/mcp-wizard/config.json') },
      last_check: new Date(),
    };
  }
}
