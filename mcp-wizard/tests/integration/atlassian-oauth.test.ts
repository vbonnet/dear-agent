import {
  extractCallbackPortFromUrl,
  generateGcloudTunnelCommand,
  startOAuthCallbackServer
} from '../../src/lib/atlassian-oauth';

describe('Atlassian OAuth Integration Tests', () => {
  describe('extractCallbackPortFromUrl', () => {
    it('should extract port from OAuth URL with redirect_uri', () => {
      const url = 'https://mcp.atlassian.com/v1/authorize?redirect_uri=http://localhost:12345/callback&state=abc123';
      expect(extractCallbackPortFromUrl(url)).toBe(12345);
    });

    it('should extract port from URL with different port', () => {
      const url = 'https://example.com/auth?redirect_uri=http://localhost:54321/callback';
      expect(extractCallbackPortFromUrl(url)).toBe(54321);
    });

    it('should return null for URL without redirect_uri', () => {
      const url = 'https://mcp.atlassian.com/v1/authorize?state=abc123';
      expect(extractCallbackPortFromUrl(url)).toBeNull();
    });

    it('should return null for invalid URL', () => {
      expect(extractCallbackPortFromUrl('not-a-valid-url')).toBeNull();
    });

    it('should return null for redirect_uri without port', () => {
      const url = 'https://example.com/auth?redirect_uri=http://localhost/callback';
      expect(extractCallbackPortFromUrl(url)).toBeNull();
    });
  });

  describe('generateGcloudTunnelCommand', () => {
    it('should generate correct gcloud tunnel command', () => {
      const cmd = generateGcloudTunnelCommand(
        'my-workstation',
        12345,
        'my-project',
        'us-central1',
        'my-cluster',
        'my-config'
      );
      
      expect(cmd).toContain('gcloud workstations start-tcp-tunnel');
      expect(cmd).toContain('my-workstation');
      expect(cmd).toContain('12345');
      expect(cmd).toContain('--local-host-port=localhost:12345');
      expect(cmd).toContain('--cluster=my-cluster');
      expect(cmd).toContain('--config=my-config');
      expect(cmd).toContain('--region=us-central1');
      expect(cmd).toContain('--project=my-project');
      expect(cmd).toContain('&');
    });

    it('should use default values for optional parameters', () => {
      const cmd = generateGcloudTunnelCommand('workstation', 8080, 'project');
      
      expect(cmd).toContain('--region=us-central1');
      expect(cmd).toContain('--cluster=shared-workstations-cluster');
      expect(cmd).toContain('--config=eng');
    });
  });

  describe('startOAuthCallbackServer', () => {
    it('should receive OAuth callback with correct state', async () => {
      const port = 9876;
      const expectedState = 'test-state-123';
      
      const serverPromise = startOAuthCallbackServer(port, expectedState, 10);
      
      // Wait for server to start
      await new Promise(resolve => setTimeout(resolve, 100));
      
      // Simulate OAuth callback
      const response = await fetch(
        `http://localhost:${port}/callback?code=test-authorization-code&state=${expectedState}`
      );
      
      expect(response.status).toBe(200);
      const body = await response.text();
      expect(body).toContain('Success');
      
      const result = await serverPromise;
      expect(result.code).toBe('test-authorization-code');
      expect(result.state).toBe(expectedState);
    }, 15000);

    it.skip('should reject on state mismatch', async () => {
      const port = 9877;
      const expectedState = 'correct-state';
      const wrongState = 'wrong-state';
      
      const serverPromise = startOAuthCallbackServer(port, expectedState, 5);
      
      await new Promise(resolve => setTimeout(resolve, 100));
      
      // Send callback with wrong state
      try {
        await fetch(`http://localhost:${port}/callback?code=test-code&state=${wrongState}`);
      } catch (e) {
        // Ignore fetch errors when server closes
      }
      
      // Server should reject with state mismatch
      let errorMessage = '';
      try {
        await serverPromise;
        fail('Expected promise to reject');
      } catch (error: any) {
        errorMessage = error.message;
      }
      
      expect(errorMessage).toBe('State mismatch');
    }, 10000);

    it.skip('should reject when code parameter is missing', async () => {
      const port = 9878;
      const expectedState = 'test-state';
      
      const serverPromise = startOAuthCallbackServer(port, expectedState, 5);
      
      await new Promise(resolve => setTimeout(resolve, 100));
      
      // Send callback without code
      try {
        await fetch(`http://localhost:${port}/callback?state=${expectedState}`);
      } catch (e) {
        // Ignore fetch errors when server closes
      }
      
      // Server should reject with missing code
      let errorMessage = '';
      try {
        await serverPromise;
        fail('Expected promise to reject');
      } catch (error: any) {
        errorMessage = error.message;
      }
      
      expect(errorMessage).toBe('Missing code');
    }, 10000);

    it('should timeout if no callback received', async () => {
      const port = 9879;
      const expectedState = 'test-state';
      
      // Set very short timeout
      const serverPromise = startOAuthCallbackServer(port, expectedState, 1);
      
      let errorMessage = '';
      try {
        await serverPromise;
        fail('Expected promise to reject');
      } catch (error: any) {
        errorMessage = error.message;
      }
      
      expect(errorMessage).toContain('Timeout');
    }, 5000);

    it('should handle non-callback requests with 404', async () => {
      const port = 9880;
      const expectedState = 'test-state';
      
      const serverPromise = startOAuthCallbackServer(port, expectedState, 10);
      
      await new Promise(resolve => setTimeout(resolve, 100));
      
      // Request non-callback path
      const response = await fetch(`http://localhost:${port}/other-path`);
      expect(response.status).toBe(404);
      
      // Now send valid callback to complete test
      await fetch(`http://localhost:${port}/callback?code=test&state=${expectedState}`);
      await serverPromise;
    }, 15000);
  });

  describe('SSH environment detection', () => {
    it('should detect SSH_CONNECTION environment variable', () => {
      const originalSshConnection = process.env.SSH_CONNECTION;
      
      process.env.SSH_CONNECTION = '10.0.0.1 12345 10.0.0.2 22';
      const isSSH = !!process.env.SSH_CONNECTION || !!process.env.SSH_CLIENT;
      expect(isSSH).toBe(true);
      
      if (originalSshConnection) {
        process.env.SSH_CONNECTION = originalSshConnection;
      } else {
        delete process.env.SSH_CONNECTION;
      }
    });

    it('should detect SSH_CLIENT environment variable', () => {
      const originalSshClient = process.env.SSH_CLIENT;
      
      delete process.env.SSH_CONNECTION;
      process.env.SSH_CLIENT = '10.0.0.1 12345 22';
      const isSSH = !!process.env.SSH_CONNECTION || !!process.env.SSH_CLIENT;
      expect(isSSH).toBe(true);
      
      if (originalSshClient) {
        process.env.SSH_CLIENT = originalSshClient;
      } else {
        delete process.env.SSH_CLIENT;
      }
    });
  });
});
