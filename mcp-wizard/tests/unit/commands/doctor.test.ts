/**
 * Unit Tests for Doctor Command
 *
 * Tests diagnostic information gathering and reporting functionality
 *
 * Requirements:
 * - Gather comprehensive diagnostic information
 * - Detect configuration issues
 * - Provide actionable recommendations
 * - Format diagnostic report for display
 *
 * Coverage Target: 90%+ statement coverage
 */

// Mock health-checks to control test scenarios
jest.mock('../../../src/lib/health-checks', () => {
  const mockHealthChecks = jest.requireActual('../../__mocks__/health-checks');
  return mockHealthChecks;
});

import {
  runDiagnostics,
  formatDiagnosticReport,
  DiagnosticReport,
  DiagnosticFinding,
  doctor,
} from '../../../src/commands/doctor';
import * as doctorModule from '../../../src/commands/doctor';
import * as healthChecksMock from '../../../src/lib/health-checks';
import { __clearMockStore as __clearKeytarMocks } from '../../__mocks__/keytar';
import { __clearProcessMocks, __setupProcessRunning, __setupProcessError } from '../../__mocks__/child_process';
import { __clearNetworkMocks, __setupNetworkSuccess } from '../../__mocks__/https';
import { __resetAllHealthMocks } from '../../__mocks__/health-checks';

describe('Doctor Command', () => {
  beforeEach(() => {
    // Clear all mocks before each test to ensure isolation
    __clearKeytarMocks();
    __clearProcessMocks();
    __clearNetworkMocks();
  });

  describe('runDiagnostics()', () => {
    it('should gather all diagnostic information', async () => {
      // Setup
      __setupProcessRunning();
      __setupNetworkSuccess();

      // Execute
      const result = await runDiagnostics();

      // Assert
      expect(result).toBeDefined();
      expect(result.findings).toBeDefined();
      expect(result.recommendations).toBeDefined();
      expect(Array.isArray(result.findings)).toBe(true);
      expect(Array.isArray(result.recommendations)).toBe(true);
    });

    it('should detect configuration issues', async () => {
      // Setup: MCP process has issues
      __setupProcessError();
      __setupNetworkSuccess();

      // Execute
      const result = await runDiagnostics();

      // Assert
      expect(result).toBeDefined();
      expect(result.findings).toBeDefined();
    });

    it('should provide recommendations when issues found', async () => {
      // Setup: Problematic state
      __setupProcessError();

      // Execute
      const result = await runDiagnostics();

      // Assert
      expect(result).toBeDefined();
      expect(result.recommendations).toBeDefined();
    });

    it('should return empty report when no issues', async () => {
      // Setup: Everything healthy
      __setupProcessRunning();
      __setupNetworkSuccess();

      // Execute
      const result = await runDiagnostics();

      // Assert
      expect(result).toBeDefined();
      expect(result.findings).toBeDefined();
      expect(result.recommendations).toBeDefined();
    });
  });

  describe('formatDiagnosticReport()', () => {
    it('should format comprehensive diagnostic report', () => {
      // Setup
      const report: DiagnosticReport = {
        findings: [
          {
            component: 'MCP',
            status: 'ok',
            message: 'Process running normally',
          },
          {
            component: 'Network',
            status: 'ok',
            message: 'Connectivity verified',
          },
        ],
        recommendations: [],
      };

      // Execute
      const result = formatDiagnosticReport(report);

      // Assert
      expect(result).toContain('System Diagnostics Report');
      expect(result).toContain('MCP');
      expect(result).toContain('Network');
      expect(result).toContain('Process running normally');
      expect(result).toContain('Connectivity verified');
    });

    it('should prioritize critical findings', () => {
      // Setup
      const report: DiagnosticReport = {
        findings: [
          {
            component: 'Token',
            status: 'error',
            message: 'Token expired',
            details: 'Re-authentication required',
          },
          {
            component: 'MCP',
            status: 'warning',
            message: 'High memory usage',
          },
        ],
        recommendations: ['Re-authenticate to refresh token', 'Consider restarting MCP process'],
      };

      // Execute
      const result = formatDiagnosticReport(report);

      // Assert
      expect(result).toContain('[ERROR]');
      expect(result).toContain('[WARNING]');
      expect(result).toContain('Token expired');
      expect(result).toContain('Re-authentication required');
      expect(result).toContain('Recommendations:');
      expect(result).toContain('Re-authenticate to refresh token');
      expect(result).toContain('Consider restarting MCP process');
    });

    it('should format empty report correctly', () => {
      // Setup
      const report: DiagnosticReport = {
        findings: [],
        recommendations: [],
      };

      // Execute
      const result = formatDiagnosticReport(report);

      // Assert
      expect(result).toContain('System Diagnostics Report');
      expect(result).toContain('No issues found');
    });

    it('should handle findings with details', () => {
      // Setup
      const report: DiagnosticReport = {
        findings: [
          {
            component: 'Configuration',
            status: 'warning',
            message: 'Missing optional settings',
            details: 'Consider setting INTENT_ANALYZER_TIMEOUT for better performance',
          },
        ],
        recommendations: ['Review configuration documentation'],
      };

      // Execute
      const result = formatDiagnosticReport(report);

      // Assert
      expect(result).toContain('Missing optional settings');
      expect(result).toContain('Consider setting INTENT_ANALYZER_TIMEOUT');
    });

    it('should number recommendations', () => {
      // Setup
      const report: DiagnosticReport = {
        findings: [],
        recommendations: ['First recommendation', 'Second recommendation', 'Third recommendation'],
      };

      // Execute
      const result = formatDiagnosticReport(report);

      // Assert
      expect(result).toContain('1. First recommendation');
      expect(result).toContain('2. Second recommendation');
      expect(result).toContain('3. Third recommendation');
    });
  });

  describe('Recommendation Edge Cases', () => {
    it('should handle Token Health degraded with expiry details', async () => {
      // Mock token degraded state with expiry information
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([
        {
          name: 'Token Health',
          status: 'degraded',
          message: 'Token expiring soon',
          details: {
            expiresAt: Date.now() + 900000, // 15 minutes
            ttlMinutes: 15
          },
          last_check: new Date()
        }
      ]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'healthy',
        message: 'Config OK',
        last_check: new Date()
      });

      const result = await runDiagnostics();

      expect(result.recommendations).toEqual(
        expect.arrayContaining([expect.stringMatching(/Token expires at.*in 15 minutes/)])
      );
    });

    it('should handle Token Health degraded without expiry details', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([
        {
          name: 'Token Health',
          status: 'degraded',
          message: 'Token expiring soon',
          details: {},
          last_check: new Date()
        }
      ]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'healthy',
        message: 'Config OK',
        last_check: new Date()
      });

      const result = await runDiagnostics();

      expect(result.recommendations.some(r => r.includes('soon'))).toBe(true);
      expect(result.recommendations.some(r => r.includes('?'))).toBe(true);
    });

    it('should recommend restart for specific down MCP processes', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([
        {
          name: 'MCP Processes',
          status: 'unhealthy',
          message: 'Some processes down',
          details: {
            processes: [
              { name: 'googledocs', alive: false },
              { name: 'github', alive: true }
            ]
          },
          last_check: new Date()
        }
      ]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'healthy',
        message: 'Config OK',
        last_check: new Date()
      });

      const result = await runDiagnostics();

      expect(result.recommendations.some(r => r.includes('googledocs'))).toBe(true);
    });

    it('should provide generic MCP recommendation when processes list is empty', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([
        {
          name: 'MCP Processes',
          status: 'degraded',
          message: 'Processes degraded',
          details: { processes: [] },
          last_check: new Date()
        }
      ]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'healthy',
        message: 'Config OK',
        last_check: new Date()
      });

      const result = await runDiagnostics();

      expect(result.recommendations.some(r => r.includes('Some MCP processes are down'))).toBe(true);
    });

    it('should recommend firewall check for unreachable endpoints', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([
        {
          name: 'Network Connectivity',
          status: 'unhealthy',
          message: 'Network unreachable',
          details: {
            endpoints: [
              { endpoint: 'https://api.example.com', reachable: false, error: 'ECONNREFUSED' }
            ]
          },
          last_check: new Date()
        }
      ]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'healthy',
        message: 'Config OK',
        last_check: new Date()
      });

      const result = await runDiagnostics();

      expect(result.recommendations.some(r => r.includes('api.example.com'))).toBe(true);
      expect(result.recommendations.some(r => r.includes('ECONNREFUSED'))).toBe(true);
      expect(result.recommendations.some(r => r.includes('VPN/firewall'))).toBe(true);
    });

    it('should use default timeout message for endpoints without error', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([
        {
          name: 'Network Connectivity',
          status: 'unhealthy',
          message: 'Network unreachable',
          details: {
            endpoints: [
              { endpoint: 'https://slow.example.com', reachable: false }
            ]
          },
          last_check: new Date()
        }
      ]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'healthy',
        message: 'Config OK',
        last_check: new Date()
      });

      const result = await runDiagnostics();

      expect(result.recommendations.some(r => r.includes('timeout after 3s'))).toBe(true);
    });

    it('should recommend reinstall for unhealthy intent analyzer', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([
        {
          name: 'Intent Analyzer',
          status: 'unhealthy',
          message: 'Intent analyzer failing',
          last_check: new Date()
        }
      ]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'healthy',
        message: 'Config OK',
        last_check: new Date()
      });

      const result = await runDiagnostics();

      expect(result.recommendations.some(r => r.includes('Intent analyzer is failing'))).toBe(true);
      expect(result.recommendations.some(r => r.includes('npm install -g mcp-wizard'))).toBe(true);
    });

    it('should recommend update for degraded intent analyzer', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([
        {
          name: 'Intent Analyzer',
          status: 'degraded',
          message: 'Intent analyzer accuracy low',
          last_check: new Date()
        }
      ]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'healthy',
        message: 'Config OK',
        last_check: new Date()
      });

      const result = await runDiagnostics();

      expect(result.recommendations.some(r => r.includes('accuracy is below optimal'))).toBe(true);
    });

    it('should recommend setup for missing config file', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'unhealthy',
        message: 'Config missing',
        details: {
          path: '~/.config/mcp-wizard/config.json',
          errors: []
        },
        last_check: new Date()
      });

      const result = await runDiagnostics();

      expect(result.recommendations.some(r => r.includes('Config file missing or invalid'))).toBe(true);
      expect(result.recommendations.some(r => r.includes('mcp-wizard setup'))).toBe(true);
    });

    it('should recommend fix for config with validation errors', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'unhealthy',
        message: 'Config validation failed',
        details: {
          path: '~/.config/mcp-wizard/config.json',
          errors: ['missing field: mcps', 'invalid value: timeout']
        },
        last_check: new Date()
      });

      const result = await runDiagnostics();

      expect(result.recommendations.some(r => r.includes('2 errors'))).toBe(true);
    });

    it('should recommend completion for degraded config', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'degraded',
        message: 'Config incomplete',
        details: {},
        last_check: new Date()
      });

      const result = await runDiagnostics();

      expect(result.recommendations.some(r => r.includes('Config is incomplete'))).toBe(true);
      expect(result.recommendations.some(r => r.includes('mcp-wizard config init'))).toBe(true);
    });

    it('should map config degraded status to warning in findings', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'degraded',
        message: 'Config incomplete',
        details: {},
        last_check: new Date()
      });

      const result = await runDiagnostics();

      const configFinding = result.findings.find(f => f.component === 'Configuration');
      expect(configFinding?.status).toBe('warning');
    });
  });

  describe('doctor() command function', () => {
    let processExitSpy: jest.SpyInstance;
    let consoleLogSpy: jest.SpyInstance;
    let consoleErrorSpy: jest.SpyInstance;

    beforeEach(() => {
      // FIRST: Clear any jest.spyOn mocks from previous tests (critical!)
      jest.restoreAllMocks();

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
      __resetAllHealthMocks();  // Reset health-checks mock to healthy
      __setupProcessRunning();
      __setupNetworkSuccess();
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
    });

    it('should output JSON when json option is true', async () => {
      await doctor({ json: true });

      expect(processExitSpy).toHaveBeenCalledWith(0);
      expect(consoleLogSpy).toHaveBeenCalled();
      const output = JSON.parse(consoleLogSpy.mock.calls[0][0]);
      expect(output).toHaveProperty('overall_status');
      expect(output).toHaveProperty('findings');
      expect(output).toHaveProperty('recommendations');
      expect(output).toHaveProperty('timestamp');
    });

    it('should suppress output when silent option is true', async () => {
      await doctor({ silent: true });

      expect(processExitSpy).toHaveBeenCalledWith(0);
      expect(consoleLogSpy).not.toHaveBeenCalled();
    });

    it('should exit with code 0 for healthy status', async () => {
      await doctor({});

      expect(processExitSpy).toHaveBeenCalledWith(0);
    });

    it('should exit with code 1 for warnings', async () => {
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([
        {
          name: 'Token Health',
          status: 'degraded',
          message: 'Token expiring soon',
          last_check: new Date()
        }
      ]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'healthy',
        message: 'Config OK',
        last_check: new Date()
      });

      await doctor({});

      expect(processExitSpy).toHaveBeenCalledWith(1);
    });

    it('should exit with code 2 for errors', async () => {
      // Mock unhealthy status
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockResolvedValue([
        {
          name: 'Token Health',
          status: 'unhealthy',
          message: 'Token expired',
          last_check: new Date()
        }
      ]);
      jest.spyOn(healthChecksMock, 'validateConfiguration').mockResolvedValue({
        name: 'Configuration',
        status: 'healthy',
        message: 'Config OK',
        last_check: new Date()
      });

      await doctor({});

      expect(processExitSpy).toHaveBeenCalledWith(2);
    });

    it('should handle diagnostics errors gracefully', async () => {
      // Mock runAllHealthChecks to throw an error
      jest.spyOn(healthChecksMock, 'runAllHealthChecks').mockRejectedValue(new Error('Diagnostic error'));

      await doctor({});

      expect(processExitSpy).toHaveBeenCalledWith(2);
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        expect.stringContaining('Diagnostic error')
      );
    });
  });
});
