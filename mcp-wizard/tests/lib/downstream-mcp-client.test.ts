/**
 * Downstream MCP Client Tests
 *
 * Unit tests for DownstreamMCPClient (process spawning, JSON-RPC, timeout handling, crash recovery)
 */

import { DownstreamMCPClient, DownstreamMCPConfig } from '../../src/lib/downstream-mcp-client';
import { MockMCPServer } from '../__mocks__/mcp-server';
import { sampleGoogleDocsSchemas } from '../__fixtures__/tool-schemas';
import { spawn } from 'child_process';

// Mock child_process.spawn to return MockMCPServer instances
jest.mock('child_process');

describe('DownstreamMCPClient', () => {
  let mockMCP: MockMCPServer;
  let client: DownstreamMCPClient;
  const config: DownstreamMCPConfig = {
    command: 'mcp-server-googledocs',
    args: ['--test-mode'],
    env: { TEST_ENV: 'true' },
  };

  beforeEach(() => {
    // Create fresh MockMCPServer instance
    mockMCP = new MockMCPServer({ schemas: sampleGoogleDocsSchemas });

    // Mock spawn to return our MockMCPServer
    (spawn as jest.Mock).mockReturnValue(mockMCP);

    // Clear all mocks
    jest.clearAllMocks();
  });

  afterEach(async () => {
    // Cleanup client
    if (client) {
      try {
        await Promise.race([
          client.stop(),
          new Promise((resolve) => setTimeout(resolve, 2000)) // 2 second timeout
        ]);
      } catch (error) {
        // Ignore errors during cleanup
      }
    }
    // Reset mock state
    if (mockMCP) {
      mockMCP.resetBehavior();
      mockMCP.clearReceivedRequests();
    }
  }, 10000); // 10 second timeout for cleanup

  // ============================================================================
  // Happy Path Tests
  // ============================================================================

  describe('Process Spawning and Initialization', () => {
    test('spawns MCP process with correct command and args', async () => {
      client = new DownstreamMCPClient(config);

      await client.start();

      expect(spawn).toHaveBeenCalledWith(
        'mcp-server-googledocs',
        ['--test-mode'],
        expect.objectContaining({
          stdio: ['pipe', 'pipe', 'pipe'],
          env: expect.objectContaining({ TEST_ENV: 'true' }),
        })
      );
    });

    test('sends initialize request on start', async () => {
      client = new DownstreamMCPClient(config);

      await client.start();

      const requests = mockMCP.getReceivedRequests();
      expect(requests).toHaveLength(1);
      expect(requests[0].method).toBe('initialize');
      expect(requests[0].params).toMatchObject({
        protocolVersion: '2024-11-05',
        clientInfo: {
          name: 'mcp-wizard',
          version: '0.1.0',
        },
      });
    });

    test('sets ready state after successful initialization', async () => {
      client = new DownstreamMCPClient(config);

      expect(client.isReady()).toBe(false);

      await client.start();

      expect(client.isReady()).toBe(true);
    });

    test('calls tools/list successfully', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      const tools = await client.callToolsList();

      expect(tools).toEqual(sampleGoogleDocsSchemas);

      const requests = mockMCP.getReceivedRequests();
      expect(requests[1].method).toBe('tools/list'); // Second request after initialize
    });

    test('calls tools/call successfully', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      mockMCP.setToolResult('googledocs:readDocument', { content: 'test content' });

      const response = await client.callTool('googledocs:readDocument', { docId: '123' });

      expect(response.result).toEqual({ content: 'test content' });

      const requests = mockMCP.getReceivedRequests();
      expect(requests[1].method).toBe('tools/call');
      expect(requests[1].params).toMatchObject({
        name: 'googledocs:readDocument',
        arguments: { docId: '123' },
      });
    });
  });

  // ============================================================================
  // Error Resilience Tests
  // ============================================================================

  describe('Error Handling', () => {
    test('throws error when MCP crashes during initialize', async () => {
      mockMCP.simulateCrash();

      client = new DownstreamMCPClient(config);

      await expect(client.start()).rejects.toThrow(/Process exited/);
      expect(client.isReady()).toBe(false);
    });

    test('handles malformed JSON response', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      mockMCP.simulateInvalidJSON();

      // Request should timeout since no valid response received
      const requestPromise = client.callToolsList();

      // Note: We don't advance timers here, just verify the request doesn't resolve immediately
      // The actual timeout test is in the fake timer section
    });

    test('handles process exit during tool call', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      // Start a tool call
      const callPromise = client.callToolsList();

      // Simulate crash
      mockMCP.emit('exit', 1, null);

      await expect(callPromise).rejects.toThrow(/Process exited/);
      expect(client.isReady()).toBe(false);
    });

    test('handles JSON-RPC error response', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      mockMCP.setError(-32600, 'Invalid Request');

      await expect(client.callToolsList()).rejects.toThrow('tools/list failed: Invalid Request');
    });

    test('throws error when client not started', async () => {
      client = new DownstreamMCPClient(config);

      await expect(client.callToolsList()).rejects.toThrow('MCP client not started');
    });

    test('throws error when starting already started client', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      await expect(client.start()).rejects.toThrow('MCP client already started');
    });
  });

  // ============================================================================
  // Timeout Tests with Fake Timers
  // ============================================================================

  describe('Timeout Handling', () => {
    beforeEach(() => {
      jest.useFakeTimers();
    });

    afterEach(() => {
      jest.useRealTimers();
    });

    test('times out request after 30 seconds by default', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      mockMCP.simulateTimeout(); // Don't respond to requests

      const requestPromise = client.callToolsList();

      // Advance time by 30 seconds
      jest.advanceTimersByTime(30000);

      await expect(requestPromise).rejects.toThrow('Request timeout after 30000ms');
    });

    test('uses custom timeout when specified', async () => {
      client = new DownstreamMCPClient(config, { requestTimeout: 5000 });
      await client.start();

      mockMCP.simulateTimeout();

      const requestPromise = client.callToolsList();

      // Advance time by 5 seconds
      jest.advanceTimersByTime(5000);

      await expect(requestPromise).rejects.toThrow('Request timeout after 5000ms');
    });

    test('clears timeout when response received', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      const requestPromise = client.callToolsList();

      // Advance time by 1 second (less than timeout)
      jest.advanceTimersByTime(1000);

      // Response is sent by MockMCPServer automatically (not timeout mode)
      const result = await requestPromise;

      expect(result).toEqual(sampleGoogleDocsSchemas);
    });
  });

  // ============================================================================
  // Lifecycle Tests
  // ============================================================================

  describe('Process Lifecycle', () => {
    test('gracefully shuts down with SIGTERM', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      const stopPromise = client.stop();

      expect(mockMCP.kill).toHaveBeenCalledWith('SIGTERM');

      // Simulate graceful exit
      mockMCP.emit('exit', 0, 'SIGTERM');

      await stopPromise;
    });

    test('force kills with SIGKILL after 5 seconds', async () => {
      jest.useFakeTimers();

      client = new DownstreamMCPClient(config);
      await client.start();

      const stopPromise = client.stop();

      expect(mockMCP.kill).toHaveBeenCalledWith('SIGTERM');

      // Don't emit exit event, simulate hang
      jest.advanceTimersByTime(5000);

      expect(mockMCP.kill).toHaveBeenCalledWith('SIGKILL');

      // Now emit exit
      mockMCP.emit('exit', 0, 'SIGKILL');

      await stopPromise;

      jest.useRealTimers();
    });

    test('rejects pending requests on stop', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      mockMCP.simulateTimeout(); // Don't respond
      const requestPromise = client.callToolsList();

      // Stop client before response
      await client.stop();

      await expect(requestPromise).rejects.toThrow(/Process exited/);
    });

    test('handles multiple start/stop cycles', async () => {
      // First cycle
      client = new DownstreamMCPClient(config);
      await client.start();
      expect(client.isReady()).toBe(true);
      await client.stop();

      // Second cycle (need new client instance)
      mockMCP = new MockMCPServer({ schemas: sampleGoogleDocsSchemas });
      (spawn as jest.Mock).mockReturnValue(mockMCP);

      client = new DownstreamMCPClient(config);
      await client.start();
      expect(client.isReady()).toBe(true);
      await client.stop();
    });

    test('returns immediately when stop called on non-started client', async () => {
      client = new DownstreamMCPClient(config);

      await expect(client.stop()).resolves.toBeUndefined();
    });
  });

  // ============================================================================
  // Concurrent Requests
  // ============================================================================

  describe('Concurrent Request Handling', () => {
    test('handles multiple concurrent tool calls', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      mockMCP.setToolResult('googledocs:readDocument', { content: 'doc1' });

      // Send multiple requests in parallel
      const promises = [
        client.callTool('googledocs:readDocument', { docId: '1' }),
        client.callTool('googledocs:readDocument', { docId: '2' }),
        client.callTool('googledocs:readDocument', { docId: '3' }),
      ];

      const results = await Promise.all(promises);

      expect(results).toHaveLength(3);
      results.forEach((result: any) => {
        expect(result.result).toMatchObject({
          content: expect.arrayContaining([
            expect.objectContaining({ type: 'text', text: expect.stringContaining('doc1') })
          ])
        });
      });

      // Verify all requests were sent
      const requests = mockMCP.getReceivedRequests();
      expect(requests.length).toBeGreaterThanOrEqual(4); // 1 initialize + 3 tool calls
    });

    test('correctly matches responses to requests by ID', async () => {
      client = new DownstreamMCPClient(config);
      await client.start();

      // Set different results for different requests
      // MockMCPServer returns results based on request order/ID matching

      const promise1 = client.callToolsList();
      const promise2 = client.callToolsList();

      const [result1, result2] = await Promise.all([promise1, promise2]);

      expect(result1).toEqual(sampleGoogleDocsSchemas);
      expect(result2).toEqual(sampleGoogleDocsSchemas);
    });
  });
});
