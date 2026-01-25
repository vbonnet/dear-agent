import {
  detectEnvironment,
  requestDeviceCode,
  displayUserCode,
  pollForToken,
  deviceFlowAuth,
  generateCodeVerifier,
  generateCodeChallenge,
  generateState,
  generatePKCE,
  selectRandomPort,
  buildAuthorizationUrl,
  launchBrowser,
  exchangeCodeForTokens,
  browserPKCEAuth,
  authenticate,
  isBrowserLaunchError,
} from '../../src/lib/auth';
import * as open from 'open';

// Mock open package with proper default export
jest.mock('open', () => {
  const mockFn = jest.fn().mockResolvedValue({} as any);
  return {
    __esModule: true,
    default: mockFn,
  };
});

// Mock dependencies
jest.mock('../../src/lib/errors', () => ({
  retryWithBackoff: jest.fn((fn) => fn()),
  sanitizeError: jest.fn((error) => error),
}));
jest.mock('../../src/lib/token-storage');

// Mock global fetch
global.fetch = jest.fn();

describe('Auth Module - Device Flow', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Clear environment variables
    delete process.env.SSH_CLIENT;
    delete process.env.SSH_TTY;
    delete process.env.SSH_CONNECTION;
    delete process.env.CLOUD_SHELL;
  });

  describe('detectEnvironment', () => {
    test('detects SSH via SSH_CLIENT', () => {
      process.env.SSH_CLIENT = '192.168.1.1 54321 22';
      const result = detectEnvironment();
      expect(result.type).toBe('headless');
      expect(result.sshDetected).toBe(true);
    });

    test('detects SSH via SSH_TTY', () => {
      process.env.SSH_TTY = '/dev/pts/0';
      const result = detectEnvironment();
      expect(result.type).toBe('headless');
      expect(result.sshDetected).toBe(true);
    });

    test('detects SSH via SSH_CONNECTION', () => {
      process.env.SSH_CONNECTION = '192.168.1.1 54321 10.0.0.1 22';
      const result = detectEnvironment();
      expect(result.type).toBe('headless');
      expect(result.sshDetected).toBe(true);
    });

    test('detects GCP Cloud Shell', () => {
      process.env.CLOUD_SHELL = 'true';
      const result = detectEnvironment();
      expect(result.type).toBe('headless');
      expect(result.cloudShellDetected).toBe(true);
    });

    test('returns interactive for local terminal', () => {
      // No SSH or cloud shell env vars
      // Note: process.stdout.isTTY is true in test environment
      const result = detectEnvironment();
      expect(result.sshDetected).toBe(false);
      expect(result.cloudShellDetected).toBe(false);
    });
  });

  describe('requestDeviceCode', () => {
    test('makes correct POST request to Okta', async () => {
      const mockResponse = {
        device_code: 'device-123',
        user_code: 'ABCD-1234',
        verification_uri: 'https://okta.com/activate',
        verification_uri_complete: 'https://okta.com/activate?user_code=ABCD-1234',
        expires_in: 600,
        interval: 5,
      };

      (global.fetch as jest.Mock).mockResolvedValue({
        ok: true,
        json: async () => mockResponse,
      });

      const result = await requestDeviceCode(
        '[REDACTED_EMPLOYER].okta.com',
        'client-123',
        ['openid', 'profile']
      );

      expect(global.fetch).toHaveBeenCalledWith(
        'https://[REDACTED_EMPLOYER].okta.com/oauth2/v1/device/authorize',
        expect.objectContaining({
          method: 'POST',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
          },
        })
      );

      expect(result).toEqual(mockResponse);
    });

    test('handles 400 error (invalid client)', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: false,
        status: 400,
        text: async () => 'Invalid client',
        json: async () => ({ error: 'invalid_client' }),
      });

      await expect(
        requestDeviceCode('[REDACTED_EMPLOYER].okta.com', 'invalid-client', ['openid'])
      ).rejects.toThrow();
    });

    test('handles 500 error (server error)', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: false,
        status: 500,
        text: async () => 'Internal server error',
        json: async () => ({ error: 'server_error' }),
      });

      await expect(
        requestDeviceCode('[REDACTED_EMPLOYER].okta.com', 'client-123', ['openid'])
      ).rejects.toThrow();
    });

    test('handles network error', async () => {
      (global.fetch as jest.Mock).mockRejectedValue(new Error('Network error'));

      await expect(
        requestDeviceCode('[REDACTED_EMPLOYER].okta.com', 'client-123', ['openid'])
      ).rejects.toThrow();
    });

    test('validates response has required fields', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: true,
        json: async () => ({
          // Missing device_code, expires_in, interval
          user_code: 'ABCD-1234',
          verification_uri: 'https://okta.com/activate',
        }),
      });

      await expect(
        requestDeviceCode('[REDACTED_EMPLOYER].okta.com', 'client-123', ['openid'])
      ).rejects.toThrow('Invalid device authorization response');
    });
  });

  describe('displayUserCode', () => {
    let consoleSpy: jest.SpyInstance;

    beforeEach(() => {
      consoleSpy = jest.spyOn(console, 'log').mockImplementation();
    });

    afterEach(() => {
      consoleSpy.mockRestore();
    });

    test('displays verification URI and user code', () => {
      displayUserCode('https://okta.com/activate', 'ABCD-1234', 600);

      expect(consoleSpy).toHaveBeenCalled();
      const output = consoleSpy.mock.calls.map((call) => call[0]).join('\n');
      expect(output).toContain('https://okta.com/activate');
      expect(output).toContain('ABCD-1234');
      expect(output).toContain('10 minutes');
    });

    test('calculates expiration time in minutes', () => {
      displayUserCode('https://okta.com/activate', 'ABCD-1234', 300);

      const output = consoleSpy.mock.calls.map((call) => call[0]).join('\n');
      expect(output).toContain('5 minutes');
    });
  });

  describe('pollForToken', () => {
    beforeEach(() => {
      jest.useFakeTimers();
    });

    afterEach(() => {
      jest.useRealTimers();
    });

    test('polls until success', async () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      const mockTokenResponse = {
        access_token: 'access-token-123',
        token_type: 'Bearer',
        expires_in: 3600,
        refresh_token: 'refresh-token-123',
      };

      // First call: authorization_pending
      // Second call: success
      (global.fetch as jest.Mock)
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'authorization_pending' }),
        })
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockTokenResponse,
        });

      const pollPromise = pollForToken(
        '[REDACTED_EMPLOYER].okta.com',
        'client-123',
        'device-123',
        5,
        600
      );

      // Advance timers to trigger first poll (5s interval)
      await jest.advanceTimersByTimeAsync(5000);

      // Advance timers to trigger second poll
      await jest.advanceTimersByTimeAsync(5000);

      const result = await pollPromise;

      expect(result).toEqual(mockTokenResponse);
      expect(consoleSpy).toHaveBeenCalledWith('Waiting for authorization...');
      expect(consoleSpy).toHaveBeenCalledWith('✓ Authorization successful!');

      consoleSpy.mockRestore();
    });

    test('handles slow_down response', async () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      const mockTokenResponse = {
        access_token: 'access-token-123',
        token_type: 'Bearer',
        expires_in: 3600,
        refresh_token: 'refresh-token-123',
      };

      // First call: slow_down
      // Second call: success
      (global.fetch as jest.Mock)
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'slow_down' }),
        })
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockTokenResponse,
        });

      const pollPromise = pollForToken(
        '[REDACTED_EMPLOYER].okta.com',
        'client-123',
        'device-123',
        5,
        600
      );

      // Advance timers
      await jest.advanceTimersByTimeAsync(5000);

      // After slow_down, interval should be increased by 5s (now 10s total)
      await jest.advanceTimersByTimeAsync(10000);

      const result = await pollPromise;

      expect(result).toEqual(mockTokenResponse);
      expect(consoleSpy).toHaveBeenCalledWith('Reducing polling frequency...');

      consoleSpy.mockRestore();
    });

    test('throws on access_denied', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: false,
        json: async () => ({
          error: 'access_denied',
          error_description: 'User denied authorization',
        }),
      });

      const pollPromise = pollForToken(
        '[REDACTED_EMPLOYER].okta.com',
        'client-123',
        'device-123',
        5,
        600
      );

      // Need to catch the error expectation before advancing timers
      const expectation = expect(pollPromise).rejects.toThrow('Authorization denied');

      await jest.advanceTimersByTimeAsync(5000);

      await expectation;
    });

    test('throws on expired_token', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: false,
        json: async () => ({
          error: 'expired_token',
          error_description: 'Device code expired',
        }),
      });

      const pollPromise = pollForToken(
        '[REDACTED_EMPLOYER].okta.com',
        'client-123',
        'device-123',
        5,
        600
      );

      // Need to catch the error expectation before advancing timers
      const expectation = expect(pollPromise).rejects.toThrow('Device code expired');

      await jest.advanceTimersByTimeAsync(5000);

      await expectation;
    });

    test('times out after expiresIn', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: false,
        json: async () => ({ error: 'authorization_pending' }),
      });

      const pollPromise = pollForToken(
        '[REDACTED_EMPLOYER].okta.com',
        'client-123',
        'device-123',
        5,
        10 // 10 second timeout
      );

      // Need to catch the error expectation before advancing timers
      const expectation = expect(pollPromise).rejects.toThrow('timed out');

      // Advance past timeout
      await jest.advanceTimersByTimeAsync(15000);

      await expectation;
    });
  });

  describe('deviceFlowAuth', () => {
    beforeEach(() => {
      jest.useFakeTimers();
    });

    afterEach(() => {
      jest.useRealTimers();
    });

    test('orchestrates full device flow', async () => {
      const { storeOktaToken } = require('../../src/lib/token-storage');
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      const mockDeviceResponse = {
        device_code: 'device-123',
        user_code: 'ABCD-1234',
        verification_uri: 'https://okta.com/activate',
        expires_in: 600,
        interval: 5,
      };

      const mockTokenResponse = {
        access_token: 'access-token-123',
        token_type: 'Bearer',
        expires_in: 3600,
        refresh_token: 'refresh-token-123',
      };

      // Mock device code request
      (global.fetch as jest.Mock)
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockDeviceResponse,
        })
        // Mock token polling (success on first poll)
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockTokenResponse,
        });

      const authPromise = deviceFlowAuth({
        oktaDomain: '[REDACTED_EMPLOYER].okta.com',
        clientId: 'client-123',
        scopes: ['openid', 'profile'],
      });

      // Advance timers for polling
      await jest.advanceTimersByTimeAsync(5000);

      await authPromise;

      expect(storeOktaToken).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'authorized_user',
          client_id: 'client-123',
          refresh_token: 'refresh-token-123',
          access_token: 'access-token-123',
        })
      );

      expect(consoleSpy).toHaveBeenCalledWith('✓ Tokens stored securely in OS keychain');

      consoleSpy.mockRestore();
    });

    test('throws error if configuration missing', async () => {
      await expect(
        deviceFlowAuth({
          oktaDomain: '',
          clientId: '',
          scopes: ['openid'],
        })
      ).rejects.toThrow('Missing Okta configuration');
    });
  });
});

describe('Auth Module - PKCE Flow', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('generateCodeVerifier', () => {
    test('generates 128-character verifier', () => {
      const verifier = generateCodeVerifier();
      expect(verifier).toHaveLength(128);
    });

    test('uses base64url alphabet', () => {
      const verifier = generateCodeVerifier();
      expect(verifier).toMatch(/^[A-Za-z0-9_-]+$/);
    });

    test('generates unique verifiers', () => {
      const verifiers = new Set();
      for (let i = 0; i < 100; i++) {
        verifiers.add(generateCodeVerifier());
      }
      expect(verifiers.size).toBe(100);
    });
  });

  describe('generateCodeChallenge', () => {
    test('generates 43-character challenge', () => {
      const verifier = generateCodeVerifier();
      const challenge = generateCodeChallenge(verifier);
      expect(challenge).toHaveLength(43);
    });

    test('is deterministic', () => {
      const verifier = 'test-verifier-123';
      const challenge1 = generateCodeChallenge(verifier);
      const challenge2 = generateCodeChallenge(verifier);
      expect(challenge1).toBe(challenge2);
    });

    test('different verifiers produce different challenges', () => {
      const challenge1 = generateCodeChallenge('verifier1');
      const challenge2 = generateCodeChallenge('verifier2');
      expect(challenge1).not.toBe(challenge2);
    });
  });

  describe('generateState', () => {
    test('generates 32-character state', () => {
      const state = generateState();
      expect(state).toHaveLength(32);
    });

    test('generates unique states', () => {
      const states = new Set();
      for (let i = 0; i < 100; i++) {
        states.add(generateState());
      }
      expect(states.size).toBe(100);
    });
  });

  describe('generatePKCE', () => {
    test('returns verifier, challenge, and state', () => {
      const pkce = generatePKCE();
      expect(pkce).toHaveProperty('verifier');
      expect(pkce).toHaveProperty('challenge');
      expect(pkce).toHaveProperty('state');
      expect(pkce.verifier).toHaveLength(128);
      expect(pkce.challenge).toHaveLength(43);
      expect(pkce.state).toHaveLength(32);
    });
  });

  describe('selectRandomPort', () => {
    test('returns port in range 3000-9000', () => {
      for (let i = 0; i < 10; i++) {
        const port = selectRandomPort();
        expect(port).toBeGreaterThanOrEqual(3000);
        expect(port).toBeLessThanOrEqual(9000);
      }
    });
  });

  describe('buildAuthorizationUrl', () => {
    test('includes all required parameters', () => {
      const url = buildAuthorizationUrl(
        '[REDACTED_EMPLOYER].okta.com',
        'client-123',
        'http://localhost:3456/callback',
        ['openid', 'profile'],
        'challenge-abc',
        'state-xyz'
      );

      expect(url).toContain('https://[REDACTED_EMPLOYER].okta.com/oauth2/v1/authorize');
      expect(url).toContain('client_id=client-123');
      expect(url).toContain('redirect_uri=http%3A%2F%2Flocalhost%3A3456%2Fcallback');
      expect(url).toContain('response_type=code');
      expect(url).toContain('scope=openid+profile');
      expect(url).toContain('code_challenge=challenge-abc');
      expect(url).toContain('code_challenge_method=S256');
      expect(url).toContain('state=state-xyz');
    });
  });

  describe('launchBrowser', () => {
    let mockOpenDefault: jest.Mock;

    beforeEach(() => {
      // Get the mocked open.default function
      mockOpenDefault = open.default as unknown as jest.Mock;
      mockOpenDefault.mockClear();
      mockOpenDefault.mockResolvedValue({} as any);
    });

    test('calls open.default with URL', async () => {
      await launchBrowser('https://okta.com/authorize');

      expect(mockOpenDefault).toHaveBeenCalledWith('https://okta.com/authorize');
    });

    test('throws on browser launch error', async () => {
      mockOpenDefault.mockRejectedValue(new Error('ENOENT'));

      await expect(launchBrowser('https://okta.com/authorize')).rejects.toThrow(
        'Failed to launch browser'
      );
    });
  });

  describe('exchangeCodeForTokens', () => {
    test('exchanges code for tokens', async () => {
      const mockResponse = {
        access_token: 'access-123',
        token_type: 'Bearer',
        expires_in: 3600,
        refresh_token: 'refresh-123',
      };

      (global.fetch as jest.Mock).mockResolvedValue({
        ok: true,
        json: async () => mockResponse,
      });

      const result = await exchangeCodeForTokens(
        '[REDACTED_EMPLOYER].okta.com',
        'client-123',
        'code-abc',
        'verifier-xyz',
        'http://localhost:3456/callback'
      );

      expect(result).toEqual(mockResponse);
      expect(global.fetch).toHaveBeenCalledWith(
        'https://[REDACTED_EMPLOYER].okta.com/oauth2/v1/token',
        expect.objectContaining({
          method: 'POST',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
          },
        })
      );
    });

    test('throws on invalid token response', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: true,
        json: async () => ({ invalid: 'response' }),
      });

      await expect(
        exchangeCodeForTokens(
          '[REDACTED_EMPLOYER].okta.com',
          'client-123',
          'code-abc',
          'verifier-xyz',
          'http://localhost:3456/callback'
        )
      ).rejects.toThrow('Invalid token response from Okta');
    });
  });

  describe('isBrowserLaunchError', () => {
    test('detects browser launch errors', () => {
      expect(isBrowserLaunchError(new Error('ENOENT'))).toBe(true);
      expect(isBrowserLaunchError(new Error('spawn failed'))).toBe(true);
      expect(isBrowserLaunchError(new Error('Failed to launch browser'))).toBe(true);
      expect(isBrowserLaunchError(new Error('Network error'))).toBe(false);
    });
  });

  describe('authenticate', () => {
    test('uses device flow for headless environment', async () => {
      process.env.SSH_CLIENT = '192.168.1.1 54321 22';

      const mockDeviceResponse = {
        device_code: 'device-123',
        user_code: 'ABCD-1234',
        verification_uri: 'https://okta.com/activate',
        expires_in: 600,
        interval: 5,
      };

      const mockTokenResponse = {
        access_token: 'access-123',
        token_type: 'Bearer',
        expires_in: 3600,
        refresh_token: 'refresh-123',
      };

      (global.fetch as jest.Mock)
        .mockResolvedValueOnce({
          ok: true,
          json: async () => mockDeviceResponse,
        })
        .mockResolvedValue({
          ok: true,
          json: async () => mockTokenResponse,
        });

      jest.useFakeTimers();

      const authPromise = authenticate({
        oktaDomain: '[REDACTED_EMPLOYER].okta.com',
        clientId: 'client-123',
        scopes: ['openid'],
      });

      await jest.advanceTimersByTimeAsync(5000);
      await authPromise;

      jest.useRealTimers();

      delete process.env.SSH_CLIENT;
    });
  });

  describe('Edge Cases', () => {
    /**
     * Test 1: PKCE verifier length validation
     */
    test('generates PKCE verifier with exactly 128 characters', () => {
      const verifier = generateCodeVerifier();
      expect(verifier).toHaveLength(128);
    });

    /**
     * Test 2: PKCE challenge format validation
     */
    test('generates PKCE challenge in base64url format (43 chars)', () => {
      const verifier = generateCodeVerifier();
      const challenge = generateCodeChallenge(verifier);

      // SHA-256 hash (32 bytes) → 43 base64url characters
      expect(challenge).toHaveLength(43);
      // Should only contain base64url characters
      expect(challenge).toMatch(/^[A-Za-z0-9_-]+$/);
    });

    /**
     * Test 3: State parameter length validation
     */
    test('generates state parameter with exactly 32 characters', () => {
      const state = generateState();
      expect(state).toHaveLength(32);
    });

    /**
     * Test 4: PKCE generation consistency
     */
    test('generatePKCE returns complete PKCE object with all fields', () => {
      const pkce = generatePKCE();

      expect(pkce.verifier).toHaveLength(128);
      expect(pkce.challenge).toHaveLength(43);
      expect(pkce.state).toHaveLength(32);
    });

    /**
     * Test 5: Expired authorization code handling
     */
    test('exchangeCodeForTokens handles expired authorization code', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: false,
        status: 400,
        json: async () => ({
          error: 'invalid_grant',
          error_description: 'Authorization code expired',
        }),
      });

      await expect(
        exchangeCodeForTokens(
          '[REDACTED_EMPLOYER].okta.com',
          'client-123',
          'expired-code',
          'verifier',
          'http://localhost:3000/callback'
        )
      ).rejects.toThrow('Authorization failed');
    });

    /**
     * Test 6: Already-used authorization code handling
     */
    test('exchangeCodeForTokens handles already-used authorization code', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: false,
        status: 400,
        json: async () => ({
          error: 'invalid_grant',
          error_description: 'Authorization code already used',
        }),
      });

      await expect(
        exchangeCodeForTokens(
          '[REDACTED_EMPLOYER].okta.com',
          'client-123',
          'used-code',
          'verifier',
          'http://localhost:3000/callback'
        )
      ).rejects.toThrow('Authorization failed');
    });

    /**
     * Test 7: Token response missing access_token
     */
    test('exchangeCodeForTokens validates token response has access_token', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: true,
        json: async () => ({
          token_type: 'Bearer',
          expires_in: 3600,
          // Missing access_token
        }),
      });

      await expect(
        exchangeCodeForTokens(
          '[REDACTED_EMPLOYER].okta.com',
          'client-123',
          'code',
          'verifier',
          'http://localhost:3000/callback'
        )
      ).rejects.toThrow('Invalid token response');
    });

    /**
     * Test 8: Token response missing expires_in
     */
    test('exchangeCodeForTokens validates token response has expires_in', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: true,
        json: async () => ({
          access_token: 'token',
          token_type: 'Bearer',
          // Missing expires_in
        }),
      });

      await expect(
        exchangeCodeForTokens(
          '[REDACTED_EMPLOYER].okta.com',
          'client-123',
          'code',
          'verifier',
          'http://localhost:3000/callback'
        )
      ).rejects.toThrow('Invalid token response');
    });

    /**
     * Test 9: Device code response validation
     */
    test('requestDeviceCode validates response has required fields', async () => {
      (global.fetch as jest.Mock).mockResolvedValue({
        ok: true,
        json: async () => ({
          device_code: 'device-123',
          user_code: 'ABCD-1234',
          // Missing verification_uri, expires_in, interval
        }),
      });

      await expect(
        requestDeviceCode('[REDACTED_EMPLOYER].okta.com', 'client-123', ['openid'])
      ).rejects.toThrow('Invalid device authorization response');
    });

    /**
     * Test 10: Configuration validation (missing oktaDomain)
     */
    test('deviceFlowAuth validates oktaDomain is provided', async () => {
      await expect(
        deviceFlowAuth({
          oktaDomain: '',
          clientId: 'client-123',
          scopes: ['openid'],
        })
      ).rejects.toThrow('Missing Okta configuration');
    });

    /**
     * Test 11: Configuration validation (missing clientId)
     */
    test('deviceFlowAuth validates clientId is provided', async () => {
      await expect(
        deviceFlowAuth({
          oktaDomain: '[REDACTED_EMPLOYER].okta.com',
          clientId: '',
          scopes: ['openid'],
        })
      ).rejects.toThrow('Missing Okta configuration');
    });

    /**
     * Test 12: Authorization URL building
     */
    test('buildAuthorizationUrl constructs correct OAuth URL', () => {
      const url = buildAuthorizationUrl(
        '[REDACTED_EMPLOYER].okta.com',
        'client-123',
        'http://localhost:3000/callback',
        ['openid', 'profile', 'email'],
        'challenge-abc',
        'state-xyz'
      );

      expect(url).toContain('https://[REDACTED_EMPLOYER].okta.com/oauth2/v1/authorize');
      expect(url).toContain('client_id=client-123');
      expect(url).toContain('redirect_uri=http%3A%2F%2Flocalhost%3A3000%2Fcallback');
      expect(url).toContain('response_type=code');
      expect(url).toContain('scope=openid+profile+email');
      expect(url).toContain('code_challenge=challenge-abc');
      expect(url).toContain('code_challenge_method=S256');
      expect(url).toContain('state=state-xyz');
    });
  });

  describe('Performance Tests', () => {
    /**
     * Test 1: PKCE generation performance
     */
    test('PKCE generation completes within 100ms', () => {
      const start = Date.now();
      generatePKCE();
      const duration = Date.now() - start;

      expect(duration).toBeLessThan(100);
    });

    /**
     * Test 2: Code verifier generation performance
     */
    test('code verifier generation completes within 50ms', () => {
      const start = Date.now();
      generateCodeVerifier();
      const duration = Date.now() - start;

      expect(duration).toBeLessThan(50);
    });

    /**
     * Test 3: Code challenge generation performance
     */
    test('code challenge generation completes within 50ms', () => {
      const start = Date.now();
      const verifier = generateCodeVerifier();

      const start2 = Date.now();
      generateCodeChallenge(verifier);
      const duration = Date.now() - start2;

      expect(duration).toBeLessThan(50);
    });

    /**
     * Test 4: State generation performance
     */
    test('state generation completes within 50ms', () => {
      const start = Date.now();
      generateState();
      const duration = Date.now() - start;

      expect(duration).toBeLessThan(50);
    });

    /**
     * Test 5: Environment detection performance
     */
    test('environment detection completes within 10ms', () => {
      const start = Date.now();
      detectEnvironment();
      const duration = Date.now() - start;

      expect(duration).toBeLessThan(10);
    });

    /**
     * Test 6: Random port selection performance
     */
    test('random port selection completes within 10ms', () => {
      const start = Date.now();
      for (let i = 0; i < 100; i++) {
        selectRandomPort();
      }
      const duration = Date.now() - start;

      // 100 iterations should complete in less than 10ms
      expect(duration).toBeLessThan(10);
    });
  });
});
