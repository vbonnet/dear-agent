/**
 * Integration tests for token migration flow
 * Tests the full migration path: plaintext file → keychain → file deletion
 */

import * as fs from 'fs/promises';
import * as os from 'os';
import * as path from 'path';
import {
  storeOktaToken,
  getOktaToken,
  migrateTokensToKeychain,
  TokenResponse,
} from '../../src/lib/token-storage';
import * as keytar from 'keytar';

// Mock keytar module
jest.mock('keytar');

// Mock fs/promises module
jest.mock('fs/promises');

describe('Token Migration Integration Tests', () => {
  const mockTokenPath = path.join(os.homedir(), '.config', 'mcp-wizard', 'okta-token.json');
  const { __clearMockStore } = jest.requireMock('keytar');

  beforeEach(() => {
    // Clear mock store
    if (__clearMockStore) {
      __clearMockStore();
    }
    jest.clearAllMocks();
  });

  describe('Full Migration Flow', () => {
    test('successfully migrates plaintext token to keychain and deletes file', async () => {
      const mockPlaintextToken = {
        type: 'authorized_user',
        client_id: '123456789.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test123',
        refresh_token: '1//test-refresh-token',
      };

      // Mock file exists
      (fs.access as jest.Mock).mockResolvedValue(undefined);
      // Mock file read
      (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(mockPlaintextToken));
      // Mock file delete
      (fs.unlink as jest.Mock).mockResolvedValue(undefined);

      // Perform migration
      const migrated = await migrateTokensToKeychain();

      // Verify migration occurred
      expect(migrated).toBe(true);

      // Verify file was read
      expect(fs.readFile).toHaveBeenCalledWith(mockTokenPath, 'utf-8');

      // Verify token was stored in keychain
      expect(keytar.setPassword).toHaveBeenCalledWith(
        'mcp-wizard-google-oauth',
        'refresh-token',
        '1//test-refresh-token'
      );

      // Verify plaintext file was deleted
      expect(fs.unlink).toHaveBeenCalledWith(mockTokenPath);
    });

    test('migration followed by token retrieval works correctly', async () => {
      const mockPlaintextToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//test-refresh',
      };

      // Mock migration flow
      (fs.access as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(mockPlaintextToken));
      (fs.unlink as jest.Mock).mockResolvedValue(undefined);

      // Perform migration
      await migrateTokensToKeychain();

      // Now mock keytar retrieval (simulating token in keychain)
      (keytar.getPassword as jest.Mock).mockImplementation(
        async (service: string, account: string) => {
          const mockData: Record<string, string> = {
            'type': 'authorized_user',
            'client-id': '123.apps.googleusercontent.com',
            'client-secret': 'GOCSPX-test',
            'refresh-token': '1//test-refresh',
          };
          return mockData[account] || null;
        }
      );

      // Retrieve token from keychain
      const retrievedToken = await getOktaToken();

      // Verify retrieved token matches original
      expect(retrievedToken.refresh_token).toBe('1//test-refresh');
      expect(retrievedToken.client_id).toBe('123.apps.googleusercontent.com');
    });
  });

  describe('Edge Cases', () => {
    test('handles missing plaintext file gracefully', async () => {
      // Mock file doesn't exist
      (fs.access as jest.Mock).mockRejectedValue(new Error('ENOENT: file not found'));

      // Perform migration
      const migrated = await migrateTokensToKeychain();

      // Verify no migration occurred
      expect(migrated).toBe(false);

      // Verify file was not read
      expect(fs.readFile).not.toHaveBeenCalled();

      // Verify file was not deleted
      expect(fs.unlink).not.toHaveBeenCalled();
    });

    test('handles corrupted plaintext file (invalid JSON)', async () => {
      // Mock file exists with corrupted content
      (fs.access as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockResolvedValue('{ this is not valid json');

      // Attempt migration
      await expect(migrateTokensToKeychain()).rejects.toThrow('Corrupted token file');

      // Verify file was NOT deleted (user needs to fix/re-auth)
      expect(fs.unlink).not.toHaveBeenCalled();
    });

    test('handles plaintext file with missing required fields', async () => {
      const mockInvalidToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        // Missing client_secret and refresh_token
      };

      (fs.access as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(mockInvalidToken));

      // Attempt migration
      await expect(migrateTokensToKeychain()).rejects.toThrow('Invalid token structure');

      // Verify file was NOT deleted
      expect(fs.unlink).not.toHaveBeenCalled();
    });

    test('preserves plaintext file if keychain storage fails', async () => {
      const mockPlaintextToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//test-refresh',
      };

      (fs.access as jest.Mock).mockResolvedValue(undefined);
      (fs.readFile as jest.Mock).mockResolvedValue(JSON.stringify(mockPlaintextToken));

      // Mock keychain storage failure
      (keytar.setPassword as jest.Mock).mockRejectedValue(new Error('Keychain unavailable'));

      // Attempt migration
      await expect(migrateTokensToKeychain()).rejects.toThrow();

      // Verify file was NOT deleted (failed before storage)
      expect(fs.unlink).not.toHaveBeenCalled();
    });
  });

  describe('Idempotency', () => {
    test('second migration attempt returns false (file already migrated)', async () => {
      const mockPlaintextToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//test-refresh',
      };

      // First migration
      (fs.access as jest.Mock).mockResolvedValueOnce(undefined);
      (fs.readFile as jest.Mock).mockResolvedValueOnce(JSON.stringify(mockPlaintextToken));
      (fs.unlink as jest.Mock).mockResolvedValueOnce(undefined);

      const firstMigration = await migrateTokensToKeychain();
      expect(firstMigration).toBe(true);

      // Second migration attempt (file now deleted)
      (fs.access as jest.Mock).mockRejectedValueOnce(new Error('File not found'));

      const secondMigration = await migrateTokensToKeychain();
      expect(secondMigration).toBe(false);
    });
  });

  describe('Token Lifecycle', () => {
    test('full lifecycle: store → retrieve → delete → verify deleted', async () => {
      const token: TokenResponse = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//test-refresh',
      };

      // Store token
      await storeOktaToken(token);

      expect(keytar.setPassword).toHaveBeenCalledWith(
        'mcp-wizard-google-oauth',
        'refresh-token',
        '1//test-refresh'
      );

      // Mock retrieval
      (keytar.getPassword as jest.Mock).mockImplementation(
        async (service: string, account: string) => {
          const mockData: Record<string, string> = {
            'type': 'authorized_user',
            'client-id': '123.apps.googleusercontent.com',
            'client-secret': 'GOCSPX-test',
            'refresh-token': '1//test-refresh',
          };
          return mockData[account] || null;
        }
      );

      // Retrieve token
      const retrieved = await getOktaToken();
      expect(retrieved.refresh_token).toBe('1//test-refresh');

      // Delete token
      await expect(async () => {
        await import('../../src/lib/token-storage').then(m => m.deleteOktaToken());
      }).resolves;

      // Mock retrieval after deletion (returns null)
      (keytar.getPassword as jest.Mock).mockResolvedValue(null);

      // Verify token no longer exists
      await expect(getOktaToken()).rejects.toThrow('No token found');
    });
  });
});
