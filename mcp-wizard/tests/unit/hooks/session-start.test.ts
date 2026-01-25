/**
 * Unit Tests for SessionStart Hook
 *
 * Tests session initialization hook functionality
 *
 * Requirements:
 * - Execute at session initialization
 * - Run health check at session start
 * - Handle health check failures gracefully
 * - Not block session start on errors
 *
 * Coverage Target: 90%+ statement coverage
 */

import { onSessionStart, registerSessionStartHook } from '../../../src/hooks/session-start';
import * as healthModule from '../../../src/commands/health';
import { __clearMockStore as __clearKeytarMocks } from '../../__mocks__/keytar';
import { __clearProcessMocks, __setupProcessRunning } from '../../__mocks__/child_process';
import { __clearNetworkMocks, __setupNetworkSuccess } from '../../__mocks__/https';

// Mock the health module
jest.mock('../../../src/commands/health');

describe('SessionStart Hook', () => {
  let consoleLogSpy: jest.SpyInstance;
  let consoleErrorSpy: jest.SpyInstance;

  beforeEach(() => {
    // Clear all mocks before each test
    __clearKeytarMocks();
    __clearProcessMocks();
    __clearNetworkMocks();
    jest.clearAllMocks();

    // Spy on console methods
    consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
    consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();
  });

  afterEach(() => {
    // Restore console methods
    consoleLogSpy.mockRestore();
    consoleErrorSpy.mockRestore();
  });

  describe('onSessionStart()', () => {
    it('should execute at session initialization', async () => {
      // Setup
      const mockCheckHealth = jest.fn().mockResolvedValue({
        overall: 'healthy',
        checks: {
          token: { status: 'healthy' },
          mcp: { status: 'healthy' },
          network: { status: 'healthy' },
          intentAnalyzer: { status: 'healthy' },
        },
      });
      (healthModule.checkHealth as jest.Mock) = mockCheckHealth;

      // Execute
      await onSessionStart();

      // Assert
      expect(mockCheckHealth).toHaveBeenCalled();
    });

    it('should run health check', async () => {
      // Setup
      const mockCheckHealth = jest.fn().mockResolvedValue({
        overall: 'healthy',
        checks: {
          token: { status: 'healthy' },
          mcp: { status: 'healthy' },
          network: { status: 'healthy' },
          intentAnalyzer: { status: 'healthy' },
        },
      });
      (healthModule.checkHealth as jest.Mock) = mockCheckHealth;

      // Execute
      await onSessionStart();

      // Assert
      expect(mockCheckHealth).toHaveBeenCalledTimes(1);
    });

    it('should handle health check failures gracefully', async () => {
      // Setup
      const mockCheckHealth = jest.fn().mockRejectedValue(new Error('Health check failed'));
      (healthModule.checkHealth as jest.Mock) = mockCheckHealth;

      // Execute
      await expect(onSessionStart()).resolves.not.toThrow();

      // Assert: Should log error but not throw
      expect(consoleErrorSpy).toHaveBeenCalled();
    });

    it('should not block session start on errors', async () => {
      // Setup
      const mockCheckHealth = jest.fn().mockRejectedValue(new Error('Critical failure'));
      (healthModule.checkHealth as jest.Mock) = mockCheckHealth;

      // Execute - should complete without throwing
      const startTime = Date.now();
      await onSessionStart();
      const duration = Date.now() - startTime;

      // Assert: Should complete quickly (not hang)
      expect(duration).toBeLessThan(1000);
      expect(consoleErrorSpy).toHaveBeenCalled();
    });

    it('should log warning when health status is not healthy', async () => {
      // Setup
      const mockCheckHealth = jest.fn().mockResolvedValue({
        overall: 'warning',
        checks: {
          token: { status: 'healthy' },
          mcp: { status: 'warning' },
          network: { status: 'healthy' },
          intentAnalyzer: { status: 'healthy' },
        },
      });
      (healthModule.checkHealth as jest.Mock) = mockCheckHealth;

      // Execute
      await onSessionStart();

      // Assert
      expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('warning'));
    });

    it('should log critical status', async () => {
      // Setup
      const mockCheckHealth = jest.fn().mockResolvedValue({
        overall: 'critical',
        checks: {
          token: { status: 'critical' },
          mcp: { status: 'healthy' },
          network: { status: 'healthy' },
          intentAnalyzer: { status: 'healthy' },
        },
      });
      (healthModule.checkHealth as jest.Mock) = mockCheckHealth;

      // Execute
      await onSessionStart();

      // Assert
      expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('critical'));
    });

    it('should not log when status is healthy', async () => {
      // Setup
      const mockCheckHealth = jest.fn().mockResolvedValue({
        overall: 'healthy',
        checks: {
          token: { status: 'healthy' },
          mcp: { status: 'healthy' },
          network: { status: 'healthy' },
          intentAnalyzer: { status: 'healthy' },
        },
      });
      (healthModule.checkHealth as jest.Mock) = mockCheckHealth;

      // Execute
      await onSessionStart();

      // Assert: Should not log when healthy
      expect(consoleLogSpy).not.toHaveBeenCalled();
    });
  });

  describe('registerSessionStartHook()', () => {
    it('should register hook without errors', () => {
      // Execute
      expect(() => registerSessionStartHook()).not.toThrow();
    });

    it('should be callable multiple times', () => {
      // Execute
      expect(() => {
        registerSessionStartHook();
        registerSessionStartHook();
        registerSessionStartHook();
      }).not.toThrow();
    });
  });
});
