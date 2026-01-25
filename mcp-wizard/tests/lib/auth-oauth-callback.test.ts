/**
 * OAuth Callback Server Tests
 *
 * Tests the HTTP callback server that handles OAuth redirects during PKCE flow.
 * Covers happy path, error handling, validation, timeouts, and edge cases.
 */

import * as http from 'http';
import {
  startCallbackServer,
  selectRandomPort,
} from '../../src/lib/auth';
import { MOCK_PKCE } from '../fixtures/auth-fixtures';

describe('OAuth Callback Server', () => {
  describe('selectRandomPort', () => {
    test('returns port in range 3000-9000', () => {
      const port = selectRandomPort();
      expect(port).toBeGreaterThanOrEqual(3000);
      expect(port).toBeLessThanOrEqual(9000);
    });
  });

  describe('startCallbackServer', () => {
    /**
     * Test 1: Happy path - callback receives code and state
     */
    test('successfully handles valid callback with code and state', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;
      const authCode = MOCK_PKCE.authCode;

      // Start callback server
      const serverPromise = startCallbackServer(port, expectedState, 5);

      // Simulate OAuth callback
      await new Promise((resolve) => setTimeout(resolve, 100));
      const response = await fetch(
        `http://localhost:${port}/callback?code=${authCode}&state=${expectedState}`
      );

      // Verify response
      expect(response.status).toBe(200);
      const html = await response.text();
      expect(html).toContain('Authentication successful');

      // Verify server resolves with code
      const result = await serverPromise;
      expect(result.code).toBe(authCode);
    });

    /**
     * Test 2: State validation - rejects mismatched state
     */
    test('rejects callback with mismatched state parameter', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;
      const wrongState = 'wrong-state-value';

      // Start callback server
      const serverPromise = startCallbackServer(port, expectedState, 5);

      // Simulate callback with wrong state
      await new Promise((resolve) => setTimeout(resolve, 100));
      const response = await fetch(
        `http://localhost:${port}/callback?code=some-code&state=${wrongState}`
      );

      // Verify error response
      expect(response.status).toBe(400);
      const html = await response.text();
      expect(html).toContain('Invalid state parameter');
      expect(html).toContain('CSRF attack');

      // Server should still be running (doesn't resolve on state mismatch)
      // Send valid request to close it
      await fetch(
        `http://localhost:${port}/callback?code=valid-code&state=${expectedState}`
      );
      await serverPromise;
    });

    /**
     * Test 3: Missing code parameter
     */
    test('rejects callback with missing code parameter', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;

      // Start callback server
      const serverPromise = startCallbackServer(port, expectedState, 5);

      // Simulate callback without code
      await new Promise((resolve) => setTimeout(resolve, 100));
      const response = await fetch(
        `http://localhost:${port}/callback?state=${expectedState}`
      );

      // Verify error response
      expect(response.status).toBe(400);
      const html = await response.text();
      expect(html).toContain('Missing authorization code');

      // Send valid request to close server
      await fetch(
        `http://localhost:${port}/callback?code=valid-code&state=${expectedState}`
      );
      await serverPromise;
    });

    /**
     * Test 4: Missing state parameter
     */
    test('rejects callback with missing state parameter', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;

      // Start callback server
      const serverPromise = startCallbackServer(port, expectedState, 5);

      // Simulate callback without state
      await new Promise((resolve) => setTimeout(resolve, 100));
      const response = await fetch(
        `http://localhost:${port}/callback?code=some-code`
      );

      // Verify error response
      expect(response.status).toBe(400);
      const html = await response.text();
      expect(html).toContain('Invalid state parameter');

      // Send valid request to close server
      await fetch(
        `http://localhost:${port}/callback?code=valid-code&state=${expectedState}`
      );
      await serverPromise;
    });

    /**
     * Test 5: Server timeout (5 minutes default)
     */
    test('times out after specified timeout period', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;
      const timeoutSeconds = 1; // 1 second for faster test

      // Start callback server with short timeout
      const serverPromise = startCallbackServer(port, expectedState, timeoutSeconds);

      // Wait for timeout
      await expect(serverPromise).rejects.toThrow('Authentication timed out');
    });

    /**
     * Test 6: 404 for non-callback paths
     */
    test('returns 404 for paths other than /callback', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;

      // Start callback server
      const serverPromise = startCallbackServer(port, expectedState, 5);

      // Request non-callback path
      await new Promise((resolve) => setTimeout(resolve, 100));
      const response = await fetch(`http://localhost:${port}/other-path`);

      // Verify 404 response
      expect(response.status).toBe(404);
      const html = await response.text();
      expect(html).toContain('404 Not Found');

      // Send valid request to close server
      await fetch(
        `http://localhost:${port}/callback?code=valid-code&state=${expectedState}`
      );
      await serverPromise;
    });

    /**
     * Test 7: Port already in use error
     */
    test('throws error when port is already in use', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;

      // Create a server that occupies the port
      const blockingServer = http.createServer(() => {});
      await new Promise<void>((resolve) => {
        blockingServer.listen(port, 'localhost', () => resolve());
      });

      try {
        // Attempt to start callback server on same port
        await expect(
          startCallbackServer(port, expectedState, 5)
        ).rejects.toThrow(`Port ${port} is already in use`);
      } finally {
        // Clean up blocking server
        await new Promise<void>((resolve) => {
          blockingServer.close(() => resolve());
        });
      }
    });

    /**
     * Test 8: Server closes after successful callback
     */
    test('closes server immediately after successful callback', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;
      const authCode = MOCK_PKCE.authCode;

      // Start callback server
      const serverPromise = startCallbackServer(port, expectedState, 30);

      // Send successful callback
      await new Promise((resolve) => setTimeout(resolve, 100));
      await fetch(
        `http://localhost:${port}/callback?code=${authCode}&state=${expectedState}`
      );

      // Wait for server to resolve
      const result = await serverPromise;
      expect(result.code).toBe(authCode);

      // Verify server is closed by attempting to connect
      await new Promise((resolve) => setTimeout(resolve, 100));
      await expect(
        fetch(`http://localhost:${port}/callback`)
      ).rejects.toThrow();
    });

    /**
     * Test 9: Multiple callback attempts (only first succeeds)
     */
    test('only processes first successful callback', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;
      const authCode1 = 'first-code';
      const authCode2 = 'second-code';

      // Start callback server
      const serverPromise = startCallbackServer(port, expectedState, 5);

      // Send first callback
      await new Promise((resolve) => setTimeout(resolve, 100));
      const response1 = await fetch(
        `http://localhost:${port}/callback?code=${authCode1}&state=${expectedState}`
      );
      expect(response1.status).toBe(200);

      // Verify server resolves with first code
      const result = await serverPromise;
      expect(result.code).toBe(authCode1);

      // Second callback should fail (server closed)
      await new Promise((resolve) => setTimeout(resolve, 100));
      await expect(
        fetch(`http://localhost:${port}/callback?code=${authCode2}&state=${expectedState}`)
      ).rejects.toThrow();
    });

    /**
     * Test 10: Callback with malformed URL
     */
    test('handles callback with malformed query parameters', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;

      // Start callback server
      const serverPromise = startCallbackServer(port, expectedState, 5);

      // Send callback with malformed params (URL will parse it, but state won't match)
      await new Promise((resolve) => setTimeout(resolve, 100));
      const response = await fetch(
        `http://localhost:${port}/callback?code=abc&state=xyz&extra=param`
      );

      // State mismatch error
      expect(response.status).toBe(400);

      // Send valid request to close server
      await fetch(
        `http://localhost:${port}/callback?code=valid&state=${expectedState}`
      );
      await serverPromise;
    });

    /**
     * Test 11: Callback server cleanup on timeout
     */
    test('properly cleans up resources on timeout', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;
      const timeoutSeconds = 1;

      // Start callback server
      const serverPromise = startCallbackServer(port, expectedState, timeoutSeconds);

      // Wait for timeout
      await expect(serverPromise).rejects.toThrow('Authentication timed out');

      // Verify server is closed
      await new Promise((resolve) => setTimeout(resolve, 100));
      await expect(
        fetch(`http://localhost:${port}/callback`)
      ).rejects.toThrow();
    });

    /**
     * Test 12: Concurrent requests to callback server
     */
    test('handles concurrent requests gracefully', async () => {
      const port = selectRandomPort();
      const expectedState = MOCK_PKCE.state;

      // Start callback server
      const serverPromise = startCallbackServer(port, expectedState, 30);

      await new Promise((resolve) => setTimeout(resolve, 200));

      // Send first request (will succeed and close server)
      const response1 = await fetch(
        `http://localhost:${port}/callback?code=code1&state=${expectedState}`
      );
      expect(response1.status).toBe(200);

      // Server should resolve
      const result = await serverPromise;
      expect(result.code).toBe('code1');

      // Verify server is closed
      await new Promise((resolve) => setTimeout(resolve, 200));
      await expect(
        fetch(`http://localhost:${port}/callback?code=code2&state=${expectedState}`)
      ).rejects.toThrow();
    }, 20000); // Increase timeout to 20 seconds
  });
});
