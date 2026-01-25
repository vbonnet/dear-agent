/**
 * Integration tests for token revocation and logout flow
 * Tests full logout workflow with mocked keychain and OAuth server
 */

import { logoutCommand } from '../../src/lib/logout';
import * as tokenStorage from '../../src/lib/token-storage';
import { google } from 'googleapis';

// Mock keytar for keychain operations
jest.mock('keytar');

// Mock googleapis
jest.mock('googleapis');

describe('Token Revocation - Integration Tests', () => {
  let consoleLogSpy: jest.SpyInstance;
  let consoleWarnSpy: jest.SpyInstance;
  let mockRevokeToken: jest.Mock;
  let mockOAuth2Client: any;

  // Import mock keytar helper
  const keytar = require('keytar');

  beforeEach(() => {
    // Clear mocks
    jest.clearAllMocks();

    // Clear mock keychain
    if (keytar.__clearMockStore) {
      keytar.__clearMockStore();
    }

    // Spy on console
    consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
    consoleWarnSpy = jest.spyOn(console, 'warn').mockImplementation();

    // Setup mock OAuth2 client
    mockRevokeToken = jest.fn().mockResolvedValue(undefined);
    mockOAuth2Client = {
      revokeToken: mockRevokeToken,
    };

    (google.auth.OAuth2 as any) = jest.fn().mockImplementation(() => mockOAuth2Client);
  });

  afterEach(() => {
    consoleLogSpy.mockRestore();
    consoleWarnSpy.mockRestore();
  });

  // ==================== Full Logout Flow Tests ====================

  describe('Full Logout Flow', () => {
    test('complete flow: get tokens → revoke at OAuth → clear keychain', async () => {
      // Store a token in mock keychain
      const mockToken = {
        type: 'authorized_user',
        client_id: '123456789.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test123',
        refresh_token: '1//test-refresh-token',
        access_token: 'ya29.test-access-token',
      };

      await tokenStorage.storeOktaToken(mockToken);

      // Verify token exists before logout
      const tokenBefore = await tokenStorage.getOktaToken();
      expect(tokenBefore).toBeDefined();
      expect(tokenBefore.refresh_token).toBe('1//test-refresh-token');

      // Perform logout
      await logoutCommand();

      // Verify tokens were revoked (both access and refresh)
      expect(mockRevokeToken).toHaveBeenCalledTimes(2);
      expect(mockRevokeToken).toHaveBeenNthCalledWith(1, 'ya29.test-access-token');
      expect(mockRevokeToken).toHaveBeenNthCalledWith(2, '1//test-refresh-token');

      // Verify success messages
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Tokens revoked at Google OAuth server');
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Local credentials cleared');

      // Verify token deleted from keychain
      await expect(tokenStorage.getOktaToken()).rejects.toThrow('No token found');
    });

    test('verify revocation endpoint called with correct OAuth client configuration', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: 'test-client-id.apps.googleusercontent.com',
        client_secret: 'GOCSPX-secret-key',
        refresh_token: '1//refresh-123',
      };

      await tokenStorage.storeOktaToken(mockToken);

      await logoutCommand();

      // Verify OAuth2 client created with correct credentials
      expect(google.auth.OAuth2).toHaveBeenCalledWith(
        'test-client-id.apps.googleusercontent.com',
        'GOCSPX-secret-key',
        'http://localhost'
      );

      // Verify revoke called on the client
      expect(mockRevokeToken).toHaveBeenCalledWith('1//refresh-123');
    });
  });

  // ==================== Revocation Failure Scenarios ====================

  describe('Revocation Failure Scenarios', () => {
    test('network unreachable → warn, clear local', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      await tokenStorage.storeOktaToken(mockToken);

      // Mock network error
      const networkError = new Error('Network error');
      (networkError as any).code = 'ENOTFOUND';
      mockRevokeToken.mockRejectedValue(networkError);

      await logoutCommand();

      // Verify warning messages
      expect(consoleWarnSpy).toHaveBeenCalledWith('⚠ Warning: Failed to revoke tokens at Google OAuth server');
      expect(consoleWarnSpy).toHaveBeenCalledWith(expect.stringContaining('Network unreachable'));
      expect(consoleWarnSpy).toHaveBeenCalledWith('  Local credentials will still be cleared');

      // Verify local cleanup still happened
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Local credentials cleared');
      await expect(tokenStorage.getOktaToken()).rejects.toThrow('No token found');
    });

    test('OAuth server returns 500 → warn, clear local', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      await tokenStorage.storeOktaToken(mockToken);

      // Mock server error
      mockRevokeToken.mockRejectedValue(new Error('Request failed with status code 500'));

      await logoutCommand();

      // Verify warning shown
      expect(consoleWarnSpy).toHaveBeenCalledWith('⚠ Warning: Failed to revoke tokens at Google OAuth server');
      expect(consoleWarnSpy).toHaveBeenCalledWith(expect.stringContaining('Revocation failed'));

      // Verify local cleanup happened
      await expect(tokenStorage.getOktaToken()).rejects.toThrow('No token found');
    });

    test('timeout → warn, clear local', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      await tokenStorage.storeOktaToken(mockToken);

      // Mock timeout error
      const timeoutError = new Error('Request timeout');
      (timeoutError as any).code = 'ETIMEDOUT';
      mockRevokeToken.mockRejectedValue(timeoutError);

      await logoutCommand();

      // Verify warning includes network unreachable
      expect(consoleWarnSpy).toHaveBeenCalledWith(expect.stringContaining('Network unreachable'));

      // Verify local cleanup happened
      await expect(tokenStorage.getOktaToken()).rejects.toThrow('No token found');
    });
  });

  // ==================== Token Lifecycle Tests ====================

  describe('Token Lifecycle', () => {
    test('store → logout → verify tokens gone', async () => {
      // 1. Store tokens
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      await tokenStorage.storeOktaToken(mockToken);

      // 2. Verify stored
      const storedToken = await tokenStorage.getOktaToken();
      expect(storedToken.refresh_token).toBe('1//refresh-token');

      // 3. Logout
      await logoutCommand();

      // 4. Verify gone
      await expect(tokenStorage.getOktaToken()).rejects.toThrow('No token found');
    });

    test('multiple logouts are idempotent', async () => {
      // First logout (with token)
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      await tokenStorage.storeOktaToken(mockToken);
      await logoutCommand();

      // Verify first logout succeeded
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Tokens revoked at Google OAuth server');

      // Clear console mocks
      consoleLogSpy.mockClear();
      consoleWarnSpy.mockClear();

      // Second logout (no token)
      await logoutCommand();

      // Verify second logout is no-op
      expect(consoleLogSpy).toHaveBeenCalledWith('Already logged out');
      expect(mockRevokeToken).toHaveBeenCalledTimes(1); // Only first logout
    });
  });

  // ==================== CLI Integration Test ====================

  describe('CLI Integration', () => {
    test('CLI command calls logoutCommand() correctly', async () => {
      // This test simulates what the CLI does
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      await tokenStorage.storeOktaToken(mockToken);

      // Simulate CLI calling logoutCommand with options
      await logoutCommand({ silent: false });

      // Verify success messages shown (not silent)
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Tokens revoked at Google OAuth server');

      // Clear mocks
      consoleLogSpy.mockClear();

      // Store token again for silent mode test
      await tokenStorage.storeOktaToken(mockToken);

      // Simulate CLI calling logoutCommand with silent flag
      await logoutCommand({ silent: true });

      // Verify no success messages in silent mode
      expect(consoleLogSpy).not.toHaveBeenCalled();
    });
  });

  // ==================== OAuth Client Configuration Tests ====================

  describe('OAuth Client Configuration', () => {
    test('OAuth client created with correct credentials from keychain', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: 'unique-client-id.apps.googleusercontent.com',
        client_secret: 'GOCSPX-unique-secret',
        refresh_token: '1//unique-refresh-token',
      };

      await tokenStorage.storeOktaToken(mockToken);

      await logoutCommand();

      // Verify OAuth2 constructor called with exact credentials
      expect(google.auth.OAuth2).toHaveBeenCalledWith(
        'unique-client-id.apps.googleusercontent.com',
        'GOCSPX-unique-secret',
        'http://localhost'
      );
    });

    test('revocation handles token already revoked gracefully', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      await tokenStorage.storeOktaToken(mockToken);

      // Mock "invalid_token" response (token already revoked or expired)
      mockRevokeToken.mockRejectedValue(new Error('invalid_token: Token has been revoked'));

      await logoutCommand();

      // Should succeed (idempotent behavior)
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Tokens revoked at Google OAuth server');
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Local credentials cleared');
    });

    test('only refresh token revoked when no access token present', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
        // No access_token
      };

      await tokenStorage.storeOktaToken(mockToken);

      await logoutCommand();

      // Verify only refresh token revoked (access token not present)
      expect(mockRevokeToken).toHaveBeenCalledTimes(1);
      expect(mockRevokeToken).toHaveBeenCalledWith('1//refresh-token');
    });
  });
});
