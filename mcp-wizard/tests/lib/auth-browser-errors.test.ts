/**
 * Browser Error Handling and Fallback Tests
 *
 * Tests browser launch error detection, environment detection, and
 * automatic fallback to device flow when browser-based auth fails.
 */

import {
  detectEnvironment,
  isBrowserLaunchError,
  launchBrowser,
  authenticate,
  deviceFlowAuth,
  browserPKCEAuth,
} from '../../src/lib/auth';
import { MOCK_ENV } from '../fixtures/auth-fixtures';
import * as open from 'open';

// Mock dependencies
jest.mock('../../src/lib/token-storage');
jest.mock('open', () => {
  const mockFn = jest.fn().mockResolvedValue({} as any);
  return {
    __esModule: true,
    default: mockFn,
  };
});

describe('Browser Error Handling and Fallback', () => {
  describe('detectEnvironment', () => {
    /**
     * Test 1: SSH environment detection
     */
    test('detects SSH environment via SSH_CLIENT', () => {
      const originalEnv = { ...process.env };
      process.env.SSH_CLIENT = '192.168.1.1 12345 22';

      const result = detectEnvironment();

      expect(result.type).toBe('headless');
      expect(result.sshDetected).toBe(true);

      process.env = originalEnv;
    });

    /**
     * Test 2: SSH environment detection via SSH_TTY
     */
    test('detects SSH environment via SSH_TTY', () => {
      const originalEnv = { ...process.env };
      process.env.SSH_TTY = '/dev/pts/0';

      const result = detectEnvironment();

      expect(result.type).toBe('headless');
      expect(result.sshDetected).toBe(true);

      process.env = originalEnv;
    });

    /**
     * Test 3: Cloud shell environment detection
     */
    test('detects Google Cloud Shell environment', () => {
      const originalEnv = { ...process.env };
      process.env.CLOUD_SHELL = 'true';

      const result = detectEnvironment();

      expect(result.type).toBe('headless');
      expect(result.cloudShellDetected).toBe(true);

      process.env = originalEnv;
    });

    /**
     * Test 4: No TTY environment detection
     */
    test('detects headless when no TTY available', () => {
      // Mock process.stdout.isTTY
      const originalIsTTY = process.stdout.isTTY;
      Object.defineProperty(process.stdout, 'isTTY', {
        value: false,
        configurable: true,
      });

      const result = detectEnvironment();

      expect(result.type).toBe('headless');
      expect(result.ttyAvailable).toBe(false);

      // Restore
      Object.defineProperty(process.stdout, 'isTTY', {
        value: originalIsTTY,
        configurable: true,
      });
    });

    /**
     * Test 5: Interactive environment detection
     */
    test('detects interactive environment when TTY available and no SSH', () => {
      const originalEnv = { ...process.env };
      delete process.env.SSH_CLIENT;
      delete process.env.SSH_TTY;
      delete process.env.SSH_CONNECTION;
      delete process.env.CLOUD_SHELL;

      // Mock TTY available
      const originalIsTTY = process.stdout.isTTY;
      Object.defineProperty(process.stdout, 'isTTY', {
        value: true,
        configurable: true,
      });

      const result = detectEnvironment();

      expect(result.type).toBe('interactive');
      expect(result.sshDetected).toBe(false);
      expect(result.cloudShellDetected).toBe(false);
      expect(result.ttyAvailable).toBe(true);

      // Restore
      process.env = originalEnv;
      Object.defineProperty(process.stdout, 'isTTY', {
        value: originalIsTTY,
        configurable: true,
      });
    });
  });

  describe('isBrowserLaunchError', () => {
    /**
     * Test 6: Detects ENOENT error (browser not found)
     */
    test('identifies ENOENT as browser launch error', () => {
      const error = new Error('spawn ENOENT: browser not found');
      expect(isBrowserLaunchError(error)).toBe(true);
    });

    /**
     * Test 7: Detects spawn error
     */
    test('identifies spawn errors as browser launch error', () => {
      const error = new Error('spawn failed to launch browser');
      expect(isBrowserLaunchError(error)).toBe(true);
    });

    /**
     * Test 8: Detects generic browser error
     */
    test('identifies browser keyword errors', () => {
      const error = new Error('browser launch failed');
      expect(isBrowserLaunchError(error)).toBe(true);
    });

    /**
     * Test 9: Detects our custom browser launch error
     */
    test('identifies our custom browser launch error message', () => {
      const error = new Error('Failed to launch browser: ENOENT');
      expect(isBrowserLaunchError(error)).toBe(true);
    });

    /**
     * Test 10: Does not misidentify other errors
     */
    test('does not misidentify network errors', () => {
      const error = new Error('Network request failed');
      expect(isBrowserLaunchError(error)).toBe(false);
    });

    /**
     * Test 11: Does not misidentify timeout errors
     */
    test('does not misidentify timeout errors', () => {
      const error = new Error('Authentication timed out');
      expect(isBrowserLaunchError(error)).toBe(false);
    });
  });

  describe('launchBrowser error scenarios', () => {
    beforeEach(() => {
      jest.clearAllMocks();
    });

    /**
     * Test 12: Browser launch fails with ENOENT
     */
    test('throws error when browser binary not found', async () => {
      (open.default as jest.Mock).mockRejectedValue(new Error('spawn ENOENT'));

      await expect(
        launchBrowser('https://example.com')
      ).rejects.toThrow('Failed to launch browser');
    });

    /**
     * Test 13: Browser launch fails with permission denied
     */
    test('throws error when browser launch permission denied', async () => {
      (open.default as jest.Mock).mockRejectedValue(new Error('EACCES: permission denied'));

      await expect(
        launchBrowser('https://example.com')
      ).rejects.toThrow('Failed to launch browser');
    });

    /**
     * Test 14: Browser launch succeeds
     */
    test('successfully launches browser when available', async () => {
      (open.default as jest.Mock).mockResolvedValue(undefined);

      await launchBrowser('https://example.com');

      expect(open.default).toHaveBeenCalledWith('https://example.com');
    });
  });

  describe('authenticate fallback behavior', () => {
    beforeEach(() => {
      jest.clearAllMocks();

      // Mock token storage
      const tokenStorage = require('../../src/lib/token-storage');
      tokenStorage.storeOktaToken.mockResolvedValue(undefined);
    });

    /**
     * Test 15: Uses device flow in SSH environment
     */
    test('automatically uses device flow in SSH environment', async () => {
      const originalEnv = { ...process.env };
      process.env.SSH_CLIENT = '192.168.1.1 12345 22';

      // Mock device flow functions
      const mockFetch = jest.spyOn(global, 'fetch');
      mockFetch.mockImplementation((url: any) => {
        if (url.includes('/device/authorize')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({
              device_code: 'device-code',
              user_code: 'ABCD-1234',
              verification_uri: 'https://[REDACTED_EMPLOYER].okta.com/activate',
              expires_in: 600,
              interval: 5,
            }),
          } as any);
        }
        if (url.includes('/token')) {
          return Promise.resolve({
            ok: true,
            json: () => Promise.resolve({
              access_token: 'access-token',
              token_type: 'Bearer',
              expires_in: 3600,
              refresh_token: 'refresh-token',
            }),
          } as any);
        }
        return Promise.reject(new Error('Unknown URL'));
      });

      // Mock console.log to suppress output
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      // Use fake timers to speed up polling
      jest.useFakeTimers();

      try {
        const authPromise = authenticate({
          oktaDomain: MOCK_ENV.oktaDomain,
          clientId: MOCK_ENV.clientId,
          scopes: MOCK_ENV.scopes,
        });

        // Advance timers to simulate polling
        await jest.advanceTimersByTimeAsync(10000);

        await authPromise;

        // Verify device flow was used (console messages)
        expect(consoleSpy).toHaveBeenCalledWith(
          expect.stringContaining('Headless environment detected')
        );
      } finally {
        jest.useRealTimers();
        mockFetch.mockRestore();
        consoleSpy.mockRestore();
        process.env = originalEnv;
      }
    }, 15000);
  });
});
