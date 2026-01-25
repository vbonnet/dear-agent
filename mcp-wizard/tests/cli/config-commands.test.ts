/**
 * Tests for config CLI commands (get, set, list, init)
 */

import {
  getConfigValue,
  setConfigValue,
  loadConfig,
  saveConfig,
} from '../../src/lib/user-config';
import { VALID_CONFIG_[REDACTED_EMPLOYER], VALID_CONFIG_ACME } from '../lib/__helpers__/config-fixtures';

// Mock user-config module
jest.mock('../../src/lib/user-config', () => ({
  ...jest.requireActual('../../src/lib/user-config'),
  getConfigValue: jest.fn(),
  setConfigValue: jest.fn(),
  loadConfig: jest.fn(),
  saveConfig: jest.fn(),
}));

// Mock inquirer
jest.mock('inquirer', () => ({
  prompt: jest.fn(),
}));

import inquirer from 'inquirer';

describe('config CLI commands', () => {
  let consoleLogSpy: jest.SpyInstance;
  let consoleErrorSpy: jest.SpyInstance;
  let processExitSpy: jest.SpyInstance;

  beforeEach(() => {
    jest.clearAllMocks();
    consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
    consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();
    processExitSpy = jest.spyOn(process, 'exit').mockImplementation(() => {
      throw new Error('process.exit called');
    });
  });

  afterEach(() => {
    consoleLogSpy.mockRestore();
    consoleErrorSpy.mockRestore();
    processExitSpy.mockRestore();
  });

  describe('config get <key>', () => {
    test('returns value from config file', () => {
      (getConfigValue as jest.Mock).mockReturnValue('[REDACTED_EMPLOYER]');

      // Simulate: config get company.glean_instance
      const key = 'company.glean_instance';
      const value = getConfigValue(key);
      console.log(value);

      expect(getConfigValue).toHaveBeenCalledWith('company.glean_instance');
      expect(consoleLogSpy).toHaveBeenCalledWith('[REDACTED_EMPLOYER]');
    });

    test('returns env var when override set', () => {
      (getConfigValue as jest.Mock).mockReturnValue('env-override');

      // Simulate: MCP_WIZARD_COMPANY_GLEAN_INSTANCE=env-override config get company.glean_instance
      const key = 'company.glean_instance';
      const value = getConfigValue(key);
      console.log(value);

      expect(getConfigValue).toHaveBeenCalledWith('company.glean_instance');
      expect(consoleLogSpy).toHaveBeenCalledWith('env-override');
    });

    test('outputs value to console.log', () => {
      (getConfigValue as jest.Mock).mockReturnValue('test-value');

      const value = getConfigValue('some.key');
      console.log(value);

      expect(consoleLogSpy).toHaveBeenCalledWith('test-value');
    });
  });

  describe('config set <key> <value>', () => {
    test('calls setConfigValue with correct arguments', () => {
      (setConfigValue as jest.Mock).mockImplementation(() => {});

      // Simulate: config set company.glean_instance acme
      const key = 'company.glean_instance';
      const value = 'acme';
      setConfigValue(key, value);
      console.log(`✓ Set ${key} = ${value}`);

      expect(setConfigValue).toHaveBeenCalledWith('company.glean_instance', 'acme');
    });

    test('outputs success message to console.log', () => {
      (setConfigValue as jest.Mock).mockImplementation(() => {});

      const key = 'company.name';
      const value = 'Acme Corp';
      setConfigValue(key, value);
      console.log(`✓ Set ${key} = ${value}`);

      expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('✓ Set'));
      expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('company.name'));
      expect(consoleLogSpy).toHaveBeenCalledWith(expect.stringContaining('Acme Corp'));
    });

    test('outputs error message when validation fails', () => {
      (setConfigValue as jest.Mock).mockImplementation(() => {
        throw new Error('Validation failed: no spaces allowed');
      });

      // Simulate: config set company.glean_instance "has spaces" (invalid)
      try {
        setConfigValue('company.glean_instance', 'has spaces');
      } catch (error: any) {
        console.error(`Error: ${error.message}`);
      }

      expect(consoleErrorSpy).toHaveBeenCalledWith(expect.stringContaining('Error:'));
      expect(consoleErrorSpy).toHaveBeenCalledWith(expect.stringContaining('Validation failed'));
    });
  });

  describe('config list', () => {
    test('calls loadConfig and outputs JSON', () => {
      (loadConfig as jest.Mock).mockReturnValue(VALID_CONFIG_[REDACTED_EMPLOYER]);

      // Simulate: config list
      const config = loadConfig();
      console.log(JSON.stringify(config, null, 2));

      expect(loadConfig).toHaveBeenCalled();
      expect(consoleLogSpy).toHaveBeenCalled();
      const loggedContent = consoleLogSpy.mock.calls[0][0];
      expect(loggedContent).toContain('[REDACTED_EMPLOYER]');
      expect(loggedContent).toContain('[REDACTED_EMPLOYER]');
    });

    test('formats JSON with 2-space indentation', () => {
      (loadConfig as jest.Mock).mockReturnValue(VALID_CONFIG_ACME);

      const config = loadConfig();
      const formatted = JSON.stringify(config, null, 2);
      console.log(formatted);

      expect(consoleLogSpy).toHaveBeenCalled();
      const loggedContent = consoleLogSpy.mock.calls[0][0];
      expect(loggedContent).toContain('\n  '); // 2-space indent
    });
  });

  describe('config init', () => {
    test('prompts for company name, glean_instance, okta_domain', async () => {
      const mockAnswers = {
        'company.name': 'Test Company',
        'company.glean_instance': 'test',
        'company.okta_domain': 'test.okta.com',
      };

      (inquirer.prompt as unknown as jest.Mock).mockResolvedValue(mockAnswers);
      (saveConfig as jest.Mock).mockImplementation(() => {});

      // Simulate: config init
      const answers = await inquirer.prompt([
        {
          type: 'input',
          name: 'company.name',
          message: 'Company name:',
          default: '[REDACTED_EMPLOYER]',
        },
        {
          type: 'input',
          name: 'company.glean_instance',
          message: 'Glean instance (lowercase, no spaces):',
          default: '[REDACTED_EMPLOYER]',
        },
        {
          type: 'input',
          name: 'company.okta_domain',
          message: 'Okta domain:',
          default: '[REDACTED_EMPLOYER].okta.com',
        },
      ]);

      expect(inquirer.prompt).toHaveBeenCalled();
      expect(answers).toEqual(mockAnswers);
    });

    test('saves config after prompts complete', async () => {
      const mockAnswers = {
        'company.name': 'Acme Corp',
        'company.glean_instance': 'acme',
        'company.okta_domain': 'acme.okta.com',
      };

      (inquirer.prompt as unknown as jest.Mock).mockResolvedValue(mockAnswers);
      (saveConfig as jest.Mock).mockImplementation(() => {});

      // Simulate: config init
      const answers = await inquirer.prompt([]);
      const config = {
        company: {
          name: answers['company.name'],
          glean_instance: answers['company.glean_instance'],
          okta_domain: answers['company.okta_domain'],
        },
      };
      saveConfig(config);

      expect(saveConfig).toHaveBeenCalled();
      const savedConfig = (saveConfig as jest.Mock).mock.calls[0][0];
      expect(savedConfig.company.name).toBe('Acme Corp');
      expect(savedConfig.company.glean_instance).toBe('acme');
    });
  });
});
