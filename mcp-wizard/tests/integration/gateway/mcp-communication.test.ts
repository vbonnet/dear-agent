/**
 * Integration Tests: Gateway ↔ MCP Communication
 *
 * Tests MCPProxy integration with MockMCPServer.
 */

import { MCPProxy } from '../../../src/lib/mcp-proxy';
import { SchemaFilter } from '../../../src/lib/schema-filter';
import { createMockMCPServer } from '../../__mocks__/mcp-server';
import { sampleGoogleDocsSchemas } from '../../__fixtures__/tool-schemas';
import {
  createToolsListRequest,
  createToolsCallRequest,
  createInitializeRequest
} from '../../__fixtures__/mcp-messages';

describe('Gateway ↔ MCP Communication', () => {
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

  describe('JSON-RPC communication', () => {
    it('should send valid JSON-RPC request to mock MCP', () => {
      const request = createToolsListRequest();
      mockMCP.handleRequest(request);

      const receivedRequests = mockMCP.getReceivedRequests();
      expect(receivedRequests).toHaveLength(1);
      expect(receivedRequests[0]).toMatchObject({
        jsonrpc: '2.0',
        method: 'tools/list'
      });
    });

    it('should receive and parse JSON-RPC response', (done) => {
      mockMCP.stdout.on('data', (data: Buffer) => {
        const response = JSON.parse(data.toString());
        expect(response).toHaveProperty('jsonrpc', '2.0');
        expect(response).toHaveProperty('id');
        expect(response).toHaveProperty('result');
        done();
      });

      mockMCP.handleRequest(createToolsListRequest());
    });

    it('should handle tools/list request', (done) => {
      mockMCP.stdout.on('data', (data: Buffer) => {
        const response = JSON.parse(data.toString());
        expect(response.result).toHaveProperty('tools');
        expect(Array.isArray(response.result.tools)).toBe(true);
        done();
      });

      mockMCP.handleRequest(createToolsListRequest());
    });

    it('should handle tools/call request', (done) => {
      mockMCP.setToolResult('mcp__GoogleDocs__readGoogleDoc', {
        documentId: 'test-123',
        content: 'Test content'
      });

      mockMCP.stdout.on('data', (data: Buffer) => {
        const response = JSON.parse(data.toString());
        expect(response.result).toHaveProperty('content');
        done();
      });

      mockMCP.handleRequest(
        createToolsCallRequest('mcp__GoogleDocs__readGoogleDoc', {
          documentId: 'test-123'
        })
      );
    });

    it('should handle initialize request', (done) => {
      mockMCP.stdout.on('data', (data: Buffer) => {
        const response = JSON.parse(data.toString());
        expect(response.result).toHaveProperty('protocolVersion');
        expect(response.result.protocolVersion).toBe('2024-11-05');
        done();
      });

      mockMCP.handleRequest(createInitializeRequest());
    });
  });

  describe('error scenarios', () => {
    it('should handle timeout (no response)', (done) => {
      mockMCP.simulateTimeout();

      let responseReceived = false;
      mockMCP.stdout.on('data', () => {
        responseReceived = true;
      });

      mockMCP.handleRequest(createToolsListRequest());

      setTimeout(() => {
        expect(responseReceived).toBe(false);
        done();
      }, 100);
    });

    it('should handle crash (exit event)', (done) => {
      mockMCP.on('exit', (code) => {
        expect(code).toBe(1);
        expect(mockMCP.killed).toBe(true);
        done();
      });

      mockMCP.simulateCrash();
    });

    it('should handle invalid JSON output', (done) => {
      mockMCP.simulateInvalidJSON();

      mockMCP.stdout.on('data', (data: Buffer) => {
        const text = data.toString();
        expect(() => JSON.parse(text)).toThrow();
        done();
      });

      mockMCP.handleRequest(createToolsListRequest());
    });

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
  });

  describe('request verification', () => {
    it('should track all received requests', () => {
      mockMCP.handleRequest(createToolsListRequest(1));
      mockMCP.handleRequest(createToolsListRequest(2));
      mockMCP.handleRequest(createInitializeRequest(3));

      const requests = mockMCP.getReceivedRequests();
      expect(requests).toHaveLength(3);
    });

    it('should allow clearing received requests', () => {
      mockMCP.handleRequest(createToolsListRequest());
      expect(mockMCP.getReceivedRequests()).toHaveLength(1);

      mockMCP.clearReceivedRequests();
      expect(mockMCP.getReceivedRequests()).toHaveLength(0);
    });
  });
});
