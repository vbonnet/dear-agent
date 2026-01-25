/**
 * Integration tests for config system
 * Tests end-to-end workflows with minimal mocking
 */

import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import {
  getUserConfigPath,
  loadConfig,
  saveConfig,
  getConfigValue,
  setConfigValue,
  UserConfig,
} from '../../src/lib/user-config';
import { VALID_CONFIG_ACME } from '../lib/__helpers__/config-fixtures';

// Mock only fs (to avoid real file I/O)
jest.mock('fs');
jest.mock('os');
jest.mock('path');

describe('config system integration', () => {
  const mockConfigPath = '/home/testuser/.config/mcp-wizard/config.json';
  const mockHomeDir = '/home/testuser';
  const originalEnv = process.env;

  let consoleLogSpy: jest.SpyInstance;
  let consoleWarnSpy: jest.SpyInstance;

  beforeEach(() => {
    jest.clearAllMocks();
    jest.resetModules();
    process.env = { ...originalEnv };

    // Mock os and path
    (os.homedir as jest.Mock).mockReturnValue(mockHomeDir);
    (path.join as jest.Mock).mockImplementation((...parts: string[]) => parts.join('/'));
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
    process.env = originalEnv;
    consoleLogSpy.mockRestore();
    consoleWarnSpy.mockRestore();
  });

  describe('Full config lifecycle', () => {
    test('init → set → get → list workflow', () => {
      // Step 1: Init (create default config)
      (fs.existsSync as jest.Mock).mockReturnValue(false);
      (fs.mkdirSync as jest.Mock).mockImplementation();
      (fs.writeFileSync as jest.Mock).mockImplementation();
      (fs.renameSync as jest.Mock).mockImplementation();

      const initialConfig = loadConfig();
      expect(initialConfig.company.glean_instance).toBe('[REDACTED_EMPLOYER]'); // Default

      // Step 2: Set (update config)
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(initialConfig));

      setConfigValue('company.glean_instance', 'acme');

      expect(fs.writeFileSync).toHaveBeenCalled();
      // Get the last write call (setConfigValue makes multiple writes)
      const writeCall = (fs.writeFileSync as jest.Mock).mock.calls[(fs.writeFileSync as jest.Mock).mock.calls.length - 1];
      const writtenContent = JSON.parse(writeCall[1]);
      expect(writtenContent.company.glean_instance).toBe('acme');

      // Step 3: Get (retrieve value)
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify(writtenContent));

      const value = getConfigValue('company.glean_instance');
      expect(value).toBe('acme');

      // Step 4: List (show all config)
      const finalConfig = loadConfig();
      expect(finalConfig.company.glean_instance).toBe('acme');
      expect(finalConfig.company.name).toBe('[REDACTED_EMPLOYER]'); // Unchanged
    });
  });

  describe('Environment variable overrides', () => {
    test('MCP_WIZARD_COMPANY_GLEAN_INSTANCE overrides file value', () => {
      // Setup: Config file has '[REDACTED_EMPLOYER]'
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(
        JSON.stringify({ company: { name: '[REDACTED_EMPLOYER]', glean_instance: '[REDACTED_EMPLOYER]', okta_domain: '[REDACTED_EMPLOYER].okta.com' } })
      );

      // Override with env var
      process.env.MCP_WIZARD_COMPANY_GLEAN_INSTANCE = 'acme';

      const value = getConfigValue('company.glean_instance');

      expect(value).toBe('acme'); // Env var wins
    });

    test('multiple env vars override multiple file values', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(
        JSON.stringify({ company: { name: '[REDACTED_EMPLOYER]', glean_instance: '[REDACTED_EMPLOYER]', okta_domain: '[REDACTED_EMPLOYER].okta.com' } })
      );

      // Multiple overrides
      process.env.MCP_WIZARD_COMPANY_GLEAN_INSTANCE = 'acme';
      process.env.MCP_WIZARD_COMPANY_NAME = 'Acme Corp';

      const gleanValue = getConfigValue('company.glean_instance');
      const nameValue = getConfigValue('company.name');

      expect(gleanValue).toBe('acme');
      expect(nameValue).toBe('Acme Corp');
    });
  });

  describe('XDG directory resolution', () => {
    test('config saved to XDG_CONFIG_HOME when set', () => {
      process.env.XDG_CONFIG_HOME = '/custom/config';
      (fs.mkdirSync as jest.Mock).mockImplementation();
      (fs.writeFileSync as jest.Mock).mockImplementation();
      (fs.renameSync as jest.Mock).mockImplementation();
      (fs.existsSync as jest.Mock).mockReturnValue(false);

      saveConfig(VALID_CONFIG_ACME);

      expect(fs.mkdirSync).toHaveBeenCalledWith('/custom/config/mcp-wizard', { recursive: true });
      expect(fs.writeFileSync).toHaveBeenCalledWith(
        expect.stringContaining('/custom/config/mcp-wizard/config.json.tmp'),
        expect.any(String),
        'utf8'
      );
    });

    test('config saved to ~/.config when XDG_CONFIG_HOME unset', () => {
      delete process.env.XDG_CONFIG_HOME;
      (fs.mkdirSync as jest.Mock).mockImplementation();
      (fs.writeFileSync as jest.Mock).mockImplementation();
      (fs.renameSync as jest.Mock).mockImplementation();
      (fs.existsSync as jest.Mock).mockReturnValue(false);

      saveConfig(VALID_CONFIG_ACME);

      expect(fs.mkdirSync).toHaveBeenCalledWith(`${mockHomeDir}/.config/mcp-wizard`, { recursive: true });
      expect(fs.writeFileSync).toHaveBeenCalledWith(
        expect.stringContaining(`${mockHomeDir}/.config/mcp-wizard/config.json.tmp`),
        expect.any(String),
        'utf8'
      );
    });
  });

  describe('Graceful degradation', () => {
    test('invalid config file → loadConfig returns defaults', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue(JSON.stringify({ invalid: 'config' }));

      const config = loadConfig();

      expect(config.company.glean_instance).toBe('[REDACTED_EMPLOYER]'); // Default
      expect(consoleWarnSpy).toHaveBeenCalledWith(expect.stringContaining('Failed to load config'));
    });

    test('malformed JSON → loadConfig returns defaults', () => {
      (fs.existsSync as jest.Mock).mockReturnValue(true);
      (fs.readFileSync as jest.Mock).mockReturnValue('{invalid json}');

      const config = loadConfig();

      expect(config.company.glean_instance).toBe('[REDACTED_EMPLOYER]'); // Default
      expect(consoleWarnSpy).toHaveBeenCalledWith(expect.stringContaining('not valid JSON'));
    });
  });
});
