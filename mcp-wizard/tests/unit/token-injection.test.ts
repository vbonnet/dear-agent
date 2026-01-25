/**
 * Unit tests for Token Injection Layer
 *
 * Tests:
 * - Token health checking
 * - Token refresh logic
 * - Proactive refresh at 50% TTL
 * - Re-authentication on expiry
 * - MCP spawning with token
 *
 * @module token-injection.test
 */

import { ChildProcess } from 'child_process';
import {
  checkTokenHealth,
  refreshOktaToken,
  getValidOktaToken,
  spawnMCPWithToken,
  needsTokenRefresh,
  TokenInjectionConfig,
  TokenHealth,
} from '../../src/lib/token-injection';
import * as tokenStorage from '../../src/lib/token-storage';
import * as auth from '../../src/lib/auth';
import { spawn } from 'child_process';

// Mock dependencies
jest.mock('../../src/lib/token-storage');
jest.mock('../../src/lib/auth');
jest.mock('child_process', () => ({
  spawn: jest.fn(),
}));
jest.mock('../../src/lib/errors', () => ({
  retryWithBackoff: jest.fn((fn) => fn()), // Execute immediately without retry
  sanitizeError: jest.fn((err) => err),
}));

// Mock fetch globally
global.fetch = jest.fn();

describe('Token Injection Layer', () => {
  const mockConfig: TokenInjectionConfig = {
    oktaDomain: '[REDACTED_EMPLOYER].okta.com',
    clientId: 'test-client-id',
    scopes: ['openid', 'profile', 'email'],
  };

  beforeEach(() => {
    jest.clearAllMocks();
    (global.fetch as jest.Mock).mockReset();
  });

  describe('checkTokenHealth', () => {
    it('should return invalid/expired for missing token', () => {
      const health = checkTokenHealth(undefined);

      expect(health.valid).toBe(false);
      expect(health.isExpired).toBe(true);
      expect(health.needsRefresh).toBe(true);
    });

    it('should return invalid/expired for token without access_token', () => {
      const token = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
      };

      const health = checkTokenHealth(token);

      expect(health.valid).toBe(false);
      expect(health.isExpired).toBe(true);
      expect(health.needsRefresh).toBe(true);
    });

    it('should return valid but needs refresh for token without expires_at', () => {
      const token = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'access',
      };

      const health = checkTokenHealth(token);

      expect(health.valid).toBe(true);
      expect(health.isExpired).toBe(false);
      expect(health.needsRefresh).toBe(true);
    });

    it('should return invalid/expired for expired token', () => {
      const token = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'access',
        expires_at: Date.now() - 1000, // Expired 1 second ago
      };

      const health = checkTokenHealth(token);

      expect(health.valid).toBe(false);
      expect(health.isExpired).toBe(true);
      expect(health.needsRefresh).toBe(true);
      expect(health.remainingTTL).toBe(0);
    });

    it('should return invalid/expired for token expiring soon (< 5 minutes)', () => {
      const token = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'access',
        expires_at: Date.now() + 4 * 60 * 1000, // Expires in 4 minutes
      };

      const health = checkTokenHealth(token);

      expect(health.valid).toBe(false);
      expect(health.isExpired).toBe(true);
      expect(health.needsRefresh).toBe(true);
    });

    it('should return valid but needs refresh for token at 40% TTL (< 50% threshold)', () => {
      // Token expires in 24 minutes (40% of 60 minutes = 24 minutes)
      const token = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'access',
        expires_at: Date.now() + 24 * 60 * 1000,
      };

      const health = checkTokenHealth(token);

      expect(health.valid).toBe(true);
      expect(health.isExpired).toBe(false);
      expect(health.needsRefresh).toBe(true); // < 50% threshold
      expect(health.remainingTTL).toBeGreaterThan(0);
    });

    it('should return valid and no refresh for token at 60% TTL (> 50% threshold)', () => {
      // Token expires in 36 minutes (60% of 60 minutes = 36 minutes)
      const token = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'access',
        expires_at: Date.now() + 36 * 60 * 1000,
      };

      const health = checkTokenHealth(token);

      expect(health.valid).toBe(true);
      expect(health.isExpired).toBe(false);
      expect(health.needsRefresh).toBe(false); // > 50% threshold
      expect(health.remainingTTL).toBeGreaterThan(0);
    });

    it('should return valid and no refresh for fresh token (55 minutes remaining)', () => {
      // Token expires in 55 minutes (91% of 60 minutes)
      const token = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'access',
        expires_at: Date.now() + 55 * 60 * 1000,
      };

      const health = checkTokenHealth(token);

      expect(health.valid).toBe(true);
      expect(health.isExpired).toBe(false);
      expect(health.needsRefresh).toBe(false);
    });
  });

  describe('refreshOktaToken', () => {
    const mockToken = {
      type: 'authorized_user',
      client_id: 'test',
      client_secret: 'secret',
      refresh_token: 'refresh-token-123',
      access_token: 'old-access-token',
      expires_at: Date.now() + 10 * 60 * 1000,
    };

    it('should successfully refresh token', async () => {
      const mockResponse = {
        access_token: 'new-access-token',
        expires_in: 3600,
      };

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      (tokenStorage.storeOktaToken as jest.Mock).mockResolvedValueOnce(undefined);

      const refreshed = await refreshOktaToken(mockConfig, mockToken);

      expect(refreshed.access_token).toBe('new-access-token');
      expect(refreshed.expires_at).toBeGreaterThan(Date.now());
      expect(refreshed.refresh_token).toBe('refresh-token-123'); // Preserved
      expect(tokenStorage.storeOktaToken).toHaveBeenCalledWith(
        expect.objectContaining({
          access_token: 'new-access-token',
          refresh_token: 'refresh-token-123',
        })
      );
    });

    it('should update refresh_token if new one provided', async () => {
      const mockResponse = {
        access_token: 'new-access-token',
        refresh_token: 'new-refresh-token',
        expires_in: 3600,
      };

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => mockResponse,
      });

      (tokenStorage.storeOktaToken as jest.Mock).mockResolvedValueOnce(undefined);

      const refreshed = await refreshOktaToken(mockConfig, mockToken);

      expect(refreshed.refresh_token).toBe('new-refresh-token');
    });

    it('should throw error if refresh_token missing', async () => {
      const tokenWithoutRefresh = {
        ...mockToken,
        refresh_token: '',
      };

      await expect(refreshOktaToken(mockConfig, tokenWithoutRefresh)).rejects.toThrow(
        'No refresh_token available'
      );
    });

    it('should throw error on HTTP 400 (invalid refresh token)', async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        status: 400,
        json: async () => ({
          error: 'invalid_grant',
          error_description: 'Refresh token expired',
        }),
      });

      await expect(refreshOktaToken(mockConfig, mockToken)).rejects.toThrow(
        'Token refresh failed'
      );
    });

    it('should throw error on HTTP 401 (unauthorized)', async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: async () => ({
          error: 'unauthorized_client',
        }),
      });

      await expect(refreshOktaToken(mockConfig, mockToken)).rejects.toThrow(
        'Token refresh failed'
      );
    });

    it('should throw error on HTTP 500 (server error)', async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: async () => ({ error: 'internal_server_error' }),
      });

      await expect(refreshOktaToken(mockConfig, mockToken)).rejects.toThrow(
        'Okta server error'
      );
    });

    it('should throw error if response missing access_token', async () => {
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          // Missing access_token
          expires_in: 3600,
        }),
      });

      await expect(refreshOktaToken(mockConfig, mockToken)).rejects.toThrow(
        'Invalid token refresh response'
      );
    });
  });

  describe('getValidOktaToken', () => {
    it('should return valid token without refresh', async () => {
      const freshToken = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'access-token-123',
        expires_at: Date.now() + 55 * 60 * 1000, // Fresh token (55 minutes)
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValueOnce(freshToken);

      const token = await getValidOktaToken(mockConfig);

      expect(token).toBe('access-token-123');
      expect(auth.authenticate).not.toHaveBeenCalled();
    });

    it('should refresh token proactively when at 40% TTL', async () => {
      const tokenNeedingRefresh = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'old-access-token',
        expires_at: Date.now() + 24 * 60 * 1000, // 40% TTL (24 min)
      };

      const refreshedToken = {
        ...tokenNeedingRefresh,
        access_token: 'new-access-token',
        expires_at: Date.now() + 60 * 60 * 1000,
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValueOnce(tokenNeedingRefresh);

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          access_token: 'new-access-token',
          expires_in: 3600,
        }),
      });

      (tokenStorage.storeOktaToken as jest.Mock).mockResolvedValueOnce(undefined);

      const token = await getValidOktaToken(mockConfig);

      expect(token).toBe('new-access-token');
      expect(global.fetch).toHaveBeenCalled();
      expect(auth.authenticate).not.toHaveBeenCalled();
    });

    it('should re-authenticate when token expired', async () => {
      const expiredToken = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'expired-token',
        expires_at: Date.now() - 1000, // Expired
      };

      const newToken = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'new-refresh',
        access_token: 'new-access-token',
        expires_at: Date.now() + 60 * 60 * 1000,
      };

      (tokenStorage.getOktaToken as jest.Mock)
        .mockResolvedValueOnce(expiredToken)
        .mockResolvedValueOnce(newToken);

      (auth.authenticate as jest.Mock).mockResolvedValueOnce(undefined);

      const token = await getValidOktaToken(mockConfig);

      expect(token).toBe('new-access-token');
      expect(auth.authenticate).toHaveBeenCalledWith(mockConfig);
    });

    it('should re-authenticate when no token found', async () => {
      const newToken = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'new-access-token',
        expires_at: Date.now() + 60 * 60 * 1000,
      };

      (tokenStorage.getOktaToken as jest.Mock)
        .mockRejectedValueOnce(new Error('No token found'))
        .mockResolvedValueOnce(newToken);

      (auth.authenticate as jest.Mock).mockResolvedValueOnce(undefined);

      const token = await getValidOktaToken(mockConfig);

      expect(token).toBe('new-access-token');
      expect(auth.authenticate).toHaveBeenCalledWith(mockConfig);
    });

    it('should fall back to re-authentication if refresh fails', async () => {
      const tokenNeedingRefresh = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'old-access-token',
        expires_at: Date.now() + 24 * 60 * 1000, // 40% TTL
      };

      const newToken = {
        ...tokenNeedingRefresh,
        access_token: 'new-access-token',
        expires_at: Date.now() + 60 * 60 * 1000,
      };

      (tokenStorage.getOktaToken as jest.Mock)
        .mockResolvedValueOnce(tokenNeedingRefresh)
        .mockResolvedValueOnce(newToken);

      // Refresh fails
      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: async () => ({ error: 'invalid_grant' }),
      });

      // Re-authentication succeeds
      (auth.authenticate as jest.Mock).mockResolvedValueOnce(undefined);

      const token = await getValidOktaToken(mockConfig);

      expect(token).toBe('new-access-token');
      expect(auth.authenticate).toHaveBeenCalledWith(mockConfig);
    });

    it('should throw error if re-authentication produces expired token', async () => {
      const expiredToken = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'expired-token',
        expires_at: Date.now() - 1000,
      };

      (tokenStorage.getOktaToken as jest.Mock)
        .mockResolvedValueOnce(expiredToken)
        .mockResolvedValueOnce(expiredToken); // Still expired after auth

      (auth.authenticate as jest.Mock).mockResolvedValueOnce(undefined);

      await expect(getValidOktaToken(mockConfig)).rejects.toThrow(
        'Failed to obtain valid token after re-authentication'
      );
    });
  });

  describe('spawnMCPWithToken', () => {
    it('should spawn MCP process with OKTA_TOKEN env var', async () => {
      const freshToken = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'access-token-123',
        expires_at: Date.now() + 55 * 60 * 1000,
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValueOnce(freshToken);

      const mockChildProcess = {
        on: jest.fn(),
      } as unknown as ChildProcess;

      (spawn as jest.Mock).mockReturnValueOnce(mockChildProcess);

      const result = await spawnMCPWithToken(['mcp-server-gdocs', '--port', '3000'], mockConfig);

      expect(result).toBe(mockChildProcess);
      expect(spawn).toHaveBeenCalledWith('mcp-server-gdocs', ['--port', '3000'], {
        env: expect.objectContaining({
          OKTA_TOKEN: 'access-token-123',
        }),
        stdio: ['pipe', 'pipe', 'pipe'],
      });
    });

    it('should refresh token before spawning if needed', async () => {
      const tokenNeedingRefresh = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'old-access-token',
        expires_at: Date.now() + 24 * 60 * 1000, // 40% TTL
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValueOnce(tokenNeedingRefresh);

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          access_token: 'new-access-token',
          expires_in: 3600,
        }),
      });

      (tokenStorage.storeOktaToken as jest.Mock).mockResolvedValueOnce(undefined);

      const mockChildProcess = {
        on: jest.fn(),
      } as unknown as ChildProcess;

      (spawn as jest.Mock).mockReturnValueOnce(mockChildProcess);

      const result = await spawnMCPWithToken(['mcp-server-gdocs'], mockConfig);

      expect(spawn).toHaveBeenCalledWith('mcp-server-gdocs', [], {
        env: expect.objectContaining({
          OKTA_TOKEN: 'new-access-token',
        }),
        stdio: ['pipe', 'pipe', 'pipe'],
      });
    });

    it('should preserve existing environment variables', async () => {
      const freshToken = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'access-token-123',
        expires_at: Date.now() + 55 * 60 * 1000,
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValueOnce(freshToken);

      const mockChildProcess = {
        on: jest.fn(),
      } as unknown as ChildProcess;

      (spawn as jest.Mock).mockReturnValueOnce(mockChildProcess);

      // Set test env var
      process.env.TEST_VAR = 'test-value';

      await spawnMCPWithToken(['mcp-server-gdocs'], mockConfig);

      expect(spawn).toHaveBeenCalledWith('mcp-server-gdocs', [], {
        env: expect.objectContaining({
          OKTA_TOKEN: 'access-token-123',
          TEST_VAR: 'test-value',
        }),
        stdio: ['pipe', 'pipe', 'pipe'],
      });

      // Cleanup
      delete process.env.TEST_VAR;
    });
  });

  describe('needsTokenRefresh', () => {
    it('should return true if no token found', async () => {
      (tokenStorage.getOktaToken as jest.Mock).mockRejectedValueOnce(
        new Error('No token found')
      );

      const result = await needsTokenRefresh(mockConfig);

      expect(result).toBe(true);
    });

    it('should return true if token expired', async () => {
      const expiredToken = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'expired-token',
        expires_at: Date.now() - 1000,
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValueOnce(expiredToken);

      const result = await needsTokenRefresh(mockConfig);

      expect(result).toBe(true);
    });

    it('should return true if token needs refresh (< 50% TTL)', async () => {
      const tokenNeedingRefresh = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'old-access-token',
        expires_at: Date.now() + 24 * 60 * 1000, // 40% TTL
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValueOnce(tokenNeedingRefresh);

      const result = await needsTokenRefresh(mockConfig);

      expect(result).toBe(true);
    });

    it('should return false if token is fresh', async () => {
      const freshToken = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'access-token-123',
        expires_at: Date.now() + 55 * 60 * 1000, // Fresh
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValueOnce(freshToken);

      const result = await needsTokenRefresh(mockConfig);

      expect(result).toBe(false);
    });
  });

  describe('Integration scenarios', () => {
    it('should handle complete refresh cycle: expired → refresh → spawn', async () => {
      // Scenario: Token at 40% TTL, refresh, then spawn MCP
      const tokenAt40Percent = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'old-access-token',
        expires_at: Date.now() + 24 * 60 * 1000,
      };

      (tokenStorage.getOktaToken as jest.Mock).mockResolvedValueOnce(tokenAt40Percent);

      (global.fetch as jest.Mock).mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          access_token: 'refreshed-access-token',
          expires_in: 3600,
        }),
      });

      (tokenStorage.storeOktaToken as jest.Mock).mockResolvedValueOnce(undefined);

      const mockChildProcess = {
        on: jest.fn(),
      } as unknown as ChildProcess;

      (spawn as jest.Mock).mockReturnValueOnce(mockChildProcess);

      const result = await spawnMCPWithToken(['mcp-server-gdocs'], mockConfig);

      expect(spawn).toHaveBeenCalledWith('mcp-server-gdocs', [], {
        env: expect.objectContaining({
          OKTA_TOKEN: 'refreshed-access-token',
        }),
        stdio: ['pipe', 'pipe', 'pipe'],
      });
    });

    it('should handle complete re-auth cycle: expired → re-auth → spawn', async () => {
      const expiredToken = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'refresh',
        access_token: 'expired-token',
        expires_at: Date.now() - 1000,
      };

      const newToken = {
        type: 'authorized_user',
        client_id: 'test',
        client_secret: 'secret',
        refresh_token: 'new-refresh',
        access_token: 'new-access-token',
        expires_at: Date.now() + 60 * 60 * 1000,
      };

      (tokenStorage.getOktaToken as jest.Mock)
        .mockResolvedValueOnce(expiredToken)
        .mockResolvedValueOnce(newToken);

      (auth.authenticate as jest.Mock).mockResolvedValueOnce(undefined);

      const mockChildProcess = {
        on: jest.fn(),
      } as unknown as ChildProcess;

      (spawn as jest.Mock).mockReturnValueOnce(mockChildProcess);

      const result = await spawnMCPWithToken(['mcp-server-gdocs'], mockConfig);

      expect(auth.authenticate).toHaveBeenCalledWith(mockConfig);
      expect(spawn).toHaveBeenCalledWith('mcp-server-gdocs', [], {
        env: expect.objectContaining({
          OKTA_TOKEN: 'new-access-token',
        }),
        stdio: ['pipe', 'pipe', 'pipe'],
      });
    });
  });
});
