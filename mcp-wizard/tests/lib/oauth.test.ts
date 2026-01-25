import * as fs from 'fs/promises';
import { validateCredentials, saveToken, checkGitTracking, ensureGitignore } from '../../src/lib/oauth';

// Mock fs module
jest.mock('fs/promises');

// Mock keytar module (needed for saveToken which uses token-storage)
jest.mock('keytar');

// Mock open module to avoid ESM import issues
jest.mock('open', () => ({
  default: jest.fn(),
}));

// Mock inquirer to avoid ESM import issues
jest.mock('inquirer', () => ({
  default: {
    prompt: jest.fn(),
  },
}));

describe('OAuth Module', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Mock fs.access to reject (no legacy token file exists) to skip migration
    (fs.access as jest.Mock).mockRejectedValue(new Error('File not found'));
  });

  describe('validateCredentials', () => {
    test('validates well-formed credentials.json', async () => {
      const validCreds = {
        installed: {
          client_id: '123456789.apps.googleusercontent.com',
          client_secret: 'GOCSPX-test123',
          redirect_uris: ['urn:ietf:wg:oauth:2.0:oob'],
        },
      };

      await expect(validateCredentials(validCreds)).resolves.toBeUndefined();
    });

    test('rejects credentials without installed section', async () => {
      const badCreds = {
        web: {
          client_id: '123456789.apps.googleusercontent.com',
        },
      };

      await expect(validateCredentials(badCreds)).rejects.toThrow(
        'Missing "installed" section (expected Desktop app credentials)'
      );
    });

    test('rejects credentials without client_id', async () => {
      const badCreds = {
        installed: {
          client_secret: 'GOCSPX-test123',
          redirect_uris: ['urn:ietf:wg:oauth:2.0:oob'],
        },
      };

      await expect(validateCredentials(badCreds)).rejects.toThrow('Missing client_id');
    });

    test('rejects client_id with wrong format', async () => {
      const badCreds = {
        installed: {
          client_id: 'invalid-client-id',
          client_secret: 'GOCSPX-test123',
          redirect_uris: ['urn:ietf:wg:oauth:2.0:oob'],
        },
      };

      await expect(validateCredentials(badCreds)).rejects.toThrow(
        'client_id must end with .apps.googleusercontent.com'
      );
    });

    test('rejects credentials without client_secret', async () => {
      const badCreds = {
        installed: {
          client_id: '123456789.apps.googleusercontent.com',
          redirect_uris: ['urn:ietf:wg:oauth:2.0:oob'],
        },
      };

      await expect(validateCredentials(badCreds)).rejects.toThrow('Missing client_secret');
    });

    test('rejects credentials without redirect_uris', async () => {
      const badCreds = {
        installed: {
          client_id: '123456789.apps.googleusercontent.com',
          client_secret: 'GOCSPX-test123',
        },
      };

      await expect(validateCredentials(badCreds)).rejects.toThrow('Missing redirect_uris');
    });
  });

  describe('saveToken', () => {
    beforeEach(() => {
      jest.clearAllMocks();
      // Ensure fs.access rejects (no migration file)
      (fs.access as jest.Mock).mockRejectedValue(new Error('File not found'));
    });

    test('saves token to OS keychain (not filesystem)', async () => {
      const mockTokens = {
        access_token: 'ya29.test123',
        refresh_token: '1//test-refresh',
        expiry_date: Date.now() + 3600000,
      };

      const mockCredentials = {
        installed: {
          client_id: '123456789.apps.googleusercontent.com',
          client_secret: 'GOCSPX-test123',
          redirect_uris: ['urn:ietf:wg:oauth:2.0:oob'],
        },
      };

      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();

      await saveToken(mockTokens, '/tmp/test-token.json', mockCredentials);

      // Verify token was stored in keychain (via keytar mock)
      const keytar = require('keytar');
      expect(keytar.setPassword).toHaveBeenCalledWith(
        'mcp-wizard-google-oauth',
        'client-id',
        '123456789.apps.googleusercontent.com'
      );
      expect(keytar.setPassword).toHaveBeenCalledWith(
        'mcp-wizard-google-oauth',
        'client-secret',
        'GOCSPX-test123'
      );
      expect(keytar.setPassword).toHaveBeenCalledWith(
        'mcp-wizard-google-oauth',
        'refresh-token',
        '1//test-refresh'
      );

      // Verify success message
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Token stored securely in OS keychain');

      // Verify no filesystem writes (new behavior - keychain only)
      expect(fs.writeFile).not.toHaveBeenCalled();

      consoleLogSpy.mockRestore();
    });

    test('throws error if keychain is unavailable', async () => {
      const mockTokens = {
        refresh_token: '1//test-refresh',
      };

      const mockCredentials = {
        installed: {
          client_id: '123456789.apps.googleusercontent.com',
          client_secret: 'GOCSPX-test123',
          redirect_uris: ['urn:ietf:wg:oauth:2.0:oob'],
        },
      };

      // Mock keychain failure (retry logic attempts 3 times, so mock 3 failures)
      const keytar = require('keytar');
      keytar.setPassword
        .mockRejectedValueOnce(new Error('Keychain unavailable'))
        .mockRejectedValueOnce(new Error('Keychain unavailable'))
        .mockRejectedValueOnce(new Error('Keychain unavailable'));

      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      await expect(saveToken(mockTokens, '/tmp/test-token.json', mockCredentials))
        .rejects.toThrow('Keychain unavailable');

      // Verify error was logged
      expect(consoleErrorSpy).toHaveBeenCalledWith(
        'Failed to store token in keychain:',
        expect.stringContaining('Keychain')
      );

      consoleErrorSpy.mockRestore();
    });
  });

  describe('checkGitTracking', () => {
    test('placeholder - requires child_process mocking', async () => {
      // This test would require mocking child_process.exec
      // For now, we'll skip implementation as it requires more complex mocking
      expect(true).toBe(true);
    });
  });

  describe('ensureGitignore', () => {
    beforeEach(() => {
      jest.clearAllMocks();
    });

    test('adds entry to .gitignore if not present', async () => {
      (fs.readFile as jest.Mock).mockResolvedValue('node_modules/\ndist/\n');
      (fs.appendFile as jest.Mock).mockResolvedValue(undefined);

      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();

      await ensureGitignore('credentials.json', '.gitignore');

      expect(fs.appendFile).toHaveBeenCalledWith('.gitignore', '\ncredentials.json\n');
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Added credentials.json to .gitignore');

      consoleLogSpy.mockRestore();
    });

    test('does not add duplicate entry to .gitignore', async () => {
      (fs.readFile as jest.Mock).mockResolvedValue('node_modules/\ncredentials.json\ndist/\n');

      await ensureGitignore('credentials.json', '.gitignore');

      expect(fs.appendFile).not.toHaveBeenCalled();
    });

    test('creates .gitignore if it does not exist', async () => {
      (fs.readFile as jest.Mock).mockRejectedValue(new Error('File not found'));
      (fs.appendFile as jest.Mock).mockResolvedValue(undefined);

      const consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();

      await ensureGitignore('token.json', '.gitignore');

      expect(fs.appendFile).toHaveBeenCalledWith('.gitignore', '\ntoken.json\n');
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Added token.json to .gitignore');

      consoleLogSpy.mockRestore();
    });
  });
});
