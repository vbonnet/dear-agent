/**
 * Test Scenario Fixtures for Health Check Tests
 *
 * Represents typical health check scenarios for comprehensive testing
 */

import { __clearMockStore as __clearKeytarMocks } from '../../__mocks__/keytar';
import {
  __clearProcessMocks,
  __setupProcessRunning,
  __setupProcessNotFound,
  __setupProcessError,
} from '../../__mocks__/child_process';
import {
  __clearNetworkMocks,
  __setupNetworkSuccess,
  __setupNetworkTimeout,
  __setupNetworkError,
} from '../../__mocks__/https';
import {
  __resetAllHealthMocks,
  __setMockTokenStatus,
  __setMockMCPStatus,
  __setMockNetworkStatus,
  __setMockIntentAnalyzerStatus,
} from '../../__mocks__/health-checks';

/**
 * Health check scenario definition
 */
export interface HealthCheckScenario {
  name: string;
  description: string;
  mockSetup: () => void;
  expectedStatus: 'healthy' | 'warning' | 'critical';
  expectedWarnings?: string[];
  expectedErrors?: string[];
}

/**
 * Scenario 1: All Healthy
 * All health checks pass, system is fully operational
 */
export const ALL_HEALTHY: HealthCheckScenario = {
  name: 'All Healthy',
  description: 'All health checks pass, system fully operational',
  mockSetup: () => {
    __clearKeytarMocks();
    __clearProcessMocks();
    __clearNetworkMocks();
    __resetAllHealthMocks();
    __setupProcessRunning();
    __setupNetworkSuccess();
  },
  expectedStatus: 'healthy',
  expectedWarnings: [],
  expectedErrors: [],
};

/**
 * Scenario 2: Token Expired
 * Authentication token has expired, needs re-authentication
 */
export const TOKEN_EXPIRED: HealthCheckScenario = {
  name: 'Token Expired',
  description: 'Authentication token expired, re-authentication required',
  mockSetup: () => {
    __clearKeytarMocks();
    __clearProcessMocks();
    __clearNetworkMocks();
    __resetAllHealthMocks();
    __setMockTokenStatus('unhealthy');
    __setupProcessRunning();
    __setupNetworkSuccess();
  },
  expectedStatus: 'critical',
  expectedErrors: ['Token expired'],
};

/**
 * Scenario 3: MCP Down
 * MCP process is not running or not responding
 */
export const MCP_DOWN: HealthCheckScenario = {
  name: 'MCP Down',
  description: 'MCP process not running or not responding',
  mockSetup: () => {
    __clearKeytarMocks();
    __clearProcessMocks();
    __clearNetworkMocks();
    __resetAllHealthMocks();
    __setMockMCPStatus('unhealthy');
    __setupProcessNotFound(); // MCP process not found
    __setupNetworkSuccess();
  },
  expectedStatus: 'critical',
  expectedErrors: ['MCP process not running'],
};

/**
 * Scenario 4: Network Failure
 * Network connectivity issues preventing health checks
 */
export const NETWORK_FAILURE: HealthCheckScenario = {
  name: 'Network Failure',
  description: 'Network connectivity issues',
  mockSetup: () => {
    __clearKeytarMocks();
    __clearProcessMocks();
    __clearNetworkMocks();
    __resetAllHealthMocks();
    __setMockNetworkStatus('degraded');
    __setupProcessRunning();
    __setupNetworkError(); // Network request fails
  },
  expectedStatus: 'warning',
  expectedWarnings: ['Network check failed'],
};

/**
 * Scenario 5: Intent Analyzer Degraded
 * Intent analyzer service is experiencing performance issues
 */
export const INTENT_ANALYZER_DEGRADED: HealthCheckScenario = {
  name: 'Intent Analyzer Degraded',
  description: 'Intent analyzer experiencing performance degradation',
  mockSetup: () => {
    __clearKeytarMocks();
    __clearProcessMocks();
    __clearNetworkMocks();
    __resetAllHealthMocks();
    __setMockIntentAnalyzerStatus('degraded');
    __setupProcessRunning();
    __setupNetworkTimeout(); // Intent analyzer times out
  },
  expectedStatus: 'warning',
  expectedWarnings: ['Intent analyzer slow or degraded'],
};

/**
 * Scenario 6: Mixed States
 * Some health checks pass, others fail (combination scenario)
 */
export const MIXED_STATES: HealthCheckScenario = {
  name: 'Mixed States',
  description: 'Some checks healthy, some failing',
  mockSetup: () => {
    __clearKeytarMocks();
    __clearProcessMocks();
    __clearNetworkMocks();
    __resetAllHealthMocks();
    __setMockNetworkStatus('degraded');
    __setupProcessRunning(); // MCP running (healthy)
    __setupNetworkError(); // Network failing (warning)
  },
  expectedStatus: 'warning',
  expectedWarnings: ['Network check failed'],
};

/**
 * All scenarios for iteration in tests
 */
export const ALL_SCENARIOS: HealthCheckScenario[] = [
  ALL_HEALTHY,
  TOKEN_EXPIRED,
  MCP_DOWN,
  NETWORK_FAILURE,
  INTENT_ANALYZER_DEGRADED,
  MIXED_STATES,
];
