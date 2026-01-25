/**
 * Unit tests for gcp-setup.ts
 * Tests GCP OAuth setup guide workflow
 */

import inquirer from 'inquirer';
import * as fs from 'fs/promises';
import * as open from 'open';
import { runGcpSetupGuide } from '../../src/guides/gcp-setup';
import { MOCK_GCP_CREDENTIALS } from '../fixtures/guide-fixtures';

jest.mock('inquirer');
jest.mock('open');
jest.mock('fs/promises');
jest.mock('../../src/lib/oauth');

describe('runGcpSetupGuide', () => {
  beforeEach(() => {
    jest.clearAllMocks();

    // Mock fs operations
    (fs.readdir as jest.Mock).mockResolvedValue(['client_secret_123.json']);
    (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(MOCK_GCP_CREDENTIALS));
    (fs.copyFile as jest.Mock).mockResolvedValue(undefined);
    (fs.mkdir as jest.Mock).mockResolvedValue(undefined);
  });

  describe('Happy path', () => {
    test('successfully guides user through GCP setup with browser opening', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;
      const mockOpen = open.default as jest.MockedFunction<typeof open.default>;

      // Mock user responses through all steps
      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })  // Step 1: Enable APIs
        .mockResolvedValueOnce({ confirm: '' })  // Wait for APIs enabled
        .mockResolvedValueOnce({ openBrowser: true })  // Step 2: OAuth consent
        .mockResolvedValueOnce({ confirm: '' })  // Wait for consent configured
        .mockResolvedValueOnce({ openBrowser: true })  // Step 3: Create credentials
        .mockResolvedValueOnce({ confirm: '' })  // Wait for credentials downloaded
        .mockResolvedValueOnce({ credentialsPath: '~/Downloads/client_secret_123.json' });  // Step 4: Provide path

      // Mock validateCredentials to succeed
      const { validateCredentials } = require('../../src/lib/oauth');
      validateCredentials.mockReturnValue(true);

      const credentialsPath = await runGcpSetupGuide();

      // Verify browser was opened for all steps
      expect(mockOpen).toHaveBeenCalledTimes(3);
      expect(mockOpen).toHaveBeenCalledWith(expect.stringContaining('apis/library'));
      expect(mockOpen).toHaveBeenCalledWith(expect.stringContaining('credentials/consent'));
      expect(mockOpen).toHaveBeenCalledWith(expect.stringContaining('apis/credentials'));

      // Verify credentials path returned
      expect(credentialsPath).toContain('gcp-credentials.json');
    });

    test('allows user to skip opening browser for all steps', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;
      const mockOpen = open.default as jest.MockedFunction<typeof open.default>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: false })  // Skip all browsers
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: false })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: false })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ credentialsPath: '~/Downloads/client_secret_123.json' });

      const { validateCredentials } = require('../../src/lib/oauth');
      validateCredentials.mockReturnValue(true);

      await runGcpSetupGuide();

      // Verify browser was not opened
      expect(mockOpen).not.toHaveBeenCalled();
    });
  });

  describe('Credentials validation', () => {
    test('validates credentials file format', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ credentialsPath: '~/Downloads/client_secret_123.json' });

      const { validateCredentials } = require('../../src/lib/oauth');
      validateCredentials.mockReturnValue(true);

      await runGcpSetupGuide();

      // Verify validateCredentials was called
      expect(validateCredentials).toHaveBeenCalled();
    });

    test('rejects invalid credentials file', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      // Mock invalid credentials
      (fs.readFile as jest.Mock).mockResolvedValue('invalid json');

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ credentialsPath: '~/Downloads/invalid.json' });

      // Should throw error for invalid JSON
      await expect(runGcpSetupGuide()).rejects.toThrow();
    });
  });

  describe('File operations', () => {
    test('handles wildcard in credentials path', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ credentialsPath: '~/Downloads/client_secret_*.json' });

      const { validateCredentials } = require('../../src/lib/oauth');
      validateCredentials.mockReturnValue(true);

      await runGcpSetupGuide();

      // Verify readdir was called to resolve wildcard
      expect(fs.readdir).toHaveBeenCalled();
    });

    test('creates target directory if it does not exist', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ credentialsPath: '~/Downloads/client_secret_123.json' });

      const { validateCredentials } = require('../../src/lib/oauth');
      validateCredentials.mockReturnValue(true);

      await runGcpSetupGuide();

      // Verify mkdir was called
      expect(fs.mkdir).toHaveBeenCalled();
    });

    test('handles file read errors gracefully', async () => {
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      // Mock fs.readFile to throw error
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('File not found'));

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ credentialsPath: '~/Downloads/missing.json' });

      await expect(runGcpSetupGuide()).rejects.toThrow('File not found');
    });
  });

  describe('User interaction', () => {
    test('displays project name in guide', async () => {
      const consoleSpy = jest.spyOn(console, 'log');
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: true })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ credentialsPath: '~/Downloads/client_secret_123.json' });

      const { validateCredentials } = require('../../src/lib/oauth');
      validateCredentials.mockReturnValue(true);

      await runGcpSetupGuide();

      // Verify project name is mentioned
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('shared-dev-ai-pct45x'));

      consoleSpy.mockRestore();
    });

    test('provides manual URLs when browser opening declined', async () => {
      const consoleSpy = jest.spyOn(console, 'log');
      const mockPrompt = inquirer.prompt as jest.MockedFunction<typeof inquirer.prompt>;

      mockPrompt
        .mockResolvedValueOnce({ openBrowser: false })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: false })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ openBrowser: false })
        .mockResolvedValueOnce({ confirm: '' })
        .mockResolvedValueOnce({ credentialsPath: '~/Downloads/client_secret_123.json' });

      const { validateCredentials } = require('../../src/lib/oauth');
      validateCredentials.mockReturnValue(true);

      await runGcpSetupGuide();

      // Verify manual URLs are printed
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('Manually navigate to:'));

      consoleSpy.mockRestore();
    });
  });
});
