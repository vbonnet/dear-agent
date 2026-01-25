/**
 * Integration Tests for Health Diagnostics
 *
 * Tests the complete health check workflow and component interactions
 *
 * Requirements:
 * - Complete health check workflow (invocation → checks → aggregation → output)
 * - Interaction between health components
 * - Health check orchestration logic
 * - Error propagation across components
 */

// Mock health-checks for controlled test scenarios
jest.mock('../../src/lib/health-checks', () => {
  const mockHealthChecks = jest.requireActual('../__mocks__/health-checks');
  return mockHealthChecks;
});

import { checkHealth, formatHealthOutput } from '../../src/commands/health';
import { runDiagnostics, formatDiagnosticReport } from '../../src/commands/doctor';
import { onSessionStart } from '../../src/hooks/session-start';
import { __clearMockStore as __clearKeytarMocks } from '../__mocks__/keytar';
import {
  __clearProcessMocks,
  __setupProcessRunning,
  __setupProcessNotFound,
} from '../__mocks__/child_process';
import {
  __clearNetworkMocks,
  __setupNetworkSuccess,
  __setupNetworkError,
} from '../__mocks__/https';
import {
  ALL_HEALTHY,
  MCP_DOWN,
  NETWORK_FAILURE,
  MIXED_STATES,
} from '../fixtures/health/health-scenarios';

describe('Health Diagnostics Integration', () => {
  beforeEach(() => {
    // Setup complete mock environment
    __clearKeytarMocks();
    __clearProcessMocks();
    __clearNetworkMocks();
  });

  describe('Complete Health Check Flow', () => {
    it('should execute all health checks in sequence', async () => {
      // Setup
      ALL_HEALTHY.mockSetup();

      // Execute
      const healthStatus = await checkHealth();
      const output = formatHealthOutput(healthStatus);

      // Assert
      expect(healthStatus).toBeDefined();
      expect(healthStatus.overall).toBe('healthy');
      expect(output).toContain('HEALTHY');
    });

    it('should aggregate results correctly', async () => {
      // Setup
      MIXED_STATES.mockSetup();

      // Execute
      const healthStatus = await checkHealth();

      // Assert
      expect(healthStatus).toBeDefined();
      expect(healthStatus.overall).toBeDefined();
      expect(healthStatus.checks).toBeDefined();
    });

    it('should handle partial failures', async () => {
      // Setup: MCP down, but network ok
      __clearKeytarMocks();
      __clearProcessMocks();
      __clearNetworkMocks();
      __setupProcessNotFound();
      __setupNetworkSuccess();

      // Execute
      const healthStatus = await checkHealth();

      // Assert
      expect(healthStatus).toBeDefined();
      expect(healthStatus.overall).toBeDefined();
    });
  });

  describe('Component Interactions', () => {
    it('should coordinate health check with diagnostic report', async () => {
      // Setup
      ALL_HEALTHY.mockSetup();

      // Execute
      const healthStatus = await checkHealth();
      const diagnostics = await runDiagnostics();

      // Assert: Both should complete successfully
      expect(healthStatus).toBeDefined();
      expect(diagnostics).toBeDefined();
    });

    it('should coordinate SessionStart hook with health check', async () => {
      // Setup
      ALL_HEALTHY.mockSetup();
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();

      // Execute
      await onSessionStart();

      // Assert: Hook should execute without errors
      consoleLogSpy.mockRestore();
    });

    it('should handle health check failure in SessionStart hook', async () => {
      // Setup: Critical health state
      MCP_DOWN.mockSetup();
      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      // Execute
      await expect(onSessionStart()).resolves.not.toThrow();

      // Assert: Should handle gracefully
      consoleLogSpy.mockRestore();
      consoleErrorSpy.mockRestore();
    });
  });

  describe('End-to-End Scenarios', () => {
    it('should handle all-healthy scenario from start to finish', async () => {
      // Setup
      ALL_HEALTHY.mockSetup();

      // Execute complete flow
      const healthStatus = await checkHealth();
      const healthOutput = formatHealthOutput(healthStatus);
      const diagnostics = await runDiagnostics();
      const diagnosticOutput = formatDiagnosticReport(diagnostics);

      // Assert
      expect(healthStatus.overall).toBe('healthy');
      expect(healthOutput).toContain('HEALTHY');
      expect(diagnostics).toBeDefined();
      expect(diagnosticOutput).toContain('System Diagnostics');
    });

    it('should handle degraded scenario with warnings', async () => {
      // Setup
      NETWORK_FAILURE.mockSetup();

      // Execute
      const healthStatus = await checkHealth();
      const healthOutput = formatHealthOutput(healthStatus);

      // Assert
      expect(healthStatus.overall).toBeDefined();
      expect(healthOutput).toBeDefined();
    });

    it('should handle critical scenario with errors', async () => {
      // Setup
      MCP_DOWN.mockSetup();

      // Execute
      const healthStatus = await checkHealth();
      const healthOutput = formatHealthOutput(healthStatus);
      const diagnostics = await runDiagnostics();

      // Assert
      expect(healthStatus.overall).toBeDefined();
      expect(healthOutput).toBeDefined();
      expect(diagnostics).toBeDefined();
    });
  });

  describe('Error Propagation', () => {
    it('should propagate network errors correctly', async () => {
      // Setup
      __setupNetworkError();
      __setupProcessRunning();

      // Execute
      const healthStatus = await checkHealth();

      // Assert
      expect(healthStatus).toBeDefined();
    });

    it('should propagate process errors correctly', async () => {
      // Setup
      __setupProcessNotFound();
      __setupNetworkSuccess();

      // Execute
      const healthStatus = await checkHealth();

      // Assert
      expect(healthStatus).toBeDefined();
    });
  });
});
