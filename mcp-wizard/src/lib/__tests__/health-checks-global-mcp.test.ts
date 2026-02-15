/**
 * Tests for Global MCP Health Check
 *
 * Part of Task 1.3: mcp-wizard Integration for global MCP support.
 */

import { checkGlobalMCPHealth } from '../health-checks';
import * as fs from 'fs/promises';

// Mock dependencies
jest.mock('fs/promises');
jest.mock('../health-cache', () => ({
  getCached: jest.fn(() => null),
  setCached: jest.fn(),
}));

// Mock fetch globally
global.fetch = jest.fn() as jest.MockedFunction<typeof fetch>;

describe('checkGlobalMCPHealth', () => {
  const mockFsReadFile = fs.readFile as jest.MockedFunction<typeof fs.readFile>;

  beforeEach(() => {
    jest.clearAllMocks();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('should return healthy when global MCPs not enabled', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
    };

    mockFsReadFile.mockResolvedValue(JSON.stringify(mockConfig));

    const result = await checkGlobalMCPHealth({ force: true });

    expect(result.status).toBe('healthy');
    expect(result.message).toBe('Global MCPs not enabled');
    expect(result.details?.enabled).toBe(false);
  });

  it('should return healthy when server responds OK', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
      globalMcps: {
        enabled: true,
        healthCheckUrl: 'http://localhost:8001/health',
      },
    };

    mockFsReadFile.mockResolvedValue(JSON.stringify(mockConfig));

    const mockResponse = {
      ok: true,
      json: async () => ({
        uptime: 12345,
        sessionCount: 3,
      }),
    } as Response;

    (global.fetch as jest.MockedFunction<typeof fetch>).mockResolvedValue(mockResponse);

    const result = await checkGlobalMCPHealth({ force: true });

    expect(result.status).toBe('healthy');
    expect(result.message).toContain('HTTP server healthy');
    expect(result.message).toContain('12345');
    expect(result.details?.enabled).toBe(true);
    expect(result.details?.uptime).toBe(12345);
    expect(result.details?.sessionCount).toBe(3);
  });

  it('should return unhealthy when server responds with error status', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
      globalMcps: {
        enabled: true,
        healthCheckUrl: 'http://localhost:8001/health',
      },
    };

    mockFsReadFile.mockResolvedValue(JSON.stringify(mockConfig));

    const mockResponse = {
      ok: false,
      status: 503,
      statusText: 'Service Unavailable',
    } as Response;

    (global.fetch as jest.MockedFunction<typeof fetch>).mockResolvedValue(mockResponse);

    const result = await checkGlobalMCPHealth({ force: true });

    expect(result.status).toBe('unhealthy');
    expect(result.message).toContain('HTTP 503 Service Unavailable');
    expect(result.details?.status).toBe(503);
  });

  it('should return unhealthy when network request fails', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
      globalMcps: {
        enabled: true,
        healthCheckUrl: 'http://localhost:8001/health',
      },
    };

    mockFsReadFile.mockResolvedValue(JSON.stringify(mockConfig));

    (global.fetch as jest.MockedFunction<typeof fetch>).mockRejectedValue(
      new Error('Connection refused')
    );

    const result = await checkGlobalMCPHealth({ force: true });

    expect(result.status).toBe('unhealthy');
    expect(result.message).toContain('Health check failed: Connection refused');
    expect(result.details?.error).toBe('Connection refused');
  });

  it('should use custom health check URL from config', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
      globalMcps: {
        enabled: true,
        healthCheckUrl: 'http://custom:9000/health',
      },
    };

    mockFsReadFile.mockResolvedValue(JSON.stringify(mockConfig));

    const mockResponse = {
      ok: true,
      json: async () => ({ uptime: 100 }),
    } as Response;

    (global.fetch as jest.MockedFunction<typeof fetch>).mockResolvedValue(mockResponse);

    const result = await checkGlobalMCPHealth({ force: true });

    expect(global.fetch).toHaveBeenCalledWith(
      'http://custom:9000/health',
      expect.objectContaining({
        method: 'GET',
      })
    );
    expect(result.status).toBe('healthy');
  });

  it('should handle config read errors', async () => {
    mockFsReadFile.mockRejectedValue(new Error('ENOENT: config file not found'));

    const result = await checkGlobalMCPHealth({ force: true });

    expect(result.status).toBe('unhealthy');
    expect(result.message).toContain('Config error');
    expect(result.details?.error).toContain('ENOENT');
  });

  it('should handle invalid JSON in config', async () => {
    mockFsReadFile.mockResolvedValue('{ invalid json');

    const result = await checkGlobalMCPHealth({ force: true });

    expect(result.status).toBe('unhealthy');
    expect(result.message).toContain('Config error');
  });

  it('should abort request after timeout', async () => {
    jest.useFakeTimers();

    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
      globalMcps: {
        enabled: true,
        healthCheckUrl: 'http://localhost:8001/health',
      },
    };

    mockFsReadFile.mockResolvedValue(JSON.stringify(mockConfig));

    // Mock fetch to hang indefinitely
    (global.fetch as jest.MockedFunction<typeof fetch>).mockImplementation(
      () => new Promise(() => {})
    );

    const checkPromise = checkGlobalMCPHealth({ force: true });

    // Fast-forward past the 5s timeout
    jest.advanceTimersByTime(6000);

    const result = await checkPromise;

    expect(result.status).toBe('unhealthy');

    jest.useRealTimers();
  });
});
