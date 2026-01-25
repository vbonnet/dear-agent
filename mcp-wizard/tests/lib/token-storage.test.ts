/**
 * Unit tests for token-storage module
 * Uses mocked keytar to avoid requiring real OS keychain
 */

import * as fs from 'fs/promises';
import * as os from 'os';
import * as path from 'path';
import {
  storeOktaToken,
  getOktaToken,
  deleteOktaToken,
  migrateTokensToKeychain,
  TokenResponse,
} from '../../src/lib/token-storage';
import * as keytar from 'keytar';

// Mock keytar module
jest.mock('keytar');

// Mock fs/promises module for migration tests
jest.mock('fs/promises');

describe('Token Storage Module', () => {
  // Import mock helper
  const { __clearMockStore } = jest.requireMock('keytar');

  beforeEach(() => {
    // Clear mock store before each test
    if (__clearMockStore) {
      __clearMockStore();
    }
    jest.clearAllMocks();
  });

  describe('storeOktaToken', () => {
    test('stores all required fields in keychain', async () => {
      const token: TokenResponse = {
        type: 'authorized_user',
        client_id: '123456789.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test123',
        refresh_token: '1//test-refresh-token',
      };

      await storeOktaToken(token);

      // Verify all fields were stored
      expect(keytar.setPassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'type', 'authorized_user');
      expect(keytar.setPassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'client-id', '123456789.apps.googleusercontent.com');
      expect(keytar.setPassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'client-secret', 'GOCSPX-test123');
      expect(keytar.setPassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'refresh-token', '1//test-refresh-token');
    });

    test('stores optional fields if present', async () => {
      const token: TokenResponse = {
        type: 'authorized_user',
        client_id: '123456789.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test123',
        refresh_token: '1//test-refresh-token',
        access_token: 'ya29.test-access-token',
        expires_at: 1234567890,
      };

      await storeOktaToken(token);

      // Verify optional fields were stored
      expect(keytar.setPassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'access-token', 'ya29.test-access-token');
      expect(keytar.setPassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'expires-at', '1234567890');
    });

    test('throws error if keychain unavailable (libsecret missing)', async () => {
      const token: TokenResponse = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//test',
      };

      // Mock keytar to throw libsecret error (reject all retry attempts)
      (keytar.setPassword as jest.Mock).mockRejectedValue(
        new Error('Error: Cannot find module libsecret')
      );

      await expect(storeOktaToken(token)).rejects.toThrow('Keychain service unavailable');
      await expect(storeOktaToken(token)).rejects.toThrow('Install libsecret');
    });
  });

  describe('getOktaToken', () => {
    test('retrieves all required fields from keychain', async () => {
      // Mock keytar responses
      (keytar.getPassword as jest.Mock).mockImplementation(
        async (service: string, account: string) => {
          const mockData: Record<string, string> = {
            'type': 'authorized_user',
            'client-id': '123456789.apps.googleusercontent.com',
            'client-secret': 'GOCSPX-test123',
            'refresh-token': '1//test-refresh-token',
          };
          return mockData[account] || null;
        }
      );

      const token = await getOktaToken();

      expect(token.type).toBe('authorized_user');
      expect(token.client_id).toBe('123456789.apps.googleusercontent.com');
      expect(token.client_secret).toBe('GOCSPX-test123');
      expect(token.refresh_token).toBe('1//test-refresh-token');
    });

    test('retrieves optional fields if present', async () => {
      // Mock keytar responses with optional fields
      (keytar.getPassword as jest.Mock).mockImplementation(
        async (service: string, account: string) => {
          const mockData: Record<string, string> = {
            'type': 'authorized_user',
            'client-id': '123.apps.googleusercontent.com',
            'client-secret': 'GOCSPX-test',
            'refresh-token': '1//test',
            'access-token': 'ya29.test',
            'expires-at': '1234567890',
          };
          return mockData[account] || null;
        }
      );

      const token = await getOktaToken();

      expect(token.access_token).toBe('ya29.test');
      expect(token.expires_at).toBe(1234567890);
    });

    test('throws error when token not found', async () => {
      // Mock keytar to return null (token not found)
      (keytar.getPassword as jest.Mock).mockResolvedValue(null);

      await expect(getOktaToken()).rejects.toThrow('No token found - run: mcp-wizard auth');
    });

    test('throws error when required fields missing', async () => {
      // Mock partial data (missing refresh_token)
      (keytar.getPassword as jest.Mock).mockImplementation(
        async (service: string, account: string) => {
          const mockData: Record<string, string | null> = {
            'type': 'authorized_user',
            'client-id': '123.apps.googleusercontent.com',
            'client-secret': 'GOCSPX-test',
            'refresh-token': null, // Missing!
          };
          return mockData[account] || null;
        }
      );

      await expect(getOktaToken()).rejects.toThrow('No token found');
    });
  });

  describe('deleteOktaToken', () => {
    test('deletes all token fields from keychain', async () => {
      await deleteOktaToken();

      // Verify all fields were deleted
      expect(keytar.deletePassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'type');
      expect(keytar.deletePassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'client-id');
      expect(keytar.deletePassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'client-secret');
      expect(keytar.deletePassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'refresh-token');
      expect(keytar.deletePassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'access-token');
      expect(keytar.deletePassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'expires-at');
    });

    test('is idempotent - does not throw if token does not exist', async () => {
      // Mock deletePassword to return false (didn't exist)
      (keytar.deletePassword as jest.Mock).mockResolvedValue(false);

      await expect(deleteOktaToken()).resolves.not.toThrow();
    });

    test('does not throw if keychain service unavailable', async () => {
      // Mock deletePassword to throw error
      (keytar.deletePassword as jest.Mock).mockRejectedValue(new Error('Service not found'));

      await expect(deleteOktaToken()).resolves.not.toThrow();
    });
  });

  describe('migrateTokensToKeychain', () => {
    const mockTokenPath = path.join(os.homedir(), '.config', 'mcp-wizard', 'okta-token.json');

    test('returns false when no plaintext file exists', async () => {
      // Mock fs.access to throw (file doesn't exist)
      (fs.access as jest.Mock).mockRejectedValue(new Error('File not found'));

      const result = await migrateTokensToKeychain();

      expect(result).toBe(false);
      expect(fs.readFile).not.toHaveBeenCalled();
    });

    test('migrates plaintext token to keychain successfully', async () => {
      const mockPlaintextToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//test-refresh',
      };

      // Mock file exists
      (fs.access as jest.Mock).mockResolvedValue(undefined);
      // Mock file read
      (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(mockPlaintextToken));
      // Mock file delete
      (fs.unlink as jest.Mock).mockResolvedValue(undefined);

      const result = await migrateTokensToKeychain();

      expect(result).toBe(true);
      expect(fs.readFile).toHaveBeenCalledWith(mockTokenPath, 'utf-8');
      expect(keytar.setPassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'refresh-token', '1//test-refresh');
      expect(fs.unlink).toHaveBeenCalledWith(mockTokenPath);
    });

    test('migrates token with optional fields', async () => {
      const mockPlaintextToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//test-refresh',
        access_token: 'ya29.test',
        expires_at: 1234567890,
      };

      (fs.access as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(mockPlaintextToken));
      (fs.unlink as jest.Mock).mockResolvedValue(undefined);

      const result = await migrateTokensToKeychain();

      expect(result).toBe(true);
      expect(keytar.setPassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'access-token', 'ya29.test');
      expect(keytar.setPassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'expires-at', '1234567890');
    });

    test('throws error on corrupted JSON', async () => {
      (fs.access as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockResolvedValue('{ invalid json');

      await expect(migrateTokensToKeychain()).rejects.toThrow('Corrupted token file');
    });

    test('throws error on invalid token structure (missing refresh_token)', async () => {
      const mockInvalidToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        // Missing refresh_token and client_secret
      };

      (fs.access as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(mockInvalidToken));

      await expect(migrateTokensToKeychain()).rejects.toThrow('Invalid token structure');
    });

    test('does not delete plaintext file if keychain storage fails', async () => {
      const mockPlaintextToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//test-refresh',
      };

      (fs.access as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(mockPlaintextToken));
      // Reject all retry attempts (not just once)
      (keytar.setPassword as jest.Mock).mockRejectedValue(new Error('Keychain unavailable'));

      await expect(migrateTokensToKeychain()).rejects.toThrow();
      expect(fs.unlink).not.toHaveBeenCalled();
    });

    test('defaults type to "authorized_user" if missing', async () => {
      const mockPlaintextToken = {
        // Missing type field
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//test-refresh',
      };

      (fs.access as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(mockPlaintextToken));
      (fs.unlink as jest.Mock).mockResolvedValue(undefined);

      const result = await migrateTokensToKeychain();

      expect(result).toBe(true);
      expect(keytar.setPassword).toHaveBeenCalledWith('mcp-wizard-google-oauth', 'type', 'authorized_user');
    });
  });
});
