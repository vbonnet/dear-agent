/**
 * GoogleDocs MCP: Device Flow E2E Tests
 * Tests OAuth Device Authorization Grant flow with oauth2-mock-server
 */

import { googleDocsConfig } from '../fixtures/mcp-configs';
import { advancePolling } from '../helpers/oauth-flows';
import {
  deviceFlowAuth,
  requestDeviceCode,
  pollForToken,
  displayUserCode,
  detectEnvironment
} from '../../../../src/lib/auth';
import * as tokenStorage from '../../../../src/lib/token-storage';

// Mock external dependencies only (not auth functions)
jest.mock('open');
jest.mock('ora', () => require('../../../__mocks__/ora.js'));
jest.mock('inquirer', () => require('../../../__mocks__/inquirer.js'));

// Mock keytar for token storage
jest.mock('keytar', () => ({
  getPassword: jest.fn(),
  setPassword: jest.fn(),
  deletePassword: jest.fn(),
}));

// Mock fetch for OAuth server requests
global.fetch = jest.fn() as jest.Mock;

describe('GoogleDocs - Device Flow E2E Tests', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe('Device Flow Success Scenarios', () => {
    test('completes device flow with authorization pending → success', async () => {
      const mockFetch = global.fetch as jest.Mock;

      // Step 1: Mock device code request
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          device_code: 'DEVICE-CODE-TEST-12345',
          user_code: 'WXYZ-9876',
          verification_uri: 'http://localhost:8080/activate',
          verification_uri_complete: 'http://localhost:8080/activate?user_code=WXYZ-9876',
          expires_in: 600,
          interval: 1
        })
      });

      // Step 2: Mock token polling (authorization_pending → success)
      mockFetch
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({
            error: 'authorization_pending',
            error_description: 'Authorization pending. Continue polling.'
          })
        })
        .mockResolvedValueOnce({
          ok: true,
          json: async () => ({
            access_token: 'test-access-token',
            token_type: 'Bearer',
            refresh_token: 'test-refresh-token',
            expires_in: 3600,
            scope: googleDocsConfig.scopes.join(' ')
          })
        });

      // Mock token storage
      const mockSetPassword = require('keytar').setPassword;
      mockSetPassword.mockResolvedValue(undefined);

      // Execute device flow
      await deviceFlowAuth({
        oktaDomain: 'localhost:8080',
        clientId: googleDocsConfig.clientId,
        scopes: googleDocsConfig.scopes
      });

      // Verify device code request (implementation uses https://)
      expect(mockFetch).toHaveBeenCalledWith(
        'https://localhost:8080/oauth2/v1/device/authorize',
        expect.objectContaining({
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' }
        })
      );

      // Verify token storage was called
      expect(mockSetPassword).toHaveBeenCalled();
    });

    test('validates device code request format', async () => {
      const mockFetch = global.fetch as jest.Mock;

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          device_code: 'DEVICE-CODE-TEST',
          user_code: 'ABCD-1234',
          verification_uri: 'http://localhost:8080/activate',
          expires_in: 600,
          interval: 5
        })
      });

      const result = await requestDeviceCode(
        'localhost:8080',
        googleDocsConfig.clientId,
        googleDocsConfig.scopes
      );

      expect(result.device_code).toBe('DEVICE-CODE-TEST');
      expect(result.user_code).toBe('ABCD-1234');
      expect(result.verification_uri).toBe('http://localhost:8080/activate');
      expect(result.interval).toBe(5);
    });

    test('displays user code correctly', () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      displayUserCode('http://localhost:8080/activate', 'WXYZ-9876', 600);

      expect(consoleSpy).toHaveBeenCalled();
      const output = consoleSpy.mock.calls.map(call => call.join(' ')).join('\n');
      expect(output).toContain('WXYZ-9876');
      expect(output).toContain('http://localhost:8080/activate');

      consoleSpy.mockRestore();
    });
  });

  describe('Device Flow Error Scenarios', () => {
    test('handles slow_down response correctly', async () => {
      const mockFetch = global.fetch as jest.Mock;

      jest.useFakeTimers();

      // Mock responses: authorization_pending, slow_down, success
      mockFetch
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'authorization_pending' })
        })
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({
            error: 'slow_down',
            error_description: 'Polling too fast. Increase interval by 5s.'
          })
        })
        .mockResolvedValueOnce({
          ok: true,
          json: async () => ({
            access_token: 'test-token',
            token_type: 'Bearer',
            expires_in: 3600
          })
        });

      const pollingPromise = pollForToken(
        'localhost:8080',
        googleDocsConfig.clientId,
        'device-code-test',
        1,  // 1 second interval
        600
      );

      // Advance timers to trigger polling
      await jest.advanceTimersByTimeAsync(1000); // First poll
      await jest.advanceTimersByTimeAsync(1000); // Second poll (slow_down)
      await jest.advanceTimersByTimeAsync(6000); // Third poll (interval increased by 5s)

      const result = await pollingPromise;

      expect(result.access_token).toBe('test-token');
      expect(mockFetch).toHaveBeenCalledTimes(3);

      jest.useRealTimers();
    });

    test('handles user denial gracefully', async () => {
      const mockFetch = global.fetch as jest.Mock;

      jest.useFakeTimers();

      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => ({
          error: 'access_denied',
          error_description: 'The user denied the authorization request'
        })
      });

      const pollingPromise = pollForToken(
        'localhost:8080',
        googleDocsConfig.clientId,
        'device-code-test',
        1,
        600
      );

      // Catch promise rejection to prevent unhandled rejection
      pollingPromise.catch(() => {});

      await jest.advanceTimersByTimeAsync(1000);

      await expect(pollingPromise).rejects.toThrow('Authorization denied');

      jest.useRealTimers();
    });

    test('handles device code expiration', async () => {
      const mockFetch = global.fetch as jest.Mock;

      jest.useFakeTimers();

      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => ({
          error: 'expired_token',
          error_description: 'The device code has expired'
        })
      });

      const pollingPromise = pollForToken(
        'localhost:8080',
        googleDocsConfig.clientId,
        'device-code-test',
        1,
        600
      );

      // Catch promise rejection to prevent unhandled rejection
      pollingPromise.catch(() => {});

      await jest.advanceTimersByTimeAsync(1000);

      await expect(pollingPromise).rejects.toThrow('Device code expired');

      jest.useRealTimers();
    });

    test('handles OAuth provider unavailable', async () => {
      const mockFetch = global.fetch as jest.Mock;

      mockFetch.mockRejectedValueOnce(new Error('Network error'));

      await expect(
        requestDeviceCode(
          'localhost:8080',
          googleDocsConfig.clientId,
          googleDocsConfig.scopes
        )
      ).rejects.toThrow();
    });
  });

  describe('Token Storage Integration', () => {
    test('stores tokens in keychain on success', async () => {
      const mockFetch = global.fetch as jest.Mock;
      const mockSetPassword = require('keytar').setPassword;

      // Mock device code response
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          device_code: 'device-code-test',
          user_code: 'USER-CODE',
          verification_uri: 'http://localhost:8080/activate',
          expires_in: 600,
          interval: 1
        })
      });

      // Mock successful token response
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          access_token: 'access-token-test',
          token_type: 'Bearer',
          refresh_token: 'refresh-token-test',
          expires_in: 3600
        })
      });

      mockSetPassword.mockResolvedValue(undefined);

      jest.useFakeTimers();

      const authPromise = deviceFlowAuth({
        oktaDomain: 'localhost:8080',
        clientId: googleDocsConfig.clientId,
        scopes: googleDocsConfig.scopes
      });

      await jest.advanceTimersByTimeAsync(1000);
      await authPromise;

      // Verify token storage (tokens stored as individual fields)
      expect(mockSetPassword).toHaveBeenCalled();
      const accessTokenCall = mockSetPassword.mock.calls.find(
        (call: any[]) => call[1] === 'access-token'
      );
      expect(accessTokenCall[2]).toBe('access-token-test');

      jest.useRealTimers();
    });
  });

  describe('Environment Detection', () => {
    test('detects headless environment (SSH)', () => {
      const originalEnv = process.env.SSH_CLIENT;
      process.env.SSH_CLIENT = '192.168.1.100 12345 22';

      const env = detectEnvironment();

      expect(env.type).toBe('headless');
      expect(env.sshDetected).toBe(true);

      process.env.SSH_CLIENT = originalEnv;
    });

    test('detects interactive environment', () => {
      const originalEnv = {
        SSH_CLIENT: process.env.SSH_CLIENT,
        SSH_TTY: process.env.SSH_TTY,
        SSH_CONNECTION: process.env.SSH_CONNECTION,
        CLOUD_SHELL: process.env.CLOUD_SHELL
      };

      delete process.env.SSH_CLIENT;
      delete process.env.SSH_TTY;
      delete process.env.SSH_CONNECTION;
      delete process.env.CLOUD_SHELL;

      const env = detectEnvironment();

      expect(env.sshDetected).toBe(false);
      expect(env.cloudShellDetected).toBe(false);

      // Restore
      process.env.SSH_CLIENT = originalEnv.SSH_CLIENT;
      process.env.SSH_TTY = originalEnv.SSH_TTY;
      process.env.SSH_CONNECTION = originalEnv.SSH_CONNECTION;
      process.env.CLOUD_SHELL = originalEnv.CLOUD_SHELL;
    });
  });
});
