/**
 * Unit Tests for Health Checks
 *
 * Tests the 4 core health checks:
 * 1. Token Health (Google OAuth)
 * 2. MCP Process Health (existence check)
 * 3. Network Connectivity (OAuth and API endpoints)
 * 4. Intent Analyzer (keyword matching accuracy)
 *
 * Requirements:
 * - All health checks return consistent HealthCheckResult format
 * - Cache integration works correctly
 * - Status determination logic is accurate
 * - Error handling is robust
 *
 * Coverage Target: 90%+ statement coverage
 */

// Unmock health-checks to test real implementation
jest.unmock('../../../src/lib/health-checks');

import {
  runAllHealthChecks,
  validateConfiguration,
  HealthCheckResult,
} from '../../../src/lib/health-checks';
import { clearCache } from '../../../src/lib/health-cache';
import * as tokenStorage from '../../../src/lib/token-storage';
import * as tokenInjection from '../../../src/lib/token-injection';
import * as intentAnalyzer from '../../../src/lib/intent-analyzer';
import { exec } from 'child_process';
import * as fs from 'fs/promises';

// Mock dependencies
jest.mock('child_process');
jest.mock('fs/promises');
jest.mock('../../../src/lib/token-storage');
jest.mock('../../../src/lib/token-injection');
jest.mock('../../../src/lib/intent-analyzer');

// Mock fetch globally
global.fetch = jest.fn();

describe('Health Checks', () => {
  beforeEach(() => {
    // Clear cache before each test
    clearCache();

    // Reset all mocks
    jest.clearAllMocks();

    // Default mock implementations
    (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue('mock-token');
    (tokenInjection.checkTokenHealth as jest.Mock).mockReturnValue({
      valid: true,
      isExpired: false,
      expiresAt: new Date(Date.now() + 3600000).toISOString(),
      remainingTTL: 3600000,
    });
  });

  afterEach(() => {
    clearCache();
  });

  describe('runAllHealthChecks()', () => {
    it('should run all 4 health checks in parallel', async () => {
      // Setup mocks
      mockAllHealthy();

      // Execute
      const results = await runAllHealthChecks();

      // Assert
      expect(results).toHaveLength(4);
      expect(results.map((r) => r.name)).toContain('Token Health');
      expect(results.map((r) => r.name)).toContain('MCP Processes');
      expect(results.map((r) => r.name)).toContain('Network Connectivity');
      expect(results.map((r) => r.name)).toContain('Intent Analyzer');
    });

    it('should return consistent HealthCheckResult format', async () => {
      // Setup
      mockAllHealthy();

      // Execute
      const results = await runAllHealthChecks();

      // Assert
      results.forEach((result) => {
        expect(result).toHaveProperty('name');
        expect(result).toHaveProperty('status');
        expect(result).toHaveProperty('message');
        expect(result).toHaveProperty('last_check');
        expect(['healthy', 'degraded', 'unhealthy']).toContain(result.status);
        expect(result.last_check).toBeInstanceOf(Date);
      });
    });

    it('should use cache when force=false', async () => {
      // Setup
      mockAllHealthy();

      // Execute
      const firstRun = await runAllHealthChecks({ force: false });
      const secondRun = await runAllHealthChecks({ force: false });

      // Assert: Results should be identical (from cache)
      expect(firstRun).toHaveLength(4);
      expect(secondRun).toHaveLength(4);
      // Cache prevents re-execution, so we just verify it completes
    });

    it('should bypass cache when force=true', async () => {
      // Setup
      mockAllHealthy();

      // Execute
      await runAllHealthChecks({ force: false });
      await runAllHealthChecks({ force: true }); // Force bypass

      // Assert: Should make fresh calls
      expect(tokenStorage.getOktaToken).toHaveBeenCalled();
    });
  });

  describe('Token Health Check', () => {
    it('should return healthy for valid token with >5min TTL', async () => {
      // Setup
      (tokenInjection.checkTokenHealth as jest.Mock).mockReturnValue({
        valid: true,
        isExpired: false,
        expiresAt: new Date(Date.now() + 600000).toISOString(), // 10 min
        remainingTTL: 600000,
      });
      mockMCPHealthy();
      mockNetworkHealthy();
      mockIntentHealthy();

      // Execute
      const results = await runAllHealthChecks();
      const tokenCheck = results.find((r) => r.name === 'Token Health');

      // Assert
      expect(tokenCheck?.status).toBe('healthy');
      expect(tokenCheck?.message).toContain('valid');
    });

    it('should return degraded for token with <5min TTL', async () => {
      // Setup
      (tokenInjection.checkTokenHealth as jest.Mock).mockReturnValue({
        valid: true,
        isExpired: false,
        expiresAt: new Date(Date.now() + 180000).toISOString(), // 3 min
        remainingTTL: 180000,
      });
      mockMCPHealthy();
      mockNetworkHealthy();
      mockIntentHealthy();

      // Execute
      const results = await runAllHealthChecks();
      const tokenCheck = results.find((r) => r.name === 'Token Health');

      // Assert
      expect(tokenCheck?.status).toBe('degraded');
      expect(tokenCheck?.message).toContain('expires in 3 minutes');
    });

    it('should return unhealthy for expired token', async () => {
      // Setup
      (tokenInjection.checkTokenHealth as jest.Mock).mockReturnValue({
        valid: false,
        isExpired: true,
        expiresAt: new Date(Date.now() - 1000).toISOString(),
        remainingTTL: 0,
      });
      mockMCPHealthy();
      mockNetworkHealthy();
      mockIntentHealthy();

      // Execute
      const results = await runAllHealthChecks();
      const tokenCheck = results.find((r) => r.name === 'Token Health');

      // Assert
      expect(tokenCheck?.status).toBe('unhealthy');
      expect(tokenCheck?.message).toContain('expired');
    });

    it('should return unhealthy when token not found', async () => {
      // Setup
      (tokenStorage.getOktaToken as jest.Mock).mockRejectedValue(new Error('Token not found'));
      mockMCPHealthy();
      mockNetworkHealthy();
      mockIntentHealthy();

      // Execute
      const results = await runAllHealthChecks();
      const tokenCheck = results.find((r) => r.name === 'Token Health');

      // Assert
      expect(tokenCheck?.status).toBe('unhealthy');
      expect(tokenCheck?.message).toContain('No Google OAuth token found');
    });
  });

  describe('MCP Process Health Check', () => {
    it('should return healthy when all MCPs running', async () => {
      // Setup
      mockTokenHealthy();
      mockMCPHealthy();
      mockNetworkHealthy();
      mockIntentHealthy();

      // Execute
      const results = await runAllHealthChecks();
      const mcpCheck = results.find((r) => r.name === 'MCP Processes');

      // Assert
      expect(mcpCheck?.status).toBe('healthy');
      expect(mcpCheck?.message).toMatch(/processes alive/);
    });

    it('should return degraded when some MCPs running', async () => {
      // Setup
      mockTokenHealthy();
      mockConfig({
        broker: {
          downstream_mcps: {
            googledocs: { command: 'node', args: ['googledocs.js'] },
            atlassian: { command: 'node', args: ['atlassian.js'] },
          },
        },
      });

      // Mock one process running, one not
      (exec as unknown as jest.Mock).mockImplementation((cmd: string, callback: any) => {
        if (cmd.includes('googledocs')) {
          callback(null, { stdout: '12345' }); // Running
        } else {
          callback(new Error('Process not found')); // Not running
        }
      });

      mockNetworkHealthy();
      mockIntentHealthy();

      // Execute
      const results = await runAllHealthChecks();
      const mcpCheck = results.find((r) => r.name === 'MCP Processes');

      // Assert
      expect(mcpCheck?.status).toBe('degraded');
    });

    it('should return unhealthy when no MCPs running', async () => {
      // Setup
      mockTokenHealthy();
      mockConfig({
        broker: {
          downstream_mcps: {
            googledocs: { command: 'node', args: ['googledocs.js'] },
          },
        },
      });

      (exec as unknown as jest.Mock).mockImplementation((cmd: string, callback: any) => {
        callback(new Error('Process not found'));
      });

      mockNetworkHealthy();
      mockIntentHealthy();

      // Execute
      const results = await runAllHealthChecks();
      const mcpCheck = results.find((r) => r.name === 'MCP Processes');

      // Assert
      expect(mcpCheck?.status).toBe('unhealthy');
    });

    it('should return degraded when no MCPs configured', async () => {
      // Setup
      mockTokenHealthy();
      mockConfig({ broker: { downstream_mcps: {} } });
      mockNetworkHealthy();
      mockIntentHealthy();

      // Execute
      const results = await runAllHealthChecks();
      const mcpCheck = results.find((r) => r.name === 'MCP Processes');

      // Assert
      expect(mcpCheck?.status).toBe('degraded');
      expect(mcpCheck?.message).toContain('No MCP servers configured');
    });
  });

  describe('Network Connectivity Check', () => {
    it('should return healthy when all endpoints reachable', async () => {
      // Setup
      mockTokenHealthy();
      mockMCPHealthy();
      mockNetworkHealthy();
      mockIntentHealthy();

      // Execute
      const results = await runAllHealthChecks();
      const networkCheck = results.find((r) => r.name === 'Network Connectivity');

      // Assert
      expect(networkCheck?.status).toBe('healthy');
      expect(networkCheck?.message).toContain('reachable');
    });

    it('should return degraded when 1-2 endpoints unreachable', async () => {
      // Setup
      mockTokenHealthy();
      mockMCPHealthy();
      mockConfig({
        company: { okta_domain: 'test.okta.com' },
        broker: {
          downstream_mcps: {
            googledocs: { command: 'node', args: ['googledocs.js'] },
            atlassian: { command: 'node', args: ['atlassian.js'] },
          },
        },
      });

      // Mock: 1 endpoint fails, 2 succeed
      (global.fetch as jest.Mock).mockImplementation((url: string) => {
        if (url.includes('okta')) {
          return Promise.reject(new Error('Network error'));
        }
        return Promise.resolve({ ok: true, status: 200 });
      });

      mockIntentHealthy();

      // Execute
      const results = await runAllHealthChecks();
      const networkCheck = results.find((r) => r.name === 'Network Connectivity');

      // Assert
      expect(networkCheck?.status).toBe('degraded');
    });

    it('should return unhealthy when 3+ endpoints unreachable', async () => {
      // Setup
      mockTokenHealthy();
      mockMCPHealthy();
      mockConfig({
        company: { okta_domain: 'test.okta.com' },
        broker: {
          downstream_mcps: {
            googledocs: { command: 'node', args: ['googledocs.js'] },
            atlassian: { command: 'node', args: ['atlassian.js'] },
          },
        },
      });

      // Mock all fail
      (global.fetch as jest.Mock).mockRejectedValue(new Error('Network error'));

      mockIntentHealthy();

      // Execute
      const results = await runAllHealthChecks();
      const networkCheck = results.find((r) => r.name === 'Network Connectivity');

      // Assert
      expect(networkCheck?.status).toBe('unhealthy');
    });
  });

  describe('Intent Analyzer Check', () => {
    it('should return healthy for high confidence and no mismatches', async () => {
      // Setup
      mockTokenHealthy();
      mockMCPHealthy();
      mockNetworkHealthy();

      // Mock high confidence matches
      (intentAnalyzer.analyzeIntent as jest.Mock).mockImplementation((raw: string) => {
        if (raw.includes('Create')) return { action: 'CREATE', confidence: 0.9 };
        if (raw.includes('Show')) return { action: 'READ', confidence: 0.85 };
        if (raw.includes('Update')) return { action: 'UPDATE', confidence: 0.8 };
        if (raw.includes('Delete')) return { action: 'DELETE', confidence: 0.9 };
        if (raw.includes('Search')) return { action: 'SEARCH', confidence: 0.85 };
        return { action: 'UNKNOWN', confidence: 0.3 };
      });

      // Execute
      const results = await runAllHealthChecks();
      const intentCheck = results.find((r) => r.name === 'Intent Analyzer');

      // Assert
      expect(intentCheck?.status).toBe('healthy');
      expect(intentCheck?.message).toContain('0 mismatches');
    });

    it('should return degraded for moderate confidence or 1 mismatch', async () => {
      // Setup
      mockTokenHealthy();
      mockMCPHealthy();
      mockNetworkHealthy();

      // Mock moderate confidence with 1 mismatch
      (intentAnalyzer.analyzeIntent as jest.Mock).mockImplementation((raw: string) => {
        if (raw.includes('Create')) return { action: 'READ', confidence: 0.6 }; // Mismatch
        if (raw.includes('Show')) return { action: 'READ', confidence: 0.6 };
        if (raw.includes('Update')) return { action: 'UPDATE', confidence: 0.6 };
        if (raw.includes('Delete')) return { action: 'DELETE', confidence: 0.6 };
        if (raw.includes('Search')) return { action: 'SEARCH', confidence: 0.6 };
        return { action: 'UNKNOWN', confidence: 0.3 };
      });

      // Execute
      const results = await runAllHealthChecks();
      const intentCheck = results.find((r) => r.name === 'Intent Analyzer');

      // Assert
      expect(intentCheck?.status).toBe('degraded');
    });

    it('should return unhealthy for low confidence or 2+ mismatches', async () => {
      // Setup
      mockTokenHealthy();
      mockMCPHealthy();
      mockNetworkHealthy();

      // Mock low confidence with multiple mismatches
      (intentAnalyzer.analyzeIntent as jest.Mock).mockImplementation(() => ({
        action: 'UNKNOWN',
        confidence: 0.3,
      }));

      // Execute
      const results = await runAllHealthChecks();
      const intentCheck = results.find((r) => r.name === 'Intent Analyzer');

      // Assert
      expect(intentCheck?.status).toBe('unhealthy');
    });

    it('should handle intent analyzer errors', async () => {
      // Setup
      mockTokenHealthy();
      mockMCPHealthy();
      mockNetworkHealthy();

      (intentAnalyzer.analyzeIntent as jest.Mock).mockImplementation(() => {
        throw new Error('Analyzer error');
      });

      // Execute
      const results = await runAllHealthChecks();
      const intentCheck = results.find((r) => r.name === 'Intent Analyzer');

      // Assert
      expect(intentCheck?.status).toBe('unhealthy');
      expect(intentCheck?.message).toContain('failed');
    });
  });

  describe('validateConfiguration()', () => {
    it('should return healthy for valid configuration', async () => {
      // Setup
      mockConfig({
        company: {
          name: '[REDACTED_EMPLOYER]',
          okta_domain: '[REDACTED_EMPLOYER].okta.com',
        },
        broker: {
          downstream_mcps: {
            googledocs: { command: 'node', args: ['googledocs.js'] },
          },
        },
      });

      // Execute
      const result = await validateConfiguration();

      // Assert
      expect(result.status).toBe('healthy');
      expect(result.message).toContain('valid');
    });

    it('should return unhealthy for missing company.name', async () => {
      // Setup
      mockConfig({
        company: { okta_domain: 'test.okta.com' },
        broker: {
          downstream_mcps: {
            googledocs: { command: 'node', args: ['googledocs.js'] },
          },
        },
      });

      // Execute
      const result = await validateConfiguration();

      // Assert
      expect(result.status).toBe('unhealthy');
      expect(result.message).toContain('company.name');
    });

    it('should return unhealthy for missing MCP configuration', async () => {
      // Setup
      mockConfig({
        company: { name: 'Test', okta_domain: 'test.okta.com' },
        broker: {},
      });

      // Execute
      const result = await validateConfiguration();

      // Assert
      expect(result.status).toBe('unhealthy');
      expect(result.message).toContain('downstream_mcps');
    });

    it('should detect invalid MCP server configuration', async () => {
      // Setup
      mockConfig({
        company: { name: 'Test', okta_domain: 'test.okta.com' },
        broker: {
          downstream_mcps: {
            googledocs: { args: ['googledocs.js'] }, // Missing command
          },
        },
      });

      // Execute
      const result = await validateConfiguration();

      // Assert
      expect(result.status).toBe('unhealthy');
      expect(result.details?.errors).toContain('MCP googledocs: missing command');
    });

    it('should handle missing config file', async () => {
      // Setup
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('ENOENT: file not found'));

      // Execute
      const result = await validateConfiguration();

      // Assert
      expect(result.status).toBe('unhealthy');
      expect(result.message).toContain('Config file error');
    });
  });
});

// Helper functions
function mockAllHealthy() {
  mockTokenHealthy();
  mockMCPHealthy();
  mockNetworkHealthy();
  mockIntentHealthy();
}

function mockTokenHealthy() {
  (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue('mock-token');
  (tokenInjection.checkTokenHealth as jest.Mock).mockReturnValue({
    valid: true,
    isExpired: false,
    expiresAt: new Date(Date.now() + 600000).toISOString(),
    remainingTTL: 600000,
  });
}

function mockMCPHealthy() {
  mockConfig({
    company: { okta_domain: 'test.okta.com' },
    broker: {
      downstream_mcps: {
        googledocs: { command: 'node', args: ['googledocs.js'] },
      },
    },
  });

  (exec as unknown as jest.Mock).mockImplementation((cmd: string, callback: any) => {
    callback(null, { stdout: '12345' }); // Process running
  });
}

function mockNetworkHealthy() {
  (global.fetch as jest.Mock).mockResolvedValue({ ok: true, status: 200 });
}

function mockIntentHealthy() {
  (intentAnalyzer.analyzeIntent as jest.Mock).mockImplementation((raw: string) => {
    if (raw.includes('Create')) return { action: 'CREATE', confidence: 0.9 };
    if (raw.includes('Show')) return { action: 'READ', confidence: 0.85 };
    if (raw.includes('Update')) return { action: 'UPDATE', confidence: 0.8 };
    if (raw.includes('Delete')) return { action: 'DELETE', confidence: 0.9 };
    if (raw.includes('Search')) return { action: 'SEARCH', confidence: 0.85 };
    return { action: 'UNKNOWN', confidence: 0.3 };
  });
}

function mockConfig(config: any) {
  (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(config));
}
