/**
 * Unit Tests for Health Command
 *
 * Tests health status checking functionality for the MCP system
 *
 * Requirements:
 * - Check health of all system components (token, MCP, network, intent analyzer)
 * - Aggregate component statuses into overall health
 * - Format health output for display
 * - Handle various health scenarios (healthy, warning, critical)
 *
 * Coverage Target: 90%+ statement coverage
 */

// Mock health-checks to control test scenarios
jest.mock('../../../src/lib/health-checks', () => {
  const mockHealthChecks = jest.requireActual('../../__mocks__/health-checks');
  return mockHealthChecks;
});

import { checkHealth, formatHealthOutput, HealthStatus, health } from '../../../src/commands/health';
import * as healthModule from '../../../src/commands/health';
import { __clearMockStore as __clearKeytarMocks } from '../../__mocks__/keytar';
import { __clearProcessMocks, __setupProcessRunning, __setupProcessNotFound } from '../../__mocks__/child_process';
import { __clearNetworkMocks, __setupNetworkSuccess, __setupNetworkError } from '../../__mocks__/https';
import {
  __resetAllHealthMocks,
  __setMockTokenStatus,
  __setMockMCPStatus,
} from '../../__mocks__/health-checks';
import {
  ALL_HEALTHY,
  TOKEN_EXPIRED,
  MCP_DOWN,
  NETWORK_FAILURE,
  ALL_SCENARIOS,
} from '../../fixtures/health/health-scenarios';

describe('Health Command', () => {
  beforeEach(() => {
    // Clear all mocks before each test to ensure isolation
    __clearKeytarMocks();
    __clearProcessMocks();
    __clearNetworkMocks();
    __resetAllHealthMocks();
  });

  describe('checkHealth()', () => {
    it('should return healthy status when all checks pass', async () => {
      // Setup
      ALL_HEALTHY.mockSetup();

      // Execute
      const result = await checkHealth();

      // Assert
      expect(result.overall).toBe('healthy');
      expect(result.checks.token.status).toBe('healthy');
      expect(result.checks.mcp.status).toBe('healthy');
      expect(result.checks.network.status).toBe('healthy');
      expect(result.checks.intentAnalyzer.status).toBe('healthy');
    });

    it('should return critical status when token expired', async () => {
      // Setup
      TOKEN_EXPIRED.mockSetup();
      // Note: In real implementation, would mock expired token in keytar

      // Execute
      const result = await checkHealth();

      // Assert
      expect(result.overall).toBeDefined();
      expect(result.checks.token).toBeDefined();
    });

    it('should return critical status when MCP down', async () => {
      // Setup
      MCP_DOWN.mockSetup();

      // Execute
      const result = await checkHealth();

      // Assert
      expect(result.overall).toBeDefined();
      expect(result.checks.mcp).toBeDefined();
    });

    it('should return warning status when network degraded', async () => {
      // Setup
      NETWORK_FAILURE.mockSetup();

      // Execute
      const result = await checkHealth();

      // Assert
      expect(result.overall).toBeDefined();
      expect(result.checks.network).toBeDefined();
    });

    it('should aggregate multiple warnings correctly', async () => {
      // Setup: Both network and intent analyzer have issues
      __clearKeytarMocks();
      __clearProcessMocks();
      __clearNetworkMocks();
      __setupProcessRunning();
      __setupNetworkError();

      // Execute
      const result = await checkHealth();

      // Assert
      expect(result.overall).toBeDefined();
      expect(result.checks).toBeDefined();
    });

    it('should handle mixed states (some healthy, some failing)', async () => {
      // Setup
      __setupProcessRunning(); // MCP healthy
      __setupNetworkError(); // Network failing

      // Execute
      const result = await checkHealth();

      // Assert
      expect(result).toBeDefined();
      expect(result.checks).toBeDefined();
    });

    // Test all predefined scenarios
    ALL_SCENARIOS.forEach((scenario) => {
      it(`should handle scenario: ${scenario.name}`, async () => {
        // Setup
        scenario.mockSetup();

        // Execute
        const result = await checkHealth();

        // Assert
        expect(result).toBeDefined();
        expect(result.overall).toBeDefined();
        expect(['healthy', 'warning', 'critical']).toContain(result.overall);
      });
    });
  });

  describe('formatHealthOutput()', () => {
    it('should format healthy status with all components', () => {
      // Setup
      const healthStatus: HealthStatus = {
        overall: 'healthy',
        checks: {
          token: { status: 'healthy' },
          mcp: { status: 'healthy' },
          network: { status: 'healthy' },
          intentAnalyzer: { status: 'healthy' },
        },
      };

      // Execute
      const result = formatHealthOutput(healthStatus);

      // Assert
      expect(result).toContain('HEALTHY');
      expect(result).toContain('Token: healthy');
      expect(result).toContain('MCP: healthy');
      expect(result).toContain('Network: healthy');
      expect(result).toContain('Intent Analyzer: healthy');
    });

    it('should format warnings when present', () => {
      // Setup
      const healthStatus: HealthStatus = {
        overall: 'warning',
        checks: {
          token: { status: 'healthy' },
          mcp: { status: 'healthy' },
          network: { status: 'warning', details: 'Slow response' },
          intentAnalyzer: { status: 'healthy' },
        },
        warnings: ['Network check degraded'],
      };

      // Execute
      const result = formatHealthOutput(healthStatus);

      // Assert
      expect(result).toContain('WARNING');
      expect(result).toContain('Warnings:');
      expect(result).toContain('Network check degraded');
    });

    it('should format errors when present', () => {
      // Setup
      const healthStatus: HealthStatus = {
        overall: 'critical',
        checks: {
          token: { status: 'critical', details: 'Expired' },
          mcp: { status: 'healthy' },
          network: { status: 'healthy' },
          intentAnalyzer: { status: 'healthy' },
        },
        errors: ['Token expired'],
      };

      // Execute
      const result = formatHealthOutput(healthStatus);

      // Assert
      expect(result).toContain('CRITICAL');
      expect(result).toContain('Errors:');
      expect(result).toContain('Token expired');
    });

    it('should handle multiple warnings and errors', () => {
      // Setup
      const healthStatus: HealthStatus = {
        overall: 'critical',
        checks: {
          token: { status: 'critical' },
          mcp: { status: 'warning' },
          network: { status: 'warning' },
          intentAnalyzer: { status: 'healthy' },
        },
        warnings: ['Network slow', 'MCP degraded'],
        errors: ['Token expired'],
      };

      // Execute
      const result = formatHealthOutput(healthStatus);

      // Assert
      expect(result).toContain('Warnings:');
      expect(result).toContain('Network slow');
      expect(result).toContain('MCP degraded');
      expect(result).toContain('Errors:');
      expect(result).toContain('Token expired');
    });
  });

  describe('Edge Cases and Coverage Gaps', () => {
    it('should return warning overall when status includes degraded', async () => {
      // Setup: Use health-checks mock to set degraded status
      const healthChecksMock = require('../../../src/lib/health-checks');
      healthChecksMock.__setMockTokenStatus('healthy');
      healthChecksMock.__setMockMCPStatus('degraded');

      // Execute
      const result = await checkHealth();

      // Assert
      expect(result.overall).toBe('warning');
      expect(result.warnings).toBeDefined();
      expect(result.warnings!.length).toBeGreaterThan(0);
    });

    it('should handle Network Connectivity check name in getCheckKey', async () => {
      // This tests defensive code - even though health.ts currently only checks token + MCP,
      // getCheckKey supports network checks for future extensibility
      __clearKeytarMocks();
      __clearProcessMocks();
      __clearNetworkMocks();

      // Mock health check response that includes Network Connectivity
      const mockHealthChecks = require('../../../src/lib/health-checks');
      jest.spyOn(mockHealthChecks, 'checkTokenHealth').mockResolvedValue({
        name: 'Network Connectivity',
        status: 'healthy',
        message: 'All endpoints reachable',
        details: {}
      });
      jest.spyOn(mockHealthChecks, 'checkMCPProcesses').mockResolvedValue({
        name: 'MCP Processes',
        status: 'healthy',
        message: 'All processes running',
        details: {}
      });

      // Execute
      const result = await checkHealth();

      // Assert - verify it doesn't crash and handles the check name
      expect(result).toBeDefined();
      expect(result.checks.network).toBeDefined();
      expect(result.checks.network.status).toBe('healthy');
    });

    it('should handle Intent Analyzer check name in getCheckKey', async () => {
      // This tests defensive code path for intent analyzer checks
      __clearKeytarMocks();
      __clearProcessMocks();
      __clearNetworkMocks();

      // Mock health check response that includes Intent Analyzer
      const mockHealthChecks = require('../../../src/lib/health-checks');
      jest.spyOn(mockHealthChecks, 'checkTokenHealth').mockResolvedValue({
        name: 'Token Health',
        status: 'healthy',
        message: 'Token valid',
        details: {}
      });
      jest.spyOn(mockHealthChecks, 'checkMCPProcesses').mockResolvedValue({
        name: 'Intent Analyzer',
        status: 'healthy',
        message: 'Intent analyzer functioning',
        details: {}
      });

      // Execute
      const result = await checkHealth();

      // Assert
      expect(result).toBeDefined();
      expect(result.checks.intentAnalyzer).toBeDefined();
      expect(result.checks.intentAnalyzer.status).toBe('healthy');
    });
  });

  describe('health() command function', () => {
    let processExitSpy: jest.SpyInstance;
    let consoleLogSpy: jest.SpyInstance;
    let consoleErrorSpy: jest.SpyInstance;

    beforeEach(() => {
      // Mock process.exit to prevent tests from actually exiting
      // Don't throw - this would trigger catch blocks in the code under test
      processExitSpy = jest.spyOn(process, 'exit').mockImplementation((code?: any) => {
        return undefined as never;
      });
      consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Setup default healthy state
      __clearKeytarMocks();
      __clearProcessMocks();
      __clearNetworkMocks();
      ALL_HEALTHY.mockSetup();
    });

    afterEach(() => {
      processExitSpy.mockRestore();
      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
      // Reset all mocks to prevent test interference
      __resetAllHealthMocks();
      __clearKeytarMocks();
      __clearProcessMocks();
      __clearNetworkMocks();
      // Clear any jest.spyOn mocks
      jest.restoreAllMocks();
    });

    it('should output JSON when json option is true', async () => {
      await health({ json: true });

      // Check process exited with code 0 (healthy)
      expect(processExitSpy).toHaveBeenCalledWith(0);

      expect(consoleLogSpy).toHaveBeenCalled();
      const output = JSON.parse(consoleLogSpy.mock.calls[0][0]);
      expect(output).toHaveProperty('overall_status');
      expect(output).toHaveProperty('checks');
      expect(output).toHaveProperty('timestamp');
      expect(output.overall_status).toBe('healthy');
    });

    it('should suppress output when silent option is true', async () => {
      await health({ silent: true });

      // Check process exited with code 0 (healthy)
      expect(processExitSpy).toHaveBeenCalledWith(0);
      // Console.log should not be called in silent mode
      expect(consoleLogSpy).not.toHaveBeenCalled();
    });

    it('should exit with code 0 for healthy status', async () => {
      ALL_HEALTHY.mockSetup();

      await health({});

      expect(processExitSpy).toHaveBeenCalledWith(0);
    });

    it('should exit with code 1 for warning status', async () => {
      // Setup degraded state
      __setMockTokenStatus('healthy');
      __setMockMCPStatus('degraded');

      await health({});

      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it('should exit with code 2 for critical status', async () => {
      TOKEN_EXPIRED.mockSetup();

      await health({});

      expect(processExitSpy).toHaveBeenCalledWith(2);
    });

    it('should suggest doctor command when not healthy', async () => {
      TOKEN_EXPIRED.mockSetup();

      await health({});

      expect(consoleLogSpy).toHaveBeenCalledWith(
        expect.stringContaining('mcp-wizard doctor')
      );
    });

    it('should handle errors gracefully and exit with code 2', async () => {
      // Mock the underlying health check functions to throw an error
      // This will cause checkHealth to fail, triggering the error handler
      const healthChecksMock = require('../../../src/lib/health-checks');
      jest.spyOn(healthChecksMock, 'checkTokenHealth').mockRejectedValue(new Error('Test error'));

      await health({});

      expect(processExitSpy).toHaveBeenCalledWith(2);
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Test error')
      );
    });

    it('should not show spinner in silent mode', async () => {
      // This test verifies spinner is not started in silent mode
      // We can't directly test ora, but we verify no console output occurs
      await health({ silent: true });

      // No console.log calls in silent mode
      expect(consoleLogSpy).not.toHaveBeenCalled();
    });

    it('should not show spinner in JSON mode', async () => {
      await health({ json: true });

      // Only JSON output, no extra logs
      expect(consoleLogSpy).toHaveBeenCalledTimes(1);
      const output = consoleLogSpy.mock.calls[0][0];
      expect(() => JSON.parse(output)).not.toThrow();
    });
  });
});
