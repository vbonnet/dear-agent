/**
 * Unit tests for logout module
 * Tests OAuth token revocation and local credential cleanup
 */

import { logoutCommand, hasToken } from '../../src/lib/logout';
import * as tokenStorage from '../../src/lib/token-storage';
import { google } from 'googleapis';

// Mock dependencies
jest.mock('keytar');
jest.mock('../../src/lib/token-storage');
jest.mock('googleapis');

describe('Logout Module - Unit Tests', () => {
  // Mock console methods
  let consoleLogSpy: jest.SpyInstance;
  let consoleWarnSpy: jest.SpyInstance;

  // Mock OAuth2 client
  let mockRevokeToken: jest.Mock;
  let mockOAuth2Client: any;

  beforeEach(() => {
    // Clear all mocks
    jest.clearAllMocks();

    // Spy on console methods
    consoleLogSpy = jest.spyOn(console, 'log').mockImplementation();
    consoleWarnSpy = jest.spyOn(console, 'warn').mockImplementation();

    // Setup mock OAuth2 client
    mockRevokeToken = jest.fn().mockResolvedValue(undefined);
    mockOAuth2Client = {
      revokeToken: mockRevokeToken,
    };

    // Mock google.auth.OAuth2 constructor
    (google.auth.OAuth2 as any) = jest.fn().mockImplementation(() => mockOAuth2Client);
  });

  afterEach(() => {
    // Restore console methods
    consoleLogSpy.mockRestore();
    consoleWarnSpy.mockRestore();
  });

  // ==================== Happy Path Tests ====================

  describe('Happy Path', () => {
    test('successfully revokes access token', async () => {
      // Mock token with access token
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
        access_token: 'ya29.access-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      await logoutCommand();

      // Verify access token revoked
      expect(mockRevokeToken).toHaveBeenCalledWith('ya29.access-token');
      expect(mockRevokeToken).toHaveBeenCalledTimes(2); // access + refresh
    });

    test('successfully revokes refresh token', async () => {
      // Mock token without access token
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      await logoutCommand();

      // Verify refresh token revoked
      expect(mockRevokeToken).toHaveBeenCalledWith('1//refresh-token');
      expect(mockRevokeToken).toHaveBeenCalledTimes(1); // only refresh
    });

    test('clears keychain after revocation', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      await logoutCommand();

      // Verify keychain cleared
      expect(tokenStorage.deleteOktaToken).toHaveBeenCalledTimes(1);
      // Verify success messages
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Tokens revoked at Google OAuth server');
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Local credentials cleared');
    });
  });

  // ==================== Error Handling Tests ====================

  describe('Error Handling', () => {
    test('handles missing tokens gracefully (no-op)', async () => {
      // Mock getOktaToken to throw "no token found"
      (tokenStorage.getOktaToken as jest.Mock).mockRejectedValue(
        new Error('No token found - run: mcp-wizard auth')
      );

      await logoutCommand();

      // Verify no revocation attempted
      expect(mockRevokeToken).not.toHaveBeenCalled();
      // Verify no keychain deletion attempted
      expect(tokenStorage.deleteOktaToken).not.toHaveBeenCalled();
      // Verify "already logged out" message
      expect(consoleLogSpy).toHaveBeenCalledWith('Already logged out');
    });

    test('handles network failure (warns, clears local)', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      // Mock network error
      const networkError = new Error('Network error');
      (networkError as any).code = 'ENOTFOUND';
      mockRevokeToken.mockRejectedValue(networkError);

      await logoutCommand();

      // Verify warning shown
      expect(consoleWarnSpy).toHaveBeenCalledWith('⚠ Warning: Failed to revoke tokens at Google OAuth server');
      expect(consoleWarnSpy).toHaveBeenCalledWith(expect.stringContaining('Network unreachable'));
      // Verify local cleanup still happened
      expect(tokenStorage.deleteOktaToken).toHaveBeenCalled();
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Local credentials cleared');
    });

    test('handles 401 Unauthorized (warns, clears local)', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      // Mock 401 error
      mockRevokeToken.mockRejectedValue(new Error('Request failed with status code 401'));

      await logoutCommand();

      // Verify warning shown
      expect(consoleWarnSpy).toHaveBeenCalledWith('⚠ Warning: Failed to revoke tokens at Google OAuth server');
      // Verify local cleanup still happened
      expect(tokenStorage.deleteOktaToken).toHaveBeenCalled();
    });

    test('handles 500 Server Error (warns, clears local)', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      // Mock server error
      mockRevokeToken.mockRejectedValue(new Error('Request failed with status code 500'));

      await logoutCommand();

      // Verify warning shown
      expect(consoleWarnSpy).toHaveBeenCalledWith('⚠ Warning: Failed to revoke tokens at Google OAuth server');
      // Verify local cleanup still happened
      expect(tokenStorage.deleteOktaToken).toHaveBeenCalled();
    });

    test('handles revocation endpoint timeout', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      // Mock timeout error
      const timeoutError = new Error('Request timeout');
      (timeoutError as any).code = 'ETIMEDOUT';
      mockRevokeToken.mockRejectedValue(timeoutError);

      await logoutCommand();

      // Verify warning includes network unreachable
      expect(consoleWarnSpy).toHaveBeenCalledWith(expect.stringContaining('Network unreachable'));
      // Verify local cleanup still happened
      expect(tokenStorage.deleteOktaToken).toHaveBeenCalled();
    });

    test('handles partial failure (access revoked, refresh fails)', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
        access_token: 'ya29.access-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      // Mock: access token succeeds, refresh token fails
      mockRevokeToken
        .mockResolvedValueOnce(undefined) // access token succeeds
        .mockRejectedValueOnce(new Error('Network error')); // refresh token fails

      await logoutCommand();

      // Verify both tokens attempted
      expect(mockRevokeToken).toHaveBeenCalledTimes(2);
      // Verify warning shown
      expect(consoleWarnSpy).toHaveBeenCalledWith('⚠ Warning: Failed to revoke tokens at Google OAuth server');
      // Verify local cleanup still happened
      expect(tokenStorage.deleteOktaToken).toHaveBeenCalled();
    });

    test('handles keychain deletion failure', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockRejectedValue(
        new Error('Keychain service unavailable')
      );

      // This SHOULD throw because keychain deletion is critical
      await expect(logoutCommand()).rejects.toThrow('Failed to clear local credentials');
    });

    test('throws on unrecoverable errors (keychain unavailable on retrieval)', async () => {
      // Mock getOktaToken to throw keychain error (not "no token found")
      (tokenStorage.getOktaToken as jest.Mock).mockRejectedValue(
        new Error('Keychain service unavailable')
      );

      await expect(logoutCommand()).rejects.toThrow('Keychain service unavailable');
    });
  });

  // ==================== Silent Mode Tests ====================

  describe('Silent Mode', () => {
    test('silent mode suppresses success messages', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      await logoutCommand({ silent: true });

      // Verify NO success messages
      expect(consoleLogSpy).not.toHaveBeenCalled();
    });

    test('silent mode shows errors to stderr', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      // Mock network error
      const networkError = new Error('Network error');
      (networkError as any).code = 'ENOTFOUND';
      mockRevokeToken.mockRejectedValue(networkError);

      await logoutCommand({ silent: true });

      // Verify warnings still shown (errors go to stderr)
      expect(consoleWarnSpy).toHaveBeenCalledWith('⚠ Warning: Failed to revoke tokens at Google OAuth server');
    });
  });

  // ==================== Idempotency Tests ====================

  describe('Idempotency', () => {
    test('second logout is no-op (tokens already cleared)', async () => {
      // Mock "no token found" (already logged out)
      (tokenStorage.getOktaToken as jest.Mock).mockRejectedValue(
        new Error('No token found - run: mcp-wizard auth')
      );

      await logoutCommand();

      // Verify no revocation attempted
      expect(mockRevokeToken).not.toHaveBeenCalled();
      // Verify no keychain deletion attempted
      expect(tokenStorage.deleteOktaToken).not.toHaveBeenCalled();
      // Verify "already logged out" message
      expect(consoleLogSpy).toHaveBeenCalledWith('Already logged out');
    });

    test('revocation endpoint handles already-revoked token gracefully', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      // Mock "invalid_token" error (token already revoked)
      mockRevokeToken.mockRejectedValue(new Error('invalid_token: Token is invalid or expired'));

      await logoutCommand();

      // Should succeed (idempotent - token already revoked is OK)
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Tokens revoked at Google OAuth server');
      expect(consoleLogSpy).toHaveBeenCalledWith('✓ Local credentials cleared');
    });
  });

  // ==================== Integration with token-storage Tests ====================

  describe('Integration with token-storage', () => {
    test('uses getOktaToken() correctly', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      await logoutCommand();

      // Verify getOktaToken called exactly once
      expect(tokenStorage.getOktaToken).toHaveBeenCalledTimes(1);
      expect(tokenStorage.getOktaToken).toHaveBeenCalledWith();
    });

    test('uses deleteOktaToken() correctly', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);
      (tokenStorage.deleteOktaToken as jest.Mock).mockResolvedValue(undefined);

      await logoutCommand();

      // Verify deleteOktaToken called exactly once
      expect(tokenStorage.deleteOktaToken).toHaveBeenCalledTimes(1);
      expect(tokenStorage.deleteOktaToken).toHaveBeenCalledWith();
    });
  });

  // ==================== Helper Function Tests ====================

  describe('hasToken() helper', () => {
    test('returns true when token exists', async () => {
      const mockToken = {
        type: 'authorized_user',
        client_id: '123.apps.googleusercontent.com',
        client_secret: 'GOCSPX-test',
        refresh_token: '1//refresh-token',
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValue(mockToken);

      const result = await hasToken();

      expect(result).toBe(true);
    });

    test('returns false when token does not exist', async () => {
      (tokenStorage.getOktaToken as jest.Mock).mockRejectedValue(
        new Error('No token found - run: mcp-wizard auth')
      );

      const result = await hasToken();

      expect(result).toBe(false);
    });
  });
});
