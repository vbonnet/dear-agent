/**
 * Tests for Disable Global MCPs Command
 *
 * Part of Task 1.3: mcp-wizard Integration for global MCP support.
 */

import { disableGlobalMcpsCommand } from '../disable-global-mcps';
import { loadConfig, saveConfig } from '../../lib/user-config';
import { sanitizeError } from '../../lib/errors';

// Mock dependencies
jest.mock('../../lib/user-config');
jest.mock('../../lib/errors');

describe('disableGlobalMcpsCommand', () => {
  const mockLoadConfig = loadConfig as jest.MockedFunction<typeof loadConfig>;
  const mockSaveConfig = saveConfig as jest.MockedFunction<typeof saveConfig>;
  const mockSanitizeError = sanitizeError as jest.MockedFunction<typeof sanitizeError>;

  beforeEach(() => {
    jest.clearAllMocks();
    // Suppress console output during tests
    jest.spyOn(console, 'log').mockImplementation();
    jest.spyOn(console, 'error').mockImplementation();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('should disable existing global MCPs config', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
      globalMcps: {
        enabled: true,
        healthCheckUrl: 'http://localhost:8001/health',
        discoveryUrl: 'http://localhost:8001/discovery',
        temporalUrl: 'http://localhost:7233',
      },
    };

    mockLoadConfig.mockResolvedValue(mockConfig);
    mockSaveConfig.mockResolvedValue();

    await disableGlobalMcpsCommand({});

    expect(mockSaveConfig).toHaveBeenCalledWith({
      company: mockConfig.company,
      globalMcps: {
        enabled: false,
        healthCheckUrl: 'http://localhost:8001/health',
        discoveryUrl: 'http://localhost:8001/discovery',
        temporalUrl: 'http://localhost:7233',
      },
    });

    expect(console.log).toHaveBeenCalledWith(
      expect.stringContaining('Global MCP discovery disabled')
    );
  });

  it('should create disabled config if none exists', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
    };

    mockLoadConfig.mockResolvedValue(mockConfig);
    mockSaveConfig.mockResolvedValue();

    await disableGlobalMcpsCommand({});

    expect(mockSaveConfig).toHaveBeenCalledWith({
      company: mockConfig.company,
      globalMcps: {
        enabled: false,
      },
    });
  });

  it('should support silent mode', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
      globalMcps: {
        enabled: true,
      },
    };

    mockLoadConfig.mockResolvedValue(mockConfig);
    mockSaveConfig.mockResolvedValue();

    await disableGlobalMcpsCommand({ silent: true });

    expect(console.log).not.toHaveBeenCalled();
    expect(mockSaveConfig).toHaveBeenCalled();
  });

  it('should handle errors gracefully', async () => {
    const mockError = new Error('Config save failed');
    mockLoadConfig.mockResolvedValue({
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
    });
    mockSaveConfig.mockRejectedValue(mockError);
    mockSanitizeError.mockReturnValue({
      message: 'Config save failed',
      fix: 'Check file permissions',
    } as any);

    const exitSpy = jest.spyOn(process, 'exit').mockImplementation((code?: any) => {
      throw new Error(`process.exit(${code})`);
    });

    await expect(disableGlobalMcpsCommand({})).rejects.toThrow('process.exit(1)');

    expect(exitSpy).toHaveBeenCalledWith(1);
    expect(console.error).toHaveBeenCalledWith(
      expect.stringContaining('Failed to disable global MCPs')
    );

    exitSpy.mockRestore();
  });

  it('should preserve other globalMcps properties when disabling', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
      globalMcps: {
        enabled: true,
        healthCheckUrl: 'http://custom:9000/health',
        discoveryUrl: 'http://custom:9000/discovery',
        temporalUrl: 'http://custom:9001',
      },
    };

    mockLoadConfig.mockResolvedValue(mockConfig);
    mockSaveConfig.mockResolvedValue();

    await disableGlobalMcpsCommand({});

    expect(mockSaveConfig).toHaveBeenCalledWith({
      company: mockConfig.company,
      globalMcps: {
        enabled: false,
        healthCheckUrl: 'http://custom:9000/health',
        discoveryUrl: 'http://custom:9000/discovery',
        temporalUrl: 'http://custom:9001',
      },
    });
  });
});
