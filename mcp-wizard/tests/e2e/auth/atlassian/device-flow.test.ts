/**
 * Atlassian MCP: Device Flow E2E Tests
 * Tests OAuth Device Authorization Grant flow for Jira + Confluence
 */

import { atlassianConfig } from '../fixtures/mcp-configs';
import { advancePolling } from '../helpers/oauth-flows';
import {
  deviceFlowAuth,
  requestDeviceCode,
  pollForToken
} from '../../../../src/lib/auth';

// Mock external dependencies only
jest.mock('open');
jest.mock('ora', () => require('../../../__mocks__/ora.js'));
jest.mock('inquirer', () => require('../../../__mocks__/inquirer.js'));

// Mock keytar for token storage
jest.mock('keytar', () => ({
  getPassword: jest.fn(),
  setPassword: jest.fn(),
  deletePassword: jest.fn(),
}));

// Mock fetch
global.fetch = jest.fn() as jest.Mock;

describe('Atlassian - Device Flow E2E Tests', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe('Atlassian-Specific OAuth Scopes', () => {
    test('requests correct Jira and Confluence scopes', async () => {
      const mockFetch = global.fetch as jest.Mock;

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          device_code: 'atlassian-device-code',
          user_code: 'JIRA-1234',
          verification_uri: 'http://localhost:8080/activate',
          expires_in: 600,
          interval: 5
        })
      });

      const result = await requestDeviceCode(
        'localhost:8080',
        atlassianConfig.clientId,
        atlassianConfig.scopes
      );

      // Verify request was made with correct scopes
      const requestBody = mockFetch.mock.calls[0][1].body;
      const params = new URLSearchParams(requestBody);
      const scopeParam = params.get('scope');
      expect(scopeParam).toContain('read:jira-work');
      expect(scopeParam).toContain('read:confluence-content.all');
      expect(scopeParam).toContain('read:me');
    });

    test('completes device flow with Atlassian scopes', async () => {
      const mockFetch = global.fetch as jest.Mock;
      const mockSetPassword = require('keytar').setPassword;

      // Mock device code request
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          device_code: 'atlassian-device-code',
          user_code: 'CONF-5678',
          verification_uri: 'http://localhost:8080/activate',
          expires_in: 600,
          interval: 1
        })
      });

      // Mock token polling success
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          access_token: 'test-atlassian-access-token',
          token_type: 'Bearer',
          refresh_token: 'test-atlassian-refresh-token',
          expires_in: 3600,
          scope: atlassianConfig.scopes.join(' ')
        })
      });

      mockSetPassword.mockResolvedValue(undefined);

      jest.useFakeTimers();

      const authPromise = deviceFlowAuth({
        oktaDomain: 'localhost:8080',
        clientId: atlassianConfig.clientId,
        scopes: atlassianConfig.scopes
      });

      await jest.advanceTimersByTimeAsync(1000);
      await authPromise;

      // Verify token storage with Atlassian scopes (tokens stored as individual fields)
      expect(mockSetPassword).toHaveBeenCalled();
      // Token storage uses multiple setPassword calls for individual fields
      const accessTokenCall = mockSetPassword.mock.calls.find(
        (call: any[]) => call[1] === 'access-token'
      );
      expect(accessTokenCall[2]).toBe('test-atlassian-access-token');

      jest.useRealTimers();
    });

    test('validates Atlassian-specific scope format', async () => {
      const mockFetch = global.fetch as jest.Mock;

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          device_code: 'device-code',
          user_code: 'USER-CODE',
          verification_uri: 'http://localhost:8080/activate',
          expires_in: 600,
          interval: 5
        })
      });

      await requestDeviceCode(
        'localhost:8080',
        atlassianConfig.clientId,
        atlassianConfig.scopes
      );

      const requestBody = mockFetch.mock.calls[0][1].body;
      const params = new URLSearchParams(requestBody);
      const scopeParam = params.get('scope');

      // Verify scopes are space-separated
      expect(scopeParam).toBe('read:jira-work read:confluence-content.all read:me');

      // Verify each scope follows Atlassian naming convention
      const scopes = scopeParam!.split(' ');
      scopes.forEach(scope => {
        expect(scope).toMatch(/^(read|write):[a-z-]+(\.[a-z]+)?$/);
      });
    });
  });

  describe('Error Handling', () => {
    test('handles authorization denial', async () => {
      const mockFetch = global.fetch as jest.Mock;

      jest.useFakeTimers();

      mockFetch.mockResolvedValueOnce({
        ok: false,
        json: async () => ({
          error: 'access_denied',
          error_description: 'User denied Atlassian access'
        })
      });

      const pollingPromise = pollForToken(
        'localhost:8080',
        atlassianConfig.clientId,
        'device-code',
        1,
        600
      );

      // Catch promise rejection
      pollingPromise.catch(() => {});  // Prevent unhandled rejection

      await jest.advanceTimersByTimeAsync(1000);

      await expect(pollingPromise).rejects.toThrow('Authorization denied');

      jest.useRealTimers();
    });

    test('handles invalid Atlassian client ID', async () => {
      const mockFetch = global.fetch as jest.Mock;

      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 401,
        text: async () => 'Client authentication failed'
      });

      await expect(
        requestDeviceCode(
          'localhost:8080',
          'invalid-client-id',
          atlassianConfig.scopes
        )
      ).rejects.toThrow();
    });

    test('handles Atlassian API rate limiting', async () => {
      const mockFetch = global.fetch as jest.Mock;

      jest.useFakeTimers();

      // Simulate rate limit with slow_down
      mockFetch
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({ error: 'authorization_pending' })
        })
        .mockResolvedValueOnce({
          ok: false,
          json: async () => ({
            error: 'slow_down',
            error_description: 'Rate limit reached'
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
        atlassianConfig.clientId,
        'device-code',
        1,
        600
      );

      await jest.advanceTimersByTimeAsync(1000);  // First poll
      await jest.advanceTimersByTimeAsync(1000);  // slow_down
      await jest.advanceTimersByTimeAsync(6000);  // Third poll (5s increase)

      const result = await pollingPromise;
      expect(result.access_token).toBe('test-token');

      jest.useRealTimers();
    });
  });

  describe('Atlassian Multi-Product Support', () => {
    test('supports both Jira and Confluence scopes simultaneously', async () => {
      const mockFetch = global.fetch as jest.Mock;

      const combinedScopes = [
        'read:jira-work',
        'write:jira-work',
        'read:confluence-content.all',
        'write:confluence-content',
        'read:me'
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          device_code: 'multi-product-code',
          user_code: 'MULTI-1234',
          verification_uri: 'http://localhost:8080/activate',
          expires_in: 600,
          interval: 5
        })
      });

      await requestDeviceCode(
        'localhost:8080',
        atlassianConfig.clientId,
        combinedScopes
      );

      const requestBody = mockFetch.mock.calls[0][1].body;
      const params = new URLSearchParams(requestBody);
      const scopeParam = params.get('scope');
      expect(scopeParam).toContain('read:jira-work');
      expect(scopeParam).toContain('write:jira-work');
      expect(scopeParam).toContain('read:confluence-content.all');
      expect(scopeParam).toContain('write:confluence-content');
    });
  });

  describe('Token Refresh', () => {
    test('stores refresh token for Atlassian APIs', async () => {
      const mockFetch = global.fetch as jest.Mock;
      const mockSetPassword = require('keytar').setPassword;

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          device_code: 'device-code',
          user_code: 'USER-CODE',
          verification_uri: 'http://localhost:8080/activate',
          expires_in: 600,
          interval: 1
        })
      });

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          access_token: 'atlassian-access',
          token_type: 'Bearer',
          refresh_token: 'atlassian-refresh',
          expires_in: 3600
        })
      });

      mockSetPassword.mockResolvedValue(undefined);

      jest.useFakeTimers();

      const authPromise = deviceFlowAuth({
        oktaDomain: 'localhost:8080',
        clientId: atlassianConfig.clientId,
        scopes: atlassianConfig.scopes
      });

      await jest.advanceTimersByTimeAsync(1000);
      await authPromise;

      // Tokens are stored as individual fields, not as JSON
      const refreshTokenCall = mockSetPassword.mock.calls.find(
        (call: any[]) => call[1] === 'refresh-token'
      );
      const accessTokenCall = mockSetPassword.mock.calls.find(
        (call: any[]) => call[1] === 'access-token'
      );

      expect(refreshTokenCall[2]).toBe('atlassian-refresh');
      expect(accessTokenCall[2]).toBe('atlassian-access');

      jest.useRealTimers();
    });
  });
});
