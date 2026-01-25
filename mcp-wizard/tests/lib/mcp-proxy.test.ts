/**
 * MCP Proxy Tests
 *
 * Tests for MCP protocol implementation (tools/list, tools/call, initialize)
 */

import {
  MCPProxy,
  createMCPProxy,
  MCPRequest,
  MCPResponse,
  DownstreamMCP,
} from '../../src/lib/mcp-proxy';
import { createSchemaFilter, MCPToolSchema } from '../../src/lib/schema-filter';

// ============================================================================
// Mock Data
// ============================================================================

const MOCK_ATLASSIAN_SCHEMAS: MCPToolSchema[] = [
  {
    name: 'atlassian_create_issue',
    description: 'Create a new Jira issue',
    inputSchema: {
      type: 'object',
      properties: {
        title: { type: 'string' },
        description: { type: 'string' },
      },
      required: ['title'],
    },
  },
  {
    name: 'atlassian_read_issue',
    description: 'Read a Jira issue',
  },
];

const MOCK_GOOGLEDOCS_SCHEMAS: MCPToolSchema[] = [
  {
    name: 'googledocs_create_document',
    description: 'Create a new Google Document',
  },
  {
    name: 'googledocs_read_document',
    description: 'Read a Google Document',
  },
];

function createMockMCPs(): Map<string, DownstreamMCP> {
  const mcps = new Map<string, DownstreamMCP>();

  mcps.set('atlassian', {
    name: 'atlassian',
    command: 'atlassian-mcp',
    args: [],
    schemas: MOCK_ATLASSIAN_SCHEMAS,
  });

  mcps.set('googledocs', {
    name: 'googledocs',
    command: 'googledocs-mcp',
    args: [],
    schemas: MOCK_GOOGLEDOCS_SCHEMAS,
  });

  return mcps;
}

function createMockRequest(
  method: string,
  params?: unknown,
  id: string | number = 1
): MCPRequest {
  return {
    jsonrpc: '2.0',
    id,
    method,
    params,
  };
}

// ============================================================================
// Initialize Tests
// ============================================================================

describe('MCPProxy - Initialize', () => {
  test('handles initialize request', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const request = createMockRequest('initialize', {
      protocolVersion: '2024-11-05',
      clientInfo: { name: 'test-client', version: '1.0.0' },
    });

    const response = await proxy.handleRequest(request);

    expect(response.jsonrpc).toBe('2.0');
    expect(response.id).toBe(1);
    expect(response.result).toMatchObject({
      protocolVersion: '2024-11-05',
      serverInfo: {
        name: 'mcp-wizard-broker',
        version: '0.1.0',
      },
      capabilities: {
        tools: {
          listChanged: false,
        },
      },
    });
  });
});

// ============================================================================
// tools/list Tests
// ============================================================================

describe('MCPProxy - tools/list', () => {
  test('returns all schemas on first call', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const request = createMockRequest('tools/list');
    const response = await proxy.handleRequest(request);

    expect(response.jsonrpc).toBe('2.0');
    expect(response.id).toBe(1);
    expect(response.result).toHaveProperty('tools');

    const result = response.result as { tools: MCPToolSchema[] };
    expect(result.tools.length).toBe(4); // 2 Atlassian + 2 GoogleDocs
  });

  test('filters schemas based on intent (not yet implemented)', async () => {
    // Note: Intent extraction is not yet implemented in MCPProxy
    // This test documents the expected behavior
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const request = createMockRequest('tools/list');
    const response = await proxy.handleRequest(request);

    // Currently returns all schemas (no filtering)
    const result = response.result as { tools: MCPToolSchema[] };
    expect(result.tools.length).toBe(4);
  });

  test('handles empty downstream MCPs', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: new Map(),
    });

    const request = createMockRequest('tools/list');
    const response = await proxy.handleRequest(request);

    const result = response.result as { tools: MCPToolSchema[] };
    expect(result.tools).toEqual([]);
  });

  test('caches schemas across multiple calls', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const request1 = createMockRequest('tools/list', undefined, 1);
    const request2 = createMockRequest('tools/list', undefined, 2);

    const response1 = await proxy.handleRequest(request1);
    const response2 = await proxy.handleRequest(request2);

    const result1 = response1.result as { tools: MCPToolSchema[] };
    const result2 = response2.result as { tools: MCPToolSchema[] };

    expect(result1.tools).toEqual(result2.tools);
  });
});

// ============================================================================
// tools/call Tests
// ============================================================================

describe('MCPProxy - tools/call', () => {
  test('returns error when tool name is missing', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const request = createMockRequest('tools/call', {});
    const response = await proxy.handleRequest(request);

    expect(response.error).toBeDefined();
    expect(response.error?.code).toBe(-32602);
    expect(response.error?.message).toContain('Missing tool name');
  });

  test('returns error for unknown tool', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const request = createMockRequest('tools/call', {
      name: 'nonexistent_tool',
      arguments: {},
    });

    const response = await proxy.handleRequest(request);

    expect(response.error).toBeDefined();
    expect(response.error?.code).toBe(-32602);
    expect(response.error?.message).toContain('Unknown tool');
  });

  test('finds correct MCP for tool', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
      traceMode: true,
    });

    // Load schemas first
    await proxy.handleRequest(createMockRequest('tools/list'));

    const request = createMockRequest('tools/call', {
      name: 'atlassian_create_issue',
      arguments: { title: 'Test Issue' },
    });

    const response = await proxy.handleRequest(request);

    // Currently not implemented, so should return error
    expect(response.error).toBeDefined();
    expect(response.error?.message).toContain('not yet implemented');
  });
});

// ============================================================================
// Error Handling Tests
// ============================================================================

describe('MCPProxy - Error Handling', () => {
  test('returns error for unknown method', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const request = createMockRequest('unknown/method');
    const response = await proxy.handleRequest(request);

    expect(response.error).toBeDefined();
    expect(response.error?.code).toBe(-32601);
    expect(response.error?.message).toContain('Method not found');
  });

  test('preserves request ID in error response', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const request = createMockRequest('unknown/method', undefined, 'test-id-123');
    const response = await proxy.handleRequest(request);

    expect(response.id).toBe('test-id-123');
    expect(response.error).toBeDefined();
  });

  test('handles numeric and string request IDs', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const request1 = createMockRequest('initialize', undefined, 1);
    const request2 = createMockRequest('initialize', undefined, 'string-id');

    const response1 = await proxy.handleRequest(request1);
    const response2 = await proxy.handleRequest(request2);

    expect(response1.id).toBe(1);
    expect(response2.id).toBe('string-id');
  });
});

// ============================================================================
// Trace Mode Tests
// ============================================================================

describe('MCPProxy - Trace Mode', () => {
  test('trace mode logs to stderr', async () => {
    const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
      traceMode: true,
    });

    await proxy.handleRequest(createMockRequest('initialize'));

    expect(consoleErrorSpy).toHaveBeenCalled();
    expect(consoleErrorSpy.mock.calls.some((call) =>
      call[0].includes('[MCPProxy]')
    )).toBe(true);

    consoleErrorSpy.mockRestore();
  });

  test('trace mode disabled by default', async () => {
    const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    await proxy.handleRequest(createMockRequest('initialize'));

    // Should not log (trace mode off)
    const mcpProxyLogs = consoleErrorSpy.mock.calls.filter((call) =>
      call[0].includes('[MCPProxy]')
    );
    expect(mcpProxyLogs.length).toBe(0);

    consoleErrorSpy.mockRestore();
  });
});

// ============================================================================
// Schema Management Tests
// ============================================================================

describe('MCPProxy - Schema Management', () => {
  test('getAllSchemas returns loaded schemas', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    // Trigger schema loading
    await proxy.handleRequest(createMockRequest('tools/list'));

    const schemas = proxy.getAllSchemas();
    expect(schemas.length).toBe(4);
  });

  test('reloadSchemas refreshes schema list', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    // Initial load
    await proxy.handleRequest(createMockRequest('tools/list'));
    const schemas1 = proxy.getAllSchemas();

    // Reload
    await proxy.reloadSchemas();
    const schemas2 = proxy.getAllSchemas();

    expect(schemas1).toEqual(schemas2);
  });

  test('handles MCPs without schemas', async () => {
    const mcps = new Map<string, DownstreamMCP>();
    mcps.set('empty', {
      name: 'empty',
      command: 'empty-mcp',
      args: [],
      // No schemas
    });

    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: mcps,
    });

    await proxy.handleRequest(createMockRequest('tools/list'));
    const schemas = proxy.getAllSchemas();

    expect(schemas).toEqual([]);
  });
});

// ============================================================================
// Integration Tests
// ============================================================================

describe('MCPProxy - Integration', () => {
  test('complete MCP handshake flow', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    // 1. Initialize
    const initResponse = await proxy.handleRequest(
      createMockRequest('initialize', {
        protocolVersion: '2024-11-05',
        clientInfo: { name: 'test', version: '1.0' },
      })
    );
    expect(initResponse.result).toBeDefined();

    // 2. List tools
    const listResponse = await proxy.handleRequest(
      createMockRequest('tools/list')
    );
    expect(listResponse.result).toHaveProperty('tools');

    const result = listResponse.result as { tools: MCPToolSchema[] };
    expect(result.tools.length).toBeGreaterThan(0);
  });

  test('handles multiple concurrent requests', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const requests = [
      proxy.handleRequest(createMockRequest('initialize', {}, 1)),
      proxy.handleRequest(createMockRequest('tools/list', {}, 2)),
      proxy.handleRequest(createMockRequest('tools/list', {}, 3)),
    ];

    const responses = await Promise.all(requests);

    expect(responses[0].id).toBe(1);
    expect(responses[1].id).toBe(2);
    expect(responses[2].id).toBe(3);
    expect(responses.every((r) => r.jsonrpc === '2.0')).toBe(true);
  });
});

// ============================================================================
// JSON-RPC Compliance Tests
// ============================================================================

describe('MCPProxy - JSON-RPC Compliance', () => {
  test('response has correct JSON-RPC version', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const response = await proxy.handleRequest(createMockRequest('initialize'));
    expect(response.jsonrpc).toBe('2.0');
  });

  test('successful response has result field', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const response = await proxy.handleRequest(createMockRequest('initialize'));
    expect(response.result).toBeDefined();
    expect(response.error).toBeUndefined();
  });

  test('error response has error field', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const response = await proxy.handleRequest(createMockRequest('unknown'));
    expect(response.error).toBeDefined();
    expect(response.result).toBeUndefined();
  });

  test('error has code and message', async () => {
    const proxy = createMCPProxy({
      schemaFilter: createSchemaFilter(),
      downstreamMCPs: createMockMCPs(),
    });

    const response = await proxy.handleRequest(createMockRequest('unknown'));
    expect(response.error?.code).toBeDefined();
    expect(response.error?.message).toBeDefined();
    expect(typeof response.error?.code).toBe('number');
    expect(typeof response.error?.message).toBe('string');
  });
});
