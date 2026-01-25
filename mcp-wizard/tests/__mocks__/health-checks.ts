/**
 * Mock for health-checks module
 *
 * Provides controllable mock implementations for health check functions
 */

import { HealthCheckResult } from '../../src/lib/health-checks';

// Mock state
let mockTokenStatus: 'healthy' | 'degraded' | 'unhealthy' = 'healthy';
let mockMCPStatus: 'healthy' | 'degraded' | 'unhealthy' = 'healthy';
let mockNetworkStatus: 'healthy' | 'degraded' | 'unhealthy' = 'healthy';
let mockIntentAnalyzerStatus: 'healthy' | 'degraded' | 'unhealthy' = 'healthy';

/**
 * Mock implementations
 */
export async function checkTokenHealth(options?: { force?: boolean }): Promise<HealthCheckResult> {
  return {
    name: 'Token Health',
    status: mockTokenStatus,
    message: mockTokenStatus === 'healthy' ? 'Token valid' : 'Token expired',
    last_check: new Date(),
  };
}

export async function checkMCPProcesses(options?: { force?: boolean }): Promise<HealthCheckResult> {
  return {
    name: 'MCP Processes',
    status: mockMCPStatus,
    message: mockMCPStatus === 'healthy' ? 'All MCPs running' : 'MCP process not running',
    last_check: new Date(),
  };
}

export async function checkNetworkConnectivity(options?: { force?: boolean }): Promise<HealthCheckResult> {
  return {
    name: 'Network Connectivity',
    status: mockNetworkStatus,
    message: mockNetworkStatus === 'healthy' ? 'Network OK' : 'Network check failed',
    last_check: new Date(),
  };
}

export async function checkIntentAnalyzer(options?: { force?: boolean }): Promise<HealthCheckResult> {
  return {
    name: 'Intent Analyzer',
    status: mockIntentAnalyzerStatus,
    message: mockIntentAnalyzerStatus === 'healthy' ? 'Intent analyzer OK' : 'Intent analyzer degraded',
    last_check: new Date(),
  };
}

export async function runAllHealthChecks(options?: { force?: boolean }): Promise<HealthCheckResult[]> {
  return Promise.all([
    checkTokenHealth(options),
    checkMCPProcesses(options),
    checkNetworkConnectivity(options),
    checkIntentAnalyzer(options),
  ]);
}

/**
 * Mock control functions (for test setup)
 */
export function __setMockTokenStatus(status: 'healthy' | 'degraded' | 'unhealthy') {
  mockTokenStatus = status;
}

export function __setMockMCPStatus(status: 'healthy' | 'degraded' | 'unhealthy') {
  mockMCPStatus = status;
}

export function __setMockNetworkStatus(status: 'healthy' | 'degraded' | 'unhealthy') {
  mockNetworkStatus = status;
}

export function __setMockIntentAnalyzerStatus(status: 'healthy' | 'degraded' | 'unhealthy') {
  mockIntentAnalyzerStatus = status;
}

export async function validateConfiguration(): Promise<HealthCheckResult> {
  return {
    name: 'Configuration',
    status: 'healthy',
    message: 'Config file valid',
    last_check: new Date(),
  };
}

export function __resetAllHealthMocks() {
  mockTokenStatus = 'healthy';
  mockMCPStatus = 'healthy';
  mockNetworkStatus = 'healthy';
  mockIntentAnalyzerStatus = 'healthy';
}
