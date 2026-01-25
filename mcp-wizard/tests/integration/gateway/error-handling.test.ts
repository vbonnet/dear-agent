/**
 * Integration Tests: Error Handling
 *
 * Tests gateway error handling scenarios (timeout, crash, invalid JSON).
 */

import { MCPProxy } from '../../../src/lib/mcp-proxy';
import { SchemaFilter } from '../../../src/lib/schema-filter';
import { createMockMCPServer } from '../../__mocks__/mcp-server';
import { sampleGoogleDocsSchemas } from '../../__fixtures__/tool-schemas';
import {
  createToolsListRequest,
  createToolsCallRequest,
  ERROR_CODES
} from '../../__fixtures__/mcp-messages';

describe('Error Handling', () => {
  let mockMCP: ReturnType<typeof createMockMCPServer>;

  beforeEach(() => {
    mockMCP = createMockMCPServer({
      schemas: sampleGoogleDocsSchemas,
      name: 'test-mcp'
    });
  });

  afterEach(() => {
    if (!mockMCP.killed) {
      mockMCP.kill();
    }
  });

  describe('MCP timeout handling', () => {
    it('should handle MCP timeout gracefully', (done) => {
      mockMCP.simulateTimeout();

      let responseReceived = false;
      mockMCP.stdout.on('data', () => {
        responseReceived = true;
      });

      mockMCP.handleRequest(createToolsListRequest());

      // Wait for timeout period
      setTimeout(() => {
        expect(responseReceived).toBe(false);
        done();
      }, 100);
    });

    it('should not block other requests during timeout', (done) => {
      mockMCP.simulateTimeout();

      // First request will timeout
      mockMCP.handleRequest(createToolsListRequest(1));

      // Reset timeout simulation
      mockMCP.resetBehavior();

      // Second request should succeed
      mockMCP.stdout.on('data', (data: Buffer) => {
        const response = JSON.parse(data.toString());
        expect(response.id).toBe(2);
        expect(response.result).toBeDefined();
        done();
      });

      mockMCP.handleRequest(createToolsListRequest(2));
    });
  });

  describe('MCP crash handling', () => {
    it('should handle MCP crash gracefully', (done) => {
      mockMCP.on('exit', (code) => {
        expect(code).toBe(1);
        expect(mockMCP.killed).toBe(true);
        done();
      });

      mockMCP.simulateCrash();
    });

    it('should detect crashed MCP', (done) => {
      let exitDetected = false;

      mockMCP.on('exit', () => {
        exitDetected = true;
      });

      mockMCP.simulateCrash();

      setTimeout(() => {
        expect(exitDetected).toBe(true);
        expect(mockMCP.killed).toBe(true);
        done();
      }, 50);
    });
  });

  describe('invalid JSON handling', () => {
    it('should handle invalid JSON output', (done) => {
      mockMCP.simulateInvalidJSON();

      mockMCP.stdout.on('data', (data: Buffer) => {
        const text = data.toString();
        expect(() => JSON.parse(text)).toThrow();
        done();
      });

      mockMCP.handleRequest(createToolsListRequest());
    });

    it('should detect malformed JSON-RPC response', (done) => {
      mockMCP.simulateInvalidJSON();

      mockMCP.stdout.on('data', (data: Buffer) => {
        const text = data.toString();
        expect(text).toBe('{ invalid json ');
        done();
      });

      mockMCP.handleRequest(createToolsListRequest());
    });
  });

  describe('MCP error responses', () => {
    it('should handle MCP returning error response', (done) => {
      mockMCP.setError(-32603, 'Internal error');

      mockMCP.stdout.on('data', (data: Buffer) => {
        const response = JSON.parse(data.toString());
        expect(response.error).toBeDefined();
        expect(response.error.code).toBe(-32603);
        expect(response.error.message).toBe('Internal error');
        done();
      });

      mockMCP.handleRequest(createToolsListRequest());
    });

    it('should propagate MCP error codes correctly', (done) => {
      const errorCode = ERROR_CODES.INVALID_PARAMS;
      mockMCP.setError(errorCode, 'Invalid parameters');

      mockMCP.stdout.on('data', (data: Buffer) => {
        const response = JSON.parse(data.toString());
        expect(response.error.code).toBe(errorCode);
        done();
      });

      mockMCP.handleRequest(createToolsListRequest());
    });

    it('should include error message in response', (done) => {
      const errorMessage = 'Custom error message';
      mockMCP.setError(-32000, errorMessage);

      mockMCP.stdout.on('data', (data: Buffer) => {
        const response = JSON.parse(data.toString());
        expect(response.error.message).toBe(errorMessage);
        done();
      });

      mockMCP.handleRequest(createToolsListRequest());
    });
  });

  describe('graceful degradation', () => {
    it('should allow clearing error state', () => {
      mockMCP.setError(-32603, 'Error');

      // Clear error state by setting new tool result
      mockMCP.setToolResult('mcp__GoogleDocs__readGoogleDoc', {
        content: 'Success'
      });

      // Error should be cleared
      expect(() => {
        mockMCP.handleRequest(
          createToolsCallRequest('mcp__GoogleDocs__readGoogleDoc')
        );
      }).not.toThrow();
    });

    it('should track requests even during errors', () => {
      mockMCP.setError(-32603, 'Error');
      mockMCP.handleRequest(createToolsListRequest());

      const requests = mockMCP.getReceivedRequests();
      expect(requests.length).toBe(1);
    });

    it('should recover from error state on next request', (done) => {
      let responseCount = 0;

      mockMCP.stdout.on('data', (data: Buffer) => {
        responseCount++;
        const response = JSON.parse(data.toString());

        if (responseCount === 1) {
          // First response should have error
          expect(response.error).toBeDefined();
          expect(response.id).toBe(1);

          // Clear error for second request
          mockMCP.clearError();
          mockMCP.handleRequest(createToolsListRequest(2));
        } else if (responseCount === 2) {
          // Second response should not have error
          expect(response.error).toBeUndefined();
          expect(response.result).toBeDefined();
          expect(response.id).toBe(2);
          done();
        }
      });

      // First request with error
      mockMCP.setError(-32603, 'Error');
      mockMCP.handleRequest(createToolsListRequest(1));
    });
  });

  describe('request validation errors', () => {
    it('should handle missing request ID', () => {
      const invalidRequest = {
        jsonrpc: '2.0' as const,
        method: 'tools/list'
        // Missing 'id'
      };

      expect(() => {
        mockMCP.handleRequest(invalidRequest as any);
      }).not.toThrow();
    });

    it('should handle invalid method', (done) => {
      mockMCP.stdout.on('data', (data: Buffer) => {
        const response = JSON.parse(data.toString());
        expect(response.error).toBeDefined();
        expect(response.error.code).toBe(ERROR_CODES.METHOD_NOT_FOUND);
        done();
      });

      mockMCP.handleRequest({
        jsonrpc: '2.0',
        id: 1,
        method: 'invalid/method'
      } as any);
    });
  });
});
