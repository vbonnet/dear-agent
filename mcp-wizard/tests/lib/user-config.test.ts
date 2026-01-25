/**
 * Unit tests for user-config module
 */

import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import {
  getUserConfigPath,
  getDefaultConfig,
  loadConfig,
  saveConfig,
  validateConfig,
  getConfigValue,
  setConfigValue,
  UserConfig,
} from '../../src/lib/user-config';
import {
  VALID_CONFIG_[REDACTED_EMPLOYER],
  VALID_CONFIG_ACME,
  INVALID_CONFIG_MISSING_COMPANY,
  INVALID_CONFIG_EMPTY_NAME,
  INVALID_CONFIG_SPACES_IN_GLEAN,
  INVALID_CONFIG_UPPERCASE_GLEAN,
  INVALID_CONFIG_BAD_OKTA,
  MALFORMED_JSON,
} from './__helpers__/config-fixtures';

// Mock modules
jest.mock('fs');
jest.mock('os');
jest.mock('path');

describe('user-config', () => {
  const mockConfigPath = '/home/testuser/.config/mcp-wizard/config.json';
  const mockHomeDir = '/home/testuser';

  let consoleLogSpy: jest.SpyInstance;
  let consoleWarnSpy: jest.SpyInstance;

  beforeEach(() => {
    jest.clearAllMocks();

    // Mock os.homedir
    (os.homedir as jest.Mock).mockReturnValue(mockHomeDir);

    // Mock path.join to return predictable paths
    (path.join as jest.Mock).mockImplementation((...parts: string[]) => parts.join('/'));

    // Mock path.dirname to return parent directory
    (path.dirname as jest.Mock).mockImplementation((p: string) => {
      const parts = p.split('/');
      parts.pop();
      return parts.join('/');
    });

    // Mock console
    consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
    consoleWarnSpy = jest.spyOn(console, 'warn').mockImplementation();
  });

  afterEach(() => {
    consoleLogSpy.mockRestore();
    consoleWarnSpy.mockRestore();
  });

  describe('getUserConfigPath', () => {
    const originalEnv = process.env;

    beforeEach(() => {
      jest.resetModules();
      process.env = { ...originalEnv };
    });

    afterEach(() => {
      process.env = originalEnv;
    });

    test('returns XDG_CONFIG_HOME/mcp-wizard/config.json when XDG_CONFIG_HOME set', () => {
      process.env.XDG_CONFIG_HOME = '/custom/config';

      const result = getUserConfigPath();

      expect(result).toBe('/custom/config/mcp-wizard/config.json');
    });

    test('returns ~/.config/mcp-wizard/config.json when XDG_CONFIG_HOME unset', () => {
      delete process.env.XDG_CONFIG_HOME;

      const result = getUserConfigPath();

      expect(result).toBe(`${mockHomeDir}/.config/mcp-wizard/config.json`);
    });
  });

  describe('getDefaultConfig', () => {
    test('returns [REDACTED_EMPLOYER] defaults', () => {
      const result = getDefaultConfig();

      expect(result).toEqual({
        company: {
          name: '[REDACTED_EMPLOYER]',
          glean_instance: '[REDACTED_EMPLOYER]',
          okta_domain: '[REDACTED_EMPLOYER].okta.com',
        },
      });
    });
  });

  describe('loadConfig', () => {
    beforeEach(() => {
      // Reset process.env for XDG tests
      delete process.env.XDG_CONFIG_HOME;
    });

    test('loads config from file when it exists and is valid', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(VALID_CONFIG_ACME));

      const result = loadConfig();

      expect(result).toEqual(VALID_CONFIG_ACME);
      expect(fs.readFileSync).toHaveBeenCalledWith(expect.stringContaining('config.json'), 'utf8');
    });

    test('creates default config when file missing', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(false);
      (fs.mkdirSync as jest.Mock).mockImplementation();
      (fs.writeFileSync as jest.Mock).mockImplementation();
      (fs.renameSync as jest.Mock).mockImplementation();

      const result = loadConfig();

      expect(result).toEqual(getDefaultConfig());
      expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('Creating default config'));
      expect(fs.writeFileSync).toHaveBeenCalled();
    });

    test('returns defaults when config file is malformed JSON', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(MALFORMED_JSON);

      const result = loadConfig();

      expect(result).toEqual(getDefaultConfig());
      expect(consoleWarnSpy).toHaveBeenCalledWith(expect.stringContaining('not valid JSON'));
    });

    test('returns defaults when config file is invalid', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(INVALID_CONFIG_MISSING_COMPANY));

      const result = loadConfig();

      expect(result).toEqual(getDefaultConfig());
      expect(consoleWarnSpy).toHaveBeenCalledWith(expect.stringContaining('Failed to load config'));
    });

    test('returns defaults when config file has permission denied', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockImplementation(() => {
        const error: NodeJS.ErrnoException = new Error('Permission denied');
        error.code = 'EACCES';
        throw error;
      });

      const result = loadConfig();

      expect(result).toEqual(getDefaultConfig());
      expect(consoleWarnSpy).toHaveBeenCalledWith(expect.stringContaining('permission denied'));
    });

    test('calls validateConfig after parsing', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(VALID_CONFIG_[REDACTED_EMPLOYER]));

      // Should not throw
      expect(() => loadConfig()).not.toThrow();
    });
  });

  describe('saveConfig', () => {
    const tempPath = `${mockConfigPath}.tmp`;

    beforeEach(() => {
      delete process.env.XDG_CONFIG_HOME;
      (fs.mkdirSync as jest.Mock).mockImplementation();
      (fs.writeFileSync as jest.Mock).mockImplementation();
      (fs.renameSync as jest.Mock).mockImplementation();
      (fs.existsSync as jest.Mock).mockReturnValue(false);
      (fs.unlinkSync as jest.Mock).mockImplementation();
    });

    test('creates directory when parent directory missing', () => {
      saveConfig(VALID_CONFIG_[REDACTED_EMPLOYER]);

      expect(fs.mkdirSync).toHaveBeenCalledWith(
        expect.stringContaining('.config/mcp-wizard'),
        { recursive: true }
      );
    });

    test('writes config to temp file then renames atomically', () => {
      saveConfig(VALID_CONFIG_[REDACTED_EMPLOYER]);

      expect(fs.writeFileSync).toHaveBeenCalledWith(
        expect.stringContaining('.tmp'),
        expect.any(String),
        'utf8'
      );
      expect(fs.renameSync).toHaveBeenCalledWith(
        expect.stringContaining('.tmp'),
        expect.stringContaining('config.json')
      );
    });

    test('validates config before writing', () => {
      expect(() => saveConfig(INVALID_CONFIG_MISSING_COMPANY)).toThrow('Missing required field: company');
      expect(fs.writeFileSync).not.toHaveBeenCalled();
    });

    test('cleans up temp file on error', () => {
      (fs.writeFileSync as jest.Mock).mockImplementation(() => {
        throw new Error('Write failed');
      });
      (fs.existsSync as jest.Mock).mockReturnValue(true);

      expect(() => saveConfig(VALID_CONFIG_[REDACTED_EMPLOYER])).toThrow('Write failed');
      expect(fs.unlinkSync).toHaveBeenCalledWith(expect.stringContaining('.tmp'));
    });

    test('writes JSON with 2-space indentation', () => {
      saveConfig(VALID_CONFIG_[REDACTED_EMPLOYER]);

      const writeCall = (fs.writeFileSync as jest.Mock).mock.calls[0];
      const writtenContent = writeCall[1];
      expect(writtenContent).toContain('\n  '); // 2-space indent
    });
  });

  describe('validateConfig', () => {
    test('valid config passes without throwing', () => {
      expect(() => validateConfig(VALID_CONFIG_[REDACTED_EMPLOYER])).not.toThrow();
      expect(() => validateConfig(VALID_CONFIG_ACME)).not.toThrow();
    });

    test('missing company field throws error', () => {
      expect(() => validateConfig(INVALID_CONFIG_MISSING_COMPANY)).toThrow('Missing required field: company');
    });

    test('empty company.name throws error', () => {
      expect(() => validateConfig(INVALID_CONFIG_EMPTY_NAME)).toThrow('must be a non-empty string');
    });

    test('company.glean_instance with spaces throws error', () => {
      expect(() => validateConfig(INVALID_CONFIG_SPACES_IN_GLEAN)).toThrow('no spaces allowed');
    });

    test('company.glean_instance uppercase throws error', () => {
      expect(() => validateConfig(INVALID_CONFIG_UPPERCASE_GLEAN)).toThrow('must be lowercase');
    });

    test('company.glean_instance non-string throws error', () => {
      const invalidConfig: any = {
        company: {
          name: 'Test',
          glean_instance: 123, // Number instead of string
          okta_domain: 'test.okta.com',
        },
      };
      expect(() => validateConfig(invalidConfig)).toThrow('must be a string');
    });

    test('company.okta_domain missing dot throws error', () => {
      expect(() => validateConfig(INVALID_CONFIG_BAD_OKTA)).toThrow('must be valid domain');
    });

    test('company.okta_domain non-string throws error', () => {
      const invalidConfig: any = {
        company: {
          name: 'Test',
          glean_instance: 'test',
          okta_domain: 123, // Number instead of string
        },
      };
      expect(() => validateConfig(invalidConfig)).toThrow('must be a string');
    });
  });

  describe('getConfigValue', () => {
    const originalEnv = process.env;

    beforeEach(() => {
      jest.resetModules();
      process.env = { ...originalEnv };
      delete process.env.MCP_WIZARD_COMPANY_GLEAN_INSTANCE;

      // Mock loadConfig to return test data
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(VALID_CONFIG_[REDACTED_EMPLOYER]));
    });

    afterEach(() => {
      process.env = originalEnv;
    });

    test('returns env var when set (priority 1)', () => {
      process.env.MCP_WIZARD_COMPANY_GLEAN_INSTANCE = 'env-value';

      const result = getConfigValue('company.glean_instance');

      expect(result).toBe('env-value');
    });

    test('returns file value when env var unset (priority 2)', () => {
      delete process.env.MCP_WIZARD_COMPANY_GLEAN_INSTANCE;

      const result = getConfigValue('company.glean_instance');

      expect(result).toBe('[REDACTED_EMPLOYER]');
    });

    test('returns default value when key missing (priority 3)', () => {
      const result = getConfigValue('nonexistent.key', 'default-value');

      expect(result).toBe('default-value');
    });

    test('handles nested keys correctly', () => {
      const result = getConfigValue('company.name');

      expect(result).toBe('[REDACTED_EMPLOYER]');
    });
  });

  describe('setConfigValue', () => {
    beforeEach(() => {
      delete process.env.XDG_CONFIG_HOME;

      // Mock loadConfig to return test data
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(VALID_CONFIG_[REDACTED_EMPLOYER]));

      // Mock saveConfig dependencies
      (fs.mkdirSync as jest.Mock).mockImplementation();
      (fs.writeFileSync as jest.Mock).mockImplementation();
      (fs.renameSync as jest.Mock).mockImplementation();
    });

    test('updates config and saves to file', () => {
      setConfigValue('company.glean_instance', 'acme');

      expect(fs.writeFileSync).toHaveBeenCalled();
      const writeCall = (fs.writeFileSync as jest.Mock).mock.calls[0];
      const writtenContent = JSON.parse(writeCall[1]);
      expect(writtenContent.company.glean_instance).toBe('acme');
    });

    test('throws error when validation fails', () => {
      expect(() => setConfigValue('company.glean_instance', 'HAS SPACES')).toThrow();
      expect(fs.writeFileSync).not.toHaveBeenCalled();
    });

    test('creates parent keys when missing', () => {
      setConfigValue('company.name', 'New Company');

      expect(fs.writeFileSync).toHaveBeenCalled();
      const writeCall = (fs.writeFileSync as jest.Mock).mock.calls[0];
      const writtenContent = JSON.parse(writeCall[1]);
      expect(writtenContent.company.name).toBe('New Company');
    });
  });
});
