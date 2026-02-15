/**
 * Tests for Enable Global MCPs Command
 *
 * Part of Task 1.3: mcp-wizard Integration for global MCP support.
 */

import { enableGlobalMcpsCommand } from '../enable-global-mcps';
import { loadConfig, saveConfig } from '../../lib/user-config';
import { sanitizeError } from '../../lib/errors';

// Mock dependencies
jest.mock('../../lib/user-config');
jest.mock('../../lib/errors');

describe('enableGlobalMcpsCommand', () => {
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

  it('should enable global MCPs with default URLs', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
    };

    mockLoadConfig.mockResolvedValue(mockConfig);
    mockSaveConfig.mockResolvedValue();

    await enableGlobalMcpsCommand({});

    expect(mockSaveConfig).toHaveBeenCalledWith({
      company: mockConfig.company,
      globalMcps: {
        enabled: true,
        healthCheckUrl: 'http://localhost:8001/health',
        discoveryUrl: 'http://localhost:8001/discovery',
        temporalUrl: 'http://localhost:7233',
      },
    });
  });

  it('should enable global MCPs with custom URLs', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
    };

    mockLoadConfig.mockResolvedValue(mockConfig);
    mockSaveConfig.mockResolvedValue();

    await enableGlobalMcpsCommand({
      healthUrl: 'http://custom:9000/health',
      discoveryUrl: 'http://custom:9000/discovery',
      temporalUrl: 'http://custom:9001',
    });

    expect(mockSaveConfig).toHaveBeenCalledWith({
      company: mockConfig.company,
      globalMcps: {
        enabled: true,
        healthCheckUrl: 'http://custom:9000/health',
        discoveryUrl: 'http://custom:9000/discovery',
        temporalUrl: 'http://custom:9001',
      },
    });
  });

  it('should update existing globalMcps config', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
      globalMcps: {
        enabled: false,
        healthCheckUrl: 'http://old:8001/health',
        discoveryUrl: 'http://old:8001/discovery',
        temporalUrl: 'http://old:7233',
      },
    };

    mockLoadConfig.mockResolvedValue(mockConfig);
    mockSaveConfig.mockResolvedValue();

    await enableGlobalMcpsCommand({
      healthUrl: 'http://new:9000/health',
    });

    expect(mockSaveConfig).toHaveBeenCalledWith({
      company: mockConfig.company,
      globalMcps: {
        enabled: true,
        healthCheckUrl: 'http://new:9000/health',
        discoveryUrl: 'http://old:8001/discovery',
        temporalUrl: 'http://old:7233',
      },
    });
  });

  it('should handle errors gracefully', async () => {
    const mockError = new Error('Config load failed');
    mockLoadConfig.mockRejectedValue(mockError);
    mockSanitizeError.mockReturnValue({
      message: 'Config load failed',
      fix: 'Run mcp-wizard config init',
    } as any);

    const exitSpy = jest.spyOn(process, 'exit').mockImplementation((code?: any) => {
      throw new Error(`process.exit(${code})`);
    });

    await expect(enableGlobalMcpsCommand({})).rejects.toThrow('process.exit(1)');

    expect(exitSpy).toHaveBeenCalledWith(1);
    expect(console.error).toHaveBeenCalledWith(
      expect.stringContaining('Failed to enable global MCPs')
    );

    exitSpy.mockRestore();
  });

  it('should display Temporal UI URL correctly', async () => {
    const mockConfig = {
      company: {
        name: 'Test Corp',
        glean_instance: 'test',
        okta_domain: 'test.okta.com',
      },
    };

    mockLoadConfig.mockResolvedValue(mockConfig);
    mockSaveConfig.mockResolvedValue();

    await enableGlobalMcpsCommand({});

    expect(console.log).toHaveBeenCalledWith(
      expect.stringContaining('http://localhost:8088')
    );
  });
});
