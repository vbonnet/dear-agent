/**
 * Regression Tests: MCP SDK Version Compatibility
 *
 * Tests gateway compatibility across MCP SDK versions.
 */

import { MCPProxy } from '../../../src/lib/mcp-proxy';
import { SchemaFilter } from '../../../src/lib/schema-filter';
import { mockMCPConfigs } from '../../__fixtures__/mcp-configs';
import { createInitializeRequest, createToolsListRequest, createToolsCallRequest } from '../../__fixtures__/mcp-messages';

describe('MCP SDK Compatibility', () => {
  let proxy: MCPProxy;

  beforeEach(() => {
    proxy = new MCPProxy({
      schemaFilter: new SchemaFilter(),
      downstreamMCPs: mockMCPConfigs,
      traceMode: false
    });
  });

  describe('Current Protocol Version (2024-11-05)', () => {
    it('should support current protocol version', async () => {
      const request = createInitializeRequest(1, '2024-11-05');
      const response = await proxy.handleRequest(request);

      expect(response.result).toHaveProperty('protocolVersion', '2024-11-05');
      expect(response.result).toHaveProperty('serverInfo');
      expect(response.result).toHaveProperty('capabilities');
    });

    it('should handle tools/list with current protocol', async () => {
      const request = createToolsListRequest();
      const response = await proxy.handleRequest(request);

      expect(response.result).toHaveProperty('tools');
      expect(Array.isArray((response.result as any).tools)).toBe(true);
    });

    it('should handle tools/call with current protocol', async () => {
      await proxy.handleRequest(createToolsListRequest());

      const request = createToolsCallRequest('mcp__GoogleDocs__readGoogleDoc', {
        documentId: 'test-123'
      });
      const response = await proxy.handleRequest(request);

      expect(response.id).toBe(request.id);
    });
  });

  describe('Protocol Version Negotiation', () => {
    it('should accept initialize with protocolVersion param', async () => {
      const request = createInitializeRequest(1, '2024-11-05');
      const response = await proxy.handleRequest(request);

      expect(response.error).toBeUndefined();
      expect(response.result).toBeDefined();
    });

    it('should respond with supported version', async () => {
      const request = createInitializeRequest();
      const response = await proxy.handleRequest(request);

      const result = response.result as any;
      expect(result.protocolVersion).toBe('2024-11-05');
    });
  });

  describe('Backward Compatibility', () => {
    it('should handle older JSON-RPC 2.0 format', async () => {
      const request = {
        jsonrpc: '2.0' as const,
        id: 1,
        method: 'tools/list',
        params: {}
      };

      const response = await proxy.handleRequest(request);
      expect(response.error).toBeUndefined();
      expect(response.result).toBeDefined();
    });

    it('should handle initialize without protocolVersion', async () => {
      const request = {
        jsonrpc: '2.0' as const,
        id: 0,
        method: 'initialize',
        params: {}
      };

      const response = await proxy.handleRequest(request);
      expect(response.result).toBeDefined();
    });
  });
});
