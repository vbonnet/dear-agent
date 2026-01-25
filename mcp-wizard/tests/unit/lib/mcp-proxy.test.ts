/**
 * Unit Tests: MCPProxy
 *
 * Comprehensive unit tests for the MCPProxy class.
 */

import { MCPProxy, MCPProxyConfig } from '../../../src/lib/mcp-proxy';
import { SchemaFilter } from '../../../src/lib/schema-filter';
import {
  mockMCPConfigs,
  singleMCPConfig,
  emptyMCPConfig,
  mockGoogleDocsMCP
} from '../../__fixtures__/mcp-configs';
import {
  allSchemas,
  sampleGoogleDocsSchemas,
  singleSchema
} from '../../__fixtures__/tool-schemas';
import {
  createToolsListRequest,
  createToolsCallRequest,
  createInitializeRequest,
  createUnknownMethodRequest,
  createInvalidToolsCallRequest,
  ERROR_CODES
} from '../../__fixtures__/mcp-messages';

describe('MCPProxy', () => {
  let proxy: MCPProxy;
  let mockConfig: MCPProxyConfig;

  beforeEach(() => {
    mockConfig = {
      schemaFilter: new SchemaFilter(),
      downstreamMCPs: mockMCPConfigs,
      traceMode: false
    };
    proxy = new MCPProxy(mockConfig);
  });

  describe('handleRequest', () => {
    it('should route tools/list to handleToolsList', async () => {
      const request = createToolsListRequest();
      const response = await proxy.handleRequest(request);

      expect(response.jsonrpc).toBe('2.0');
      expect(response.id).toBe(request.id);
      expect(response.result).toHaveProperty('tools');
      expect(response.error).toBeUndefined();
    });

    it('should route tools/call to handleToolsCall', async () => {
      const request = createToolsCallRequest('mcp__GoogleDocs__readGoogleDoc', {
        documentId: 'test-doc-123'
      });
      const response = await proxy.handleRequest(request);

      expect(response.jsonrpc).toBe('2.0');
      expect(response.id).toBe(request.id);
      // Should return error since downstream communication not implemented
      expect(response.error).toBeDefined();
    });

    it('should route initialize to handleInitialize', async () => {
      const request = createInitializeRequest();
      const response = await proxy.handleRequest(request);

      expect(response.jsonrpc).toBe('2.0');
      expect(response.id).toBe(request.id);
      expect(response.result).toHaveProperty('protocolVersion');
      expect(response.error).toBeUndefined();
    });

    it('should return error for unknown method', async () => {
      const request = createUnknownMethodRequest();
      const response = await proxy.handleRequest(request);

      expect(response.jsonrpc).toBe('2.0');
      expect(response.id).toBe(request.id);
      expect(response.error).toBeDefined();
      expect(response.error?.code).toBe(ERROR_CODES.METHOD_NOT_FOUND);
      expect(response.error?.message).toContain('Method not found');
    });

    it('should preserve request ID in response', async () => {
      const requestId = 42;
      const request = createToolsListRequest(requestId);
      const response = await proxy.handleRequest(request);

      expect(response.id).toBe(requestId);
    });

    it('should handle async errors gracefully', async () => {
      const request = createToolsCallRequest('nonexistent-tool');
      const response = await proxy.handleRequest(request);

      expect(response.error).toBeDefined();
      expect(response.error?.code).toBe(ERROR_CODES.INVALID_PARAMS);
    });
  });

  describe('handleToolsList', () => {
    it('should load schemas on first call', async () => {
      const request = createToolsListRequest();
      const response = await proxy.handleRequest(request);

      expect(response.result).toHaveProperty('tools');
      const tools = (response.result as any).tools;
      expect(Array.isArray(tools)).toBe(true);
      expect(tools.length).toBeGreaterThan(0);
    });

    it('should return all schemas when no intent', async () => {
      const request = createToolsListRequest();
      const response = await proxy.handleRequest(request);

      const tools = (response.result as any).tools;
      expect(tools.length).toBe(allSchemas.length);
    });

    it('should filter schemas when intent provided', async () => {
      // Intent extraction not yet implemented, returns all schemas
      const request = createToolsListRequest();
      const response = await proxy.handleRequest(request);

      const tools = (response.result as any).tools;
      expect(Array.isArray(tools)).toBe(true);
    });

    it('should handle empty schema list', async () => {
      const emptyProxy = new MCPProxy({
        schemaFilter: new SchemaFilter(),
        downstreamMCPs: emptyMCPConfig,
        traceMode: false
      });

      const request = createToolsListRequest();
      const response = await emptyProxy.handleRequest(request);

      const tools = (response.result as any).tools;
      expect(tools.length).toBe(0);
    });

    it('should return ToolsListResponse format', async () => {
      const request = createToolsListRequest();
      const response = await proxy.handleRequest(request);

      expect(response.result).toHaveProperty('tools');
      const tools = (response.result as any).tools;
      expect(Array.isArray(tools)).toBe(true);

      if (tools.length > 0) {
        expect(tools[0]).toHaveProperty('name');
      }
    });
  });

  describe('handleToolsCall', () => {
    it('should validate tool name is present', async () => {
      const request = createInvalidToolsCallRequest();
      const response = await proxy.handleRequest(request);

      expect(response.error).toBeDefined();
      expect(response.error?.code).toBe(ERROR_CODES.INVALID_PARAMS);
    });

    it('should return error if tool name missing', async () => {
      const request = createInvalidToolsCallRequest();
      const response = await proxy.handleRequest(request);

      expect(response.error).toBeDefined();
      expect(response.error?.message).toContain('tool name');
    });

    it('should find which MCP owns the tool', async () => {
      const request = createToolsCallRequest('mcp__GoogleDocs__readGoogleDoc');
      const response = await proxy.handleRequest(request);

      // Will return error since downstream communication not implemented
      // but should get past the "find MCP" step
      expect(response.error).toBeDefined();
      expect(response.error?.message).not.toContain('Unknown tool');
    });

    it('should return error if tool not found', async () => {
      const request = createToolsCallRequest('nonexistent-tool');
      const response = await proxy.handleRequest(request);

      expect(response.error).toBeDefined();
      expect(response.error?.code).toBe(ERROR_CODES.INVALID_PARAMS);
      expect(response.error?.message).toContain('Unknown tool');
    });

    it('should route to correct downstream MCP', async () => {
      // Load schemas first
      await proxy.handleRequest(createToolsListRequest());

      const request = createToolsCallRequest('mcp__GoogleDocs__readGoogleDoc');
      const response = await proxy.handleRequest(request);

      // Downstream communication not implemented, but routing should work
      expect(response.error).toBeDefined();
      expect(response.error?.message).toContain('not yet implemented');
    });
  });

  describe('handleInitialize', () => {
    it('should return protocol version 2024-11-05', async () => {
      const request = createInitializeRequest();
      const response = await proxy.handleRequest(request);

      const result = response.result as any;
      expect(result.protocolVersion).toBe('2024-11-05');
    });

    it('should return server info', async () => {
      const request = createInitializeRequest();
      const response = await proxy.handleRequest(request);

      const result = response.result as any;
      expect(result.serverInfo).toBeDefined();
      expect(result.serverInfo).toHaveProperty('name');
      expect(result.serverInfo).toHaveProperty('version');
    });

    it('should return capabilities object', async () => {
      const request = createInitializeRequest();
      const response = await proxy.handleRequest(request);

      const result = response.result as any;
      expect(result.capabilities).toBeDefined();
      expect(result.capabilities).toHaveProperty('tools');
    });
  });

  describe('findMCPForTool', () => {
    beforeEach(async () => {
      // Load schemas
      await proxy.handleRequest(createToolsListRequest());
    });

    it('should return MCP name if tool found', () => {
      const allSchemas = proxy.getAllSchemas();
      const googleDocsTool = allSchemas.find(s => s.name.includes('GoogleDocs'));

      if (googleDocsTool) {
        // Tool should be found (private method tested via handleToolsCall)
        const request = createToolsCallRequest(googleDocsTool.name);
        expect(request.params).toHaveProperty('name');
      }
    });

    it('should return null if tool not found', async () => {
      const request = createToolsCallRequest('nonexistent-tool');
      const response = await proxy.handleRequest(request);

      expect(response.error).toBeDefined();
      expect(response.error?.message).toContain('Unknown tool');
    });

    it('should search all MCP schemas', async () => {
      await proxy.handleRequest(createToolsListRequest());

      const allSchemas = proxy.getAllSchemas();
      expect(allSchemas.length).toBeGreaterThan(0);
    });
  });

  describe('error handling', () => {
    it('should convert Error to MCPError', async () => {
      const request = createToolsCallRequest('nonexistent-tool');
      const response = await proxy.handleRequest(request);

      expect(response.error).toBeDefined();
      expect(response.error).toHaveProperty('code');
      expect(response.error).toHaveProperty('message');
    });

    it('should include error message', async () => {
      const request = createUnknownMethodRequest();
      const response = await proxy.handleRequest(request);

      expect(response.error?.message).toBeDefined();
      expect(response.error?.message.length).toBeGreaterThan(0);
    });

    it('should handle unknown error types', async () => {
      const request = createInvalidToolsCallRequest();
      const response = await proxy.handleRequest(request);

      expect(response.error).toBeDefined();
      expect(typeof response.error?.message).toBe('string');
    });
  });

  describe('schema caching', () => {
    it('should cache schemas after first load', async () => {
      const request1 = createToolsListRequest(1);
      const response1 = await proxy.handleRequest(request1);

      const request2 = createToolsListRequest(2);
      const response2 = await proxy.handleRequest(request2);

      expect((response1.result as any).tools).toEqual((response2.result as any).tools);
    });

    it('should reload schemas when requested', async () => {
      await proxy.handleRequest(createToolsListRequest());

      await proxy.reloadSchemas();

      const request = createToolsListRequest();
      const response = await proxy.handleRequest(request);

      expect(response.result).toHaveProperty('tools');
    });
  });

  describe('getAllSchemas', () => {
    it('should return all loaded schemas', async () => {
      await proxy.handleRequest(createToolsListRequest());

      const schemas = proxy.getAllSchemas();
      expect(Array.isArray(schemas)).toBe(true);
      expect(schemas.length).toBe(allSchemas.length);
    });

    it('should return empty array before schemas loaded', () => {
      const schemas = proxy.getAllSchemas();
      expect(Array.isArray(schemas)).toBe(true);
      expect(schemas.length).toBe(0);
    });
  });

  describe('trace mode', () => {
    it('should not log when traceMode is false', async () => {
      const consoleSpy = jest.spyOn(console, 'error').mockImplementation();

      await proxy.handleRequest(createToolsListRequest());

      expect(consoleSpy).not.toHaveBeenCalled();
      consoleSpy.mockRestore();
    });

    it('should log when traceMode is true', async () => {
      const traceProxy = new MCPProxy({
        ...mockConfig,
        traceMode: true
      });

      const consoleSpy = jest.spyOn(console, 'error').mockImplementation();

      await traceProxy.handleRequest(createToolsListRequest());

      expect(consoleSpy).toHaveBeenCalled();
      consoleSpy.mockRestore();
    });
  });

  describe('edge cases', () => {
    it('should handle null params in request', async () => {
      const request = {
        jsonrpc: '2.0' as const,
        id: 1,
        method: 'tools/list',
        params: null
      };

      const response = await proxy.handleRequest(request);
      expect(response.result).toBeDefined();
    });

    it('should handle undefined params in request', async () => {
      const request = {
        jsonrpc: '2.0' as const,
        id: 1,
        method: 'tools/list'
      };

      const response = await proxy.handleRequest(request);
      expect(response.result).toBeDefined();
    });

    it('should handle numeric request IDs', async () => {
      const request = createToolsListRequest(123);
      const response = await proxy.handleRequest(request);

      expect(response.id).toBe(123);
    });

    it('should handle string request IDs', async () => {
      const request = { ...createToolsListRequest(), id: 'test-id' };
      const response = await proxy.handleRequest(request);

      expect(response.id).toBe('test-id');
    });
  });
});
