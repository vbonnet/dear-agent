/**
 * Integration tests for OAuth Device Authorization Grant flow
 *
 * Tests end-to-end device flow scenarios with mocked Okta endpoints
 */

import { deviceFlowAuth, pollForToken } from '../../src/lib/auth';

// Mock dependencies
jest.mock('../../src/lib/token-storage');
jest.mock('../../src/lib/errors', () => ({
  retryWithBackoff: jest.fn((fn) => fn()),
  sanitizeError: jest.fn((error) => error),
}));

// Mock global fetch
global.fetch = jest.fn();

describe('Device Flow Auth - Integration Tests', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  describe('Full Device Flow', () => {
    test('completes authorization flow with pending -> success transition', async () => {
      const { storeOktaToken } = require('../../src/lib/token-storage');
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      const mockDeviceResponse = {
        device_code: 'device-code-12345',
        user_code: 'WXYZ-9876',
        verification_uri: 'https://[REDACTED_EMPLOYER].okta.com/activate',
        verification_uri_complete: 'https://[REDACTED_EMPLOYER].okta.com/activate?user_code=WXYZ-9876',
        expires_in: 600,
        interval: 5,
      };

      const mockTokenResponse = {
        access_token: 'eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...',
        token_type: 'Bearer',
        expires_in: 3600,
        refresh_token: '1//refresh-token-here',
        scope: 'openid profile email',
      };

      // Setup mock responses
      (global.fetch as jest.Mock)
        // 1. Device code request
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockDeviceResponse,
        })
        // 2. First token poll: authorization_pending
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'authorization_pending' }),
        })
        // 3. Second token poll: authorization_pending
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'authorization_pending' }),
        })
        // 4. Third token poll: success
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockTokenResponse,
        });

      const authPromise = deviceFlowAuth({
        oktaDomain: '[REDACTED_EMPLOYER].okta.com',
        clientId: 'okta-client-id-123',
        scopes: ['openid', 'profile', 'email'],
      });

      // Simulate user authorization delay (3 polling intervals)
      await jest.advanceTimersByTimeAsync(5000); // First poll

      await jest.advanceTimersByTimeAsync(5000); // Second poll

      await jest.advanceTimersByTimeAsync(5000); // Third poll (success)

      await authPromise;

      // Verify token stored
      expect(storeOktaToken).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'authorized_user',
          client_id: 'okta-client-id-123',
          refresh_token: '1//refresh-token-here',
          access_token: 'eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...',
        })
      );

      // Verify user-facing messages
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('Initiating device authorization'));
      expect(consoleSpy).toHaveBeenCalledWith(expect.stringContaining('Waiting for authorization'));
      expect(consoleSpy).toHaveBeenCalledWith('✓ Authorization successful!');
      expect(consoleSpy).toHaveBeenCalledWith('✓ Tokens stored securely in OS keychain');

      consoleSpy.mockRestore();
    });

    test('handles slow_down response correctly', async () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      const mockTokenResponse = {
        access_token: 'access-token',
        token_type: 'Bearer',
        expires_in: 3600,
        refresh_token: 'refresh-token',
      };

      // Setup mock responses
      (global.fetch as jest.Mock)
        // 1. First poll: slow_down (Okta wants us to slow down)
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'slow_down' }),
        })
        // 2. Second poll: authorization_pending
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'authorization_pending' }),
        })
        // 3. Third poll: success
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockTokenResponse,
        });

      const pollPromise = pollForToken('[REDACTED_EMPLOYER].okta.com', 'client-id', 'device-code', 5, 600);

      // First poll (5s interval)
      await jest.advanceTimersByTimeAsync(5000);

      // After slow_down, interval increased by 5s (now 10s total)
      await jest.advanceTimersByTimeAsync(10000);

      // Third poll (still 10s interval)
      await jest.advanceTimersByTimeAsync(10000);

      const result = await pollPromise;

      expect(result.access_token).toBe('access-token');
      expect(consoleSpy).toHaveBeenCalledWith('Reducing polling frequency...');

      consoleSpy.mockRestore();
    });

    test('handles user denial gracefully', async () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      const mockDeviceResponse = {
        device_code: 'device-code-12345',
        user_code: 'WXYZ-9876',
        verification_uri: 'https://[REDACTED_EMPLOYER].okta.com/activate',
        expires_in: 600,
        interval: 5,
      };

      // Setup mock responses
      (global.fetch as jest.Mock)
        // 1. Device code request
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockDeviceResponse,
        })
        // 2. First token poll: authorization_pending
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'authorization_pending' }),
        })
        // 3. Second token poll: access_denied (user denied)
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({
            error: 'access_denied',
            error_description: 'The user denied the authorization request',
          }),
          text: async () => JSON.stringify({
            error: 'access_denied',
            error_description: 'The user denied the authorization request',
          }),
        });

      const authPromise = deviceFlowAuth({
        oktaDomain: '[REDACTED_EMPLOYER].okta.com',
        clientId: 'okta-client-id-123',
        scopes: ['openid', 'profile'],
      });

      // Need to catch the error expectation before advancing timers
      const expectation = expect(authPromise).rejects.toThrow('Authorization denied');

      // First poll
      await jest.advanceTimersByTimeAsync(5000);

      // Second poll (user denies)
      await jest.advanceTimersByTimeAsync(5000);

      await expectation;

      consoleSpy.mockRestore();
    });

    test('handles device code expiration', async () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      const mockDeviceResponse = {
        device_code: 'device-code-12345',
        user_code: 'WXYZ-9876',
        verification_uri: 'https://[REDACTED_EMPLOYER].okta.com/activate',
        expires_in: 15, // Short timeout for testing
        interval: 5,
      };

      // Setup mock responses
      (global.fetch as jest.Mock)
        // 1. Device code request
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockDeviceResponse,
        })
        // 2. First token poll: authorization_pending
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'authorization_pending' }),
        })
        // 3. Second token poll: authorization_pending
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'authorization_pending' }),
        });

      const authPromise = deviceFlowAuth({
        oktaDomain: '[REDACTED_EMPLOYER].okta.com',
        clientId: 'okta-client-id-123',
        scopes: ['openid', 'profile'],
      });

      // Need to catch the error expectation before advancing timers
      const expectation = expect(authPromise).rejects.toThrow('timed out');

      // First poll (5s)
      await jest.advanceTimersByTimeAsync(5000);

      // Second poll (10s total, but expires_in is 15s)
      await jest.advanceTimersByTimeAsync(5000);

      // Advance past expiration (20s total > 15s expires_in)
      await jest.advanceTimersByTimeAsync(10000);

      await expectation;

      consoleSpy.mockRestore();
    });

    test('stores tokens in keychain on success', async () => {
      const { storeOktaToken } = require('../../src/lib/token-storage');
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      const mockDeviceResponse = {
        device_code: 'device-code-12345',
        user_code: 'WXYZ-9876',
        verification_uri: 'https://[REDACTED_EMPLOYER].okta.com/activate',
        expires_in: 600,
        interval: 5,
      };

      const mockTokenResponse = {
        access_token: 'access-token-secure-123',
        token_type: 'Bearer',
        expires_in: 3600,
        refresh_token: 'refresh-token-secure-456',
      };

      // Setup mock responses
      (global.fetch as jest.Mock)
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockDeviceResponse,
        })
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'authorization_pending' }),
        })
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockTokenResponse,
        });

      const authPromise = deviceFlowAuth({
        oktaDomain: '[REDACTED_EMPLOYER].okta.com',
        clientId: 'okta-client-id-123',
        scopes: ['openid', 'profile', 'email'],
      });

      await jest.advanceTimersByTimeAsync(5000);
      await jest.advanceTimersByTimeAsync(5000);

      await authPromise;

      // Verify storeOktaToken called with correct structure
      expect(storeOktaToken).toHaveBeenCalledTimes(1);
      expect(storeOktaToken).toHaveBeenCalledWith({
        type: 'authorized_user',
        client_id: 'okta-client-id-123',
        client_secret: '',
        refresh_token: 'refresh-token-secure-456',
        access_token: 'access-token-secure-123',
        expires_at: expect.any(Number),
      });

      // Verify expires_at is in the future
      const storedToken = storeOktaToken.mock.calls[0][0];
      expect(storedToken.expires_at).toBeGreaterThan(Date.now());

      consoleSpy.mockRestore();
    });
  });
});
